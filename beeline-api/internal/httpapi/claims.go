package httpapi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/chainverify"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
)

// A customer paid from their own wallet and tells us the transaction hash. Nothing the browser says
// can mark an invoice paid: the server reads the transaction from NEAR (chainverify) and only a
// verified, matching payment moves the invoice open -> paid. See docs/PAYMENT_FLOW.md.

// ClaimVerifier reads a payment from the chain.
type ClaimVerifier interface {
	Verify(ctx context.Context, p chainverify.Params) chainverify.Result
}

const (
	CodeClaimRejected    = "CLAIM_REJECTED"
	CodeInvoiceAlreadyPd = "INVOICE_ALREADY_PAID"
	kindAlreadyPaid      = "ALREADY_PAID"
	kindReused           = "TX_REUSED"
)

var (
	claimTxRe    = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{43,44}$`)
	claimAccount = regexp.MustCompile(`^(([a-z\d]+[\-_])*[a-z\d]+\.)*([a-z\d]+[\-_])*[a-z\d]+$`)
	nep141Prefix = "nep141:"
	claimTimeout = 30 * time.Second
	memoPrefix   = "beeline:"
)

// invoiceMemo is the reference the payment must carry. It binds a payment to exactly one invoice.
func invoiceMemo(id string) string { return memoPrefix + id }

func (h *handlers) claimsEnabled(w http.ResponseWriter, r *http.Request) bool {
	if !h.invoicesEnabled(w, r) {
		return false
	}
	if h.d.Claims == nil {
		writeAppError(w, r.WithContext(r.Context()), h.d.Logger, apperr.New(http.StatusServiceUnavailable, apperr.CodeProviderUnavailable, "Paying from a wallet is not enabled on this server."))
		return false
	}
	return true
}

func (h *handlers) claimInvoice(w http.ResponseWriter, r *http.Request) {
	if !h.claimsEnabled(w, r) {
		return
	}
	var in struct {
		TxHash string `json:"tx_hash"`
		Sender string `json:"sender_account_id"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	in.TxHash = strings.TrimSpace(in.TxHash)
	in.Sender = strings.ToLower(strings.TrimSpace(in.Sender))
	if !claimTxRe.MatchString(in.TxHash) {
		writeAppError(w, r, h.d.Logger, apperr.Validation("tx_hash must be the NEAR transaction hash from your wallet"))
		return
	}
	if l := len(in.Sender); l < 2 || l > 64 || !claimAccount.MatchString(in.Sender) {
		writeAppError(w, r, h.d.Logger, apperr.Validation("sender_account_id must be the NEAR account that paid"))
		return
	}
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	inv, err := h.d.Invoices.Get(ctx, id)
	if errors.Is(err, invoices.ErrNotFound) {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusNotFound, apperr.CodeNotFound, "Invoice not found."))
		return
	}
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}

	// Repeating the same claim gives the same answer. A different transaction for a paid invoice is refused.
	if inv.Payment != nil {
		if inv.Payment.TxHash == in.TxHash {
			writeJSON(w, http.StatusOK, map[string]any{"status": chainverify.StatusVerified, "payment": inv.Payment})
			return
		}
		h.refuseClaim(w, r, inv.ID, kindAlreadyPaid, "This invoice is already paid, so a second payment was not counted.", in.TxHash, http.StatusConflict, CodeInvoiceAlreadyPd)
		return
	}
	if inv.Status != "open" {
		h.refuseClaim(w, r, inv.ID, kindAlreadyPaid, "This invoice is already paid, so a second payment was not counted.", in.TxHash, http.StatusConflict, CodeInvoiceAlreadyPd)
		return
	}
	if !strings.HasPrefix(inv.Asset, nep141Prefix) {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusUnprocessableEntity, apperr.CodeUnsupportedAsset, "This invoice's token cannot be paid from a wallet."))
		return
	}
	if !h.allowRun(w, r) {
		return
	}

	vctx, cancel := context.WithTimeout(ctx, claimTimeout)
	defer cancel()
	res := h.d.Claims.Verify(vctx, chainverify.Params{
		TxHash: in.TxHash, Sender: in.Sender, Token: strings.TrimPrefix(inv.Asset, nep141Prefix),
		Receiver: inv.Payout, Amount: inv.Amount, Memo: invoiceMemo(inv.ID), ExpiresAt: inv.ExpiresAt,
	})
	switch res.Status {
	case chainverify.StatusPending:
		writeJSON(w, http.StatusAccepted, map[string]any{"status": chainverify.StatusPending, "message": res.Reason})
	case chainverify.StatusRejected:
		h.refuseClaim(w, r, inv.ID, res.Code, res.Reason, in.TxHash, http.StatusUnprocessableEntity, CodeClaimRejected)
	case chainverify.StatusVerified:
		e := res.Evidence
		p, err := h.d.Invoices.RecordPayment(context.WithoutCancel(ctx), inv.ID, invoices.Payment{
			TxHash: e.TxHash, Sender: e.Sender, Receiver: e.Receiver, Token: e.Token, Amount: e.Amount, Memo: e.Memo, BlockHash: e.BlockHash, BlockTime: e.BlockTime,
		})
		switch {
		case err == nil:
			writeJSON(w, http.StatusOK, map[string]any{"status": chainverify.StatusVerified, "payment": p})
		case errors.Is(err, invoices.ErrTxReused):
			// Two requests raced, or the transaction already paid something: answer from what is stored.
			if cur, _ := h.d.Invoices.PaymentFor(ctx, inv.ID); cur != nil && cur.TxHash == in.TxHash {
				writeJSON(w, http.StatusOK, map[string]any{"status": chainverify.StatusVerified, "payment": cur})
				return
			}
			h.refuseClaim(w, r, inv.ID, kindReused, "That transaction was already used to pay an invoice, so it was not counted again.", in.TxHash, http.StatusConflict, CodeClaimRejected)
		case errors.Is(err, invoices.ErrAlreadyPaid):
			if cur, _ := h.d.Invoices.PaymentFor(ctx, inv.ID); cur != nil && cur.TxHash == in.TxHash {
				writeJSON(w, http.StatusOK, map[string]any{"status": chainverify.StatusVerified, "payment": cur})
				return
			}
			h.refuseClaim(w, r, inv.ID, kindAlreadyPaid, "This invoice is already paid, so a second payment was not counted.", in.TxHash, http.StatusConflict, CodeInvoiceAlreadyPd)
		default:
			h.d.Logger.Error("record payment", "invoice", inv.ID, "error", err)
			writeAppError(w, r, h.d.Logger, apperr.New(http.StatusInternalServerError, "INTERNAL", "We verified the payment but could not save it. Try again in a moment; nothing is lost."))
		}
	}
}

// refuseClaim records the refusal as a security event on the invoice and answers with the reason.
func (h *handlers) refuseClaim(w http.ResponseWriter, r *http.Request, invoiceID, kind, reason, tx string, status int, code string) {
	if err := h.d.Invoices.AddEvent(context.WithoutCancel(r.Context()), invoiceID, kind, reason, tx); err != nil {
		h.d.Logger.Error("record security event", "invoice", invoiceID, "error", err)
	}
	writeAppError(w, r, h.d.Logger, apperr.New(status, code, reason))
}

// invoiceEvents lists the claims that were refused for an invoice (public, like the attempts list).
func (h *handlers) invoiceEvents(w http.ResponseWriter, r *http.Request) {
	if !h.invoicesEnabled(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := h.d.Invoices.Get(r.Context(), id); errors.Is(err, invoices.ErrNotFound) {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusNotFound, apperr.CodeNotFound, "Invoice not found."))
		return
	}
	list, err := h.d.Invoices.Events(r.Context(), id, 50)
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": list})
}
