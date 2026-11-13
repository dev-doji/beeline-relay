package integration

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/chainverify"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/httpapi"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/playground"
)

// scriptedChain is a TEST DOUBLE for chainverify.Verifier: it returns the result it was given and records
// what the API asked it to check, so tests can prove the API binds the claim to the invoice's own data.
type scriptedChain struct {
	mu     sync.Mutex
	result chainverify.Result
	seen   []chainverify.Params
}

func (s *scriptedChain) Verify(_ context.Context, p chainverify.Params) chainverify.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = append(s.seen, p)
	r := s.result
	if r.Status == chainverify.StatusVerified {
		r.Evidence = &chainverify.Evidence{TxHash: p.TxHash, Sender: p.Sender, Receiver: p.Receiver, Token: p.Token, Amount: p.Amount, Memo: p.Memo, BlockHash: "BlockHash111", BlockTime: time.Now().UTC().Add(-time.Minute)}
	}
	return r
}

func (s *scriptedChain) calls() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.seen) }

type claimEnv struct {
	t     *testing.T
	srv   *httptest.Server
	svc   *invoices.Service
	chain *scriptedChain
}

func newClaimEnv(t *testing.T) *claimEnv {
	t.Helper()
	_, issuer, _ := ed25519.GenerateKey(rand.Reader)
	svc := &invoices.Service{DB: pool, Issuer: issuer, Asset: "nep141:usdc.fakes.testnet", TTL: time.Hour, Now: time.Now}
	chain := &scriptedChain{result: chainverify.Result{Status: chainverify.StatusVerified}}
	router := httpapi.NewRouter(httpapi.Deps{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:       pool,
		Invoices: svc,
		Claims:   chain,
		// The invoice routes are on only when the playground exists; an empty one is enough here.
		Playground: &playground.Service{},
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return &claimEnv{t: t, srv: srv, svc: svc, chain: chain}
}

func (e *claimEnv) invoice() *invoices.Invoice {
	e.t.Helper()
	inv, err := e.svc.Create(context.Background(), invoices.CreateInput{
		VendorName: "MS Clothing", Payout: "ms-clothing.testnet",
		Items: []invoices.Item{{Name: "Linen shirt", Quantity: 1, UnitPrice: "2000000"}},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return inv
}

func (e *claimEnv) claim(id, tx, sender string) (int, map[string]any) {
	e.t.Helper()
	b, _ := json.Marshal(map[string]string{"tx_hash": tx, "sender_account_id": sender})
	resp, err := http.Post(e.srv.URL+"/api/v1/playground/invoices/"+id+"/claims", "application/json", bytes.NewReader(b))
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (e *claimEnv) get(id string) map[string]any {
	e.t.Helper()
	resp, err := http.Get(e.srv.URL + "/api/v1/playground/invoices/" + id)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func (e *claimEnv) events(id string) []any {
	e.t.Helper()
	resp, err := http.Get(e.srv.URL + "/api/v1/playground/invoices/" + id + "/events")
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct{ Data []any }
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Data
}

// Transaction hashes: 44 base58 characters, unique per test run.
func txFor(label string) string {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	b := make([]byte, 44)
	seed := time.Now().UnixNano() + int64(len(label))*7919
	for i := range b {
		seed = seed*6364136223846793005 + 1442695040888963407
		b[i] = alphabet[int((seed>>33)&0x7fffffff)%len(alphabet)]
	}
	return string(b)
}

func TestAVerifiedClaimMarksTheInvoicePaidAndKeepsTheEvidence(t *testing.T) {
	e := newClaimEnv(t)
	inv := e.invoice()
	tx := txFor("a")
	code, out := e.claim(inv.ID, tx, " Customer.Testnet ")
	if code != 200 || out["status"] != "verified" {
		t.Fatalf("%d %v", code, out)
	}
	// The API, not the browser, decided what the payment must look like: the invoice's own data.
	if e.chain.calls() != 1 {
		t.Fatalf("calls=%d", e.chain.calls())
	}
	p := e.chain.seen[0]
	if p.Token != "usdc.fakes.testnet" || p.Receiver != "ms-clothing.testnet" || p.Amount != "2000000" || p.Memo != "beeline:"+inv.ID || p.Sender != "customer.testnet" || p.TxHash != tx {
		t.Fatalf("params: %+v", p)
	}
	got := e.get(inv.ID)
	pay, _ := got["payment"].(map[string]any)
	if got["status"] != "paid" || pay == nil || pay["tx_hash"] != tx || pay["verification"] != "chain-verified" || got["paid_tx"] != tx {
		t.Fatalf("invoice after claim: %v", got)
	}
}

func TestRepeatingTheSameClaimGivesTheSameAnswerWithoutAskingTheChain(t *testing.T) {
	e := newClaimEnv(t)
	inv := e.invoice()
	tx := txFor("b")
	e.claim(inv.ID, tx, "customer.testnet")
	for i := 0; i < 3; i++ {
		if code, out := e.claim(inv.ID, tx, "customer.testnet"); code != 200 || out["status"] != "verified" {
			t.Fatalf("repeat %d: %d %v", i, code, out)
		}
	}
	if e.chain.calls() != 1 {
		t.Fatalf("the chain must be asked once, was %d", e.chain.calls())
	}
}

func TestASecondPaymentForAPaidInvoiceIsRefusedAndRecorded(t *testing.T) {
	e := newClaimEnv(t)
	inv := e.invoice()
	e.claim(inv.ID, txFor("c1"), "customer.testnet")
	code, out := e.claim(inv.ID, txFor("c2"), "other.testnet")
	if code != http.StatusConflict || errCode(out) != "INVOICE_ALREADY_PAID" {
		t.Fatalf("%d %v", code, out)
	}
	ev := e.events(inv.ID)
	if len(ev) != 1 || ev[0].(map[string]any)["kind"] != "ALREADY_PAID" {
		t.Fatalf("events: %v", ev)
	}
	if e.get(inv.ID)["status"] != "paid" {
		t.Fatal("the invoice must stay paid")
	}
}

func TestOneTransactionCannotPayTwoInvoices(t *testing.T) {
	e := newClaimEnv(t)
	a, b := e.invoice(), e.invoice()
	tx := txFor("d")
	if code, _ := e.claim(a.ID, tx, "customer.testnet"); code != 200 {
		t.Fatal("first claim")
	}
	// Even if a verifier were fooled into approving it, the database refuses to reuse the hash.
	code, out := e.claim(b.ID, tx, "customer.testnet")
	if code != http.StatusConflict || errCode(out) != "CLAIM_REJECTED" {
		t.Fatalf("%d %v", code, out)
	}
	if e.get(b.ID)["status"] != "open" {
		t.Fatal("the second invoice must stay open")
	}
	if ev := e.events(b.ID); len(ev) != 1 || ev[0].(map[string]any)["kind"] != "TX_REUSED" {
		t.Fatalf("events: %v", ev)
	}
}

func TestARejectedClaimLeavesTheInvoiceOpenAndShowsAsASecurityEvent(t *testing.T) {
	e := newClaimEnv(t)
	inv := e.invoice()
	e.chain.result = chainverify.Result{Status: chainverify.StatusRejected, Code: chainverify.CodeWrongToken, Reason: "That payment used a different token than this invoice, so we did not count it."}
	code, out := e.claim(inv.ID, txFor("e"), "customer.testnet")
	if code != http.StatusUnprocessableEntity || errCode(out) != "CLAIM_REJECTED" || !strings.Contains(errMsg(out), "different token") {
		t.Fatalf("%d %v", code, out)
	}
	if got := e.get(inv.ID); got["status"] != "open" || got["payment"] != nil {
		t.Fatalf("invoice must stay open: %v", got)
	}
	ev := e.events(inv.ID)
	if len(ev) != 1 || ev[0].(map[string]any)["kind"] != chainverify.CodeWrongToken {
		t.Fatalf("events: %v", ev)
	}
}

func TestAPendingClaimIsNotAnAccusation(t *testing.T) {
	e := newClaimEnv(t)
	inv := e.invoice()
	e.chain.result = chainverify.Result{Status: chainverify.StatusPending, Reason: "The transaction is not final on NEAR yet."}
	code, out := e.claim(inv.ID, txFor("f"), "customer.testnet")
	if code != http.StatusAccepted || out["status"] != "pending" {
		t.Fatalf("%d %v", code, out)
	}
	if len(e.events(inv.ID)) != 0 || e.get(inv.ID)["status"] != "open" {
		t.Fatal("pending must record nothing and change nothing")
	}
}

func TestBadInputNeverReachesTheChain(t *testing.T) {
	e := newClaimEnv(t)
	inv := e.invoice()
	for _, c := range [][2]string{{"short", "customer.testnet"}, {txFor("g") + "0OIl", "customer.testnet"}, {txFor("g"), ""}, {txFor("g"), "NOT AN ACCOUNT!"}} {
		if code, _ := e.claim(inv.ID, c[0], c[1]); code == 200 || code == 202 {
			t.Errorf("accepted %v", c)
		}
	}
	if code, _ := e.claim("inv_doesnotexist", txFor("h"), "customer.testnet"); code != http.StatusNotFound {
		t.Errorf("unknown invoice: %d", code)
	}
	if e.chain.calls() != 0 {
		t.Fatal("the chain must not be asked about invalid claims")
	}
}

func TestConcurrentClaimsOfTheSamePaymentCountOnce(t *testing.T) {
	e := newClaimEnv(t)
	inv := e.invoice()
	tx := txFor("i")
	var wg sync.WaitGroup
	codes := make([]int, 8)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = e.claim(inv.ID, tx, "customer.testnet")
		}(i)
	}
	wg.Wait()
	for i, c := range codes {
		if c != 200 {
			t.Errorf("claim %d: %d (every repeat of the same payment must answer verified)", i, c)
		}
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM invoice_payments WHERE invoice_id = $1`, inv.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("payments recorded: %d (%v)", n, err)
	}
}

func errCode(out map[string]any) string {
	if e, ok := out["error"].(map[string]any); ok {
		s, _ := e["code"].(string)
		return s
	}
	return ""
}

func errMsg(out map[string]any) string {
	if e, ok := out["error"].(map[string]any); ok {
		s, _ := e["message"].(string)
		return s
	}
	return ""
}

func TestDemoPaymentRefusesInvoicesAboveTheSharedVaultLimit(t *testing.T) {
	e := newClaimEnv(t)
	big, err := e.svc.Create(context.Background(), invoices.CreateInput{
		VendorName: "MS Clothing", Payout: "ms-clothing.testnet",
		Items: []invoices.Item{{Name: "Five packs", Quantity: 5, UnitPrice: "100000000"}}, // 500 test USDC: allowed as an invoice
	})
	if err != nil {
		t.Fatalf("a 500 test USDC invoice must be allowed: %v", err)
	}
	resp, err := http.Post(e.srv.URL+"/api/v1/playground/invoices/"+big.ID+"/pay", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity && resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("the demo payment must be refused up front, got %d", resp.StatusCode)
	}
	if e.get(big.ID)["status"] != "open" {
		t.Fatal("the invoice must stay open")
	}
}
