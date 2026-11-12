package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents/intentstest"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/switchramp"
)

const (
	testDepositNear = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testBaseAddr    = "0x1111111111111111111111111111111111111111"
	testBank        = `{"bank_code":"100004","account_number":"8188216769","holder_name":"John Doe"}`
)

// quoter wraps the intents test double so a non-dry quote returns a NEAR-shaped deposit address.
type quoter struct{ *intentstest.Fake }

func (q quoter) Quote(ctx context.Context, req intents.QuoteRequest) (*intents.QuoteResponse, error) {
	resp, err := q.Fake.Quote(ctx, req)
	if err == nil && !req.Dry {
		resp.Quote.DepositAddress = testDepositNear
	}
	return resp, err
}

// fakeSwitchSession is a TEST DOUBLE returning a payout session with the given deposit address.
func fakeSwitchSession(t *testing.T, depositAddr string, hits *int) *switchramp.Client {
	t.Helper()
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*hits++
		mu.Unlock()
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"success":true,"data":{"status":"AWAITING_DEPOSIT","reference":"3b093ca1-bf25-4960-b1f0-0594464b3f90",` +
			`"destination":{"amount":34994.7,"currency":"NGN"},"deposit":{"amount":25,"address":"` + depositAddr + `","asset":"base:usdc"}}}`))
	}))
	t.Cleanup(srv.Close)
	return switchramp.New(srv.URL, "sk_test", "sandbox")
}

func router(sw *switchramp.Client, fi *intentstest.Fake, live bool, max string) http.Handler {
	d := Deps{AllowedOrigins: []string{"*"}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Switch: sw, CashoutLive: live, CashoutLiveMaxUSDC: max}
	if fi != nil {
		d.Intents = quoter{fi}
	}
	return NewRouter(d)
}

func prepareBody(amount, near string) string {
	return `{"amount_usdc":"` + amount + `","near_account":"` + near + `","bank":` + testBank + `}`
}

func TestPrepareSandboxHasNoRealBridgeAndSaysSo(t *testing.T) {
	hits := 0
	sw := fakeSwitchSession(t, "0x000000000000000000000000000000000sandbox", &hits)
	fi := intentstest.New()
	rec := post(router(sw, fi, false, ""), "/api/v1/cashout/prepare", prepareBody("25", "shop.near"))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "SANDBOX") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if len(fi.QuoteCalls) != 0 {
		t.Fatal("a sandbox placeholder address must never be sent to NEAR Intents")
	}
}

func TestPrepareQuotesExactOutputToSwitchAddressAndRefundsToTheVendor(t *testing.T) {
	hits := 0
	sw := fakeSwitchSession(t, testBaseAddr, &hits)
	fi := intentstest.New()
	rec := post(router(sw, fi, false, ""), "/api/v1/cashout/prepare", prepareBody("25", "Shop.near"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if len(fi.QuoteCalls) != 1 {
		t.Fatalf("want one route quote, got %d", len(fi.QuoteCalls))
	}
	q := fi.QuoteCalls[0]
	if q.Dry || q.SwapType != "EXACT_OUTPUT" || q.Amount != "25000000" || q.Recipient != testBaseAddr ||
		q.RefundTo != "shop.near" || q.RefundType != "ORIGIN_CHAIN" || q.DepositType != "ORIGIN_CHAIN" || q.OriginAsset != nearUSDC || q.DestinationAsset != baseUSDC {
		t.Fatalf("route quote must deliver exactly Switch's amount to Switch's address and refund to the vendor: %+v", q)
	}
	var out struct {
		Route map[string]any `json:"route"`
		Note  string         `json:"note"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Route["deposit_address"] != testDepositNear || out.Route["send_usdc"] != "25" || out.Route["refund_to"] != "shop.near" {
		t.Fatalf("route: %v", out.Route)
	}
	if !strings.Contains(out.Note, "Nothing has moved") || strings.Contains(rec.Body.String(), "sk_test") {
		t.Fatalf("must say nothing moved and never return the key: %s", rec.Body)
	}
}

func TestPrepareRejectsBadInputBeforeAnyProviderCall(t *testing.T) {
	hits := 0
	sw := fakeSwitchSession(t, testBaseAddr, &hits)
	fi := intentstest.New()
	h := router(sw, fi, false, "")
	for _, body := range []string{
		prepareBody("25", "NOT AN ACCOUNT!"),
		prepareBody("25", ""),
		prepareBody("0", "shop.near"),
		`{"amount_usdc":"5","near_account":"shop.near","bank":{"bank_code":"1","account_number":"1","holder_name":"x"}}`,
	} {
		if rec := post(h, "/api/v1/cashout/prepare", body); rec.Code == http.StatusCreated {
			t.Errorf("accepted: %s", body)
		}
	}
	if hits != 0 || len(fi.QuoteCalls) != 0 {
		t.Fatalf("no provider may be called for bad input (switch %d, intents %d)", hits, len(fi.QuoteCalls))
	}
}

func TestPrepareLiveIsOffByDefaultAndCapped(t *testing.T) {
	hits := 0
	sw := fakeSwitchSession(t, testBaseAddr, &hits)
	sw.Mode = "live"
	fi := intentstest.New()
	if rec := post(router(sw, fi, false, ""), "/api/v1/cashout/prepare", prepareBody("1", "shop.near")); rec.Code != http.StatusForbidden || hits != 0 {
		t.Fatalf("live must be refused unless CASHOUT_LIVE is on: %d (%d calls)", rec.Code, hits)
	}
	h := router(sw, fi, true, "5")
	if rec := post(h, "/api/v1/cashout/prepare", prepareBody("5.01", "shop.near")); rec.Code != http.StatusForbidden || hits != 0 {
		t.Fatalf("live must be capped: %d (%d calls)", rec.Code, hits)
	}
	if rec := post(h, "/api/v1/cashout/prepare", prepareBody("5", "shop.near")); rec.Code != http.StatusCreated {
		t.Fatalf("5 USDC is within the cap: %d %s", rec.Code, rec.Body)
	}
}

func TestPrepareReportsAFailedBridgeQuoteWithoutPretendingItWorked(t *testing.T) {
	hits := 0
	sw := fakeSwitchSession(t, testBaseAddr, &hits)
	fi := intentstest.New()
	fi.QuoteErr = errors.New("pair not available")
	rec := post(router(sw, fi, false, ""), "/api/v1/cashout/prepare", prepareBody("25", "shop.near"))
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "Nothing was sent") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestDepositNoticeValidatesAndForwardsOnly(t *testing.T) {
	fi := intentstest.New()
	fi.Statuses[testDepositNear] = []intents.StatusResponse{{Status: intents.StatusProcessing}}
	h := router(nil, fi, false, "")
	good := `{"deposit_address":"` + testDepositNear + `","tx_hash":"9Zk3wYxq7mT2vRfN8bLcA4dEhJ5uPgS6oQnX1iVyBtKa","near_sender_account":"shop.near"}`
	if rec := post(h, "/api/v1/cashout/deposit", good); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "PROCESSING") || !strings.Contains(rec.Body.String(), "provider-reported") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if len(fi.SubmitCalls) != 1 || fi.SubmitCalls[0].TxHash == "" {
		t.Fatalf("the notice must reach NEAR Intents once: %+v", fi.SubmitCalls)
	}
	before := len(fi.SubmitCalls)
	for _, bad := range []string{
		`{"deposit_address":"` + testDepositNear + `","tx_hash":"not a hash"}`,
		`{"deposit_address":"BAD ADDRESS","tx_hash":"9Zk3wYxq7mT2vRfN8bLcA4dEhJ5uPgS6oQnX1iVyBtKa"}`,
	} {
		if rec := post(h, "/api/v1/cashout/deposit", bad); rec.Code == http.StatusOK {
			t.Errorf("accepted: %s", bad)
		}
	}
	if len(fi.SubmitCalls) != before {
		t.Fatal("invalid notices must not reach NEAR Intents")
	}
}

func TestStatusShowsSwitchAndBridgeSeparately(t *testing.T) {
	hits := 0
	sw := fakeSwitchSession(t, testBaseAddr, &hits)
	fi := intentstest.New()
	fi.Statuses[testDepositNear] = []intents.StatusResponse{{Status: intents.StatusSuccess}}
	h := router(sw, fi, false, "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/cashout/3b093ca1-bf25-4960-b1f0-0594464b3f90?deposit_address="+testDepositNear, nil))
	var out struct {
		Source string         `json:"source"`
		Bridge map[string]any `json:"bridge"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != http.StatusOK || out.Source != "provider-reported (Switch)" || out.Bridge["status"] != "SUCCESS" || out.Bridge["source"] != "provider-reported (NEAR Intents)" {
		t.Fatalf("each leg must be labelled with its own provider: %d %s", rec.Code, rec.Body)
	}
}
