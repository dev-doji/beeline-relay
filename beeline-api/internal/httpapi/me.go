package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/accounts"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/switchramp"
)

// Signed-in app: a user signs in with Privy (email code), sets a shop name and payout account, then
// asks for invoices in a chat. Every route here needs a valid Privy access token, and a user only
// ever sees their own invoices and messages.

type ctxUserKey struct{}

func userID(r *http.Request) string {
	id, _ := r.Context().Value(ctxUserKey{}).(string)
	return id
}

func (h *handlers) meEnabled(w http.ResponseWriter, r *http.Request) bool {
	if h.d.Accounts == nil || h.d.Privy == nil || h.d.Invoices == nil {
		writeAppError(w, r, h.d.Logger, apperr.New(http.StatusServiceUnavailable, apperr.CodeProviderUnavailable, "Accounts are not enabled on this server."))
		return false
	}
	return true
}

// requireUser verifies the Privy access token and puts the user's ID on the request.
func (h *handlers) requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.meEnabled(w, r) {
			return
		}
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || tok == "" {
			writeAppError(w, r, h.d.Logger, apperr.New(http.StatusUnauthorized, apperr.CodeUnauthorized, "Sign in to continue."))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		id, err := h.d.Privy.Verify(ctx, tok)
		if err != nil {
			if errors.Is(err, accounts.ErrInvalidToken) {
				writeAppError(w, r, h.d.Logger, apperr.New(http.StatusUnauthorized, apperr.CodeUnauthorized, "Your sign-in expired. Please sign in again."))
				return
			}
			h.d.Logger.Warn("verify sign-in", "error", err)
			writeAppError(w, r, h.d.Logger, apperr.New(http.StatusBadGateway, apperr.CodeProviderUnavailable, "Could not check your sign-in right now. Try again."))
			return
		}
		if _, err := h.d.Accounts.Ensure(r.Context(), id); err != nil {
			writeAppError(w, r, h.d.Logger, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxUserKey{}, id)))
	}
}

func (h *handlers) meGet(w http.ResponseWriter, r *http.Request) {
	u, err := h.d.Accounts.Get(r.Context(), userID(r))
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (h *handlers) meUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ShopName    string `json:"shop_name"`
		NearAccount string `json:"near_account"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	if _, _, err := accounts.ValidateProfile(in.ShopName, in.NearAccount); err != nil {
		writeAppError(w, r, h.d.Logger, apperr.Validation(err.Error()))
		return
	}
	u, err := h.d.Accounts.UpdateProfile(r.Context(), userID(r), in.ShopName, in.NearAccount)
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (h *handlers) meInvoices(w http.ResponseWriter, r *http.Request) {
	list, err := h.d.Invoices.ListByUser(r.Context(), userID(r), 100)
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *handlers) meMessages(w http.ResponseWriter, r *http.Request) {
	list, err := h.d.Accounts.Messages(r.Context(), userID(r), 100)
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": list})
}

// chatHost lets the agent's tools act for one signed-in user, and only that user.
type chatHost struct {
	h    *handlers
	uid  string
	user *accounts.User
}

func (c *chatHost) Profile() (string, string) { return c.user.ShopName, c.user.NearAccount }

func (c *chatHost) SetProfile(ctx context.Context, shop, account string) error {
	u, err := c.h.d.Accounts.UpdateProfile(ctx, c.uid, shop, account)
	if err != nil {
		return err
	}
	c.user = u
	return nil
}

func (c *chatHost) CreateInvoice(ctx context.Context, items []invoices.Item, real bool) (*invoices.Invoice, error) {
	ci := invoices.CreateInput{VendorName: c.user.ShopName, Payout: c.user.NearAccount, Items: items, Real: real}
	if _, err := c.h.d.Invoices.Check(&ci); err != nil {
		return nil, err // a request problem (for example the 5 USDC cap): the agent explains it
	}
	return c.h.d.Invoices.CreateFor(ctx, c.uid, ci)
}

// The naira rate is Switch's payout quote for 100 USDC to a Nigerian bank (naira per USDC). It is read-only,
// and cached for a minute so a burst of invoices does not hit Switch each time.
var nairaRate struct {
	mu   sync.Mutex
	rate string
	at   time.Time
}

func (c *chatHost) NGNRate(ctx context.Context) (string, error) {
	if c.h.d.Switch == nil {
		return "", errors.New("naira conversion is not configured")
	}
	nairaRate.mu.Lock()
	defer nairaRate.mu.Unlock()
	if nairaRate.rate != "" && time.Since(nairaRate.at) < time.Minute {
		return nairaRate.rate, nil
	}
	raw, err := c.h.d.Switch.OfframpQuote(ctx, switchramp.QuoteInput{Amount: "100", Country: "NG", Asset: "base:usdc"})
	if err != nil {
		return "", err
	}
	var q struct {
		Rate json.Number `json:"rate"`
	}
	if json.Unmarshal(raw, &q) != nil || q.Rate.String() == "" {
		return "", errors.New("switch returned no rate")
	}
	nairaRate.rate, nairaRate.at = q.Rate.String(), time.Now()
	return nairaRate.rate, nil
}

func (c *chatHost) ListInvoices(ctx context.Context) ([]invoices.Invoice, error) {
	return c.h.d.Invoices.ListByUser(ctx, c.uid, 10)
}

func (c *chatHost) PermanentLinkPath() (string, error) {
	if c.user.ShopName == "" || c.user.NearAccount == "" {
		return "", errors.New("the user has no shop set up yet")
	}
	return "/pay/to?" + url.Values{"name": {c.user.ShopName}, "to": {c.user.NearAccount}}.Encode(), nil
}

// meNewChat starts a fresh conversation for the signed-in user. Earlier messages stay stored but are no longer
// shown or sent to the model.
func (h *handlers) meNewChat(w http.ResponseWriter, r *http.Request) {
	if err := h.d.Accounts.NewThread(r.Context(), userID(r)); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// meChat answers a chat message. Bee (the NEAR AI agent) handles it; if the model is unavailable the
// simple command parser still makes invoices from messages like "invoice 25 for logo design".
func (h *handlers) meChat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Message string `json:"message"`
		Wallet  string `json:"wallet"` // the wallet Privy created for the user, as reported by their browser
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	in.Message = strings.TrimSpace(in.Message)
	in.Wallet = strings.ToLower(strings.TrimSpace(in.Wallet))
	if in.Message == "" || len(in.Message) > 500 {
		writeAppError(w, r, h.d.Logger, apperr.Validation("message must be 1-500 characters"))
		return
	}
	if in.Wallet != "" && !validNearAddress(in.Wallet) {
		in.Wallet = ""
	}
	if !h.allowRun(w, r) {
		return
	}
	ctx := r.Context()
	uid := userID(r)
	user, err := h.d.Accounts.Get(ctx, uid)
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	history, err := h.d.Accounts.Messages(ctx, uid, 12)
	if err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	if err := h.d.Accounts.AddMessage(ctx, uid, "user", in.Message, nil); err != nil {
		writeAppError(w, r, h.d.Logger, err)
		return
	}
	host := &chatHost{h: h, uid: uid, user: user}

	var res *accounts.Result
	if h.d.Chat != nil {
		actx, cancel := context.WithTimeout(ctx, 90*time.Second)
		res, err = accounts.Run(actx, h.d.Chat, h.d.ChatModel, history, in.Message, in.Wallet, host)
		cancel()
		if err != nil {
			h.d.Logger.Warn("chat agent failed, using the command parser", "error", err)
			res = nil
		}
	}
	if res == nil {
		res = h.parserTurn(ctx, host, in.Message)
	}

	var invID *string
	if res.Invoice != nil {
		invID = &res.Invoice.ID
	}
	body := res.Reply
	if res.PermanentPath != "" {
		body += "\n\n" + res.PermanentPath
	}
	if err := h.d.Accounts.AddMessage(ctx, uid, "assistant", body, invID); err != nil {
		h.d.Logger.Error("save reply", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"reply": body, "invoice": res.Invoice, "needs_profile": res.NeedsProfile, "agent": h.d.Chat != nil})
}

// parserTurn is the no-model fallback: it understands the documented invoice phrasings only.
func (h *handlers) parserTurn(ctx context.Context, host *chatHost, msg string) *accounts.Result {
	if host.user.ShopName == "" || host.user.NearAccount == "" {
		return &accounts.Result{Reply: "First, tell me your shop name and the NEAR account that should receive the money.", NeedsProfile: true}
	}
	var items []invoices.Item
	var err error
	if accounts.WantsNaira(msg) {
		rate, rerr := host.NGNRate(ctx)
		if rerr != nil {
			return &accounts.Result{Reply: "I could not get today's naira rate right now. Try again in a moment, or price in USDC."}
		}
		items, err = accounts.ParseInvoiceNGN(msg, rate)
	} else {
		items, err = accounts.ParseInvoice(msg)
	}
	if err != nil {
		return &accounts.Result{Reply: err.Error()}
	}
	inv, err := host.CreateInvoice(ctx, items, false)
	if err != nil {
		return &accounts.Result{Reply: "I could not make that invoice: " + err.Error()}
	}
	return &accounts.Result{Invoice: inv, Reply: fmt.Sprintf("Here is your invoice for %s USDC, signed and payable to %s. Share the link anywhere, or show the counter QR.", unitsToUSDC(inv.Amount), inv.Payout)}
}
