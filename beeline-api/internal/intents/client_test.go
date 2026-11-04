package intents

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Recorded shape of a real non-dry 1Click quote (values sanitized for tests).
const quoteFixture = `{
  "correlationId": "11111111-2222-3333-4444-555555555555",
  "timestamp": "2026-11-04T12:00:00.000Z",
  "signature": "ed25519:TESTSIGNATURE",
  "quoteRequest": {"dry": false, "amount": "1000000"},
  "quote": {
    "depositAddress": "0xTESTDEPOSITADDRESS",
    "amountIn": "1000000",
    "minAmountIn": "1000000",
    "amountOut": "999849",
    "minAmountOut": "989850",
    "deadline": "2026-11-04T12:10:00.000Z",
    "timeEstimate": 35
  }
}`

func quoteReq() QuoteRequest {
	return QuoteRequest{SwapType: "EXACT_INPUT", Amount: "1000000", OriginAsset: "a", DestinationAsset: "b"}
}

func TestQuoteParsesAndSendsAPIKey(t *testing.T) {
	var gotKey, gotPath string
	var gotBody QuoteRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey, gotPath = r.Header.Get("X-API-Key"), r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(quoteFixture))
	}))
	defer srv.Close()

	q, err := NewClient(srv.URL, "jwt-123", time.Second).Quote(context.Background(), quoteReq())
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "jwt-123" || gotPath != "/v0/quote" || gotBody.Amount != "1000000" {
		t.Fatalf("request not sent as expected: key=%q path=%q body=%+v", gotKey, gotPath, gotBody)
	}
	if q.Quote.DepositAddress != "0xTESTDEPOSITADDRESS" || q.Quote.AmountOut != "999849" || q.Quote.Deadline == nil {
		t.Fatalf("unexpected quote: %+v", q.Quote)
	}
	if !strings.Contains(string(q.Raw), "TESTSIGNATURE") {
		t.Fatal("raw response not preserved")
	}
}

func TestQuoteNoAPIKeyHeaderWhenUnset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["X-Api-Key"]; ok {
			t.Error("X-API-Key header must not be sent when no key is configured")
		}
		_, _ = w.Write([]byte(quoteFixture))
	}))
	defer srv.Close()
	if _, err := NewClient(srv.URL, "", time.Second).Quote(context.Background(), quoteReq()); err != nil {
		t.Fatal(err)
	}
}

func TestQuoteRejectsMismatchedAmount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Replace(quoteFixture, `"amountIn": "1000000"`, `"amountIn": "999"`, 1)))
	}))
	defer srv.Close()
	_, err := NewClient(srv.URL, "", time.Second).Quote(context.Background(), quoteReq())
	assertKind(t, err, KindInvalidResponse)
}

func TestQuoteRejectsMissingDepositAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Replace(quoteFixture, `"depositAddress": "0xTESTDEPOSITADDRESS",`, ``, 1)))
	}))
	defer srv.Close()
	_, err := NewClient(srv.URL, "", time.Second).Quote(context.Background(), quoteReq())
	assertKind(t, err, KindInvalidResponse)
}

func TestErrorClassification(t *testing.T) {
	cases := map[int]ErrorKind{
		400: KindRejected,
		401: KindUnauthorized,
		404: KindNotFound,
		429: KindUnavailable,
		502: KindUnavailable,
	}
	for code, want := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"bad\u0007 thing"}`))
		}))
		_, err := NewClient(srv.URL, "", time.Second).Quote(context.Background(), quoteReq())
		srv.Close()
		e := assertKind(t, err, want)
		if e != nil && e.Message != "bad thing" {
			t.Errorf("code %d: message not sanitized: %q", code, e.Message)
		}
	}
}

func TestTimeoutIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()
	_, err := NewClient(srv.URL, "", 20*time.Millisecond).Status(context.Background(), "addr", "")
	assertKind(t, err, KindUnavailable)
}

func TestStatusRejectsUnknownStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("depositAddress") != "addr" || r.URL.Query().Get("depositMemo") != "m1" {
			t.Errorf("unexpected query %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"correlationId":"c","status":"TELEPORTED","updatedAt":"2026-11-04T12:00:00Z","swapDetails":{}}`))
	}))
	defer srv.Close()
	_, err := NewClient(srv.URL, "", time.Second).Status(context.Background(), "addr", "m1")
	assertKind(t, err, KindInvalidResponse)
}

func assertKind(t *testing.T, err error, want ErrorKind) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("expected *intents.Error with kind %s, got %v", want, err)
		return nil
	}
	if e.Kind != want {
		t.Errorf("kind = %s, want %s (%v)", e.Kind, want, err)
	}
	return e
}
