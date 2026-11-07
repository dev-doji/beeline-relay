package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/switchramp"
)

// Funding preview: what Switch (onswitch.xyz) can turn local currency into. Read-only: coverage
// and quotes move no money. The key stays on the server and is never returned.

func (h *handlers) fundEnabled(w http.ResponseWriter, r *http.Request) bool {
	if h.d.Switch == nil {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusServiceUnavailable, apperr.CodeProviderUnavailable, "Funding with local currency is not configured on this server."))
		return false
	}
	return true
}

func (h *handlers) fundError(w http.ResponseWriter, r *http.Request, err error) {
	var api *switchramp.APIError
	if errors.As(err, &api) {
		writeAppError(w, r, h.d.Logger, apperr.Validation(api.Message))
		return
	}
	h.d.Logger.Warn("switch request failed", "error", err)
	writeAppError(w, r, h.d.Logger, apperr.New(http.StatusBadGateway, apperr.CodeProviderUnavailable, "Switch could not be reached right now. Try again in a moment."))
}

func (h *handlers) fundCoverage(w http.ResponseWriter, r *http.Request) {
	if !h.fundEnabled(w, r) || !h.allowRun(w, r) {
		return
	}
	data, err := h.d.Switch.Coverage(r.Context(), r.URL.Query().Get("country"))
	if err != nil {
		h.fundError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mode": h.d.Switch.Mode, "direction": "ONRAMP", "data": data})
}

func (h *handlers) fundQuote(w http.ResponseWriter, r *http.Request) {
	if !h.fundEnabled(w, r) {
		return
	}
	var in switchramp.QuoteInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	if in.Asset == "" {
		in.Asset = "base:usdc"
	}
	if !h.allowRun(w, r) {
		return
	}
	ctx, cancel := contextWithTimeout(r, 25*time.Second)
	defer cancel()
	data, err := h.d.Switch.Quote(ctx, in)
	if err != nil {
		h.fundError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode": h.d.Switch.Mode,
		"note": "A quote only: no money moves. Switch pays out on EVM chains and Solana, not on NEAR, so funds would arrive on Base and be bridged into the vault by NEAR Intents (not built yet).",
		"data": data,
	})
}

func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}
