package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/payin"
)

// "Pay from anywhere": a customer pays a real-money invoice from any chain or exchange to a one-time NEAR
// Intents deposit address. Nothing the browser says marks the invoice paid: the status endpoint reads NEAR
// Intents and the vendor's balance on NEAR, and only agreement between them does. See internal/payin.

func (h *handlers) payinEnabled(w http.ResponseWriter, r *http.Request) bool {
	if !h.invoicesEnabled(w, r) {
		return false
	}
	if h.d.PayIn == nil {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusServiceUnavailable, apperr.CodeProviderUnavailable, "Paying from other chains is not enabled on this server."))
		return false
	}
	return true
}

func (h *handlers) payinOptions(w http.ResponseWriter, r *http.Request) {
	if !h.payinEnabled(w, r) {
		return
	}
	opts, err := h.d.PayIn.Options(r.Context())
	if err != nil {
		h.d.Logger.Error("payin options", "error", err)
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusBadGateway, apperr.CodeProviderUnavailable, "Could not load the list of tokens right now. Try again in a moment."))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": opts, "source": "provider-reported (NEAR Intents)"})
}

func (h *handlers) payinInvoice(w http.ResponseWriter, r *http.Request) *invoices.Invoice {
	inv, err := h.d.Invoices.Get(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, invoices.ErrNotFound) {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusNotFound, apperr.CodeNotFound, "Invoice not found."))
		return nil
	}
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return nil
	}
	return inv
}

func (h *handlers) payinCreate(w http.ResponseWriter, r *http.Request) {
	if !h.payinEnabled(w, r) {
		return
	}
	var in struct {
		OriginAsset string `json:"origin_asset"`
		RefundTo    string `json:"refund_to"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	in.OriginAsset = strings.TrimSpace(in.OriginAsset)
	if in.OriginAsset == "" || len(in.OriginAsset) > 200 {
		writeAppError(w, r, h.d.Logger, apperr.Validation("origin_asset is required"))
		return
	}
	inv := h.payinInvoice(w, r)
	if inv == nil {
		return
	}
	if !h.allowRun(w, r) {
		return
	}
	q, err := h.d.PayIn.Create(r.Context(), inv, in.OriginAsset, in.RefundTo)
	if err != nil {
		var ie *payin.InputError
		if errors.As(err, &ie) {
			writeAppError(w, r, h.d.Logger, apperr.New(http.StatusUnprocessableEntity, apperr.CodeUnsupportedAsset, ie.Message))
			return
		}
		var pe *intents.Error
		if errors.As(err, &pe) && pe.Kind == intents.KindRejected {
			writeAppError(w, r, h.d.Logger, apperr.New(http.StatusUnprocessableEntity, apperr.CodeProviderUnavailable, "NEAR Intents could not price that payment. Try another token or chain."))
			return
		}
		h.d.Logger.Error("payin create", "invoice", inv.ID, "error", err)
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusBadGateway, apperr.CodeProviderUnavailable, "Could not get a deposit address right now. Nothing was charged. Try again in a moment."))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"quote": q, "source": "provider-reported (NEAR Intents)"})
}

// payinStatus advances the invoice's current pay-in quote (read-only towards the provider and the chain)
// and returns it together with the invoice status.
func (h *handlers) payinStatus(w http.ResponseWriter, r *http.Request) {
	if !h.payinEnabled(w, r) {
		return
	}
	inv := h.payinInvoice(w, r)
	if inv == nil {
		return
	}
	ctx := r.Context()
	if inv.Status == "open" {
		q, err := h.d.PayIn.Refresh(ctx, inv)
		if err != nil {
			h.d.Logger.Error("payin refresh", "invoice", inv.ID, "error", err)
			writeAppError(w, r, h.d.Logger, apperr.New(http.StatusBadGateway, apperr.CodeProviderUnavailable, "Could not check the payment right now. Nothing is lost. Try again in a moment."))
			return
		}
		cur, _ := h.d.Invoices.Get(ctx, inv.ID)
		if cur == nil {
			cur = inv
		}
		if q != nil {
			switch q.Status {
			case payin.StatusRefunded, payin.StatusFailed, payin.StatusIncomplete:
				h.guardPayin(context.WithoutCancel(ctx), cur, q.Status, q.Chain, q.DepositAddr, q.ProviderState)
			case payin.StatusPaid:
				h.guardPaid(context.WithoutCancel(ctx), cur, cur.Payment)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"quote": q, "invoice_status": cur.Status, "payment": cur.Payment, "checked_at": time.Now().UTC()})
		return
	}
	q, _ := h.d.PayIn.Current(ctx, inv.ID)
	writeJSON(w, http.StatusOK, map[string]any{"quote": q, "invoice_status": inv.Status, "payment": inv.Payment, "checked_at": time.Now().UTC()})
}
