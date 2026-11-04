package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/auth"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/payment"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/quote"
)

type handlers struct{ d Deps }

func principal(r *http.Request) auth.Principal {
	p, _ := auth.FromContext(r.Context())
	return p
}

func (h *handlers) capabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"provider":    quote.ProviderNEARIntents,
		"environment": h.d.Environment,
		"features": map[string]bool{
			"live_quotes":      true,
			"payment_sessions": true,
			"reconciliation":   false,
			"evidence_export":  false,
		},
		"routes":             h.d.Catalog.Routes(),
		"route_verification": h.d.RouteVerification,
	})
}

func (h *handlers) networks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"networks": h.d.Catalog.Networks()})
}

func (h *handlers) assets(w http.ResponseWriter, r *http.Request) {
	network := r.URL.Query().Get("network")
	if network != "" && !h.d.Catalog.HasNetwork(network) {
		writeError(w, r, http.StatusUnprocessableEntity, apperr.CodeUnsupportedNetwork, "Network is not supported.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assets": h.d.Catalog.Assets(network)})
}

func (h *handlers) createQuote(w http.ResponseWriter, r *http.Request) {
	var in quote.CreateInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	q, err := h.d.Quotes.Create(r.Context(), principal(r).WorkspaceID, in)
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, q)
}

func (h *handlers) getQuote(w http.ResponseWriter, r *http.Request) {
	q, err := h.d.Quotes.Get(r.Context(), principal(r).WorkspaceID, chi.URLParam(r, "id"))
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, q)
}

type paymentResponse struct {
	*payment.Payment
	Replayed bool `json:"replayed"`
}

func (h *handlers) createPayment(w http.ResponseWriter, r *http.Request) {
	var in payment.CreateInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	p := principal(r)
	var agentKey *string
	if p.Kind == auth.KindAgent {
		agentKey = &p.KeyID
	}
	res, err := h.d.Payments.Create(r.Context(), p.WorkspaceID, agentKey, r.Header.Get("Idempotency-Key"), in)
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	status := http.StatusCreated
	if res.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, paymentResponse{Payment: res.Payment, Replayed: res.Replayed})
}

func (h *handlers) getPayment(w http.ResponseWriter, r *http.Request) {
	p, err := h.d.Payments.Get(r.Context(), principal(r).WorkspaceID, chi.URLParam(r, "id"))
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *handlers) listPayments(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	ps, err := h.d.Payments.List(r.Context(), principal(r).WorkspaceID, limit)
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"payments": ps})
}

func (h *handlers) paymentEvents(w http.ResponseWriter, r *http.Request) {
	evs, err := h.d.Payments.Events(r.Context(), principal(r).WorkspaceID, chi.URLParam(r, "id"))
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": evs})
}
