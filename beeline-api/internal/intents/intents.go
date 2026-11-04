// Package intents is the NEAR Intents 1Click adapter. All provider-specific request and
// response shapes live here; the rest of the API sees only these types.
//
// Source: https://1click.chaindefuser.com/docs/v0/openapi.yaml (reviewed 2026-10-28) and
// docs/NEAR_INTENTS_ROUTE.md.
package intents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Native 1Click execution statuses.
const (
	StatusPendingDeposit    = "PENDING_DEPOSIT"
	StatusKnownDepositTx    = "KNOWN_DEPOSIT_TX"
	StatusProcessing        = "PROCESSING"
	StatusSuccess           = "SUCCESS"
	StatusIncompleteDeposit = "INCOMPLETE_DEPOSIT"
	StatusRefunded          = "REFUNDED"
	StatusFailed            = "FAILED"
)

type QuoteRequest struct {
	Dry               bool   `json:"dry"`
	SwapType          string `json:"swapType"`
	SlippageTolerance int    `json:"slippageTolerance"`
	OriginAsset       string `json:"originAsset"`
	DepositType       string `json:"depositType"`
	DestinationAsset  string `json:"destinationAsset"`
	Amount            string `json:"amount"`
	RefundTo          string `json:"refundTo"`
	RefundType        string `json:"refundType"`
	Recipient         string `json:"recipient"`
	RecipientType     string `json:"recipientType"`
	Deadline          string `json:"deadline"`
}

type Quote struct {
	DepositAddress string     `json:"depositAddress"`
	DepositMemo    string     `json:"depositMemo"`
	AmountIn       string     `json:"amountIn"`
	MinAmountIn    string     `json:"minAmountIn"`
	AmountOut      string     `json:"amountOut"`
	MinAmountOut   string     `json:"minAmountOut"`
	Deadline       *time.Time `json:"deadline"`
	TimeEstimate   float64    `json:"timeEstimate"`
}

type QuoteResponse struct {
	CorrelationID string    `json:"correlationId"`
	Timestamp     time.Time `json:"timestamp"`
	Signature     string    `json:"signature"`
	Quote         Quote     `json:"quote"`

	// Raw is the provider's response body exactly as received, kept for evidence.
	Raw json.RawMessage `json:"-"`
}

type TxRef struct {
	Hash        string `json:"hash"`
	ExplorerURL string `json:"explorerUrl"`
}

type SwapDetails struct {
	IntentHashes             []string `json:"intentHashes"`
	NearTxHashes             []string `json:"nearTxHashes"`
	AmountIn                 string   `json:"amountIn"`
	AmountOut                string   `json:"amountOut"`
	OriginChainTxHashes      []TxRef  `json:"originChainTxHashes"`
	DestinationChainTxHashes []TxRef  `json:"destinationChainTxHashes"`
	RefundedAmount           string   `json:"refundedAmount"`
	RefundReason             string   `json:"refundReason"`
	DepositedAmount          string   `json:"depositedAmount"`
}

type StatusResponse struct {
	CorrelationID string      `json:"correlationId"`
	Status        string      `json:"status"`
	UpdatedAt     time.Time   `json:"updatedAt"`
	SwapDetails   SwapDetails `json:"swapDetails"`

	Raw json.RawMessage `json:"-"`
}

type SubmitDepositRequest struct {
	TxHash            string `json:"txHash"`
	DepositAddress    string `json:"depositAddress"`
	NearSenderAccount string `json:"nearSenderAccount,omitempty"`
	Memo              string `json:"memo,omitempty"`
}

// Provider is the internal interface to 1Click. The HTTP client implements it for live use;
// tests use a deterministic double.
type Provider interface {
	Quote(ctx context.Context, req QuoteRequest) (*QuoteResponse, error)
	Status(ctx context.Context, depositAddress, depositMemo string) (*StatusResponse, error)
	SubmitDeposit(ctx context.Context, req SubmitDepositRequest) (*StatusResponse, error)
}

// ErrorKind classifies provider failures so callers can choose safe behavior.
type ErrorKind string

const (
	// KindUnavailable: network error, timeout or 5xx. The outcome of a call is unknown.
	KindUnavailable ErrorKind = "unavailable"
	// KindRejected: the provider refused the request (4xx). Nothing was created.
	KindRejected     ErrorKind = "rejected"
	KindNotFound     ErrorKind = "not_found"
	KindUnauthorized ErrorKind = "unauthorized"
	// KindInvalidResponse: the response failed validation and must not be trusted.
	KindInvalidResponse ErrorKind = "invalid_response"
)

type Error struct {
	Kind       ErrorKind
	HTTPStatus int
	// Message is provider-supplied text: untrusted, sanitized and length-limited.
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("1click %s (http %d): %s", e.Kind, e.HTTPStatus, e.Message)
}
