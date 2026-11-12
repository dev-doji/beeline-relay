package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/switchramp"
)

// Funded cash-out: a Switch payout session plus a NEAR Intents deposit address.
//
// prepare opens a Switch payout session (which fixes the exact USDC the Base deposit address must
// receive) and asks NEAR Intents for a real, non-dry EXACT_OUTPUT quote that delivers exactly that
// amount to Switch's address. The vendor then sends the quoted USDC from THEIR OWN NEAR wallet to
// the NEAR Intents deposit address. Beeline never holds keys and never signs or sends anything.

var (
	nearAccountRe     = regexp.MustCompile(`^(([a-z\d]+[\-_])*[a-z\d]+\.)*([a-z\d]+[\-_])*[a-z\d]+$`)
	evmAddressRe      = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
	nearTxHashRe      = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{43,44}$`)
	implicitAccountRe = regexp.MustCompile(`^[0-9a-f]{64}$`)
	baseUnitsRe       = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)
)

func validNearAddress(s string) bool {
	return len(s) >= 2 && len(s) <= 64 && (implicitAccountRe.MatchString(s) || nearAccountRe.MatchString(s))
}

type cashoutPrepareIn struct {
	AmountUSDC  string          `json:"amount_usdc"`
	NearAccount string          `json:"near_account"` // where USDC is sent from, and where NEAR Intents refunds to
	Bank        switchramp.Bank `json:"bank"`
}

// switchSession is the part of Switch's payout session Beeline relies on. Amounts stay json.Number
// so they never pass through floating point.
type switchSession struct {
	Status      string `json:"status"`
	Reference   string `json:"reference"`
	Destination struct {
		Amount   json.Number `json:"amount"`
		Currency string      `json:"currency"`
	} `json:"destination"`
	Deposit struct {
		Amount  json.Number `json:"amount"`
		Address string      `json:"address"`
		Asset   string      `json:"asset"`
	} `json:"deposit"`
}

func parseSwitchSession(raw json.RawMessage) (*switchSession, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var s switchSession
	if err := dec.Decode(&s); err != nil {
		return nil, errors.New("unreadable payout session")
	}
	if s.Reference == "" || s.Deposit.Address == "" || s.Deposit.Asset != "base:usdc" || !usdcAmountRe.MatchString(s.Deposit.Amount.String()) {
		return nil, errors.New("the payout session did not contain a usable Base USDC deposit")
	}
	return &s, nil
}

// liveCashoutAllowed gates real payouts: they need CASHOUT_LIVE=on and stay under a small cap.
func (h *handlers) liveCashoutAllowed(amountUSDC string) (bool, string) {
	if h.d.Switch.Mode != "live" {
		return true, ""
	}
	if !h.d.CashoutLive {
		return false, "Real cash-outs are switched off on this server (CASHOUT_LIVE is not on)."
	}
	max := h.d.CashoutLiveMaxUSDC
	if !usdcAmountRe.MatchString(max) {
		max = "5"
	}
	if usdcToUnitsInt(amountUSDC) > usdcToUnitsInt(max) {
		return false, "Real cash-outs are capped at " + max + " USDC on this server."
	}
	return true, ""
}

func usdcToUnitsInt(s string) int64 {
	var n int64
	for _, c := range usdcToUnits(s) {
		n = n*10 + int64(c-'0')
	}
	return n
}

func (h *handlers) cashoutPrepare(w http.ResponseWriter, r *http.Request) {
	if !h.cashoutEnabled(w, r) {
		return
	}
	if h.d.Intents == nil {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusServiceUnavailable, apperr.CodeProviderUnavailable, "NEAR Intents is not configured on this server."))
		return
	}
	var in cashoutPrepareIn
	if err := decodeJSON(w, r, &in); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	q := cashoutQuoteIn{AmountUSDC: in.AmountUSDC}
	if err := q.check(); err != nil {
		writeAppError(w, r, h.d.Logger, apperr.Validation(err.Error()))
		return
	}
	in.NearAccount = strings.ToLower(strings.TrimSpace(in.NearAccount))
	if !validNearAddress(in.NearAccount) {
		writeAppError(w, r, h.d.Logger, apperr.Validation("near_account must be the NEAR account the USDC is sent from, e.g. shop.near"))
		return
	}
	if ok, msg := h.liveCashoutAllowed(q.AmountUSDC); !ok {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusForbidden, apperr.CodeValidation, msg))
		return
	}
	if !h.allowRun(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()

	raw, err := h.d.Switch.OfframpInitiate(ctx, switchramp.InitiateInput{Amount: q.AmountUSDC, Bank: in.Bank})
	if err != nil {
		h.cashoutError(w, r, err)
		return
	}
	sess, err := parseSwitchSession(raw)
	if err != nil {
		h.d.Logger.Warn("switch session rejected", "error", err)
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusBadGateway, apperr.CodeProviderInvalidResp, "Switch returned a payout session Beeline could not use. Nothing was sent."))
		return
	}
	depositUnits := usdcToUnits(sess.Deposit.Amount.String())
	if !baseUnitsRe.MatchString(depositUnits) {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusBadGateway, apperr.CodeProviderInvalidResp, "Switch returned an unusable deposit amount. Nothing was sent."))
		return
	}

	resp := map[string]any{
		"mode": h.d.Switch.Mode,
		"payout": map[string]any{
			"source": "provider-reported (Switch)", "reference": sess.Reference, "status": sess.Status,
			"naira": sess.Destination.Amount.String(), "currency": sess.Destination.Currency,
			"deposit_amount_usdc": sess.Deposit.Amount.String(), "deposit_asset": sess.Deposit.Asset,
		},
	}
	// A sandbox deposit address is not a real Base address, so the bridge cannot be funded to it.
	if !evmAddressRe.MatchString(sess.Deposit.Address) {
		resp["route"] = nil
		resp["note"] = "SANDBOX: Switch's deposit address is a placeholder, so there is nothing real to bridge to. No money can move."
		writeJSON(w, http.StatusCreated, resp)
		return
	}
	deadline := time.Now().UTC().Add(25 * time.Minute) // inside Switch's 30 minute address window
	quote, err := h.d.Intents.Quote(ctx, intents.QuoteRequest{
		Dry: false, SwapType: "EXACT_OUTPUT", SlippageTolerance: 100,
		OriginAsset: nearUSDC, DepositType: "ORIGIN_CHAIN", DestinationAsset: baseUSDC,
		Amount: depositUnits, RefundTo: in.NearAccount, RefundType: "ORIGIN_CHAIN",
		Recipient: sess.Deposit.Address, RecipientType: "DESTINATION_CHAIN",
		Deadline: deadline.Format("2006-01-02T15:04:05.000Z"),
	})
	if err != nil {
		h.d.Logger.Warn("cash-out route quote failed", "error", err)
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusBadGateway, apperr.CodeProviderUnavailable,
			"NEAR Intents could not quote the NEAR to Base step. The Switch payout session ("+sess.Reference+") stays unfunded and expires by itself. Nothing was sent."))
		return
	}
	if !validNearAddress(quote.Quote.DepositAddress) || !baseUnitsRe.MatchString(quote.Quote.AmountIn) ||
		!baseUnitsRe.MatchString(quote.Quote.MinAmountIn) || quote.Quote.DepositMemo != "" {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusBadGateway, apperr.CodeProviderInvalidResp, "NEAR Intents returned a quote Beeline could not verify. Nothing was sent."))
		return
	}
	resp["route"] = map[string]any{
		"source": "provider-reported (NEAR Intents)", "deposit_address": quote.Quote.DepositAddress,
		"send_usdc": unitsToUSDC(quote.Quote.AmountIn), "send_units": quote.Quote.AmountIn,
		"min_send_usdc": unitsToUSDC(quote.Quote.MinAmountIn), "delivers_usdc": unitsToUSDC(quote.Quote.AmountOut),
		"deliver_to": sess.Deposit.Address, "refund_to": in.NearAccount, "time_estimate_seconds": quote.Quote.TimeEstimate,
		"deadline": quote.Quote.Deadline, "token_contract": strings.TrimPrefix(nearUSDC, "nep141:"),
	}
	resp["note"] = "Nothing has moved. To go ahead, send exactly the amount shown, in USDC on NEAR, from " + in.NearAccount +
		" to the deposit address, before the deadline. Beeline cannot send it for you: only your wallet can sign."
	writeJSON(w, http.StatusCreated, resp)
}

type cashoutDepositIn struct {
	DepositAddress string `json:"deposit_address"`
	TxHash         string `json:"tx_hash"`
	NearSender     string `json:"near_sender_account"`
}

// cashoutDeposit tells NEAR Intents which NEAR transaction funded a deposit address, which speeds up
// detection. It sends nothing itself; the transaction was signed in the vendor's wallet.
func (h *handlers) cashoutDeposit(w http.ResponseWriter, r *http.Request) {
	if h.d.Intents == nil {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusServiceUnavailable, apperr.CodeProviderUnavailable, "NEAR Intents is not configured on this server."))
		return
	}
	var in cashoutDepositIn
	if err := decodeJSON(w, r, &in); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	in.NearSender = strings.ToLower(strings.TrimSpace(in.NearSender))
	if !validNearAddress(in.DepositAddress) {
		writeAppError(w, r, h.d.Logger, apperr.Validation("deposit_address must be the NEAR deposit address from the quote"))
		return
	}
	if !nearTxHashRe.MatchString(in.TxHash) {
		writeAppError(w, r, h.d.Logger, apperr.Validation("tx_hash must be the NEAR transaction hash from your wallet"))
		return
	}
	if in.NearSender != "" && !validNearAddress(in.NearSender) {
		writeAppError(w, r, h.d.Logger, apperr.Validation("near_sender_account is not a valid NEAR account"))
		return
	}
	if !h.allowRun(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	st, err := h.d.Intents.SubmitDeposit(ctx, intents.SubmitDepositRequest{TxHash: in.TxHash, DepositAddress: in.DepositAddress, NearSenderAccount: in.NearSender})
	if err != nil {
		h.d.Logger.Warn("deposit submit failed", "error", err)
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusBadGateway, apperr.CodeProviderUnavailable, "NEAR Intents did not accept the deposit notice. This is safe to retry; it does not send anything."))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "provider-reported (NEAR Intents)", "status": st.Status, "updated_at": st.UpdatedAt})
}

// bridgeStatus reads NEAR Intents' status for a deposit address. Read-only; nil when not asked for.
func (h *handlers) bridgeStatus(ctx context.Context, depositAddress string) map[string]any {
	if h.d.Intents == nil || depositAddress == "" {
		return nil
	}
	if !validNearAddress(depositAddress) {
		return map[string]any{"error": "deposit_address is not a valid NEAR address"}
	}
	st, err := h.d.Intents.Status(ctx, depositAddress, "")
	if err != nil {
		return map[string]any{"source": "provider-reported (NEAR Intents)", "status": "UNKNOWN", "error": "no status yet"}
	}
	return map[string]any{"source": "provider-reported (NEAR Intents)", "status": st.Status, "updated_at": st.UpdatedAt}
}
