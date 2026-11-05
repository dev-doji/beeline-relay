package httpapi

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/playground"
)

// RunLimiter caps playground runs per client and per day. Each run spends NEAR AI credits and
// testnet gas, so the public demo is rate limited.
type RunLimiter struct {
	PerClient int           // runs per client per window
	Window    time.Duration // e.g. one minute
	PerDay    int           // runs per UTC day across all clients
	Now       func() time.Time

	mu      sync.Mutex
	clients map[string][]time.Time
	day     string
	today   int
}

func NewRunLimiter(perClient int, window time.Duration, perDay int) *RunLimiter {
	return &RunLimiter{PerClient: perClient, Window: window, PerDay: perDay, Now: time.Now, clients: map[string][]time.Time{}}
}

// Allow records a run for client and reports whether it is within the limits.
func (l *RunLimiter) Allow(client string) (bool, string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.Now()
	if d := now.UTC().Format("2006-01-02"); d != l.day {
		l.day, l.today, l.clients = d, 0, map[string][]time.Time{}
	}
	if l.today >= l.PerDay {
		return false, "The playground reached today's run limit. Please try again tomorrow."
	}
	recent := l.clients[client][:0]
	for _, t := range l.clients[client] {
		if now.Sub(t) < l.Window {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.PerClient {
		l.clients[client] = recent
		return false, "Too many runs. Please wait a minute and try again."
	}
	l.clients[client] = append(recent, now)
	l.today++
	return true, ""
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (h *handlers) playgroundState(w http.ResponseWriter, r *http.Request) {
	if h.d.Playground == nil {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusServiceUnavailable, apperr.CodeProviderUnavailable, "The playground is not enabled on this server."))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	st, err := h.d.Playground.State(ctx)
	if err != nil {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusBadGateway, apperr.CodeProviderUnavailable, "Could not read the testnet vault: "+err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *handlers) playgroundRun(w http.ResponseWriter, r *http.Request) {
	if h.d.Playground == nil {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusServiceUnavailable, apperr.CodeProviderUnavailable, "The playground is not enabled on this server."))
		return
	}
	var in playground.Input
	if err := decodeJSON(w, r, &in); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	if h.d.PlaygroundLimiter != nil {
		if ok, msg := h.d.PlaygroundLimiter.Allow(clientIP(r)); !ok {
			writeAppError(w, r, h.d.Logger, apperr.New(http.StatusTooManyRequests, apperr.CodeRateLimited, msg))
			return
		}
	}
	// NEAR AI reviews take ~20s; the chain adds a few seconds.
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()
	res, err := h.d.Playground.Run(ctx, in)
	if err != nil {
		writeAppError(w, r, h.d.Logger, apperr.Validation(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, res)
}
