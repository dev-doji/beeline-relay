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
	"testing"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/catalog"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func newTestRouter(db Pinger) http.Handler {
	return NewRouter(Deps{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:             db,
		AllowedOrigins: []string{"http://localhost:3000"},
		Catalog:        catalog.Default(),
	})
}

func TestProtectedRoutesRequireAuth(t *testing.T) {
	for _, path := range []string{"/api/v1/payments", "/api/v1/quotes/qte_x", "/api/v1/payments/pay_x/events"} {
		rec := httptest.NewRecorder()
		newTestRouter(fakePinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", path, rec.Code)
		}
	}
}

func TestCapabilitiesNeverClaimsVerifiedByDefault(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(fakePinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil))
	if strings.Contains(rec.Body.String(), `"status":"verified"`) {
		t.Fatalf("capabilities claims a verified route without configuration: %s", rec.Body.String())
	}
}

func TestHealthOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(fakePinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Status string          `json:"status"`
		Checks map[string]bool `json:"checks"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || !body.Checks["database"] {
		t.Fatalf("unexpected body: %+v", body)
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Fatal("missing X-Request-Id header")
	}
}

func TestHealthDegradedWhenDatabaseDown(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(fakePinger{err: errors.New("boom")}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if got := rec.Body.String(); strings.Contains(got, "boom") {
		t.Fatalf("health response leaked internal error: %s", got)
	}
}

func TestUnknownRouteUsesErrorEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(fakePinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil))

	var body errorBody
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNotFound || body.Error.Code != apperr.CodeNotFound || body.Error.RequestID == "" {
		t.Fatalf("unexpected response %d %+v", rec.Code, body)
	}
}

func TestCORSAllowsOnlyConfiguredOrigins(t *testing.T) {
	for origin, wantAllowed := range map[string]bool{
		"http://localhost:3000": true,
		"https://evil.test":     false,
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		newTestRouter(fakePinger{}).ServeHTTP(rec, req)

		allowed := rec.Header().Get("Access-Control-Allow-Origin") == origin
		if allowed != wantAllowed {
			t.Errorf("origin %s: allowed=%v, want %v", origin, allowed, wantAllowed)
		}
	}
}
