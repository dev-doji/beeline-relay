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
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/mr-tron/base58"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/nearai"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/testnetquote"
)

const (
	dir        = ".testnet"
	signerFile = "test-quote-signer.json"
	issuerFile = "invoice-issuer.json"
	agentFile  = "agent-key.json"
	testToken  = "usdc.fakes.testnet"
	destAsset  = "nep141:base-0x833589fcd6edb6e08f4c7c32d4f71b54bda02913.omft.near"
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
	case "reviewer-args":
		err = reviewerArgs(os.Args[2:])
	case "invoice-issuer-args":
		err = invoiceIssuerArgs()
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "vault-testnet:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: vault-testnet keys -vault <account> | quote -vault <account> -recipient <0x…> -amount <base units> -name <label> [-forged] | reviewer-args [-model m] [-threshold n]")
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

// keys creates the test quote signer and the agent key, unless they already exist. The agent
// key file names the vault account it signs for, so it is recreated when the vault changes.
func keys(args []string) error {
	fs := flag.NewFlagSet("keys", flag.ExitOnError)
	vault := fs.String("vault", "", "vault account ID, e.g. vault.alice.testnet")
	_ = fs.Parse(args)
	if *vault == "" {
		return errors.New("-vault is required")
	}
	for _, f := range []struct{ name, account string }{{signerFile, "test-quote-signer"}, {agentFile, *vault}, {issuerFile, "invoice-issuer"}} {
		if existing, err := readKey(f.name); err == nil && existing.AccountID == f.account {
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
	q, err := testnetquote.Build(ed25519.PrivateKey(priv), testnetquote.Params{
		Vault: *vault, Token: testToken, DestinationAsset: destAsset,
		Recipient: *recipient, Amount: *amount, Now: time.Now(),
	})
	if err != nil {
		return err
	}
	out := fmt.Sprintf("pay-%s.json", *name)
	if err := writeJSON(out, map[string]string{"quote_json": q.JSON}); err != nil {
		return err
	}
	fmt.Printf("wrote %s (deposit account %s, recipient %s, amount %s, forged=%v)\n",
		filepath.Join(dir, out), q.DepositAddress, *recipient, *amount, *forged)
	return nil
}

// reviewerArgs checks the NEAR AI model's attestation live (the reviewer key must be bound in
// its Intel TDX quote with a fresh nonce) and writes .testnet/set-reviewer.json for set_reviewer.
func reviewerArgs(args []string) error {
	fs := flag.NewFlagSet("reviewer-args", flag.ExitOnError)
	model := fs.String("model", envOr("NEAR_AI_REVIEW_MODEL", "z-ai/glm-5.3-flash"), "NEAR AI Cloud model (TEE-hosted)")
	threshold := fs.String("threshold", "1", "payments of at least this many base units need a review")
	_ = fs.Parse(args)
	_ = godotenv.Load()
	key := strings.TrimSpace(os.Getenv("NEAR_AI_API_KEY"))
	if key == "" {
		return errors.New("NEAR_AI_API_KEY is not set in beeline-api/.env")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	att, err := nearai.NewClient(os.Getenv("NEAR_AI_API_BASE_URL"), key, time.Minute).CheckAttestation(ctx, *model, "")
	if err != nil {
		return fmt.Errorf("attestation check failed: %w", err)
	}
	out := map[string]string{
		"model": *model, "signing_key": att.SigningAddress,
		"policy_prompt_sha256": nearai.PolicySHA256(), "threshold": *threshold,
	}
	if err := writeJSON("set-reviewer.json", out); err != nil {
		return err
	}
	fmt.Printf("attested key %s for %s (TDX report_data binds it with nonce %s…)\nwrote %s\n",
		att.SigningAddress, *model, att.Nonce[:12], filepath.Join(dir, "set-reviewer.json"))
	return nil
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

// invoiceIssuerArgs writes .testnet/set-invoice-issuer.json with the hex public key of the
// TESTNET invoice issuer (created by `keys`), for the vault's set_invoice_issuer.
func invoiceIssuerArgs() error {
	k, err := readKey(issuerFile)
	if err != nil {
		return err
	}
	pub, err := base58.Decode(strings.TrimPrefix(k.PublicKey, "ed25519:"))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("bad invoice issuer key file")
	}
	if err := writeJSON("set-invoice-issuer.json", map[string]string{"signing_key": hex.EncodeToString(pub)}); err != nil {
		return err
	}
	fmt.Printf("invoice issuer %s\nwrote %s\n", hex.EncodeToString(pub), filepath.Join(dir, "set-invoice-issuer.json"))
	return nil
}
