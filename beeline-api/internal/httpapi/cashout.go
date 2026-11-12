package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/switchramp"
)

// Cash out to a Nigerian bank account. The route is
//
//	USDC on NEAR --(NEAR Intents)--> USDC on Base --(Switch deposit address)--> naira in the bank.
//
// Beeline never holds the vendor's keys. This file only PRICES the route (read-only quotes) and, in
// Switch sandbox mode, opens a payout session. Moving the USDC from NEAR to Switch's address is a
// transfer the vendor's wallet must authorize, and that funded step is not built or verified.

const (
	nearUSDC = "nep141:17208628f84f5d6ad33f0da3bbbeb27ffcb398eac501a31bd6ad2011e36133a1"
	baseUSDC = "nep141:base-0x833589fcd6edb6e08f4c7c32d4f71b54bda02913.omft.near"
	// Placeholders for a dry quote only: nothing is ever sent to them.
	previewRecipient = "0x0000000000000000000000000000000000000001"
	previewRefund    = "beeline-preview.near"
	maxCashoutUSDC   = "5000" // preview ceiling, not a provider limit
)

var usdcAmountRe = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})(\.[0-9]{1,6})?$`)

func (h *handlers) cashoutEnabled(w http.ResponseWriter, r *http.Request) bool {
	if h.d.Switch == nil {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusServiceUnavailable, apperr.CodeProviderUnavailable, "Cash out to a bank is not configured on this server."))
		return false
	}
	return true
}

func (h *handlers) cashoutError(w http.ResponseWriter, r *http.Request, err error) {
	var api *switchramp.APIError
	if errors.As(err, &api) {
		writeAppError(w, r, h.d.Logger, apperr.Validation(api.Message))
		return
	}
	h.d.Logger.Warn("switch request failed", "error", err)
	writeAppError(w, r, h.d.Logger, apperr.New(http.StatusBadGateway, apperr.CodeProviderUnavailable, "Switch could not be reached right now. Try again in a moment."))
}

// usdcToUnits turns "5.25" into "5250000". The caller has already matched usdcAmountRe.
func usdcToUnits(s string) string {
	whole, frac, _ := strings.Cut(s, ".")
	frac = (frac + "000000")[:6]
	return strings.TrimLeft(whole+frac, "0")
}

// unitsToUSDC turns base units back into a USDC decimal string ("5250000" -> "5.25").
func unitsToUSDC(u string) string {
	u = strings.Repeat("0", 7)[:max(0, 7-len(u))] + u
	whole, frac := u[:len(u)-6], strings.TrimRight(u[len(u)-6:], "0")
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	if frac == "" {
		return whole
	}
	return whole + "." + frac
}

type cashoutQuoteIn struct {
	AmountUSDC string `json:"amount_usdc"`
}

func (in *cashoutQuoteIn) check() error {
	in.AmountUSDC = strings.TrimSpace(in.AmountUSDC)
	if !usdcAmountRe.MatchString(in.AmountUSDC) || in.AmountUSDC == "0" || strings.Trim(in.AmountUSDC, "0.") == "" {
		return errors.New("amount_usdc must be a positive USDC amount like 25.00")
	}
	whole, _, _ := strings.Cut(in.AmountUSDC, ".")
	if len(whole) > len(maxCashoutUSDC) || (len(whole) == len(maxCashoutUSDC) && whole > maxCashoutUSDC) {
		return errors.New("amount_usdc is above the preview ceiling of " + maxCashoutUSDC + " USDC")
	}
	return nil
}

// cashoutQuote prices the whole route without moving money: a NEAR Intents dry quote for NEAR USDC
// to Base USDC, then a Switch payout quote for the amount the bridge guarantees at minimum.
func (h *handlers) cashoutQuote(w http.ResponseWriter, r *http.Request) {
	if !h.cashoutEnabled(w, r) {
		return
	}
	var in cashoutQuoteIn
	if err := decodeJSON(w, r, &in); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	if err := in.check(); err != nil {
		writeAppError(w, r, h.d.Logger, apperr.Validation(err.Error()))
		return
	}
	if !h.allowRun(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	switchAmount := in.AmountUSDC
	basis := "nominal"
	var route map[string]any
	routeErr := ""
	if h.d.Intents != nil {
		q, err := h.d.Intents.Quote(ctx, intents.QuoteRequest{
			Dry: true, SwapType: "EXACT_INPUT", SlippageTolerance: 100,
			OriginAsset: nearUSDC, DepositType: "INTENTS", DestinationAsset: baseUSDC,
			Amount: usdcToUnits(in.AmountUSDC), RefundTo: previewRefund, RefundType: "INTENTS",
			Recipient: previewRecipient, RecipientType: "DESTINATION_CHAIN",
			Deadline: time.Now().UTC().Add(30 * time.Minute).Format("2006-01-02T15:04:05.000Z"),
		})
		if err != nil {
			h.d.Logger.Warn("cash-out route quote failed", "error", err)
			routeErr = "NEAR Intents could not price NEAR USDC to Base USDC right now."
		} else if !regexp.MustCompile(`^[1-9][0-9]{0,17}$`).MatchString(q.Quote.MinAmountOut) {
			routeErr = "NEAR Intents returned a quote Beeline could not read."
		} else {
			route = map[string]any{
				"provider": "near-intents", "dry": true,
				"amount_in_usdc": unitsToUSDC(q.Quote.AmountIn), "amount_out_usdc": unitsToUSDC(q.Quote.AmountOut),
				"min_amount_out_usdc": unitsToUSDC(q.Quote.MinAmountOut), "time_estimate_seconds": q.Quote.TimeEstimate,
			}
			switchAmount, basis = unitsToUSDC(q.Quote.MinAmountOut), "bridge_minimum"
		}
	} else {
		routeErr = "NEAR Intents is not configured on this server."
	}

	payout, err := h.d.Switch.OfframpQuote(ctx, switchramp.QuoteInput{Amount: switchAmount, Country: "NG", Asset: "base:usdc"})
	if err != nil {
		h.cashoutError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":         h.d.Switch.Mode,
		"amount_usdc":  in.AmountUSDC,
		"route":        route,
		"route_error":  routeErr,
		"payout":       json.RawMessage(payout),
		"payout_basis": basis,
		"note":         "A preview: no money moved. The NEAR to Base step is priced by a dry quote, and Switch's quote is for " + switchAmount + " USDC (" + basis + "). Rates change.",
	})
}

type cashoutStartIn struct {
	AmountUSDC string          `json:"amount_usdc"`
	Bank       switchramp.Bank `json:"bank"`
}

// cashoutStart opens a Switch payout session for a bank account. Only in sandbox mode: with a live
// key it would create a real payout session, and the funded NEAR to Base step is not verified.
func (h *handlers) cashoutStart(w http.ResponseWriter, r *http.Request) {
	if !h.cashoutEnabled(w, r) {
		return
	}
	if h.d.Switch.Mode != "sandbox" {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusForbidden, apperr.CodeValidation, "Starting a cash-out is switched off while Switch is in live mode: the funded NEAR to Base step is not verified yet."))
		return
	}
	var in cashoutStartIn
	if err := decodeJSON(w, r, &in); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	q := cashoutQuoteIn{AmountUSDC: in.AmountUSDC}
	if err := q.check(); err != nil {
		writeAppError(w, r, h.d.Logger, apperr.Validation(err.Error()))
		return
	}
	if !h.allowRun(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	data, err := h.d.Switch.OfframpInitiate(ctx, switchramp.InitiateInput{Amount: q.AmountUSDC, Bank: in.Bank})
	if err != nil {
		h.cashoutError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"mode": h.d.Switch.Mode,
		"data": json.RawMessage(data),
		"note": "SANDBOX payout session: simulated, no real naira. Nothing has been sent to the deposit address. Moving USDC from NEAR to it through NEAR Intents needs the vendor's wallet and is not automated.",
	})
}

// cashoutStatus reads a payout by Switch's reference. Provider-reported only.
func (h *handlers) cashoutStatus(w http.ResponseWriter, r *http.Request) {
	if !h.cashoutEnabled(w, r) || !h.allowRun(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	data, err := h.d.Switch.Status(ctx, chi.URLParam(r, "reference"))
	if err != nil {
		h.cashoutError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mode": h.d.Switch.Mode, "source": "provider-reported (Switch)", "data": json.RawMessage(data),
		"bridge": h.bridgeStatus(ctx, r.URL.Query().Get("deposit_address"))})
}
