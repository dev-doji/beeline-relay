package playground

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/near"
)

// FromEnv builds the playground config. It returns (nil, nil) when PLAYGROUND_VAULT is unset,
// which disables the playground endpoints.
//
// Keys come from PLAYGROUND_AGENT_KEY / PLAYGROUND_QUOTE_SIGNER_KEY ("ed25519:<base58>"), or
// from the files agent-key.json / test-quote-signer.json in PLAYGROUND_KEYS_DIR (the format
// cmd/vault-testnet writes). Both are TESTNET-only keys.
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
	agent, err := near.ParseKey(vault, agentSecret)
	if err != nil {
		return nil, fmt.Errorf("PLAYGROUND_AGENT_KEY: %w", err)
	}
	signerSecret, err := readKey("PLAYGROUND_QUOTE_SIGNER_KEY", "test-quote-signer.json")
	if err != nil {
		return nil, err
	}
	signer, err := near.ParseKey("test-quote-signer", signerSecret)
	if err != nil {
		return nil, fmt.Errorf("PLAYGROUND_QUOTE_SIGNER_KEY: %w", err)
	}
	return &Config{
		Network:          "testnet",
		Vault:            vault,
		Token:            get("PLAYGROUND_TOKEN", "usdc.fakes.testnet"),
		DestinationAsset: "nep141:base-0x833589fcd6edb6e08f4c7c32d4f71b54bda02913.omft.near",
		Allowed:          Recipient{Label: "Acme Design", Address: get("PLAYGROUND_ALLOWED_RECIPIENT", "0x1111111111111111111111111111111111111111")},
		Attacker:         Recipient{Label: "Unknown address from an injected note", Address: "0x6666666666666666666666666666666666666666"},
		DefaultAmount:    "100000",
		ExplorerTxURL:    "https://testnet.nearblocks.io/txns/",
		ExplorerAcctURL:  "https://testnet.nearblocks.io/address/",
		AgentKey:         agent,
		QuoteSigner:      ed25519.PrivateKey(signer.Private),
	}, nil
}

// State is the vault's live configuration, read from the chain for the UI.
type State struct {
	Network     string            `json:"network"`
	Vault       string            `json:"vault"`
	VaultURL    string            `json:"vault_url"`
	Token       string            `json:"token"`
	Agent       string            `json:"agent_public_key"`
	Allowed     Recipient         `json:"allowed_recipient"`
	Attacker    Recipient         `json:"attacker_recipient"`
	Config      json.RawMessage   `json:"config"`
	Recipients  json.RawMessage   `json:"recipients"`
	Reviewer    json.RawMessage   `json:"reviewer"`
	Balances    json.RawMessage   `json:"balances"`
	Agents      json.RawMessage   `json:"agents"`
	Presets     map[string]Input  `json:"presets"`
	Disclosures map[string]string `json:"disclosures"`
}

// State reads the vault's views. Missing views are reported as null.
func (s *Service) State(ctx context.Context) (*State, error) {
	view := func(method string) json.RawMessage {
		raw, err := s.Chain.View(ctx, s.Cfg.Vault, method, map[string]any{})
		if err != nil || !json.Valid(raw) {
			return json.RawMessage("null")
		}
		return raw
	}
	st := &State{
		Network: s.Cfg.Network, Vault: s.Cfg.Vault, VaultURL: s.Cfg.ExplorerAcctURL + s.Cfg.Vault,
		Token: s.Cfg.Token, Agent: s.Cfg.AgentKey.PublicKey(), Allowed: s.Cfg.Allowed, Attacker: s.Cfg.Attacker,
		Config: view("get_config"), Recipients: view("get_recipients"), Reviewer: view("get_reviewer"),
		Balances: view("get_balances"), Agents: view("get_agents"), Presets: s.Presets(),
		Disclosures: map[string]string{
			"quotes":   "TESTNET-SIMULATED: 1Click has no testnet, so quotes are signed by the vault's test signer. A mainnet vault trusts only 1Click's real key.",
			"review":   "Real: a NEAR AI Cloud completion with a TEE ed25519 signature that the vault verifies on-chain.",
			"payment":  "Real NEAR testnet transaction. Test USDC (usdc.fakes.testnet) has no value.",
			"agentKey": "The agent's key is a testnet function-call key that can only call pay_quote on this vault.",
		},
	}
	if string(st.Config) == "null" {
		return nil, errors.New("the vault is not reachable or not deployed")
	}
	return st, nil
}
