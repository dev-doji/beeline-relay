// Package integration exercises the HTTP API against a real PostgreSQL database and the
// deterministic provider double. It runs only when TEST_DATABASE_URL is set.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/auth"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/catalog"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/httpapi"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents/intentstest"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/payment"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/quote"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/storage"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	_ = godotenv.Load("../../.env")
	url := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if url == "" {
		// Unit-test-only runs (and forks without the secret) skip integration tests.
		os.Exit(0)
	}
	var err error
	pool, err = storage.Open(context.Background(), url)
	if err != nil {
		panic(err)
	}
	if err := storage.Migrate(pool); err != nil {
		panic(err)
	}
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// clock is a controllable time source shared by the services under test.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type env struct {
	t        *testing.T
	srv      *httptest.Server
	provider *intentstest.Fake
	clock    *clock
	store    *auth.Store
}

func newEnv(t *testing.T) *env {
	t.Helper()
	clk := &clock{t: time.Now().UTC()}
	fake := intentstest.New()
	fake.Now = clk.Now
	cat := catalog.Default()
	router := httpapi.NewRouter(httpapi.Deps{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:       pool,
		Auth:     &auth.Store{DB: pool},
		Catalog:  cat,
		Quotes:   &quote.Service{DB: pool, Provider: fake, Catalog: cat, Now: clk.Now},
		Payments: &payment.Service{DB: pool, Now: clk.Now},
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return &env{t: t, srv: srv, provider: fake, clock: clk, store: &auth.Store{DB: pool}}
}

// workspace creates an isolated workspace and returns an agent API key for it.
func (e *env) workspace(sandbox bool) string {
	e.t.Helper()
	ctx := context.Background()
	ws, err := e.store.CreateWorkspace(ctx, "integration-"+e.t.Name(), sandbox)
	if err != nil {
		e.t.Fatal(err)
	}
	_, key, err := e.store.CreateKey(ctx, ws, auth.KindAgent, "test-agent")
	if err != nil {
		e.t.Fatal(err)
	}
	return key
}

type response struct {
	Status int
	Body   map[string]any
}

func (r response) errorCode() string {
	if e, ok := r.Body["error"].(map[string]any); ok {
		s, _ := e["code"].(string)
		return s
	}
	return ""
}

func (e *env) do(method, path, key, idemKey string, body any) response {
	e.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	out := response{Status: resp.StatusCode, Body: map[string]any{}}
	_ = json.NewDecoder(resp.Body).Decode(&out.Body)
	return out
}

func quoteBody() map[string]any {
	r := catalog.Default().Routes()[0]
	return map[string]any{
		"source_network":       r.Source.Network,
		"source_asset_id":      r.Source.ID,
		"source_amount":        "1000000",
		"destination_network":  r.Destination.Network,
		"destination_asset_id": r.Destination.ID,
		"destination_address":  "0x000000000000000000000000000000000000dEaD",
		"refund_address":       "vault.beeline-test.near",
	}
}

func (e *env) newQuote(key string) string {
	e.t.Helper()
	r := e.do(http.MethodPost, "/api/v1/quotes", key, "", quoteBody())
	if r.Status != http.StatusCreated {
		e.t.Fatalf("create quote: %d %v", r.Status, r.Body)
	}
	return r.Body["quote_id"].(string)
}

func TestCreatePaymentRecordsInitialEvent(t *testing.T) {
	e := newEnv(t)
	key := e.workspace(false)
	qid := e.newQuote(key)

	r := e.do(http.MethodPost, "/api/v1/payments", key, "order-1", map[string]any{"quote_id": qid, "client_reference": "invoice-42"})
	if r.Status != http.StatusCreated || r.Body["state"] != "awaiting_authorization" || r.Body["replayed"] != false {
		t.Fatalf("unexpected create response: %d %v", r.Status, r.Body)
	}
	if r.Body["is_simulated"] != false {
		t.Fatal("a payment in a normal workspace must not be marked simulated")
	}
	pid := r.Body["payment_id"].(string)

	ev := e.do(http.MethodGet, "/api/v1/payments/"+pid+"/events", key, "", nil)
	events := ev.Body["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	first := events[0].(map[string]any)
	if first["type"] != "payment_created" || first["sequence"] != float64(1) || first["state_after"] != "awaiting_authorization" {
		t.Fatalf("unexpected first event %v", first)
	}
}

func TestIdempotentReplayReturnsSamePayment(t *testing.T) {
	e := newEnv(t)
	key := e.workspace(false)
	qid := e.newQuote(key)
	body := map[string]any{"quote_id": qid, "client_reference": "invoice-42"}

	first := e.do(http.MethodPost, "/api/v1/payments", key, "retry-me", body)
	second := e.do(http.MethodPost, "/api/v1/payments", key, "retry-me", body)

	if second.Status != http.StatusOK || second.Body["replayed"] != true {
		t.Fatalf("replay: %d %v", second.Status, second.Body)
	}
	if first.Body["payment_id"] != second.Body["payment_id"] {
		t.Fatal("replay returned a different payment")
	}
	list := e.do(http.MethodGet, "/api/v1/payments", key, "", nil)
	if n := len(list.Body["payments"].([]any)); n != 1 {
		t.Fatalf("expected exactly one payment after replay, got %d", n)
	}
	events := e.do(http.MethodGet, "/api/v1/payments/"+first.Body["payment_id"].(string)+"/events", key, "", nil)
	if n := len(events.Body["events"].([]any)); n != 1 {
		t.Fatalf("replay must not append events, got %d", n)
	}
}

func TestIdempotencyConflictRejected(t *testing.T) {
	e := newEnv(t)
	key := e.workspace(false)
	qid := e.newQuote(key)

	e.do(http.MethodPost, "/api/v1/payments", key, "k1", map[string]any{"quote_id": qid, "client_reference": "a"})
	r := e.do(http.MethodPost, "/api/v1/payments", key, "k1", map[string]any{"quote_id": qid, "client_reference": "b"})
	if r.Status != http.StatusConflict || r.errorCode() != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("expected IDEMPOTENCY_CONFLICT, got %d %v", r.Status, r.Body)
	}

	// Conflict is reported even when the conflicting request names a quote that does not exist.
	r = e.do(http.MethodPost, "/api/v1/payments", key, "k1", map[string]any{"quote_id": "qte_does_not_exist"})
	if r.errorCode() != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("expected IDEMPOTENCY_CONFLICT for unknown quote, got %d %v", r.Status, r.Body)
	}
}

func TestQuoteCannotBackTwoPayments(t *testing.T) {
	e := newEnv(t)
	key := e.workspace(false)
	qid := e.newQuote(key)

	e.do(http.MethodPost, "/api/v1/payments", key, "k1", map[string]any{"quote_id": qid})
	r := e.do(http.MethodPost, "/api/v1/payments", key, "k2", map[string]any{"quote_id": qid})
	if r.Status != http.StatusConflict || r.errorCode() != "QUOTE_ALREADY_USED" {
		t.Fatalf("expected QUOTE_ALREADY_USED, got %d %v", r.Status, r.Body)
	}
}

func TestExpiredQuoteRejectedButReplayStillWorks(t *testing.T) {
	e := newEnv(t)
	key := e.workspace(false)
	used := e.newQuote(key)
	unused := e.newQuote(key)
	first := e.do(http.MethodPost, "/api/v1/payments", key, "before-expiry", map[string]any{"quote_id": used})

	e.clock.Advance(quote.DefaultTTL + time.Second)

	r := e.do(http.MethodPost, "/api/v1/payments", key, "after-expiry", map[string]any{"quote_id": unused})
	if r.Status != http.StatusUnprocessableEntity || r.errorCode() != "QUOTE_EXPIRED" {
		t.Fatalf("expected QUOTE_EXPIRED, got %d %v", r.Status, r.Body)
	}
	replay := e.do(http.MethodPost, "/api/v1/payments", key, "before-expiry", map[string]any{"quote_id": used})
	if replay.Status != http.StatusOK || replay.Body["payment_id"] != first.Body["payment_id"] {
		t.Fatalf("replay after expiry should return the original payment, got %d %v", replay.Status, replay.Body)
	}
}

func TestConcurrentRetriesCreateExactlyOnePayment(t *testing.T) {
	e := newEnv(t)
	key := e.workspace(false)
	qid := e.newQuote(key)

	const n = 8
	ids := make([]any, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := e.do(http.MethodPost, "/api/v1/payments", key, "agent-task-7", map[string]any{"quote_id": qid})
			if r.Status != http.StatusCreated && r.Status != http.StatusOK {
				t.Errorf("request %d: %d %v", i, r.Status, r.Body)
			}
			ids[i] = r.Body["payment_id"]
		}(i)
	}
	wg.Wait()
	for i := 1; i < n; i++ {
		if ids[i] != ids[0] {
			t.Fatalf("concurrent retries produced different payments: %v", ids)
		}
	}
	list := e.do(http.MethodGet, "/api/v1/payments", key, "", nil)
	if got := len(list.Body["payments"].([]any)); got != 1 {
		t.Fatalf("expected 1 payment, got %d", got)
	}
}

func TestWorkspaceIsolation(t *testing.T) {
	e := newEnv(t)
	keyA, keyB := e.workspace(false), e.workspace(false)
	qid := e.newQuote(keyA)
	created := e.do(http.MethodPost, "/api/v1/payments", keyA, "k1", map[string]any{"quote_id": qid})
	pid := created.Body["payment_id"].(string)

	for _, path := range []string{"/api/v1/payments/" + pid, "/api/v1/payments/" + pid + "/events", "/api/v1/quotes/" + qid} {
		if r := e.do(http.MethodGet, path, keyB, "", nil); r.Status != http.StatusNotFound {
			t.Errorf("workspace B read %s: status %d, want 404", path, r.Status)
		}
	}
	r := e.do(http.MethodPost, "/api/v1/payments", keyB, "k-b", map[string]any{"quote_id": qid})
	if r.errorCode() != "QUOTE_NOT_FOUND" {
		t.Errorf("workspace B used A's quote: %d %v", r.Status, r.Body)
	}
	if list := e.do(http.MethodGet, "/api/v1/payments", keyB, "", nil); len(list.Body["payments"].([]any)) != 0 {
		t.Error("workspace B can list A's payments")
	}
}

func TestRequestValidation(t *testing.T) {
	e := newEnv(t)
	key := e.workspace(false)
	qid := e.newQuote(key)

	if r := e.do(http.MethodPost, "/api/v1/payments", key, "", map[string]any{"quote_id": qid}); r.errorCode() != "IDEMPOTENCY_KEY_REQUIRED" {
		t.Errorf("missing key: %d %v", r.Status, r.Body)
	}
	if r := e.do(http.MethodPost, "/api/v1/payments", key, "k", map[string]any{"quote_id": qid, "state": "completed"}); r.errorCode() != "VALIDATION_ERROR" {
		t.Errorf("client-supplied state must be rejected: %d %v", r.Status, r.Body)
	}
	if r := e.do(http.MethodPost, "/api/v1/payments", "blr_not-a-real-key", "k", map[string]any{"quote_id": qid}); r.Status != http.StatusUnauthorized {
		t.Errorf("bad key: %d", r.Status)
	}
}

func TestProviderUnavailableStoresNothing(t *testing.T) {
	e := newEnv(t)
	key := e.workspace(false)
	e.provider.QuoteErr = &intents.Error{Kind: intents.KindUnavailable, Message: "timeout"}

	r := e.do(http.MethodPost, "/api/v1/quotes", key, "", quoteBody())
	if r.Status != http.StatusServiceUnavailable || r.errorCode() != "PROVIDER_UNAVAILABLE" {
		t.Fatalf("expected PROVIDER_UNAVAILABLE, got %d %v", r.Status, r.Body)
	}
}

func TestSandboxPaymentsAreMarkedSimulated(t *testing.T) {
	e := newEnv(t)
	key := e.workspace(true)
	qid := e.newQuote(key)
	r := e.do(http.MethodPost, "/api/v1/payments", key, "k", map[string]any{"quote_id": qid})
	if r.Body["is_simulated"] != true {
		t.Fatalf("sandbox payment not marked simulated: %v", r.Body)
	}
}
