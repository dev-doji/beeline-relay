package playground

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/near"
)

// FromEnv builds the playground config. It returns (nil, nil) when PLAYGROUND_VAULT is unset,
// which disables the playground endpoints.
//
// The demo accounts default to acme.<owner>, ada-fabrics.<owner> and scammer.<owner> for a vault
// named vault.<owner> (scripts/live-setup.ps1 creates them). Keys come from
// PLAYGROUND_AGENT_KEY / PLAYGROUND_INVOICE_ISSUER_KEY ("ed25519:<base58>") or from the files
// agent-key.json / invoice-issuer.json in PLAYGROUND_KEYS_DIR. Both are TESTNET-only keys.
func FromEnv(lookup func(string) (string, bool)) (*Config, error) {
	get := func(k, def string) string {
		if v, ok := lookup(k); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return def
	}
	vault := get("PLAYGROUND_VAULT", "")
	if vault == "" {
		return nil, nil
	}
	if !strings.HasSuffix(vault, ".testnet") {
		return nil, errors.New("PLAYGROUND_VAULT must be a .testnet account: the playground never runs on mainnet")
	}
	owner := strings.TrimPrefix(vault, "vault.")
	dir := get("PLAYGROUND_KEYS_DIR", "")
	readKey := func(envName, file string) (string, error) {
		if v := get(envName, ""); v != "" {
			return v, nil
		}
		if dir == "" {
			return "", fmt.Errorf("%s or PLAYGROUND_KEYS_DIR is required", envName)
		}
		raw, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", file, err)
		}
		var kf struct {
			PrivateKey string `json:"private_key"`
		}
		if err := json.Unmarshal(raw, &kf); err != nil || kf.PrivateKey == "" {
			return "", fmt.Errorf("%s has no private_key", file)
		}
		return kf.PrivateKey, nil
	}
	agentSecret, err := readKey("PLAYGROUND_AGENT_KEY", "agent-key.json")
	if err != nil {
		return nil, err
	}
	agentKey, err := near.ParseKey(vault, agentSecret)
	if err != nil {
		return nil, fmt.Errorf("PLAYGROUND_AGENT_KEY: %w", err)
	}
	issuerSecret, err := readKey("PLAYGROUND_INVOICE_ISSUER_KEY", "invoice-issuer.json")
	if err != nil {
		return nil, err
	}
	issuer, err := near.ParseKey("invoice-issuer", issuerSecret)
	if err != nil {
		return nil, fmt.Errorf("PLAYGROUND_INVOICE_ISSUER_KEY: %w", err)
	}
	return &Config{
		Network:         "testnet",
		Vault:           vault,
		Token:           get("PLAYGROUND_TOKEN", "usdc.fakes.testnet"),
		Approved:        Account{Label: "Acme Design", Account: get("PLAYGROUND_APPROVED_ACCOUNT", "acme."+owner), Role: "approved"},
		Vendor:          Account{Label: "Ada Fabrics", Account: get("PLAYGROUND_VENDOR_ACCOUNT", "ada-fabrics."+owner), Role: "vendor"},
		Scammer:         Account{Label: "Scammer", Account: get("PLAYGROUND_SCAMMER_ACCOUNT", "scammer."+owner), Role: "attacker"},
		ExplorerTxURL:   "https://testnet.nearblocks.io/txns/",
		ExplorerAcctURL: "https://testnet.nearblocks.io/address/",
		AgentKey:        agentKey,
		InvoiceIssuer:   ed25519.PrivateKey(issuer.Private),
	}, nil
}

// LiveAccount is a demo account with its live token balance.
type LiveAccount struct {
	Account
	Balance string `json:"balance"` // base units; "" if unknown
	URL     string `json:"url"`
}

// State is the vault's live configuration and the demo accounts' balances, read from the chain.
type State struct {
	Network     string            `json:"network"`
	Vault       string            `json:"vault"`
	VaultURL    string            `json:"vault_url"`
	Token       string            `json:"token"`
	Agent       string            `json:"agent_public_key"`
	Accounts    []LiveAccount     `json:"accounts"`
	Config      json.RawMessage   `json:"config"`
	Recipients  json.RawMessage   `json:"recipients"`
	Reviewer    json.RawMessage   `json:"reviewer"`
	Balances    json.RawMessage   `json:"balances"`
	Agents      json.RawMessage   `json:"agents"`
	Issuer      json.RawMessage   `json:"invoice_issuer"`
	Presets     map[string]string `json:"presets"`
	Disclosures map[string]string `json:"disclosures"`
}

// State reads the vault's views and the demo accounts' balances in parallel.
func (s *Service) State(ctx context.Context) (*State, error) {
	views := []string{"get_config", "get_recipients", "get_reviewer", "get_balances", "get_agents", "get_invoice_issuer"}
	out := make(map[string]json.RawMessage, len(views))
	accts := []Account{s.Cfg.Approved, s.Cfg.Vendor, s.Cfg.Scammer}
	bal := make([]string, len(accts))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, m := range views {
		wg.Add(1)
		go func(m string) {
			defer wg.Done()
			raw, err := s.Chain.View(ctx, s.Cfg.Vault, m, map[string]any{})
			if err != nil || !json.Valid(raw) {
				raw = json.RawMessage("null")
			}
			mu.Lock()
			out[m] = raw
			mu.Unlock()
		}(m)
	}
	for i, a := range accts {
		wg.Add(1)
		go func(i int, account string) {
			defer wg.Done()
			raw, err := s.Chain.View(ctx, s.Cfg.Token, "ft_balance_of", map[string]any{"account_id": account})
			var b string
			if err == nil && json.Unmarshal(raw, &b) == nil {
				if _, ok := new(big.Int).SetString(b, 10); ok {
					bal[i] = b
				}
			}
		}(i, a.Account)
	}
	wg.Wait()
	if string(out["get_config"]) == "null" {
		return nil, errors.New("the vault is not reachable or not deployed")
	}
	var cfg struct {
		MaxPerPayment string `json:"max_per_payment"`
	}
	_ = json.Unmarshal(out["get_config"], &cfg)
	live := make([]LiveAccount, len(accts))
	for i, a := range accts {
		live[i] = LiveAccount{Account: a, Balance: bal[i], URL: s.Cfg.ExplorerAcctURL + a.Account}
	}
	return &State{
		Network: s.Cfg.Network, Vault: s.Cfg.Vault, VaultURL: s.Cfg.ExplorerAcctURL + s.Cfg.Vault,
		Token: s.Cfg.Token, Agent: s.Cfg.AgentKey.PublicKey(), Accounts: live,
		Config: out["get_config"], Recipients: out["get_recipients"], Reviewer: out["get_reviewer"],
		Balances: out["get_balances"], Agents: out["get_agents"], Issuer: out["get_invoice_issuer"],
		Presets: s.Presets(cfg.MaxPerPayment),
		Disclosures: map[string]string{
			"agent":   "Bee is a NEAR AI model that decides whom to pay. It is deliberately naive: it follows instructions hidden in notes.",
			"review":  "A second NEAR AI model, in a TEE, reviews each payment. The vault verifies its ed25519 signature on-chain.",
			"payment": "Real NEAR testnet transactions moving test USDC (usdc.fakes.testnet, no value) to real testnet accounts.",
			"mainnet": "On mainnet, the same guardrails pay cross-chain through NEAR Intents 1Click (pay_quote), which has no testnet.",
		},
	}, nil
}
