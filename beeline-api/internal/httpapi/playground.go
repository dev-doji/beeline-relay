package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
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

func (h *handlers) invoicesEnabled(w http.ResponseWriter, r *http.Request) bool {
	if h.d.Playground == nil || h.d.Invoices == nil {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusServiceUnavailable, apperr.CodeProviderUnavailable, "Vendor invoices are not enabled on this server."))
		return false
	}
	return true
}

func (h *handlers) allowRun(w http.ResponseWriter, r *http.Request) bool {
	if h.d.PlaygroundLimiter == nil {
		return true
	}
	if ok, msg := h.d.PlaygroundLimiter.Allow(clientIP(r)); !ok {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusTooManyRequests, apperr.CodeRateLimited, msg))
		return false
	}
	return true
}

func (h *handlers) createInvoice(w http.ResponseWriter, r *http.Request) {
	if !h.invoicesEnabled(w, r) {
		return
	}
	var in invoices.CreateInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	in.Real = false // real-money invoices are made only by signed-in vendors (through Bee), never by the public form
	if _, err := h.d.Invoices.Check(&in); err != nil {
		writeAppError(w, r, h.d.Logger, apperr.Validation(err.Error()))
		return
	}
	if !h.allowRun(w, r) {
		return
	}
	inv, err := h.d.Invoices.Create(r.Context(), in)
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}

func (h *handlers) getInvoice(w http.ResponseWriter, r *http.Request) {
	if !h.invoicesEnabled(w, r) {
		return
	}
	inv, err := h.d.Invoices.Get(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, invoices.ErrNotFound) {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusNotFound, apperr.CodeNotFound, "Invoice not found."))
		return
	}
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

func (h *handlers) payInvoice(w http.ResponseWriter, r *http.Request) {
	if !h.invoicesEnabled(w, r) {
		return
	}
	// Everything here is written by whoever holds the link: an honest payer sends {}, an
	// attacker sends a message for the agent and/or edits to their copy of the link.
	var att playground.Attack
	if err := decodeJSON(w, r, &att); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	editedAmount, err := att.Normalize()
	if err != nil {
		writeAppError(w, r, h.d.Logger, apperr.Validation(err.Error()))
		return
	}
	inv, err := h.d.Invoices.Get(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, invoices.ErrNotFound) {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusNotFound, apperr.CodeNotFound, "Invoice not found."))
		return
	}
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	if !h.allowRun(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()
	res, err := h.d.Playground.PayInvoice(ctx, inv, att)
	if err != nil {
		writeAppError(w, r, h.d.Logger, apperr.Validation(err.Error()))
		return
	}
	bg := context.WithoutCancel(r.Context())
	// The vault's on-chain decision is the source of truth. Mark the invoice paid only when the
	// vault sent exactly the invoice's payee and amount.
	if res.Outcome == "paid" && res.Recipient == inv.Payout && res.Amount == inv.Amount {
		if err := h.d.Invoices.MarkPaid(bg, inv.ID, res.TxHash); err != nil {
			h.d.Logger.Error("mark invoice paid", "invoice", inv.ID, "error", err)
		}
	}
	kind := "pay"
	if att.Message != "" || (att.Payout != "" && att.Payout != inv.Payout) || (editedAmount != "" && editedAmount != inv.Amount) {
		kind = "attack"
	}
	payout := ""
	if att.Payout != inv.Payout {
		payout = att.Payout
	}
	if editedAmount == inv.Amount {
		editedAmount = ""
	}
	attempt, err := h.d.Invoices.RecordAttempt(bg, invoices.Attempt{InvoiceID: inv.ID, Kind: kind, Message: att.Message, EditedPayout: payout,
		EditedAmount: editedAmount, Outcome: res.Outcome, Layer: res.Layer, Reason: res.Reason, Recipient: res.Recipient, Amount: res.Amount, TxHash: res.TxHash})
	if err != nil {
		h.d.Logger.Error("record attempt", "invoice", inv.ID, "error", err)
	}
	updated, err := h.d.Invoices.Get(bg, inv.ID)
	if err != nil {
		updated = inv
	}
	writeJSON(w, http.StatusOK, map[string]any{"invoice": updated, "run": res, "attempt": attempt})
}

func (h *handlers) invoiceAttempts(w http.ResponseWriter, r *http.Request) {
	if !h.invoicesEnabled(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := h.d.Invoices.Get(r.Context(), id); errors.Is(err, invoices.ErrNotFound) {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusNotFound, apperr.CodeNotFound, "Invoice not found."))
		return
	}
	list, err := h.d.Invoices.Attempts(r.Context(), id, 50)
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": list})
}

// attackLog is the public log of every attempt on every pay link, with totals.
func (h *handlers) attackLog(w http.ResponseWriter, r *http.Request) {
	if !h.invoicesEnabled(w, r) {
		return
	}
	list, err := h.d.Invoices.Attempts(r.Context(), "", 50)
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	stats, err := h.d.Invoices.Stats(r.Context())
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": list, "stats": stats})
}
