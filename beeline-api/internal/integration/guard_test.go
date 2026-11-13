package integration

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/chainverify"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/guard"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/httpapi"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/playground"
)

// scriptedGuardModel is a TEST DOUBLE for the NEAR AI model behind Guard: it answers with a fixed string (or an
// error) and records what it was shown.
type scriptedGuardModel struct {
	mu    sync.Mutex
	reply string
	err   error
	seen  []string
}

func (m *scriptedGuardModel) Complete(_ context.Context, _, _, user string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen = append(m.seen, user)
	return m.reply, m.err
}

type guardEnv struct {
	srv   *httptest.Server
	svc   *invoices.Service
	chain *scriptedChain
	model *scriptedGuardModel
}

func newGuardEnv(t *testing.T, reply string, modelErr error) *guardEnv {
	t.Helper()
	_, issuer, _ := ed25519.GenerateKey(rand.Reader)
	svc := &invoices.Service{DB: pool, Issuer: issuer, Asset: "nep141:usdc.fakes.testnet", TTL: time.Hour, Now: time.Now}
	chain := &scriptedChain{result: chainverify.Result{Status: chainverify.StatusVerified}}
	model := &scriptedGuardModel{reply: reply, err: modelErr}
	router := httpapi.NewRouter(httpapi.Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: pool, Invoices: svc, Claims: chain,
		Playground: &playground.Service{},
		Guard:      &guard.Service{DB: pool, LLM: model, Model: "test-model"},
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return &guardEnv{srv: srv, svc: svc, chain: chain, model: model}
}

func (e *guardEnv) invoice(t *testing.T) *invoices.Invoice {
	t.Helper()
	inv, err := e.svc.Create(context.Background(), invoices.CreateInput{
		VendorName: "MS Clothing", Payout: "ms-clothing.testnet",
		Items: []invoices.Item{{Name: "Linen shirt", Quantity: 1, UnitPrice: "2000000"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func (e *guardEnv) claim(t *testing.T, id, tx string) int {
	t.Helper()
	body := `{"tx_hash":"` + tx + `","sender_account_id":"buyer.testnet"}`
	resp, err := http.Post(e.srv.URL+"/api/v1/playground/invoices/"+id+"/claims", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// reviews polls for Guard's verdicts (they are written in the background after the answer).
func (e *guardEnv) reviews(t *testing.T, id string, want int) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for {
		resp, err := http.Get(e.srv.URL + "/api/v1/playground/invoices/" + id + "/guard")
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			Data []map[string]any `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if len(out.Data) >= want || time.Now().After(deadline) {
			return out.Data
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestGuardReviewsARefusedClaimAndNeverRatesItClear(t *testing.T) {
	e := newGuardEnv(t, `{"verdict":"clear","reason":"Looks fine to me."}`, nil)
	inv := e.invoice(t)
	e.chain.result = chainverify.Result{Status: chainverify.StatusRejected, Code: "WRONG_AMOUNT", Reason: "The payment was for 1 USDC, not 2."}
	tx := txFor("guard-refused")
	if code := e.claim(t, inv.ID, tx); code != http.StatusUnprocessableEntity {
		t.Fatalf("the claim must be refused, got %d", code)
	}
	rs := e.reviews(t, inv.ID, 1)
	if len(rs) != 1 || rs[0]["verdict"] != "watch" || rs[0]["subject"] != "claim_refused" {
		t.Fatalf("a refused claim must be rated at least watch even if the model says clear: %v", rs)
	}
	// The agent was shown server-computed facts, including the refusal code and the claimed transaction.
	seen := strings.Join(e.model.seen, "\n")
	if !strings.Contains(seen, "WRONG_AMOUNT") || !strings.Contains(seen, tx) || !strings.Contains(seen, "buyer.testnet") {
		t.Fatalf("the agent must be shown the facts: %s", seen)
	}
	if st := e.svc; st != nil {
		cur, _ := e.svc.Get(context.Background(), inv.ID)
		if cur.Status != "open" {
			t.Fatal("Guard must not change whether an invoice is paid")
		}
	}
}

func TestGuardClearsAConfirmedPaymentAndDoesNotChangeIt(t *testing.T) {
	e := newGuardEnv(t, `{"verdict":"clear","reason":"Exact amount, right payee, first attempt."}`, nil)
	inv := e.invoice(t)
	if code := e.claim(t, inv.ID, txFor("guard-paid")); code != http.StatusOK {
		t.Fatalf("the claim must verify, got %d", code)
	}
	rs := e.reviews(t, inv.ID, 1)
	if len(rs) != 1 || rs[0]["verdict"] != "clear" || rs[0]["subject"] != "payment_confirmed" {
		t.Fatalf("a clean confirmed payment is rated clear: %v", rs)
	}
}

func TestPaymentsStillWorkWhenTheModelIsDown(t *testing.T) {
	e := newGuardEnv(t, "", errors.New("model unavailable"))
	inv := e.invoice(t)
	if code := e.claim(t, inv.ID, txFor("guard-down")); code != http.StatusOK {
		t.Fatalf("a payment must verify even when Guard cannot run, got %d", code)
	}
	time.Sleep(500 * time.Millisecond)
	if rs := e.reviews(t, inv.ID, 0); len(rs) != 0 {
		t.Fatalf("no model, no review: %v", rs)
	}
	cur, _ := e.svc.Get(context.Background(), inv.ID)
	if cur.Status != "paid" {
		t.Fatal("the invoice must still be paid")
	}
}

func TestGuardRefusesAnOutOfRangeVerdictFromTheModel(t *testing.T) {
	e := newGuardEnv(t, `{"verdict":"definitely-fine","reason":"x"}`, nil)
	inv := e.invoice(t)
	e.chain.result = chainverify.Result{Status: chainverify.StatusRejected, Code: "WRONG_TOKEN", Reason: "A look-alike token."}
	e.claim(t, inv.ID, txFor("guard-bad-verdict"))
	time.Sleep(500 * time.Millisecond)
	if rs := e.reviews(t, inv.ID, 0); len(rs) != 0 {
		t.Fatalf("an invalid verdict must not be stored: %v", rs)
	}
}
