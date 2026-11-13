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
	rate       string // naira per USDC for the fake
}

func (h *fakeHost) Profile() (string, string) { return h.shop, h.acct }
func (h *fakeHost) SetProfile(_ context.Context, shop, acct string) error {
	h.setCalls++
	h.shop, h.acct = shop, acct
	return nil
}
func (h *fakeHost) CreateInvoice(_ context.Context, items []invoices.Item, _ bool) (*invoices.Invoice, error) {
	h.created = append(h.created, items)
	return &invoices.Invoice{ID: "inv_test", Amount: "2500000", Payout: h.acct, Status: "open"}, nil
}
func (h *fakeHost) NGNRate(context.Context) (string, error) {
	if h.rate == "" {
		return "", errors.New("no rate")
	}
	return h.rate, nil
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

func TestModelCannotChooseRealMoneyOnItsOwn(t *testing.T) {
	h := &fakeHost{shop: "Ada Design", acct: "ada.near"}
	res, _ := run(t, []string{
		`{"tool":"create_invoice","args":{"items":[{"name":"logo","quantity":1,"price":"2"}],"real":true}}`,
		`{"say":"I made a test invoice instead."}`,
	}, h, "invoice 2 for logo", "")
	if len(h.created) != 0 || res.Invoice != nil {
		t.Fatal("a real-money invoice must not be made unless the user's own message asked for one")
	}
	h2 := &fakeHost{shop: "Ada Design", acct: "ada.near"}
	res2, _ := run(t, []string{
		`{"tool":"create_invoice","args":{"items":[{"name":"logo","quantity":1,"price":"2"}],"real":true}}`,
		`{"say":"Done."}`,
	}, h2, "make a real invoice for 2 USDC for logo", "")
	if len(h2.created) != 1 || res2.Invoice == nil {
		t.Fatal("when the user asks for a real invoice it is made")
	}
}

func TestNairaPricesConvertAtTheRateAndRoundUp(t *testing.T) {
	cases := []struct{ ngn, rate, want string }{
		{"5000", "1000", "5000000"},
		{"10", "3", "3333334"},              // 3.333333.. rounds up so the vendor is never short
		{"600000", "1407.764", "426207802"}, // ceil(600000 * 1e6 / 1407.764), computed independently
	}
	for _, c := range cases {
		got, err := ngnToUnits(c.ngn, c.rate)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("%s NGN at %s = %s, want %s", c.ngn, c.rate, got, c.want)
		}
	}
	for _, bad := range [][2]string{{"0", "1000"}, {"-5", "1000"}, {"1e3", "1000"}, {"100", "0"}, {"100", "abc"}, {"0.001", "1000000"}} {
		if _, err := ngnToUnits(bad[0], bad[1]); err == nil {
			t.Errorf("%v must be refused", bad)
		}
	}
}

func TestBeeConvertsNairaOnlyWhenTheUserSaidNaira(t *testing.T) {
	h := &fakeHost{shop: "Ada", acct: "ada.testnet", rate: "1000"}
	// The model asks for naira, but the user never said naira: refused, nothing created.
	res, _ := run(t, []string{
		`{"tool":"create_invoice","args":{"items":[{"name":"computers","quantity":5,"price":"3000"}],"currency":"NGN"}}`,
		`{"say":"Is that in naira or USDC?"}`,
	}, h, "invoice for 5 computers at 3000", "")
	if len(h.created) != 0 || res.Invoice != nil {
		t.Fatal("naira conversion must not happen unless the user's own message says naira")
	}
	// The user says naira: converted, in exact base units, with the naira price kept on the line.
	h2 := &fakeHost{shop: "Ada", acct: "ada.testnet", rate: "1000"}
	res2, _ := run(t, []string{
		`{"tool":"create_invoice","args":{"items":[{"name":"computers","quantity":5,"price":"3000"}],"currency":"NGN"}}`,
		`{"say":"Done."}`,
	}, h2, "invoice for 5 computers at 3000 naira each", "")
	if len(h2.created) != 1 || res2.Invoice == nil {
		t.Fatal("a naira invoice should be created when the user says naira")
	}
	it := h2.created[0][0]
	if it.UnitPrice != "3000000" || it.Quantity != 5 || !strings.Contains(it.Name, "₦3,000") {
		t.Fatalf("unexpected line: %+v", it)
	}
	// No rate available: refused, nothing created.
	h3 := &fakeHost{shop: "Ada", acct: "ada.testnet"}
	run(t, []string{
		`{"tool":"create_invoice","args":{"items":[{"name":"x","quantity":1,"price":"3000"}],"currency":"NGN"}}`,
		`{"say":"Rate unavailable."}`,
	}, h3, "invoice 3000 naira for x", "")
	if len(h3.created) != 0 {
		t.Fatal("no rate means no invoice")
	}
}
