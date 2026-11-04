// Command vault-testnet supports the TESTNET rehearsal of the agent vault (see
// docs/TESTNET_REHEARSAL.md). 1Click does not run on testnet, so this tool plays its part with
// a local TEST signing key. Quotes it produces are marked "TESTNET-SIMULATED" and can only be
// paid by a vault initialized with that test key, never by a mainnet vault, which trusts only
// 1Click's real key.
//
// Keys are written to beeline-api/.testnet/ (gitignored). They are testnet-only keys created by
// this tool; no user wallet key is ever read or stored.
//
//	go run ./cmd/vault-testnet keys -vault vault.alice.testnet
//	go run ./cmd/vault-testnet quote -vault vault.alice.testnet -recipient 0x1111… -amount 1000000 -name allowed
//	go run ./cmd/vault-testnet quote … -forged     # signed with a different key (attack demo)
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mr-tron/base58"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents"
)

const (
	dir           = ".testnet"
	signerFile    = "test-quote-signer.json"
	agentFile     = "agent-key.json"
	testToken     = "usdc.fakes.testnet"
	destAsset     = "nep141:base-0x833589fcd6edb6e08f4c7c32d4f71b54bda02913.omft.near"
	correlationID = "TESTNET-SIMULATED-QUOTE"
)

// keyFile matches the near CLI access-key file format.
type keyFile struct {
	AccountID  string `json:"account_id"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "keys":
		err = keys(os.Args[2:])
	case "quote":
		err = quote(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "vault-testnet:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: vault-testnet keys -vault <account> | quote -vault <account> -recipient <0x…> -amount <base units> -name <label> [-forged]")
	os.Exit(2)
}

func newKey(accountID string) (keyFile, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return keyFile{}, err
	}
	return keyFile{
		AccountID:  accountID,
		PublicKey:  "ed25519:" + base58.Encode(pub),
		PrivateKey: "ed25519:" + base58.Encode(priv), // NEAR format: 64-byte seed||public key
	}, nil
}

func writeJSON(name string, v any) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return os.WriteFile(filepath.Join(dir, name), b, 0o600)
}

func readKey(name string) (keyFile, error) {
	var k keyFile
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return k, fmt.Errorf("%s not found: run `vault-testnet keys` first", name)
	}
	return k, json.Unmarshal(b, &k)
}

// keys creates the test quote signer and the agent key, unless they already exist.
func keys(args []string) error {
	fs := flag.NewFlagSet("keys", flag.ExitOnError)
	vault := fs.String("vault", "", "vault account ID, e.g. vault.alice.testnet")
	_ = fs.Parse(args)
	if *vault == "" {
		return errors.New("-vault is required")
	}
	for _, f := range []struct{ name, account string }{{signerFile, "test-quote-signer"}, {agentFile, *vault}} {
		if _, err := os.Stat(filepath.Join(dir, f.name)); err == nil {
			continue
		}
		k, err := newKey(f.account)
		if err != nil {
			return err
		}
		if err := writeJSON(f.name, k); err != nil {
			return err
		}
	}
	signer, _ := readKey(signerFile)
	agent, _ := readKey(agentFile)
	fmt.Printf("TEST_QUOTE_SIGNER=%s\nAGENT_PUBLIC_KEY=%s\n", signer.PublicKey, agent.PublicKey)
	return nil
}

func quote(args []string) error {
	fs := flag.NewFlagSet("quote", flag.ExitOnError)
	vault := fs.String("vault", "", "vault account ID")
	recipient := fs.String("recipient", "", "final recipient on Base (0x…)")
	amount := fs.String("amount", "1000000", "amount in base units (6 decimals: 1000000 = 1 USDC)")
	name := fs.String("name", "quote", "label for the output file")
	forged := fs.Bool("forged", false, "sign with a throwaway key instead of the test signer (attack demo)")
	_ = fs.Parse(args)
	if *vault == "" || *recipient == "" {
		return errors.New("-vault and -recipient are required")
	}

	signer, err := readKey(signerFile)
	if err != nil {
		return err
	}
	if *forged {
		if signer, err = newKey("forger"); err != nil {
			return err
		}
	}
	priv, err := base58.Decode(signer.PrivateKey[len("ed25519:"):])
	if err != nil || len(priv) != ed25519.PrivateKeySize {
		return errors.New("bad signer key file")
	}

	// A fresh implicit account plays the role of 1Click's deposit address.
	var dep [32]byte
	_, _ = rand.Read(dep[:])
	now := time.Now().UTC()
	resp := map[string]any{
		"correlationId": correlationID,
		"timestamp":     now.Format("2006-01-02T15:04:05.000Z"),
		"signature":     "",
		"quoteRequest": map[string]any{
			"dry": false, "swapType": "EXACT_INPUT", "slippageTolerance": 100,
			"originAsset": "nep141:" + testToken, "depositType": "ORIGIN_CHAIN",
			"destinationAsset": destAsset, "amount": *amount,
			"refundTo": *vault, "refundType": "ORIGIN_CHAIN",
			"recipient": *recipient, "recipientType": "DESTINATION_CHAIN",
			"deadline": now.Add(10 * time.Minute).Format("2006-01-02T15:04:05.000Z"),
		},
		"quote": map[string]any{
			"amountIn": *amount, "minAmountIn": *amount, "amountOut": *amount, "minAmountOut": *amount,
			"depositAddress": hex.EncodeToString(dep[:]),
			"deadline":       now.Add(24 * time.Hour).Format("2006-01-02T15:04:05.000Z"),
			"timeEstimate":   22,
		},
	}
	unsigned, _ := json.Marshal(resp)
	hash, err := intents.QuoteHash(unsigned)
	if err != nil {
		return err
	}
	resp["signature"] = "ed25519:" + base58.Encode(ed25519.Sign(ed25519.PrivateKey(priv), []byte(hash)))
	quoteJSON, _ := json.Marshal(resp)

	out := fmt.Sprintf("pay-%s.json", *name)
	if err := writeJSON(out, map[string]string{"quote_json": string(quoteJSON)}); err != nil {
		return err
	}
	fmt.Printf("wrote %s (deposit account %s, recipient %s, amount %s, forged=%v)\n",
		filepath.Join(dir, out), resp["quote"].(map[string]any)["depositAddress"], *recipient, *amount, *forged)
	return nil
}
