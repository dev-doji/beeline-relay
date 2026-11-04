// Package apperr defines typed, client-safe errors. Codes match docs/spec/API_SPEC.md.
package apperr

import (
	"errors"
	"net/http"
)

type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// As extracts an *Error from err, if any.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

const (
	CodeValidation             = "VALIDATION_ERROR"
	CodeUnauthorized           = "UNAUTHORIZED"
	CodeNotFound               = "NOT_FOUND"
	CodeUnsupportedNetwork     = "UNSUPPORTED_NETWORK"
	CodeUnsupportedAsset       = "UNSUPPORTED_ASSET"
	CodeRouteUnavailable       = "ROUTE_UNAVAILABLE"
	CodeQuoteNotFound          = "QUOTE_NOT_FOUND"
	CodeQuoteExpired           = "QUOTE_EXPIRED"
	CodeQuoteAlreadyUsed       = "QUOTE_ALREADY_USED"
	CodeProviderUnavailable    = "PROVIDER_UNAVAILABLE"
	CodeProviderRejected       = "PROVIDER_REJECTED"
	CodeProviderInvalidResp    = "PROVIDER_INVALID_RESPONSE"
	CodePaymentNotFound        = "PAYMENT_NOT_FOUND"
	CodeIdempotencyKeyRequired = "IDEMPOTENCY_KEY_REQUIRED"
	CodeIdempotencyConflict    = "IDEMPOTENCY_CONFLICT"
	CodeInvalidStateTransition = "INVALID_STATE_TRANSITION"
	CodeRateLimited            = "RATE_LIMITED"
	CodeInternal               = "INTERNAL_ERROR"
)

func Validation(msg string) *Error { return New(http.StatusBadRequest, CodeValidation, msg) }
