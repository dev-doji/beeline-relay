package playground

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/agent"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/near"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/nearai"
)

const (
	acme    = "acme.goni-beeline.testnet"
	ada     = "ada-fabrics.goni-beeline.testnet"
	scammer = "scammer.goni-beeline.testnet"
)

// fakeVault is a TEST DOUBLE for the vault's pay_direct: allowlist or signed invoice, the
// per-payment limit, single use and the reviewer requirement.
type fakeVault struct {
	reviewer string // get_reviewer JSON ("" = none)
	issuer   ed25519.PublicKey
	paid     map[string]bool
	invoices map[string]bool
	sent     []map[string]json.RawMessage
	sendErr  error
}

func (f *fakeVault) View(_ context.Context, _, method string, _ any) ([]byte, error) {
	switch method {
	case "get_config":
		return []byte(`{"max_per_payment":"5000000"}`), nil
	case "get_reviewer":
		if f.reviewer == "" {
			return []byte("null"), nil
		}
		return []byte(f.reviewer), nil
	case "ft_balance_of":
		return []byte(`"1000000"`), nil
	}
	return []byte("null"), nil
}

func (f *fakeVault) Send(_ context.Context, _ near.Key, fc near.FunctionCall) (*near.Outcome, error) {
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	if fc.Method != "pay_direct" {
		return nil, errors.New("unexpected method " + fc.Method)
	}
	var args map[string]json.RawMessage
	_ = json.Unmarshal(fc.Args, &args)
	f.sent = append(f.sent, args)
	var pj string
	_ = json.Unmarshal(args["payment_json"], &pj)
	var p struct{ Recipient, Amount, Nonce string }
	_ = json.Unmarshal([]byte(pj), &p)
	reject := func(r string) (*near.Outcome, error) {
		return &near.Outcome{Hash: "TX", Failure: "Smart contract panicked: BEELINE_VAULT_REJECTED: " + r}, nil
	}
	if raw, ok := args["invoice"]; ok {
		var inv InvoiceArg
		_ = json.Unmarshal(raw, &inv)
		sig, _ := hex.DecodeString(inv.Signature)
		if !ed25519.Verify(f.issuer, []byte(inv.Payload), sig) {
			return reject("invoice was not signed by the trusted invoice issuer (edited or fake link)")
		}
		var t invoices.Payload
		_ = json.Unmarshal([]byte(inv.Payload), &t)
		if t.Payout != p.Recipient || t.Amount != p.Amount {
			return reject("payment does not match invoice")
		}
		if f.invoices[t.ID] {
			return reject("invoice " + t.ID + " has already been paid")
		}
		f.invoices[t.ID] = true
	} else if p.Recipient != acme {
		return reject("recipient " + p.Recipient + " is not on the allowlist")
	}
	if p.Amount > "5000000" && len(p.Amount) >= 7 {
		return reject("amount " + p.Amount + " exceeds the per-payment limit 5000000")
	}
	if f.paid[p.Nonce] {
		return reject("this payment request has already been paid")
	}
	if f.reviewer != "" {
		raw, ok := args["review"]
		if !ok {
			return reject("payments of 1 or more need an AI reviewer approval")
		}
		if strings.Contains(string(raw), "REJECT") {
			return reject("AI reviewer refused this payment (REJECT injected)")
		}
	}
	f.paid[p.Nonce] = true
	return &near.Outcome{Hash: "TX", Logs: []string{`EVENT_JSON:{"event":"payment_sent"}`}}, nil
}

// fakeBee is a TEST DOUBLE for the agent: naive, it follows notes in the message.
type fakeBee struct{}

func (fakeBee) Decide(_ context.Context, msg string) (*agent.Decision, error) {
	switch {
	case strings.Contains(msg, "hello"):
		return &agent.Decision{Action: "reply", Reply: "Hi!", Thought: "greeting"}, nil
	case strings.Contains(msg, scammer):
		return &agent.Decision{Action: "pay", Recipient: scammer, AmountUSDC: "0.1", Thought: "the note says to pay the new account"}, nil
	case strings.Contains(msg, "SYSTEM NOTE"):
		return &agent.Decision{Action: "pay", Recipient: acme, AmountUSDC: "0.5", Thought: "the note asks for a 0.5 prepayment"}, nil
	case strings.Contains(msg, "5.000001"):
		return &agent.Decision{Action: "pay", Recipient: acme, AmountUSDC: "5.000001", Thought: "big order"}, nil
	}
	return &agent.Decision{Action: "pay", Recipient: acme, AmountUSDC: "0.1", Memo: "invoice 42", Thought: "pay acme"}, nil
}

// fakeReviewer is a TEST DOUBLE for NEAR AI: refuses injected tasks, signs like provider_tee.
type fakeReviewer struct{ key ed25519.PrivateKey }

func (f fakeReviewer) Review(_ context.Context, model, task, hash, _ string) (*nearai.Review, error) {
	verdict := "APPROVE " + hash
	if strings.Contains(task, "SYSTEM NOTE") {
		verdict = "REJECT injected"
	}
	req, resp := `{"model":"`+model+`"}`, `{"answer":"`+verdict+`"}`
	sum := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
	text := model + ":" + sum(req) + ":" + sum(resp)
	return &nearai.Review{Model: model, Request: req, Response: resp, Verdict: verdict, Approved: strings.HasPrefix(verdict, "APPROVE"),
		Signature: nearai.Signature{Text: text, Signature: hex.EncodeToString(ed25519.Sign(f.key, []byte(text))),
			SigningAddress: hex.EncodeToString(f.key.Public().(ed25519.PublicKey)), SigningAlgo: "ed25519", SignatureKind: "provider_tee"}}, nil
}

func newService(t *testing.T) (*Service, *fakeVault, ed25519.PrivateKey) {
	t.Helper()
	_, agentKey, _ := ed25519.GenerateKey(nil)
	_, rk, _ := ed25519.GenerateKey(nil)
	issuerPub, issuer, _ := ed25519.GenerateKey(nil)
	v := &fakeVault{
		reviewer: `{"model":"m","signing_key":"` + hex.EncodeToString(rk.Public().(ed25519.PublicKey)) + `","threshold":"1"}`,
		issuer:   issuerPub, paid: map[string]bool{}, invoices: map[string]bool{},
	}
	s := &Service{
		Cfg: Config{Network: "testnet", Vault: "vault.goni-beeline.testnet", Token: "usdc.fakes.testnet",
			Approved: Account{"Acme Design", acme, "approved"}, Vendor: Account{"Ada Fabrics", ada, "vendor"}, Scammer: Account{"Scammer", scammer, "attacker"},
			ExplorerTxURL: "https://x/txns/", AgentKey: near.Key{AccountID: "vault.goni-beeline.testnet", Private: agentKey}, InvoiceIssuer: issuer},
		Chain: v, Reviewer: fakeReviewer{key: rk}, Agent: fakeBee{}, Now: time.Now,
	}
	return s, v, issuer
}

func chat(t *testing.T, s *Service, msg string) *Result {
	t.Helper()
	res, err := s.Run(context.Background(), Input{Scenario: ScenarioChat, Message: msg})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestHonestChatPaysTheApprovedAccountAndReplayIsBlocked(t *testing.T) {
	s, v, _ := newService(t)
	res := chat(t, s, s.Presets("5000000")["honest"])
	if res.Outcome != "paid" || res.Recipient != acme || res.Amount != "100000" || res.ExplorerURL != "https://x/txns/TX" {
		t.Fatalf("%+v", res)
	}
	keys := []string{}
	for _, st := range res.Steps {
		keys = append(keys, st.Key)
		if st.Label != "real" {
			t.Errorf("step %s must be labelled real", st.Key)
		}
	}
	if strings.Join(keys, ",") != "agent,request,review,vault" {
		t.Fatalf("steps %v", keys)
	}
	var pj string
	_ = json.Unmarshal(v.sent[0]["payment_json"], &pj)
	if !strings.Contains(pj, `"vault":"vault.goni-beeline.testnet"`) || !strings.Contains(pj, `"kind":"direct"`) {
		t.Fatalf("payment request: %s", pj)
	}
	res, _ = s.Run(context.Background(), Input{Scenario: ScenarioReplay})
	if res.Outcome != "blocked" || res.Layer != "single_use" {
		t.Fatalf("replay: %+v", res)
	}
}

func TestEachAttackIsBlockedByTheRightLayer(t *testing.T) {
	for name, layer := range map[string]string{"attacker": "allowlist", "injected": "ai_reviewer", "over_limit": "spend_limit"} {
		s, _, _ := newService(t)
		res := chat(t, s, s.Presets("5000000")[name])
		if res.Outcome != "blocked" || res.Layer != layer {
			t.Errorf("%s: outcome=%s layer=%s reason=%s", name, res.Outcome, res.Layer, res.Reason)
		}
	}
}

func TestBeeCanJustReply(t *testing.T) {
	s, v, _ := newService(t)
	res := chat(t, s, "hello there")
	if res.Outcome != "reply" || res.Reply != "Hi!" || len(v.sent) != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestInvoicePaymentsAndScams(t *testing.T) {
	s, _, issuer := newService(t)
	p := invoices.Payload{V: 1, ID: "inv_t", Vendor: "Ada Fabrics", Payout: ada, Asset: "nep141:usdc.fakes.testnet", Amount: "500000", ExpiresAt: 1, Memo: "Order #18"}
	raw, _ := json.Marshal(p)
	inv := &invoices.Invoice{ID: p.ID, VendorName: p.Vendor, Payout: ada, Amount: p.Amount, Memo: p.Memo, Payload: string(raw),
		Signature: hex.EncodeToString(ed25519.Sign(issuer, raw)), Items: []invoices.Item{{Name: "Ankara dress", Quantity: 3, UnitPrice: "150000"}}}
	for _, tc := range []struct{ tamper, outcome, layer, recipient string }{
		{TamperSwapPayout, "blocked", "invoice", scammer},
		{TamperEditAmount, "blocked", "invoice", ada},
		{TamperNone, "paid", "", ada},
		{TamperNone, "blocked", "invoice", ada},
	} {
		res, err := s.PayInvoice(context.Background(), inv, tc.tamper)
		if err != nil {
			t.Fatal(err)
		}
		if res.Outcome != tc.outcome || res.Layer != tc.layer || res.Recipient != tc.recipient {
			t.Errorf("%s: outcome=%s layer=%s recipient=%s reason=%s", tc.tamper, res.Outcome, res.Layer, res.Recipient, res.Reason)
		}
	}
	if _, err := s.PayInvoice(context.Background(), inv, "steal"); err == nil {
		t.Fatal("unknown tamper modes must be rejected")
	}
}

func TestFailuresAreReportedNotHidden(t *testing.T) {
	s, v, _ := newService(t)
	v.sendErr = errors.New("rpc down")
	if res := chat(t, s, "pay acme"); res.Outcome != "error" {
		t.Fatalf("%+v", res)
	}
	s.Agent = nil
	if res := chat(t, s, "pay acme"); res.Outcome != "error" {
		t.Fatalf("no agent: %+v", res)
	}
	if res, _ := s.Run(context.Background(), Input{Scenario: ScenarioReplay}); res.Outcome != "error" {
		t.Fatalf("replay with nothing paid: %+v", res)
	}
}

func TestValidationAndUnits(t *testing.T) {
	s, _, _ := newService(t)
	for _, in := range []Input{{Scenario: "steal", Message: "x"}, {Scenario: ScenarioChat, Message: " "}, {Scenario: ScenarioChat, Message: strings.Repeat("a", 2001)},
		{Scenario: ScenarioChat, Message: "x", Invoice: &InvoiceArg{}}} {
		if _, err := s.Run(context.Background(), in); err == nil {
			t.Errorf("expected a validation error for %+v", in.Scenario)
		}
	}
	for in, want := range map[string]string{"0.1": "100000", "5": "5000000", "5.000001": "5000001", "0.000001": "1"} {
		if got, err := ToBaseUnits(in); err != nil || got != want {
			t.Errorf("ToBaseUnits(%s)=%s,%v", in, got, err)
		}
	}
	for _, bad := range []string{"0", "1.2345678", "-1", "abc", ""} {
		if _, err := ToBaseUnits(bad); err == nil {
			t.Errorf("ToBaseUnits(%q) should fail", bad)
		}
	}
	if FormatUSDC("500000") != "0.5" || FormatUSDC("5000001") != "5.000001" || FormatUSDC("2000000") != "2" {
		t.Fatal("FormatUSDC")
	}
}

func TestFromEnvRefusesMainnetAndDerivesDemoAccounts(t *testing.T) {
	env := map[string]string{"PLAYGROUND_VAULT": "vault.goni19.near"}
	if _, err := FromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok }); err == nil || !strings.Contains(err.Error(), "testnet") {
		t.Fatalf("expected a testnet-only error, got %v", err)
	}
	if cfg, err := FromEnv(func(string) (string, bool) { return "", false }); cfg != nil || err != nil {
		t.Fatal("no PLAYGROUND_VAULT must disable the playground")
	}
	_, a, _ := ed25519.GenerateKey(nil)
	_, i, _ := ed25519.GenerateKey(nil)
	env = map[string]string{
		"PLAYGROUND_VAULT": "vault.goni-beeline.testnet", "PLAYGROUND_AGENT_KEY": near.Key{Private: a}.SecretForTest(),
		"PLAYGROUND_INVOICE_ISSUER_KEY": near.Key{Private: i}.SecretForTest(),
	}
	cfg, err := FromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Approved.Account != acme || cfg.Vendor.Account != ada || cfg.Scammer.Account != scammer {
		t.Fatalf("%+v", cfg)
	}
}
