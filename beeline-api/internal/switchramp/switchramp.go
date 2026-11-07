// Package switchramp is a small read-only client for Switch (onswitch.xyz), which turns local
// currency (bank transfer or mobile money, 21 African countries) into stablecoins on EVM chains
// and Solana. Docs: https://docs.onswitch.xyz. Auth is an `x-service-key` header.
//
// Only coverage and quotes are used here. A quote moves no money. Switch does not support NEAR,
// so funds would arrive on Base and be bridged into the vault by NEAR Intents (not built yet).
// A sandbox key only simulates settlement: callers must label results with Mode.
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
	"time"
)

const DefaultBaseURL = "https://api.onswitch.xyz"

var (
	countryRe = regexp.MustCompile(`^[A-Z]{2}$`)
	assetRe   = regexp.MustCompile(`^[a-z]+:[a-z]+$`)
)

// ErrUnavailable means Switch could not be reached or refused the key.
var ErrUnavailable = errors.New("switch is unavailable")

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

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("x-service-key", c.Key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: the key was refused (HTTP %d)", ErrUnavailable, res.StatusCode)
	}
	if res.StatusCode >= 400 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Message == "" {
			e.Message = fmt.Sprintf("HTTP %d", res.StatusCode)
		}
		return &APIError{Status: res.StatusCode, Message: e.Message}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%w: unreadable response", ErrUnavailable)
	}
	return nil
}

// APIError is a rejection from Switch for a bad request (for example an unsupported country).
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return "switch: " + e.Message }

// Coverage lists the onramp corridors Switch offers, optionally for one country (ISO 3166-1 alpha-2).
func (c *Client) Coverage(ctx context.Context, country string) (json.RawMessage, error) {
	q := url.Values{"direction": {"ONRAMP"}}
	if country != "" {
		if !countryRe.MatchString(country) {
			return nil, &APIError{Status: 400, Message: "country must be a 2-letter code like NG"}
		}
		q.Set("country", country)
	}
	var out struct {
		Data json.RawMessage `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/coverage", q, nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// QuoteInput is a local-currency to stablecoin quote request.
type QuoteInput struct {
	Country string  `json:"country"`
	Asset   string  `json:"asset"`
	Amount  float64 `json:"amount"` // local currency, as Switch's API takes it (display only: never used for money math here)
	Channel string  `json:"channel,omitempty"`
}

// Quote asks Switch for an onramp quote. It moves no money.
func (c *Client) Quote(ctx context.Context, in QuoteInput) (json.RawMessage, error) {
	if !countryRe.MatchString(in.Country) {
		return nil, &APIError{Status: 400, Message: "country must be a 2-letter code like NG"}
	}
	if !assetRe.MatchString(in.Asset) {
		return nil, &APIError{Status: 400, Message: "asset must look like base:usdc"}
	}
	if !(in.Amount > 0) || in.Amount > 100_000_000 {
		return nil, &APIError{Status: 400, Message: "amount must be a positive number"}
	}
	if in.Channel != "" && in.Channel != "BANK" && in.Channel != "MOBILEMONEY" {
		return nil, &APIError{Status: 400, Message: "channel must be BANK or MOBILEMONEY"}
	}
	var out struct {
		Data json.RawMessage `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, "/onramp/quote", nil, in, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}
