// Package switchramp is a small client for the OFFRAMP side of Switch (onswitch.xyz): stablecoin
// in, local currency out to a bank account. Docs: https://docs.onswitch.xyz (reviewed 2026-11-12).
// Auth is an `x-service-key` header. The same host serves sandbox and live; the key decides.
//
// Verified from the docs, not yet against a live key:
//   - POST /offramp/quote   {amount, country, asset, currency?, channel?}  (moves no money)
//   - POST /offramp/initiate {amount, country, asset, currency, channel, narration, beneficiary{...}}
//     returns data.reference and data.deposit{amount,address,asset,note[]}: the caller sends exactly
//     deposit.amount of deposit.asset to deposit.address (one-time, about 30 minutes) and Switch pays
//     the bank automatically.
//   - GET  /payment/status?reference=<uuid>  -> data.status AWAITING_DEPOSIT|PROCESSING|COMPLETED|FAILED
//
// Switch does not support NEAR, so USDC on NEAR must reach the deposit address on another chain
// (for example Base) through NEAR Intents. This package never moves funds.
package switchramp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.onswitch.xyz"

var (
	amountRe  = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})(\.[0-9]{1,6})?$`)
	acctRe    = regexp.MustCompile(`^[0-9]{10}$`)
	bankRe    = regexp.MustCompile(`^[0-9]{3,6}$`)
	nameRe    = regexp.MustCompile(`^[A-Za-z][A-Za-z .'-]{1,59}$`)
	uuidRe    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	assetRe   = regexp.MustCompile(`^[a-z]+:[a-z]+$`)
	countryRe = regexp.MustCompile(`^[A-Z]{2}$`)
)

// ErrUnavailable means Switch could not be reached or refused the key.
var ErrUnavailable = errors.New("switch is unavailable")

// APIError is a rejection from Switch for a bad request.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return "switch: " + e.Message }

func bad(msg string) error { return &APIError{Status: 400, Message: msg} }

type Client struct {
	BaseURL string
	Key     string
	Mode    string // "sandbox" or "live", as the owner declared when configuring the key
	HTTP    *http.Client
}

func New(baseURL, key, mode string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if mode != "live" {
		mode = "sandbox"
	}
	return &Client{BaseURL: baseURL, Key: key, Mode: mode, HTTP: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any) (json.RawMessage, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-service-key", c.Key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w: the key was refused (HTTP %d)", ErrUnavailable, res.StatusCode)
	}
	if res.StatusCode >= 400 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Message == "" {
			e.Message = fmt.Sprintf("HTTP %d", res.StatusCode)
		}
		return nil, &APIError{Status: res.StatusCode, Message: e.Message}
	}
	var out struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Data) == 0 {
		return nil, fmt.Errorf("%w: unreadable response", ErrUnavailable)
	}
	return out.Data, nil
}

// QuoteInput asks what a USDC amount would pay out in local currency. Amount is a decimal string
// (USDC), sent to Switch as a JSON number without passing through floating point.
type QuoteInput struct {
	Amount  string
	Country string
	Asset   string
}

func (in QuoteInput) validate() error {
	if !amountRe.MatchString(in.Amount) || in.Amount == "0" {
		return bad("amount must be a positive USDC amount like 5.00")
	}
	if !countryRe.MatchString(in.Country) {
		return bad("country must be a 2-letter code like NG")
	}
	if !assetRe.MatchString(in.Asset) {
		return bad("asset must look like base:usdc")
	}
	return nil
}

// OfframpQuote asks Switch for a payout quote. It moves no money.
func (c *Client) OfframpQuote(ctx context.Context, in QuoteInput) (json.RawMessage, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	body := fmt.Sprintf(`{"amount":%s,"country":%q,"asset":%q}`, in.Amount, in.Country, in.Asset)
	return c.doRaw(ctx, http.MethodPost, "/offramp/quote", json.RawMessage(body))
}

// Bank is the Nigerian bank account the payout goes to. Never log it.
type Bank struct {
	BankCode      string `json:"bank_code"`
	AccountNumber string `json:"account_number"`
	HolderName    string `json:"holder_name"`
}

func (b Bank) validate() error {
	if !bankRe.MatchString(b.BankCode) {
		return bad("bank_code must be the 3-6 digit code of the bank")
	}
	if !acctRe.MatchString(b.AccountNumber) {
		return bad("account_number must be a 10-digit NUBAN account number")
	}
	if !nameRe.MatchString(strings.TrimSpace(b.HolderName)) {
		return bad("holder_name must be the account holder's name")
	}
	return nil
}

// InitiateInput starts a payout session for a Nigerian bank account.
type InitiateInput struct {
	Amount string // USDC, decimal string
	Bank   Bank
}

// OfframpInitiate creates a payout session. It returns a one-time deposit address; no money moves
// until someone sends exactly deposit.amount of deposit.asset to it.
func (c *Client) OfframpInitiate(ctx context.Context, in InitiateInput) (json.RawMessage, error) {
	if err := (QuoteInput{Amount: in.Amount, Country: "NG", Asset: "base:usdc"}).validate(); err != nil {
		return nil, err
	}
	if err := in.Bank.validate(); err != nil {
		return nil, err
	}
	bank, _ := json.Marshal(map[string]string{
		"bank_code": in.Bank.BankCode, "account_number": in.Bank.AccountNumber,
		"holder_name": strings.TrimSpace(in.Bank.HolderName), "holder_type": "INDIVIDUAL",
	})
	body := fmt.Sprintf(`{"amount":%s,"country":"NG","currency":"NGN","asset":"base:usdc","channel":"BANK","narration":"Beeline cash-out","beneficiary":%s}`, in.Amount, bank)
	return c.doRaw(ctx, http.MethodPost, "/offramp/initiate", json.RawMessage(body))
}

// Status reads a payout by Switch's reference. Read-only.
func (c *Client) Status(ctx context.Context, reference string) (json.RawMessage, error) {
	if !uuidRe.MatchString(reference) {
		return nil, bad("reference must be a UUID")
	}
	return c.do(ctx, http.MethodGet, "/payment/status", url.Values{"reference": {reference}}, nil)
}

func (c *Client) doRaw(ctx context.Context, method, path string, body json.RawMessage) (json.RawMessage, error) {
	return c.do(ctx, method, path, nil, body)
}
