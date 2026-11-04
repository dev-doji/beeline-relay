package intents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const maxBodyBytes = 1 << 20

// Client calls the live 1Click API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewClient(baseURL, apiKey string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *Client) Quote(ctx context.Context, req QuoteRequest) (*QuoteResponse, error) {
	var out QuoteResponse
	raw, err := c.do(ctx, http.MethodPost, "/v0/quote", req, &out)
	if err != nil {
		return nil, err
	}
	out.Raw = raw
	if err := validateQuote(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Status(ctx context.Context, depositAddress, depositMemo string) (*StatusResponse, error) {
	q := url.Values{"depositAddress": {depositAddress}}
	if depositMemo != "" {
		q.Set("depositMemo", depositMemo)
	}
	var out StatusResponse
	raw, err := c.do(ctx, http.MethodGet, "/v0/status?"+q.Encode(), nil, &out)
	if err != nil {
		return nil, err
	}
	out.Raw = raw
	if err := validateStatus(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) SubmitDeposit(ctx context.Context, req SubmitDepositRequest) (*StatusResponse, error) {
	var out StatusResponse
	raw, err := c.do(ctx, http.MethodPost, "/v0/deposit/submit", req, &out)
	if err != nil {
		return nil, err
	}
	out.Raw = raw
	if err := validateStatus(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) (json.RawMessage, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// Timeouts and connection errors: the provider may or may not have acted.
		return nil, &Error{Kind: KindUnavailable, Message: "request failed: " + errorClass(err)}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, &Error{Kind: KindUnavailable, HTTPStatus: resp.StatusCode, Message: "reading response failed"}
	}

	if resp.StatusCode >= 300 {
		return nil, &Error{Kind: kindForStatus(resp.StatusCode), HTTPStatus: resp.StatusCode, Message: providerMessage(raw)}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, &Error{Kind: KindInvalidResponse, HTTPStatus: resp.StatusCode, Message: "response is not valid JSON for the expected schema"}
	}
	return raw, nil
}

func kindForStatus(code int) ErrorKind {
	switch {
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return KindUnauthorized
	case code == http.StatusNotFound:
		return KindNotFound
	case code == http.StatusTooManyRequests || code >= 500:
		return KindUnavailable
	default:
		return KindRejected
	}
}

// providerMessage extracts the provider's "message" field, stripped of control characters
// and capped in length, since it is untrusted text that may be shown to API clients.
func providerMessage(raw []byte) string {
	var body struct {
		Message any `json:"message"`
	}
	msg := ""
	if json.Unmarshal(raw, &body) == nil && body.Message != nil {
		msg = fmt.Sprint(body.Message)
	}
	msg = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, msg)
	if len(msg) > 300 {
		msg = msg[:300]
	}
	if msg == "" {
		msg = "no message"
	}
	return msg
}

func errorClass(err error) string {
	var ne interface{ Timeout() bool }
	if errors.As(err, &ne) && ne.Timeout() {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return "connection error"
}

func invalid(msg string) error {
	return &Error{Kind: KindInvalidResponse, HTTPStatus: http.StatusOK, Message: msg}
}

// validateQuote rejects responses that do not match what we asked for. A tampered or
// mismatched quote must never be stored as executable.
func validateQuote(req QuoteRequest, q *QuoteResponse) error {
	if q.Signature == "" || q.CorrelationID == "" {
		return invalid("quote is missing signature or correlationId")
	}
	if !req.Dry && q.Quote.DepositAddress == "" {
		return invalid("non-dry quote is missing depositAddress")
	}
	if req.SwapType == "EXACT_INPUT" && q.Quote.AmountIn != req.Amount {
		return invalid("quote amountIn does not match the requested amount")
	}
	if !isDigits(q.Quote.AmountOut) || (q.Quote.MinAmountOut != "" && !isDigits(q.Quote.MinAmountOut)) {
		return invalid("quote output amounts are not base-unit integers")
	}
	return nil
}

func validateStatus(s *StatusResponse) error {
	switch s.Status {
	case StatusPendingDeposit, StatusKnownDepositTx, StatusProcessing, StatusSuccess,
		StatusIncompleteDeposit, StatusRefunded, StatusFailed:
		return nil
	default:
		return invalid(fmt.Sprintf("unknown status %q", s.Status))
	}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
