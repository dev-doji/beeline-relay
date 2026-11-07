package switchramp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The server below is a TEST DOUBLE for Switch's API.
func fake(t *testing.T) (*Client, *[]*http.Request) {
	t.Helper()
	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Clone(context.Background()))
		if r.Header.Get("x-service-key") != "sk_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/coverage":
			_, _ = w.Write([]byte(`{"success":true,"data":[{"country":"NG","currency":["NGN"]}]}`))
		case "/onramp/quote":
			_, _ = w.Write([]byte(`{"success":true,"data":{"rate":1500}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "sk_test", "sandbox"), &seen
}

func TestCoverageAndQuoteSendTheKeyAndReturnData(t *testing.T) {
	c, seen := fake(t)
	cov, err := c.Coverage(context.Background(), "NG")
	if err != nil || !strings.Contains(string(cov), `"NGN"`) {
		t.Fatalf("coverage: %v %s", err, cov)
	}
	if q := (*seen)[0].URL.Query(); q.Get("direction") != "ONRAMP" || q.Get("country") != "NG" {
		t.Fatalf("query %v", q)
	}
	q, err := c.Quote(context.Background(), QuoteInput{Country: "NG", Asset: "base:usdc", Amount: 50000})
	if err != nil || !strings.Contains(string(q), "1500") {
		t.Fatalf("quote: %v %s", err, q)
	}
}

func TestBadInputIsRejectedBeforeAnyRequest(t *testing.T) {
	c, seen := fake(t)
	for _, in := range []QuoteInput{
		{Country: "nigeria", Asset: "base:usdc", Amount: 1}, {Country: "NG", Asset: "x", Amount: 1},
		{Country: "NG", Asset: "base:usdc", Amount: 0}, {Country: "NG", Asset: "base:usdc", Amount: -5},
		{Country: "NG", Asset: "base:usdc", Amount: 1, Channel: "CASH"},
	} {
		var api *APIError
		if _, err := c.Quote(context.Background(), in); !errors.As(err, &api) {
			t.Errorf("expected a validation error for %+v, got %v", in, err)
		}
	}
	if _, err := c.Coverage(context.Background(), "../x"); err == nil {
		t.Error("bad country must be rejected")
	}
	if len(*seen) != 0 {
		t.Fatalf("invalid input must never reach Switch, saw %d requests", len(*seen))
	}
}

func TestRefusedKeyIsReportedAsUnavailableWithoutLeakingIt(t *testing.T) {
	c, _ := fake(t)
	c.Key = "sk_wrong_value"
	_, err := c.Coverage(context.Background(), "")
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "sk_wrong_value") {
		t.Fatalf("got %v", err)
	}
}

func TestModeDefaultsToSandbox(t *testing.T) {
	if New("", "k", "").Mode != "sandbox" || New("", "k", "bogus").Mode != "sandbox" || New("", "k", "live").Mode != "live" {
		t.Fatal("mode")
	}
}
