package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes the standard error envelope. Messages must be safe for clients:
// no stack traces, provider headers or secrets.
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{
		Code:      code,
		Message:   message,
		RequestID: middleware.GetReqID(r.Context()),
	}})
}

// writeAppError maps typed errors to their envelope. Anything untyped is logged and
// reported as a generic internal error.
func writeAppError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	if ae, ok := apperr.As(err); ok {
		writeError(w, r, ae.Status, ae.Code, ae.Message)
		return
	}
	log.Error("unhandled error", "request_id", middleware.GetReqID(r.Context()), "error", err)
	writeError(w, r, http.StatusInternalServerError, apperr.CodeInternal, "Internal error.")
}

const maxRequestBody = 64 << 10

// decodeJSON strictly decodes a request body: unknown fields and trailing data are rejected.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return apperr.Validation("Request body is too large.")
		}
		return apperr.Validation("Request body is not valid JSON for this endpoint: " + err.Error())
	}
	if dec.More() {
		return apperr.Validation("Request body must contain a single JSON object.")
	}
	return nil
}
