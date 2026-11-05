// Package near is a minimal NEAR JSON-RPC client: view calls, and signing and sending
// single-action FunctionCall transactions with an ed25519 key.
//
// It is used by the playground with a TESTNET function-call key that can only call the vault's
// pay_quote. It never handles user wallets or seed phrases.
//
// Transaction encoding follows NEAR's borsh schema (nearcore `Transaction` V0 and
// `SignedTransaction`): the transaction hash is sha256(borsh(tx)), and the signature is
// ed25519 over that hash.
package near

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/mr-tron/base58"
)

// Key is an ed25519 key pair for one account (NEAR "ed25519:<base58>" format).
type Key struct {
	AccountID string
	Private   ed25519.PrivateKey
}

// ParseKey parses "ed25519:<base58 of 64-byte private key>".
func ParseKey(accountID, secret string) (Key, error) {
	raw, err := base58.Decode(strings.TrimPrefix(strings.TrimSpace(secret), "ed25519:"))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return Key{}, errors.New("near: private key must be ed25519:<base58 64-byte key>")
	}
	return Key{AccountID: accountID, Private: ed25519.PrivateKey(raw)}, nil
}

// PublicKey returns the "ed25519:<base58>" public key.
func (k Key) PublicKey() string {
	return "ed25519:" + base58.Encode(k.Private.Public().(ed25519.PublicKey))
}

// Client talks to one NEAR RPC endpoint.
type Client struct {
	RPCURL string
	HTTP   *http.Client
}

func NewClient(rpcURL string, timeout time.Duration) *Client {
	return &Client{RPCURL: rpcURL, HTTP: &http.Client{Timeout: timeout}}
}

type rpcError struct {
	Name  string          `json:"name"`
	Cause json.RawMessage `json:"cause"`
	Data  json.RawMessage `json:"data"`
}

func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "beeline", "method": method, "params": params})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.RPCURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("near rpc: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("near rpc: %w", err)
	}
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("near rpc: HTTP %d: invalid response", resp.StatusCode)
	}
	if env.Error != nil {
		return fmt.Errorf("near rpc %s: %s %s", method, env.Error.Name, truncate(string(env.Error.Cause)+string(env.Error.Data), 300))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(env.Result, out)
}

// View calls a read-only contract method and returns its raw result bytes.
func (c *Client) View(ctx context.Context, contract, method string, args any) ([]byte, error) {
	a, _ := json.Marshal(args)
	var res struct {
		Result []byte `json:"result"`
		Error  string `json:"error"`
	}
	err := c.call(ctx, "query", map[string]any{
		"request_type": "call_function", "finality": "final", "account_id": contract,
		"method_name": method, "args_base64": base64.StdEncoding.EncodeToString(a),
	}, &res)
	if err != nil {
		return nil, err
	}
	if res.Error != "" {
		return nil, fmt.Errorf("near view %s: %s", method, truncate(res.Error, 300))
	}
	return res.Result, nil
}

// FunctionCall is one contract call to sign and send.
type FunctionCall struct {
	Receiver string
	Method   string
	Args     []byte
	Gas      uint64   // in gas units (1 Tgas = 1e12)
	Deposit  *big.Int // yoctoNEAR; nil = 0
}

// Outcome is the result of an executed transaction.
type Outcome struct {
	Hash string
	// Failure is the first receipt failure message (e.g. a contract panic), or "" on success.
	Failure string
	// Logs from all receipts, in order.
	Logs []string
	// ReturnValue is the transaction's final SuccessValue, decoded (may be empty).
	ReturnValue []byte
}

// Send signs fc with key and waits until it executes.
func (c *Client) Send(ctx context.Context, key Key, fc FunctionCall) (*Outcome, error) {
	var ak struct {
		Nonce     uint64 `json:"nonce"`
		BlockHash string `json:"block_hash"`
	}
	if err := c.call(ctx, "query", map[string]any{
		"request_type": "view_access_key", "finality": "final",
		"account_id": key.AccountID, "public_key": key.PublicKey(),
	}, &ak); err != nil {
		return nil, err
	}
	blockHash, err := base58.Decode(ak.BlockHash)
	if err != nil || len(blockHash) != 32 {
		return nil, errors.New("near: bad block hash from RPC")
	}
	signed, hash, err := SignFunctionCall(key, ak.Nonce+1, blockHash, fc)
	if err != nil {
		return nil, err
	}
	var res txResult
	if err := c.call(ctx, "send_tx", map[string]any{
		"signed_tx_base64": base64.StdEncoding.EncodeToString(signed),
		"wait_until":       "EXECUTED_OPTIMISTIC",
	}, &res); err != nil {
		return nil, err
	}
	out := &Outcome{Hash: base58.Encode(hash)}
	res.fill(out)
	return out, nil
}

type txResult struct {
	Status             json.RawMessage `json:"status"`
	TransactionOutcome struct {
		Outcome outcomeBody `json:"outcome"`
	} `json:"transaction_outcome"`
	ReceiptsOutcome []struct {
		Outcome outcomeBody `json:"outcome"`
	} `json:"receipts_outcome"`
}

type outcomeBody struct {
	Logs   []string        `json:"logs"`
	Status json.RawMessage `json:"status"`
}

func (r *txResult) fill(out *Outcome) {
	out.Logs = append(out.Logs, r.TransactionOutcome.Outcome.Logs...)
	for _, ro := range r.ReceiptsOutcome {
		out.Logs = append(out.Logs, ro.Outcome.Logs...)
		if out.Failure == "" {
			out.Failure = failureMessage(ro.Outcome.Status)
		}
	}
	if out.Failure == "" {
		out.Failure = failureMessage(r.Status)
	}
	var st struct {
		SuccessValue *string `json:"SuccessValue"`
	}
	if json.Unmarshal(r.Status, &st) == nil && st.SuccessValue != nil {
		out.ReturnValue, _ = base64.StdEncoding.DecodeString(*st.SuccessValue)
	}
}

// failureMessage extracts a readable message from a {"Failure": …} status, or "".
func failureMessage(status json.RawMessage) string {
	var st struct {
		Failure json.RawMessage `json:"Failure"`
	}
	if json.Unmarshal(status, &st) != nil || len(st.Failure) == 0 {
		return ""
	}
	var exec struct {
		ActionError struct {
			Kind struct {
				FunctionCallError struct {
					ExecutionError string `json:"ExecutionError"`
				} `json:"FunctionCallError"`
			} `json:"kind"`
		} `json:"ActionError"`
	}
	if json.Unmarshal(st.Failure, &exec) == nil && exec.ActionError.Kind.FunctionCallError.ExecutionError != "" {
		return exec.ActionError.Kind.FunctionCallError.ExecutionError
	}
	return truncate(string(st.Failure), 300)
}

// SignFunctionCall builds and signs a one-action transaction. It returns the borsh-encoded
// SignedTransaction and the transaction hash.
func SignFunctionCall(key Key, nonce uint64, blockHash []byte, fc FunctionCall) ([]byte, []byte, error) {
	if len(blockHash) != 32 {
		return nil, nil, errors.New("near: block hash must be 32 bytes")
	}
	var tx borsh
	tx.str(key.AccountID)
	tx.u8(0) // KeyType::ED25519
	tx.raw(key.Private.Public().(ed25519.PublicKey))
	tx.u64(nonce)
	tx.str(fc.Receiver)
	tx.raw(blockHash)
	tx.u32(1) // one action
	tx.u8(2)  // Action::FunctionCall
	tx.str(fc.Method)
	tx.bytes(fc.Args)
	tx.u64(fc.Gas)
	if err := tx.u128(fc.Deposit); err != nil {
		return nil, nil, err
	}
	sum := sha256.Sum256(tx.buf.Bytes())
	sig := ed25519.Sign(key.Private, sum[:])

	var signed borsh
	signed.raw(tx.buf.Bytes())
	signed.u8(0) // KeyType::ED25519
	signed.raw(sig)
	return signed.buf.Bytes(), sum[:], nil
}

type borsh struct{ buf bytes.Buffer }

func (b *borsh) u8(v uint8)     { b.buf.WriteByte(v) }
func (b *borsh) raw(p []byte)   { b.buf.Write(p) }
func (b *borsh) u32(v uint32)   { _ = binary.Write(&b.buf, binary.LittleEndian, v) }
func (b *borsh) u64(v uint64)   { _ = binary.Write(&b.buf, binary.LittleEndian, v) }
func (b *borsh) str(s string)   { b.u32(uint32(len(s))); b.buf.WriteString(s) }
func (b *borsh) bytes(p []byte) { b.u32(uint32(len(p))); b.buf.Write(p) }

func (b *borsh) u128(v *big.Int) error {
	var le [16]byte
	if v != nil {
		if v.Sign() < 0 || v.BitLen() > 128 {
			return errors.New("near: deposit out of range")
		}
		be := v.Bytes()
		for i := range be {
			le[i] = be[len(be)-1-i]
		}
	}
	b.buf.Write(le[:])
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
