package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/guard"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
)

// Guard hooks. Reviews run in the background after the payment decision has been made and answered, so the
// agent can never slow down or change a payment. See internal/guard.

type ctxClaimSender struct{}

var guardInflight sync.Map // key -> struct{}, so a polling page cannot start the same review twice

// guardAsync reviews one event in the background. Safe to call with Guard switched off.
func (h *handlers) guardAsync(ev guard.Event) {
	if h.d.Guard == nil {
		return
	}
	key := ev.InvoiceID + "|" + ev.Subject + "|" + ev.Ref
	if _, busy := guardInflight.LoadOrStore(key, struct{}{}); busy {
		return
	}
	go func() {
		defer guardInflight.Delete(key)
		ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
		defer cancel()
		if _, err := h.d.Guard.Review(ctx, ev); err != nil {
			h.d.Logger.Warn("guard review skipped", "invoice", ev.InvoiceID, "subject", ev.Subject, "error", err)
		}
	}()
}

func minutesSince(t time.Time) string { return strconv.Itoa(int(time.Since(t).Minutes())) }

// baseFacts are what the server knows about the invoice and its history; none of it comes from the customer.
func (h *handlers) baseFacts(ctx context.Context, inv *invoices.Invoice) map[string]string {
	prior := 0
	if evs, err := h.d.Invoices.Events(ctx, inv.ID, 100); err == nil {
		prior = len(evs)
	}
	return map[string]string{
		"invoice_amount_usdc":    unitsToUSDC(inv.Amount),
		"invoice_network":        invoices.NetworkOf(inv.Asset),
		"invoice_age_minutes":    minutesSince(inv.CreatedAt),
		"refused_claims_so_far":  strconv.Itoa(prior),
		"invoice_already_paid":   strconv.FormatBool(inv.Status == "paid"),
		"invoice_payee_is_known": "true",
	}
}

// guardClaim reviews a refused payment claim.
func (h *handlers) guardClaim(ctx context.Context, invoiceID, kind, reason, tx string) {
	if h.d.Guard == nil {
		return
	}
	inv, err := h.d.Invoices.Get(ctx, invoiceID)
	if err != nil {
		return
	}
	f := h.baseFacts(ctx, inv)
	f["refusal_code"] = kind
	f["refusal_reason"] = reason
	f["claimed_transaction"] = tx
	if s, _ := ctx.Value(ctxClaimSender{}).(string); s != "" {
		f["claimed_sender_account"] = s
	}
	h.guardAsync(guard.Event{InvoiceID: invoiceID, Subject: guard.SubjectClaimRefused, Ref: tx, Facts: f})
}

// guardPaid reviews a confirmed payment.
func (h *handlers) guardPaid(ctx context.Context, inv *invoices.Invoice, p *invoices.Payment) {
	if h.d.Guard == nil || p == nil {
		return
	}
	f := h.baseFacts(ctx, inv)
	f["verification"] = p.Verification
	f["payer"] = p.Sender
	f["paid_amount_usdc"] = unitsToUSDC(p.Amount)
	f["paid_after_minutes"] = strconv.Itoa(int(p.VerifiedAt.Sub(inv.CreatedAt).Minutes()))
	h.guardAsync(guard.Event{InvoiceID: inv.ID, Subject: guard.SubjectPaymentConfirmed, Ref: p.TxHash, Facts: f})
}

// guardPayin reviews a payment from another chain that ended badly (refunded, failed, too small).
func (h *handlers) guardPayin(ctx context.Context, inv *invoices.Invoice, status, chain, depositAddr, provider string) {
	if h.d.Guard == nil {
		return
	}
	f := h.baseFacts(ctx, inv)
	f["payin_status"] = status
	f["origin_chain"] = chain
	f["provider_reported_status"] = provider
	h.guardAsync(guard.Event{InvoiceID: inv.ID, Subject: guard.SubjectPayinEvent, Ref: depositAddr, Facts: f})
}

// invoiceGuard lists Guard's reviews for an invoice (public, like the attempts and events lists).
func (h *handlers) invoiceGuard(w http.ResponseWriter, r *http.Request) {
	if !h.invoicesEnabled(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := h.d.Invoices.Get(r.Context(), id); errors.Is(err, invoices.ErrNotFound) {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusNotFound, apperr.CodeNotFound, "Invoice not found."))
		return
	}
	if h.d.Guard == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []guard.Review{}, "enabled": false})
		return
	}
	list, err := h.d.Guard.ForInvoice(r.Context(), id, 50)
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": list, "enabled": true, "note": fmt.Sprint("Guard advises; the chain verifier decides what is paid.")})
}
