package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunLimiterPerClientAndPerDay(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	l := NewRunLimiter(2, time.Minute, 3)
	l.Now = func() time.Time { return now }

	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("first run must pass")
	}
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("second run must pass")
	}
	if ok, msg := l.Allow("a"); ok || !strings.Contains(msg, "wait a minute") {
		t.Fatal("third run in a minute must be limited")
	}
	now = now.Add(61 * time.Second)
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("the window must slide")
	}
	if ok, msg := l.Allow("b"); ok || !strings.Contains(msg, "today") {
		t.Fatal("the daily cap must apply across clients")
	}
	now = now.Add(24 * time.Hour)
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("the daily cap must reset on a new UTC day")
	}
}

func TestPlaygroundDisabledReturns503(t *testing.T) {
	h := NewRouter(Deps{AllowedOrigins: []string{"*"}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/playground", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/playground/runs", strings.NewReader(`{"scenario":"honest"}`)),
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s: got %d", req.Method, req.URL.Path, rec.Code)
		}
	}
}
