package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents/intentstest"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/switchramp"
)

// fakeSwitch is a TEST DOUBLE for Switch's API. It records what it was asked and never moves money.
type fakeSwitch struct {
	mu    sync.Mutex
	paths []string
	last  map[string]any
}

func newFakeSwitch(t *testing.T) (*fakeSwitch, *switchramp.Client) {
	t.Helper()
	f := &fakeSwitch{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-service-key") != "sk_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		f.paths = append(f.paths, r.Method+" "+r.URL.Path)
		f.last = nil
		if r.Body != nil {
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &f.last)
		}
		f.mu.Unlock()
		switch r.URL.Path {
		case "/offramp/quote":
			_, _ = w.Write([]byte(`{"success":true,"data":{"rate":1400,"destination":{"amount":35000,"currency":"NGN"}}}`))
		case "/offramp/initiate":
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":"AWAITING_DEPOSIT","reference":"6c01338d-67e5-4d0c-a394-9d3e367f46a5","deposit":{"amount":25,"address":"0xSIMULATED","asset":"base:usdc"}}}`))
		case "/payment/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":"PROCESSING"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return f, switchramp.New(srv.URL, "sk_test", "sandbox")
}

func (f *fakeSwitch) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.paths)
}

func cashoutRouter(sw *switchramp.Client, fi *intentstest.Fake) http.Handler {
	d := Deps{AllowedOrigins: []string{"*"}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Switch: sw}
	if fi != nil {
		d.Intents = fi
	}
	return NewRouter(d)
}

func post(h http.Handler, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	return rec
}

func TestCashoutQuotePricesTheBridgeThenSwitchAtTheBridgeMinimum(t *testing.T) {
	fs, sw := newFakeSwitch(t)
	fi := intentstest.New()
	rec := post(cashoutRouter(sw, fi), "/api/v1/cashout/quote", `{"amount_usdc":"25.5"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if len(fi.QuoteCalls) != 1 {
		t.Fatalf("want one route quote, got %d", len(fi.QuoteCalls))
	}
	q := fi.QuoteCalls[0]
	if !q.Dry || q.Amount != "25500000" || q.DepositType != "INTENTS" || q.OriginAsset != nearUSDC || q.DestinationAsset != baseUSDC {
		t.Fatalf("route quote must be a dry NEAR USDC to Base USDC quote in base units: %+v", q)
	}
	if fs.last["amount"] != 25.5 || fs.last["country"] != "NG" || fs.last["asset"] != "base:usdc" {
		t.Fatalf("switch quote request: %v", fs.last)
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["payout_basis"] != "bridge_minimum" || out["route"] == nil || out["route_error"] != "" {
		t.Fatalf("response: %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), "sk_test") {
		t.Fatal("the service key must never be returned")
	}
	for _, p := range fs.paths {
		if p != "POST /offramp/quote" {
			t.Fatalf("a quote must only call the quote endpoint, called %s", p)
		}
	}
}

func TestCashoutQuoteStillAnswersWhenTheBridgeQuoteFails(t *testing.T) {
	_, sw := newFakeSwitch(t)
	fi := intentstest.New()
	fi.QuoteErr = errors.New("pair not available")
	rec := post(cashoutRouter(sw, fi), "/api/v1/cashout/quote", `{"amount_usdc":"10"}`)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != http.StatusOK || out["route"] != nil || out["route_error"] == "" || out["payout_basis"] != "nominal" {
		t.Fatalf("a failed bridge quote must be reported, not hidden: %d %s", rec.Code, rec.Body)
	}
}

func TestCashoutRejectsBadAmountsBeforeAnyProviderCall(t *testing.T) {
	fs, sw := newFakeSwitch(t)
	fi := intentstest.New()
	h := cashoutRouter(sw, fi)
	for _, a := range []string{"0", "0.00", "-1", "1e3", "5.1234567", "abc", "", "99999999999", "5001"} {
		rec := post(h, "/api/v1/cashout/quote", `{"amount_usdc":"`+a+`"}`)
		if rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadRequest {
			t.Errorf("amount %q: got %d", a, rec.Code)
		}
	}
	if fs.calls() != 0 || len(fi.QuoteCalls) != 0 {
		t.Fatal("no provider may be called for an invalid amount")
	}
}

func TestCashoutStartIsRefusedInLiveMode(t *testing.T) {
	fs, sw := newFakeSwitch(t)
	sw.Mode = "live"
	rec := post(cashoutRouter(sw, nil), "/api/v1/cashout/start",
		`{"amount_usdc":"5","bank":{"bank_code":"100004","account_number":"8188216769","holder_name":"John Doe"}}`)
	if rec.Code != http.StatusForbidden || fs.calls() != 0 {
		t.Fatalf("live mode must not open a payout session: %d, %d calls", rec.Code, fs.calls())
	}
}

func TestCashoutStartInSandboxSendsTheBankAndLabelsItSandbox(t *testing.T) {
	fs, sw := newFakeSwitch(t)
	rec := post(cashoutRouter(sw, nil), "/api/v1/cashout/start",
		`{"amount_usdc":"25","bank":{"bank_code":"100004","account_number":"8188216769","holder_name":"John Doe"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	ben, _ := fs.last["beneficiary"].(map[string]any)
	if ben["account_number"] != "8188216769" || ben["bank_code"] != "100004" || ben["holder_type"] != "INDIVIDUAL" || fs.last["asset"] != "base:usdc" {
		t.Fatalf("initiate request: %v", fs.last)
	}
	if !strings.Contains(rec.Body.String(), "SANDBOX") || !strings.Contains(rec.Body.String(), "AWAITING_DEPOSIT") {
		t.Fatalf("must be labelled sandbox and carry the provider status: %s", rec.Body)
	}
}

func TestCashoutStartValidatesTheBankBeforeCallingSwitch(t *testing.T) {
	fs, sw := newFakeSwitch(t)
	h := cashoutRouter(sw, nil)
	for _, b := range []string{
		`{"bank_code":"100004","account_number":"123","holder_name":"John Doe"}`,
		`{"bank_code":"abc","account_number":"8188216769","holder_name":"John Doe"}`,
		`{"bank_code":"100004","account_number":"8188216769","holder_name":"<script>"}`,
	} {
		if rec := post(h, "/api/v1/cashout/start", `{"amount_usdc":"5","bank":`+b+`}`); rec.Code == http.StatusCreated {
			t.Errorf("accepted a bad bank: %s", b)
		}
	}
	if fs.calls() != 0 {
		t.Fatal("Switch must not be called with an invalid bank")
	}
}

func TestCashoutStatusIsProviderReportedAndValidatesTheReference(t *testing.T) {
	_, sw := newFakeSwitch(t)
	h := cashoutRouter(sw, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/cashout/6c01338d-67e5-4d0c-a394-9d3e367f46a5", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "provider-reported") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/cashout/not-a-uuid", nil))
	if rec.Code == http.StatusOK {
		t.Fatal("a non-UUID reference must be rejected")
	}
}

func TestCashoutDisabledWithoutASwitchKey(t *testing.T) {
	rec := post(cashoutRouter(nil, nil), "/api/v1/cashout/quote", `{"amount_usdc":"5"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestUSDCUnitConversion(t *testing.T) {
	for in, want := range map[string]string{"25.5": "25500000", "1": "1000000", "0.000001": "1", "5.25": "5250000"} {
		if got := usdcToUnits(in); got != want {
			t.Errorf("usdcToUnits(%s)=%s want %s", in, got, want)
		}
		if back := unitsToUSDC(want); back != strings.TrimRight(strings.TrimRight(in, "0"), ".") && back != in {
			t.Errorf("unitsToUSDC(%s)=%s want %s", want, back, in)
		}
	}
}
