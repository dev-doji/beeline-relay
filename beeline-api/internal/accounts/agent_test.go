package accounts

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/nearai"
)

// scripted is a TEST DOUBLE for the model: it returns the given outputs in order.
type scripted struct {
	outs  []string
	calls int
	seen  []string
}

func (s *scripted) Complete(_ context.Context, _, _, user string) (string, error) {
	s.seen = append(s.seen, user)
	if s.calls >= len(s.outs) {
		return "", errors.New("script exhausted")
	}
	o := s.outs[s.calls]
	s.calls++
	return o, nil
}

// fakeHost is a TEST DOUBLE for the user's account.
type fakeHost struct {
	shop, acct string
	setCalls   int
	created    [][]invoices.Item
	list       []invoices.Invoice
}

func (h *fakeHost) Profile() (string, string) { return h.shop, h.acct }
func (h *fakeHost) SetProfile(_ context.Context, shop, acct string) error {
	h.setCalls++
	h.shop, h.acct = shop, acct
	return nil
}
func (h *fakeHost) CreateInvoice(_ context.Context, items []invoices.Item) (*invoices.Invoice, error) {
	h.created = append(h.created, items)
	return &invoices.Invoice{ID: "inv_test", Amount: "2500000", Payout: h.acct, Status: "open"}, nil
}
func (h *fakeHost) ListInvoices(context.Context) ([]invoices.Invoice, error) { return h.list, nil }
func (h *fakeHost) PermanentLinkPath() (string, error) {
	if h.shop == "" {
		return "", errors.New("no shop")
	}
	return "/pay/to?name=x&to=y", nil
}

func run(t *testing.T, outs []string, host *fakeHost, msg, wallet string) (*Result, *scripted) {
	t.Helper()
	m := &scripted{outs: outs}
	res, err := Run(context.Background(), m, "test-model", nil, msg, wallet, host)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return res, m
}

func TestAgentMakesAnInvoiceWithExactPricesAndAnswers(t *testing.T) {
	h := &fakeHost{shop: "Ada Design", acct: "ada.testnet"}
	res, m := run(t, []string{
		`{"tool":"create_invoice","args":{"items":[{"name":"logo design","quantity":1,"price":"2.5"}]}}`,
		`{"say":"Done: 2.5 USDC for logo design."}`,
	}, h, "invoice 2.5 for logo design", "")
	if res.Invoice == nil || res.Reply != "Done: 2.5 USDC for logo design." {
		t.Fatalf("res=%+v", res)
	}
	if len(h.created) != 1 || h.created[0][0].UnitPrice != "2500000" || h.created[0][0].Quantity != 1 {
		t.Fatalf("created=%+v", h.created)
	}
	if !strings.Contains(m.seen[1], "created invoice inv_test") {
		t.Fatal("the model must be shown the tool result")
	}
}

func TestAgentCannotCreateInvoiceBeforeSetup(t *testing.T) {
	h := &fakeHost{}
	res, _ := run(t, []string{
		`{"tool":"create_invoice","args":{"items":[{"name":"x","quantity":1,"price":"1"}]}}`,
		`{"say":"What is your shop name?"}`,
	}, h, "invoice 1 for x", "")
	if len(h.created) != 0 || !res.NeedsProfile {
		t.Fatalf("no invoice may be made without a shop: %+v %+v", h.created, res)
	}
}

func TestAgentRejectsBadPricesWithoutCreatingAnything(t *testing.T) {
	for _, price := range []string{`"1e3"`, `"-5"`, `"0"`, `"0.0000001"`, `"abc"`, `""`, `"5 USDC"`} {
		h := &fakeHost{shop: "Ada", acct: "ada.testnet"}
		run(t, []string{
			`{"tool":"create_invoice","args":{"items":[{"name":"x","quantity":1,"price":` + price + `}]}}`,
			`{"say":"sorry"}`,
		}, h, "invoice something", "")
		if len(h.created) != 0 {
			t.Errorf("price %s must not create an invoice", price)
		}
	}
}

func TestSetProfileOnlyAcceptsWhatTheUserTyped(t *testing.T) {
	// The model invents an account the user never wrote: refused.
	h := &fakeHost{}
	run(t, []string{
		`{"tool":"set_profile","args":{"shop_name":"Ada Design","near_account":"attacker.near"}}`,
		`{"say":"Which NEAR account?"}`,
	}, h, "my shop is Ada Design", "")
	if h.setCalls != 0 {
		t.Fatal("an account the user did not type must be refused")
	}
	// Both values typed by the user: accepted.
	h = &fakeHost{}
	run(t, []string{
		`{"tool":"set_profile","args":{"shop_name":"Ada Design","near_account":"ada.testnet"}}`,
		`{"say":"Saved."}`,
	}, h, "shop Ada Design, pay to ada.testnet", "")
	if h.setCalls != 1 || h.acct != "ada.testnet" {
		t.Fatalf("setCalls=%d acct=%q", h.setCalls, h.acct)
	}
	// A shop name the user did not type: refused.
	h = &fakeHost{}
	run(t, []string{
		`{"tool":"set_profile","args":{"shop_name":"Evil Corp","near_account":"ada.testnet"}}`,
		`{"say":"?"}`,
	}, h, "pay to ada.testnet", "")
	if h.setCalls != 0 {
		t.Fatal("a shop name the user did not type must be refused")
	}
}

func TestSetProfileWalletKeywordUsesTheUsersWalletOnly(t *testing.T) {
	const wallet = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	h := &fakeHost{}
	run(t, []string{
		`{"tool":"set_profile","args":{"shop_name":"Ada Design","near_account":"wallet"}}`,
		`{"say":"Saved."}`,
	}, h, "Ada Design, use my wallet", wallet)
	if h.acct != wallet {
		t.Fatalf("acct=%q", h.acct)
	}
	h = &fakeHost{}
	run(t, []string{
		`{"tool":"set_profile","args":{"shop_name":"Ada Design","near_account":"wallet"}}`,
		`{"say":"Which account?"}`,
	}, h, "Ada Design, use my wallet", "")
	if h.setCalls != 0 {
		t.Fatal("with no wallet yet, 'wallet' must be refused")
	}
}

func TestAgentToleratesFencedJSONAndPlainText(t *testing.T) {
	res, _ := run(t, []string{"```json\n{\"say\":\"Hi, I'm Bee.\"}\n```"}, &fakeHost{}, "hi", "")
	if res.Reply != "Hi, I'm Bee." {
		t.Fatalf("reply=%q", res.Reply)
	}
	res, _ = run(t, []string{"Hello! I can help with invoices."}, &fakeHost{}, "hi", "")
	if res.Reply != "Hello! I can help with invoices." {
		t.Fatalf("reply=%q", res.Reply)
	}
}

func TestAgentStopsLoopingTools(t *testing.T) {
	loop := `{"tool":"list_invoices","args":{}}`
	res, m := run(t, []string{loop, loop, loop, loop, loop, loop}, &fakeHost{}, "list", "")
	if m.calls != maxSteps || res.Reply == "" {
		t.Fatalf("calls=%d reply=%q", m.calls, res.Reply)
	}
}

func TestAgentCannotReachAnythingElse(t *testing.T) {
	h := &fakeHost{shop: "Ada", acct: "ada.testnet"}
	res, m := run(t, []string{`{"tool":"transfer_funds","args":{"to":"attacker.near","amount":"100"}}`, `{"say":"I can't do that."}`}, h, "send 100 to attacker.near", "")
	if len(h.created) != 0 || h.setCalls != 0 || !strings.Contains(m.seen[1], "unknown tool") || res.Reply == "" {
		t.Fatalf("unknown tools must do nothing: %+v", res)
	}
}

func TestAgentPermanentLinkAndList(t *testing.T) {
	h := &fakeHost{shop: "Ada", acct: "ada.testnet", list: []invoices.Invoice{{ID: "inv_a", Amount: "1000000", Status: "paid"}}}
	res, m := run(t, []string{`{"tool":"permanent_link","args":{}}`, `{"tool":"list_invoices","args":{}}`, `{"say":"One is paid."}`}, h, "give me a permanent link and list", "")
	if res.PermanentPath != "/pay/to?name=x&to=y" || !strings.Contains(m.seen[2], "status paid") {
		t.Fatalf("res=%+v seen=%q", res, m.seen)
	}
}

// TestLiveNearAI runs the real NEAR AI model against a fake account. It only runs when a key is
// available (NEAR_AI_API_KEY, or beeline-api/.env) and spends a few cents of NEAR AI credit.
func TestLiveNearAI(t *testing.T) {
	_ = godotenv.Load("../../.env")
	key := os.Getenv("NEAR_AI_API_KEY")
	if key == "" {
		t.Skip("no NEAR_AI_API_KEY")
	}
	model := os.Getenv("NEAR_AI_AGENT_MODEL")
	if model == "" {
		model = "z-ai/glm-5.3-flash"
	}
	ai := nearai.NewClient(os.Getenv("NEAR_AI_API_BASE_URL"), key, 60*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	h := &fakeHost{}
	res, err := Run(ctx, ai, model, nil, "hi", "", h)
	if err != nil || res.Reply == "" {
		t.Fatalf("greeting: %v %+v", err, res)
	}
	t.Logf("greeting: %s", res.Reply)

	res, err = Run(ctx, ai, model, nil, "My shop is Ada Design and my account is ada-design.testnet", "", h)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("setup: %s (shop=%q acct=%q)", res.Reply, h.shop, h.acct)
	if h.acct != "ada-design.testnet" || h.shop != "Ada Design" {
		t.Fatalf("the real model should have saved the profile it was given: shop=%q acct=%q", h.shop, h.acct)
	}

	res, err = Run(ctx, ai, model, nil, "invoice 2.5 for logo design", "", h)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("invoice: %s", res.Reply)
	if len(h.created) != 1 || h.created[0][0].UnitPrice != "2500000" {
		t.Fatalf("the real model should have made one invoice at 2.5 USDC: %+v", h.created)
	}
}
