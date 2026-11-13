package integration

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/httpapi"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/payin"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/playground"
)

// These are TEST DOUBLES for NEAR Intents and the NEAR balance read. Nothing here touches a real provider or chain.

const baseUSDC = "nep141:base-0x833589fcd6edb6e08f4c7c32d4f71b54bda02913.omft.near"

type fakeIntents struct {
	mu        sync.Mutex
	quotes    []intents.QuoteRequest
	status    string
	destTx    string
	amountIn  string
	amountOut string // overrides what the quote delivers (to test a short quote)
	addr      string
}

func (f *fakeIntents) Quote(_ context.Context, r intents.QuoteRequest) (*intents.QuoteResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.quotes = append(f.quotes, r)
	out := r.Amount
	if f.amountOut != "" {
		out = f.amountOut
	}
	dl := time.Now().Add(20 * time.Minute)
	return &intents.QuoteResponse{Raw: json.RawMessage(`{"test":"double"}`), Quote: intents.Quote{
		DepositAddress: f.addr, AmountIn: f.amountIn, AmountOut: out, Deadline: &dl,
	}}, nil
}

func (f *fakeIntents) Status(_ context.Context, _, _ string) (*intents.StatusResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status == "" {
		return nil, &intents.Error{Kind: intents.KindNotFound, HTTPStatus: 404, Message: "none"}
	}
	s := &intents.StatusResponse{Status: f.status}
	if f.destTx != "" {
		s.SwapDetails.DestinationChainTxHashes = []intents.TxRef{{Hash: f.destTx}}
	}
	return s, nil
}

func (f *fakeIntents) SubmitDeposit(context.Context, intents.SubmitDepositRequest) (*intents.StatusResponse, error) {
	return nil, fmt.Errorf("not used")
}

func (f *fakeIntents) setStatus(s, tx string) { f.mu.Lock(); f.status, f.destTx = s, tx; f.mu.Unlock() }

type fakeTokens struct{}

func (fakeTokens) Tokens(context.Context) ([]intents.Token, error) {
	return []intents.Token{
		{AssetID: baseUSDC, Blockchain: "base", Symbol: "USDC", Decimals: 6},
		{AssetID: "nep141:eth-0xdac17f958d2ee523a2206206994597c13d831ec7.omft.near", Blockchain: "eth", Symbol: "USDT", Decimals: 6},
		{AssetID: "nep141:doge.omft.near", Blockchain: "doge", Symbol: "DOGE", Decimals: 8},       // not offered
		{AssetID: "nep141:eth-0xshib.omft.near", Blockchain: "eth", Symbol: "SHIB", Decimals: 18}, // not offered
	}, nil
}

type fakeBalance struct {
	mu sync.Mutex
	v  *big.Int
}

func (b *fakeBalance) IntentsBalance(context.Context, string, string) (*big.Int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return new(big.Int).Set(b.v), nil
}
func (b *fakeBalance) set(n int64) { b.mu.Lock(); b.v = big.NewInt(n); b.mu.Unlock() }

type payinEnv struct {
	t   *testing.T
	svc *payin.Service
	inv *invoices.Service
	fi  *fakeIntents
	bal *fakeBalance
	srv *httptest.Server
}

var addrSeq int

func newPayinEnv(t *testing.T) *payinEnv {
	t.Helper()
	_, issuer, _ := ed25519.GenerateKey(rand.Reader)
	inv := &invoices.Service{DB: pool, Issuer: issuer, Asset: "nep141:usdc.fakes.testnet", TTL: time.Hour, Now: time.Now, MainnetEnabled: true, MainnetMax: 10_000_000} // a bigger cap than the product default, so the amounts here stay readable
	addrSeq++
	fi := &fakeIntents{status: "", amountIn: "2004000", addr: fmt.Sprintf("0xtestdeposit%d%d", time.Now().UnixNano(), addrSeq)}
	bal := &fakeBalance{v: big.NewInt(10_000_000)}
	svc := &payin.Service{DB: pool, Intents: fi, Tokens: fakeTokens{}, Balance: bal, Invoices: inv, Now: time.Now}
	router := httpapi.NewRouter(httpapi.Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: pool, Invoices: inv, PayIn: svc,
		Playground: &playground.Service{},
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return &payinEnv{t: t, svc: svc, inv: inv, fi: fi, bal: bal, srv: srv}
}

func (e *payinEnv) realInvoice() *invoices.Invoice {
	e.t.Helper()
	inv, err := e.inv.Create(context.Background(), invoices.CreateInput{
		VendorName: "MS Clothing", Payout: "ms-clothing.near", Real: true,
		Items: []invoices.Item{{Name: "Linen shirt", Quantity: 1, UnitPrice: "2000000"}},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return inv
}

const goodRefund = "0x1111111111111111111111111111111111111111"

func TestPayinQuoteIsBoundToTheInvoice(t *testing.T) {
	e := newPayinEnv(t)
	inv := e.realInvoice()
	q, err := e.svc.Create(context.Background(), inv, baseUSDC, goodRefund)
	if err != nil {
		t.Fatal(err)
	}
	if len(e.fi.quotes) != 1 {
		t.Fatalf("want one provider quote, got %d", len(e.fi.quotes))
	}
	r := e.fi.quotes[0]
	if r.Dry || r.SwapType != "EXACT_OUTPUT" || r.DestinationAsset != invoices.MainnetUSDC || r.Amount != inv.Amount ||
		r.Recipient != inv.Payout || r.RecipientType != "INTENTS" || r.RefundTo != goodRefund || r.OriginAsset != baseUSDC {
		t.Fatalf("the quote must be bound to this invoice: %+v", r)
	}
	if q.AmountIn != "2.004" || q.Status != payin.StatusWaiting || q.DepositAddr != e.fi.addr {
		t.Fatalf("unexpected quote: %+v", q)
	}
	// Asking again with the same details returns the live quote instead of making a second address.
	q2, err := e.svc.Create(context.Background(), inv, baseUSDC, goodRefund)
	if err != nil || q2.ID != q.ID || len(e.fi.quotes) != 1 {
		t.Fatalf("a repeat must reuse the live quote: %v %+v quotes=%d", err, q2, len(e.fi.quotes))
	}
}

func TestPayinRefusesWhatItShould(t *testing.T) {
	e := newPayinEnv(t)
	inv := e.realInvoice()
	ctx := context.Background()
	for name, c := range map[string]struct{ asset, refund string }{
		"bad refund address":      {baseUSDC, "not-an-address"},
		"refund on the wrong net": {baseUSDC, "TXYZ9999999999999999999999999999999"},
		"unlisted token":          {"nep141:doge.omft.near", goodRefund},
		"token off the shortlist": {"nep141:eth-0xshib.omft.near", goodRefund},
	} {
		if _, err := e.svc.Create(ctx, inv, c.asset, c.refund); err == nil {
			t.Fatalf("%s must be refused", name)
		}
	}
	if len(e.fi.quotes) != 0 {
		t.Fatal("a refused request must never reach the provider")
	}
	testInv, err := e.inv.Create(ctx, invoices.CreateInput{VendorName: "T", Payout: "t.testnet", Items: []invoices.Item{{Name: "x", Quantity: 1, UnitPrice: "1000000"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Create(ctx, testInv, baseUSDC, goodRefund); err == nil {
		t.Fatal("test invoices cannot be paid from other chains")
	}
	// A quote that would deliver less than the invoice is never shown.
	e.fi.amountOut = "1999999"
	if _, err := e.svc.Create(ctx, inv, baseUSDC, goodRefund); err == nil {
		t.Fatal("a short quote must be refused")
	}
}

func TestPayinPaidOnlyWhenProviderAndBalanceAgree(t *testing.T) {
	e := newPayinEnv(t)
	inv := e.realInvoice()
	ctx := context.Background()
	if _, err := e.svc.Create(ctx, inv, baseUSDC, goodRefund); err != nil {
		t.Fatal(err)
	}
	// Nothing sent yet.
	q, _ := e.svc.Refresh(ctx, inv)
	if q.Status != payin.StatusWaiting {
		t.Fatalf("want waiting, got %s", q.Status)
	}
	e.fi.setStatus(intents.StatusProcessing, "")
	if q, _ = e.svc.Refresh(ctx, inv); q.Status != payin.StatusProcessing {
		t.Fatalf("want processing, got %s", q.Status)
	}
	// The provider says SUCCESS, but the vendor's balance did not move: not paid.
	destTx := fmt.Sprintf("DestTx%d", time.Now().UnixNano())
	e.fi.setStatus(intents.StatusSuccess, destTx)
	q, _ = e.svc.Refresh(ctx, inv)
	if q.Status != payin.StatusConfirming {
		t.Fatalf("provider SUCCESS alone must not mark paid, got %s", q.Status)
	}
	cur, _ := e.inv.Get(ctx, inv.ID)
	if cur.Status != "open" || cur.Payment != nil {
		t.Fatal("invoice must stay open while the balance has not risen")
	}
	// Balance rises by less than the invoice: still not paid.
	e.bal.set(10_000_000 + 1_999_999)
	if q, _ = e.svc.Refresh(ctx, inv); q.Status != payin.StatusConfirming {
		t.Fatalf("a short balance rise must not mark paid, got %s", q.Status)
	}
	// Balance rises by the invoice amount: paid, labelled intents-delivered.
	e.bal.set(10_000_000 + 2_000_000)
	q, _ = e.svc.Refresh(ctx, inv)
	if q.Status != payin.StatusPaid {
		t.Fatalf("want paid, got %s", q.Status)
	}
	cur, _ = e.inv.Get(ctx, inv.ID)
	if cur.Status != "paid" || cur.Payment == nil || cur.Payment.Verification != "intents-delivered" || cur.Payment.TxHash != destTx {
		t.Fatalf("unexpected payment record: status=%s %+v", cur.Status, cur.Payment)
	}
	// Refreshing again changes nothing.
	if q, _ = e.svc.Refresh(ctx, cur); q.Status != payin.StatusPaid {
		t.Fatal("paid must stay paid")
	}
}

func TestPayinRefundIsRecordedAndNeverPaid(t *testing.T) {
	e := newPayinEnv(t)
	inv := e.realInvoice()
	ctx := context.Background()
	if _, err := e.svc.Create(ctx, inv, baseUSDC, goodRefund); err != nil {
		t.Fatal(err)
	}
	e.fi.setStatus(intents.StatusRefunded, "")
	q, _ := e.svc.Refresh(ctx, inv)
	if q.Status != payin.StatusRefunded {
		t.Fatalf("want refunded, got %s", q.Status)
	}
	cur, _ := e.inv.Get(ctx, inv.ID)
	if cur.Status != "open" {
		t.Fatal("a refunded payment must leave the invoice open")
	}
	evs, _ := e.inv.Events(ctx, inv.ID, 10)
	if len(evs) != 1 || evs[0].Kind != "PAYIN_REFUNDED" {
		t.Fatalf("the vendor must see the refund as a security event: %+v", evs)
	}
	// A later SUCCESS report for a terminal quote cannot regress or flip it.
	e.fi.setStatus(intents.StatusSuccess, "X")
	e.bal.set(99_000_000)
	if q, _ = e.svc.Refresh(ctx, inv); q.Status != payin.StatusRefunded {
		t.Fatalf("a terminal quote must not change, got %s", q.Status)
	}
}

func TestPayinHTTPEndpoints(t *testing.T) {
	e := newPayinEnv(t)
	inv := e.realInvoice()
	get := func(path string) (int, map[string]any) {
		resp, err := http.Get(e.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	code, out := get("/api/v1/payin/options")
	opts, _ := out["data"].([]any)
	if code != 200 || len(opts) != 2 {
		t.Fatalf("only stablecoins on the shortlist are offered: %d %v", code, out)
	}
	post := func(body string) (int, map[string]any) {
		resp, err := http.Post(e.srv.URL+"/api/v1/playground/invoices/"+inv.ID+"/payin", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var o map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&o)
		return resp.StatusCode, o
	}
	if code, _ := post(`{"origin_asset":"` + baseUSDC + `","refund_to":"bad"}`); code != 422 {
		t.Fatalf("a bad refund address is a 422, got %d", code)
	}
	code, out = post(`{"origin_asset":"` + baseUSDC + `","refund_to":"` + goodRefund + `"}`)
	if code != 200 || out["quote"] == nil {
		t.Fatalf("create: %d %v", code, out)
	}
	code, out = get("/api/v1/playground/invoices/" + inv.ID + "/payin")
	if code != 200 || out["invoice_status"] != "open" {
		t.Fatalf("status: %d %v", code, out)
	}
	// A real-money invoice cannot be claimed with a wallet transaction when no mainnet verifier is configured.
	resp, err := http.Post(e.srv.URL+"/api/v1/playground/invoices/"+inv.ID+"/claims", "application/json",
		strings.NewReader(`{"tx_hash":"`+txFor("mainnet")+`","sender_account_id":"buyer.near"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("mainnet claims without a mainnet verifier must be refused, got %d", resp.StatusCode)
	}
}
