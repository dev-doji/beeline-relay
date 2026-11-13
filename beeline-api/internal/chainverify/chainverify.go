// Package chainverify decides whether a NEAR transaction really paid an invoice. It is the rule that
// keeps "paid" honest: only this package's answer, read from the chain, can mark an invoice paid.
//
// It reads one transaction with the NEAR `tx` RPC (wait_until FINAL) and accepts it only if it is a
// direct, successful `ft_transfer` call on the canonical token contract that pays exactly the
// invoice's payee and amount with the invoice's memo and the required 1 yoctoNEAR deposit, signed by
// the claimed sender, in a final block that was produced before the invoice expired.
//
// It deliberately reads the transaction's own call, not event logs: the testnet USDC contract emits
// no NEP-297 event (checked 2026-11-13). Anything it does not recognise (wrappers, batched actions,
// ft_transfer_call) is refused, not guessed at. See docs/PAYMENT_FLOW.md.
//
// Docs: https://docs.near.org/api/rpc/transactions and
// https://github.com/near/NEPs/blob/master/neps/nep-0141.md
package chainverify

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Status of a verification attempt.
const (
	StatusVerified = "verified"
	StatusPending  = "pending"  // not seen or not final yet: ask again, this is not fraud
	StatusRejected = "rejected" // seen, and it does not pay this invoice
)

// Rejection codes. They are stored as security events and shown to vendors.
const (
	CodeNotFound        = "TX_NOT_FOUND"
	CodeFailed          = "TX_FAILED"
	CodeWrongToken      = "WRONG_TOKEN"
	CodeUnsupportedCall = "UNSUPPORTED_CALL"
	CodeWrongReceiver   = "WRONG_RECEIVER"
	CodeWrongAmount     = "WRONG_AMOUNT"
	CodeWrongMemo       = "WRONG_MEMO"
	CodeWrongDeposit    = "WRONG_DEPOSIT"
	CodeWrongSender     = "WRONG_SENDER"
	CodeExpired         = "EXPIRED"
	CodeUnavailable     = "CHAIN_UNAVAILABLE"
)

// Params is what the invoice says the payment must look like.
type Params struct {
	TxHash    string
	Sender    string    // the account the claimant says signed the transaction
	Token     string    // canonical token contract, e.g. usdc.fakes.testnet
	Receiver  string    // the invoice's payout account
	Amount    string    // base units, decimal string
	Memo      string    // "beeline:<invoice id>"
	ExpiresAt time.Time // the invoice must have been paid at or before this time
}

// Evidence is what was read from the chain, stored with the payment.
type Evidence struct {
	TxHash    string    `json:"tx_hash"`
	Sender    string    `json:"sender"`
	Receiver  string    `json:"receiver"`
	Token     string    `json:"token"`
	Amount    string    `json:"amount"`
	Memo      string    `json:"memo"`
	BlockHash string    `json:"block_hash"`
	BlockTime time.Time `json:"block_time"`
}

// Result of Verify.
type Result struct {
	Status   string
	Code     string // set when Status is rejected
	Reason   string // plain words for the customer and the vendor
	Evidence *Evidence
}

// RPC makes one NEAR JSON-RPC call and decodes `result` into out.
type RPC interface {
	Call(ctx context.Context, method string, params any, out any) error
}

// RPCError is a JSON-RPC error object from the node.
type RPCError struct {
	Name  string
	Cause string // cause.name, e.g. UNKNOWN_TRANSACTION or TIMEOUT_ERROR
	Info  string
}

func (e *RPCError) Error() string { return fmt.Sprintf("near rpc: %s %s %s", e.Name, e.Cause, e.Info) }

// Verifier checks claims against the chain through rpc.
type Verifier struct {
	RPC RPC
}

func pending(reason string) Result { return Result{Status: StatusPending, Reason: reason} }
func reject(code, reason string) Result {
	return Result{Status: StatusRejected, Code: code, Reason: reason}
}

type txResponse struct {
	FinalExecutionStatus string          `json:"final_execution_status"`
	Status               json.RawMessage `json:"status"`
	Transaction          struct {
		SignerID   string            `json:"signer_id"`
		ReceiverID string            `json:"receiver_id"`
		Actions    []json.RawMessage `json:"actions"`
	} `json:"transaction"`
	TransactionOutcome struct {
		BlockHash string `json:"block_hash"`
		Outcome   struct {
			Status json.RawMessage `json:"status"`
		} `json:"outcome"`
	} `json:"transaction_outcome"`
	ReceiptsOutcome []struct {
		Outcome struct {
			ExecutorID string          `json:"executor_id"`
			Status     json.RawMessage `json:"status"`
		} `json:"outcome"`
	} `json:"receipts_outcome"`
}

type blockResponse struct {
	Header struct {
		Hash          string          `json:"hash"`
		Timestamp     json.Number     `json:"timestamp"`
		TimestampNano json.RawMessage `json:"timestamp_nanosec"`
	} `json:"header"`
}

func hasFailure(status json.RawMessage) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(status, &m) != nil {
		return false
	}
	_, bad := m["Failure"]
	return bad
}

// Verify reads the transaction and decides. It never returns an error: a node problem is "pending".
func (v *Verifier) Verify(ctx context.Context, p Params) Result {
	var tx txResponse
	err := v.RPC.Call(ctx, "tx", map[string]any{"tx_hash": p.TxHash, "sender_account_id": p.Sender, "wait_until": "FINAL"}, &tx)
	if err != nil {
		var re *RPCError
		if errors.As(err, &re) {
			switch re.Cause {
			case "UNKNOWN_TRANSACTION":
				return reject(CodeNotFound, "No transaction with that hash exists on NEAR.")
			case "TIMEOUT_ERROR":
				return pending("The transaction was not seen on NEAR yet. This can take a few seconds.")
			}
		}
		return pending("Could not reach NEAR right now. Try again in a moment.")
	}

	// Final, and executed everywhere.
	if tx.FinalExecutionStatus != "FINAL" {
		return pending("The transaction is not final on NEAR yet. This can take a few seconds.")
	}
	if hasFailure(tx.Status) || hasFailure(tx.TransactionOutcome.Outcome.Status) {
		return reject(CodeFailed, "That transaction failed on NEAR, so no money moved to the vendor.")
	}
	for _, r := range tx.ReceiptsOutcome {
		if hasFailure(r.Outcome.Status) {
			return reject(CodeFailed, "That transaction failed on NEAR, so no money moved to the vendor.")
		}
	}
	if tx.Transaction.SignerID != p.Sender {
		return reject(CodeWrongSender, "That transaction was not signed by the account you gave.")
	}
	// A direct call to the canonical token contract. A look-alike contract fails here.
	if tx.Transaction.ReceiverID != p.Token {
		return reject(CodeWrongToken, "That payment used a different token than this invoice, so we did not count it.")
	}
	if len(tx.Transaction.Actions) != 1 {
		return reject(CodeUnsupportedCall, "That transaction does more than a single token transfer, so we could not match it to this invoice.")
	}
	var act struct {
		FunctionCall *struct {
			MethodName string `json:"method_name"`
			Args       string `json:"args"`
			Deposit    string `json:"deposit"`
		} `json:"FunctionCall"`
	}
	if json.Unmarshal(tx.Transaction.Actions[0], &act) != nil || act.FunctionCall == nil || act.FunctionCall.MethodName != "ft_transfer" {
		return reject(CodeUnsupportedCall, "That transaction is not a plain token transfer, so we could not match it to this invoice.")
	}
	raw, err := base64.StdEncoding.DecodeString(act.FunctionCall.Args)
	if err != nil {
		return reject(CodeUnsupportedCall, "We could not read that transfer, so it was not counted.")
	}
	var args struct {
		ReceiverID string  `json:"receiver_id"`
		Amount     string  `json:"amount"`
		Memo       *string `json:"memo"`
	}
	if json.Unmarshal(raw, &args) != nil {
		return reject(CodeUnsupportedCall, "We could not read that transfer, so it was not counted.")
	}
	if act.FunctionCall.Deposit != "1" {
		return reject(CodeWrongDeposit, "That transfer did not attach the required deposit, so we did not count it.")
	}
	if args.ReceiverID != p.Receiver {
		return reject(CodeWrongReceiver, "That payment went to a different account than this invoice's, so we did not count it.")
	}
	if args.Amount != p.Amount {
		return reject(CodeWrongAmount, "That payment was not the exact amount of this invoice, so we did not count it.")
	}
	if args.Memo == nil || *args.Memo != p.Memo {
		return reject(CodeWrongMemo, "That payment did not carry this invoice's reference, so we could not match it.")
	}

	// When was the block? The invoice must still have been open then.
	var blk blockResponse
	dec := map[string]any{"block_id": tx.TransactionOutcome.BlockHash}
	if err := v.RPC.Call(ctx, "block", dec, &blk); err != nil {
		return pending("Could not read the block time from NEAR yet. Try again in a moment.")
	}
	ns := strings.Trim(string(blk.Header.TimestampNano), `"`)
	if ns == "" || ns == "null" {
		ns = blk.Header.Timestamp.String()
	}
	nanos, perr := strconv.ParseInt(ns, 10, 64)
	if perr != nil || nanos <= 0 {
		return pending("Could not read the block time from NEAR yet. Try again in a moment.")
	}
	when := time.Unix(0, nanos).UTC()
	if when.After(p.ExpiresAt) {
		return reject(CodeExpired, "That payment was made after this invoice expired, so we did not count it.")
	}

	return Result{Status: StatusVerified, Evidence: &Evidence{
		TxHash: p.TxHash, Sender: p.Sender, Receiver: args.ReceiverID, Token: p.Token, Amount: args.Amount, Memo: *args.Memo,
		BlockHash: tx.TransactionOutcome.BlockHash, BlockTime: when,
	}}
}

// HTTPRPC calls one or more NEAR JSON-RPC endpoints, trying the next on a transport error.
type HTTPRPC struct {
	URLs []string
	HTTP *http.Client
}

func NewHTTPRPC(urls []string, timeout time.Duration) *HTTPRPC {
	return &HTTPRPC{URLs: urls, HTTP: &http.Client{Timeout: timeout}}
}

func (c *HTTPRPC) Call(ctx context.Context, method string, params any, out any) error {
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "beeline", "method": method, "params": params})
	if err != nil {
		return err
	}
	var last error
	for _, u := range c.URLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := c.HTTP.Do(req)
		if err != nil {
			last = err
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		res.Body.Close()
		var env struct {
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Name  string `json:"name"`
				Cause struct {
					Name string          `json:"name"`
					Info json.RawMessage `json:"info"`
				} `json:"cause"`
			} `json:"error"`
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&env); err != nil {
			last = fmt.Errorf("near rpc: unreadable response (HTTP %d)", res.StatusCode)
			continue
		}
		if env.Error != nil {
			return &RPCError{Name: env.Error.Name, Cause: env.Error.Cause.Name, Info: truncate(string(env.Error.Cause.Info), 120)}
		}
		d2 := json.NewDecoder(bytes.NewReader(env.Result))
		d2.UseNumber()
		if err := d2.Decode(out); err != nil {
			last = fmt.Errorf("near rpc: unexpected result shape: %w", err)
			continue
		}
		return nil
	}
	if last == nil {
		last = errors.New("near rpc: no endpoints configured")
	}
	return last
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
