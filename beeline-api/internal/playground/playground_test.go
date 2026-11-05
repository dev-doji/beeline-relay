package playground

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/near"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/nearai"
)

// fakeChain is a TEST DOUBLE for the vault: it applies a few of the vault's rules so the run
// logic can be tested without a network.
type fakeChain struct {
	reviewer string // get_reviewer JSON
	sent     []map[string]any
	paid     map[string]bool
	sendErr  error
}

func (f *fakeChain) View(_ context.Context, _, method string, _ any) ([]byte, error) {
	switch method {
	case "get_config":
		return []byte(`{"max_per_payment":"5000000"}`), nil
	case "get_reviewer":
		if f.reviewer == "" {
			return []byte("null"), nil
		}
		return []byte(f.reviewer), nil
	}
	return []byte("null"), nil
}

func (f *fakeChain) Send(_ context.Context, _ near.Key, fc near.FunctionCall) (*near.Outcome, error) {
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	var args map[string]any
	_ = json.Unmarshal(fc.Args, &args)
	f.sent = append(f.sent, args)
	var q struct {
		QuoteRequest struct{ Recipient, Amount string } `json:"quoteRequest"`
		Quote        struct {
			DepositAddress string `json:"depositAddress"`
		} `json:"quote"`
	}
	_ = json.Unmarshal([]byte(args["quote_json"].(string)), &q)
	reject := func(r string) (*near.Outcome, error) {
		return &near.Outcome{Hash: "TX", Failure: "Smart contract panicked: BEELINE_VAULT_REJECTED: " + r}, nil
	}
	switch {
	case q.QuoteRequest.Recipient != "0x1111111111111111111111111111111111111111":
		return reject("recipient " + q.QuoteRequest.Recipient + " is not on the allowlist")
	case q.QuoteRequest.Amount == "5000001":
		return reject("amount 5000001 exceeds the per-payment limit 5000000")
	case f.paid[q.Quote.DepositAddress]:
		return reject("this deposit address has already been paid")
	}
	if f.reviewer != "" {
		r, ok := args["review"]
		if !ok {
			return reject("payments of 1 or more need an AI reviewer approval")
		}
		if strings.Contains(r.(map[string]any)["response"].(string), "REJECT") {
			return reject("AI reviewer refused this payment (REJECT injected)")
		}
	}
	if f.paid == nil {
		f.paid = map[string]bool{}
	}
	f.paid[q.Quote.DepositAddress] = true
	return &near.Outcome{Hash: "TX", Logs: []string{`EVENT_JSON:{"event":"payment_sent"}`}}, nil
}

// fakeReviewer is a TEST DOUBLE for NEAR AI: it rejects tasks that mention "SYSTEM NOTE" and
// signs with a test key the fake vault registers.
type fakeReviewer struct{ key ed25519.PrivateKey }

func (f fakeReviewer) Review(_ context.Context, model, task, hash, _ string) (*nearai.Review, error) {
	verdict := "APPROVE " + hash
	if strings.Contains(task, "SYSTEM NOTE") {
		verdict = "REJECT injected"
	}
	req, resp := `{"model":"`+model+`"}`, `{"answer":"`+verdict+`"}`
	r := &nearai.Review{Model: model, Request: req, Response: resp, Verdict: verdict, Approved: strings.HasPrefix(verdict, "APPROVE")}
	r.Signature = signForTest(f.key, model, req, resp)
	return r, nil
}

func newService(t *testing.T, withReviewer bool) (*Service, *fakeChain) {
	t.Helper()
	_, signer, _ := ed25519.GenerateKey(nil)
	_, agent, _ := ed25519.GenerateKey(nil)
	_, rk, _ := ed25519.GenerateKey(nil)
	chain := &fakeChain{}
	svc := &Service{
		Cfg: Config{
			Network: "testnet", Vault: "vault.test.testnet", Token: "usdc.fakes.testnet",
			DestinationAsset: "nep141:base-x", Allowed: Recipient{"Acme", "0x1111111111111111111111111111111111111111"},
			Attacker: Recipient{"Attacker", "0x6666666666666666666666666666666666666666"}, DefaultAmount: "100000",
			ExplorerTxURL: "https://x/txns/", AgentKey: near.Key{AccountID: "vault.test.testnet", Private: agent}, QuoteSigner: signer,
		},
		Chain: chain, Now: time.Now,
	}
	if withReviewer {
		pub := rk.Public().(ed25519.PublicKey)
		chain.reviewer = `{"model":"m","signing_key":"` + hexOf(pub) + `","threshold":"1"}`
		svc.Reviewer = fakeReviewer{key: rk}
	}
	return svc, chain
}

func run(t *testing.T, s *Service, scenario string) *Result {
	t.Helper()
	in := s.Presets()[scenario]
	in.Scenario = scenario
	res, err := s.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("%s: %v", scenario, err)
	}
	return res
}

func TestHonestPaysAndReplayIsBlocked(t *testing.T) {
	s, chain := newService(t, true)
	res := run(t, s, ScenarioHonest)
	if res.Outcome != "paid" || res.ExplorerURL != "https://x/txns/TX" {
		t.Fatalf("honest: %+v", res)
	}
	if res.Steps[0].Label != "simulated" || res.Steps[1].Label != "real" || res.Steps[2].Label != "real" {
		t.Fatal("quote must be labelled simulated; review and payment real")
	}
	if _, ok := chain.sent[0]["review"]; !ok {
		t.Fatal("the review must be attached to pay_quote")
	}
	res = run(t, s, ScenarioReplay)
	if res.Outcome != "blocked" || res.Layer != "single_use" {
		t.Fatalf("replay: %+v", res)
	}
}

func TestEachAttackIsBlockedByTheRightLayer(t *testing.T) {
	cases := map[string]string{
		ScenarioAttacker: "allowlist",
		ScenarioOverLim:  "spend_limit",
		ScenarioInjected: "ai_reviewer",
	}
	for scenario, layer := range cases {
		s, _ := newService(t, true)
		res := run(t, s, scenario)
		if res.Outcome != "blocked" || res.Layer != layer {
			t.Errorf("%s: outcome=%s layer=%s reason=%s", scenario, res.Outcome, res.Layer, res.Reason)
		}
	}
}

func TestInjectedTaskIsRefusedByTheReviewerBeforeTheChain(t *testing.T) {
	s, _ := newService(t, true)
	res := run(t, s, ScenarioInjected)
	if res.Steps[1].Status != "blocked" || res.Steps[1].Data["signature_verified"] != true {
		t.Fatalf("review step: %+v", res.Steps[1])
	}
}

func TestForgedQuoteUsesAnotherKey(t *testing.T) {
	s, chain := newService(t, false)
	_ = run(t, s, ScenarioForged)
	q := chain.sent[0]["quote_json"].(string)
	ok, err := verifyWith(q, s.Cfg.QuoteSigner)
	if err != nil || ok {
		t.Fatalf("a forged quote must not verify under the trusted signer (ok=%v err=%v)", ok, err)
	}
}

func TestReplayBeforeAnyPaymentIsAnError(t *testing.T) {
	s, _ := newService(t, false)
	res := run(t, s, ScenarioReplay)
	if res.Outcome != "error" {
		t.Fatalf("%+v", res)
	}
}

func TestValidation(t *testing.T) {
	s, _ := newService(t, false)
	bad := []Input{
		{Scenario: "steal", Task: "x", Recipient: "0x1111111111111111111111111111111111111111", Amount: "1"},
		{Scenario: ScenarioCustom, Task: "", Recipient: "0x1111111111111111111111111111111111111111", Amount: "1"},
		{Scenario: ScenarioCustom, Task: "x", Recipient: "bob.near", Amount: "1"},
		{Scenario: ScenarioCustom, Task: "x", Recipient: "0x1111111111111111111111111111111111111111", Amount: "1.5"},
		{Scenario: ScenarioCustom, Task: strings.Repeat("a", 2001), Recipient: "0x1111111111111111111111111111111111111111", Amount: "1"},
	}
	for _, in := range bad {
		if _, err := s.Run(context.Background(), in); err == nil {
			t.Errorf("expected validation error for %+v", in.Scenario)
		}
	}
}

func TestSendFailureIsReportedNotHidden(t *testing.T) {
	s, chain := newService(t, false)
	chain.sendErr = errors.New("rpc down")
	res := run(t, s, ScenarioHonest)
	if res.Outcome != "error" || res.Steps[len(res.Steps)-1].Status != "error" {
		t.Fatalf("%+v", res)
	}
}

func TestFromEnvRefusesMainnet(t *testing.T) {
	env := map[string]string{"PLAYGROUND_VAULT": "vault.goni19.near"}
	_, err := FromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if err == nil || !strings.Contains(err.Error(), "testnet") {
		t.Fatalf("expected a testnet-only error, got %v", err)
	}
	cfg, err := FromEnv(func(string) (string, bool) { return "", false })
	if cfg != nil || err != nil {
		t.Fatal("no PLAYGROUND_VAULT must disable the playground")
	}
}
