package playground

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/near"
)

// invoiceChain is a TEST DOUBLE that applies the vault's invoice rules (signature, payee,
// amount, single use) on top of fakeChain's quote handling.
type invoiceChain struct {
	fakeChain
	issuer ed25519.PublicKey
	paid   map[string]bool
}

func (c *invoiceChain) Send(ctx context.Context, k near.Key, fc near.FunctionCall) (*near.Outcome, error) {
	var args struct {
		QuoteJSON string      `json:"quote_json"`
		Invoice   *InvoiceArg `json:"invoice"`
	}
	_ = json.Unmarshal(fc.Args, &args)
	reject := func(r string) (*near.Outcome, error) {
		return &near.Outcome{Hash: "TX", Failure: "Smart contract panicked: BEELINE_VAULT_REJECTED: " + r}, nil
	}
	if args.Invoice == nil {
		return c.fakeChain.Send(ctx, k, fc)
	}
	sig, _ := hex.DecodeString(args.Invoice.Signature)
	if !ed25519.Verify(c.issuer, []byte(args.Invoice.Payload), sig) {
		return reject("invoice was not signed by the trusted invoice issuer (edited or fake link)")
	}
	var p invoices.Payload
	_ = json.Unmarshal([]byte(args.Invoice.Payload), &p)
	var q struct {
		QuoteRequest struct{ Recipient, Amount string } `json:"quoteRequest"`
	}
	_ = json.Unmarshal([]byte(args.QuoteJSON), &q)
	if q.QuoteRequest.Recipient != p.Payout || q.QuoteRequest.Amount != p.Amount {
		return reject("quote does not match the invoice")
	}
	if c.paid[p.ID] {
		return reject("invoice " + p.ID + " has already been paid")
	}
	c.paid[p.ID] = true
	return &near.Outcome{Hash: "TX", Logs: []string{`EVENT_JSON:{"event":"payment_sent"}`}}, nil
}

func newInvoice(t *testing.T, issuer ed25519.PrivateKey) *invoices.Invoice {
	t.Helper()
	p := invoices.Payload{V: 1, ID: "inv_test", Vendor: "Ada Fabrics", Payout: "0x7777777777777777777777777777777777777777",
		Asset: "nep141:base-x", Amount: "500000", ExpiresAt: 1, Memo: "Order #18"}
	raw, _ := json.Marshal(p)
	return &invoices.Invoice{ID: p.ID, VendorName: p.Vendor, Payout: p.Payout, Amount: p.Amount, Memo: p.Memo,
		Items:   []invoices.Item{{Name: "Ankara dress", Quantity: 3, UnitPrice: "150000"}},
		Payload: string(raw), Signature: hex.EncodeToString(ed25519.Sign(issuer, raw)), Status: "open"}
}

func TestInvoicePaymentsAndScams(t *testing.T) {
	s, _ := newService(t, false)
	pub, issuer, _ := ed25519.GenerateKey(nil)
	chain := &invoiceChain{issuer: pub, paid: map[string]bool{}}
	s.Chain = chain
	inv := newInvoice(t, issuer)

	for _, tc := range []struct {
		tamper, outcome, layer, reason string
	}{
		{TamperSwapPayout, "blocked", "invoice", "not signed by the trusted invoice issuer"},
		{TamperEditAmount, "blocked", "invoice", "not signed by the trusted invoice issuer"},
		{TamperNone, "paid", "", ""},
		{TamperNone, "blocked", "invoice", "already been paid"},
	} {
		res, err := s.PayInvoice(context.Background(), inv, tc.tamper)
		if err != nil {
			t.Fatalf("%s: %v", tc.tamper, err)
		}
		if res.Outcome != tc.outcome || res.Layer != tc.layer || !strings.Contains(res.Reason, tc.reason) {
			t.Errorf("%s: outcome=%s layer=%s reason=%q", tc.tamper, res.Outcome, res.Layer, res.Reason)
		}
	}
	// The swapped link really points at the attacker in the quote the agent built.
	if _, err := s.PayInvoice(context.Background(), inv, "steal_everything"); err == nil {
		t.Fatal("unknown tamper modes must be rejected")
	}
}

func TestClientsCannotAttachAnInvoiceToOtherScenarios(t *testing.T) {
	s, _ := newService(t, false)
	in := s.Presets()[ScenarioHonest]
	in.Scenario = ScenarioHonest
	in.Invoice = &InvoiceArg{Payload: "{}", Signature: "00"}
	if _, err := s.Run(context.Background(), in); err == nil {
		t.Fatal("expected a validation error")
	}
}
