// Package intentstest provides a deterministic 1Click test double. It is for tests and the
// simulated sandbox only: it never contacts the real provider, and every deposit address it
// issues is prefixed "SIMULATED-" so it cannot be mistaken for a real one.
package intentstest

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents"
)

type Fake struct {
	mu sync.Mutex
	n  int

	// Now is used for quote timestamps and deadlines. Defaults to time.Now.
	Now func() time.Time
	// QuoteErr, when set, is returned by the next Quote call and then cleared.
	QuoteErr error
	// Statuses maps deposit address -> scripted status responses, returned in order
	// (the last one repeats).
	Statuses map[string][]intents.StatusResponse

	QuoteCalls  []intents.QuoteRequest
	SubmitCalls []intents.SubmitDepositRequest
}

func New() *Fake {
	return &Fake{Now: time.Now, Statuses: map[string][]intents.StatusResponse{}}
}

func (f *Fake) Quote(_ context.Context, req intents.QuoteRequest) (*intents.QuoteResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.QuoteCalls = append(f.QuoteCalls, req)
	if err := f.QuoteErr; err != nil {
		f.QuoteErr = nil
		return nil, err
	}
	f.n++
	now := f.Now().UTC()
	deadline := now.Add(10 * time.Minute)
	if t, err := time.Parse(time.RFC3339, req.Deadline); err == nil {
		deadline = t
	}
	resp := &intents.QuoteResponse{
		CorrelationID: fmt.Sprintf("sim-correlation-%d", f.n),
		Timestamp:     now,
		Signature:     "SIMULATED-signature",
		Quote: intents.Quote{
			AmountIn:     req.Amount,
			MinAmountIn:  req.Amount,
			AmountOut:    req.Amount, // 1:1 for the stablecoin test route
			MinAmountOut: req.Amount,
			Deadline:     &deadline,
			TimeEstimate: 35,
		},
	}
	if !req.Dry {
		resp.Quote.DepositAddress = fmt.Sprintf("SIMULATED-deposit-%d", f.n)
	}
	resp.Raw, _ = json.Marshal(resp)
	return resp, nil
}

func (f *Fake) Status(_ context.Context, depositAddress, _ string) (*intents.StatusResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	script := f.Statuses[depositAddress]
	if len(script) == 0 {
		return nil, &intents.Error{Kind: intents.KindNotFound, HTTPStatus: 404, Message: "deposit address not found"}
	}
	next := script[0]
	if len(script) > 1 {
		f.Statuses[depositAddress] = script[1:]
	}
	next.Raw, _ = json.Marshal(next)
	return &next, nil
}

func (f *Fake) SubmitDeposit(ctx context.Context, req intents.SubmitDepositRequest) (*intents.StatusResponse, error) {
	f.mu.Lock()
	f.SubmitCalls = append(f.SubmitCalls, req)
	f.mu.Unlock()
	return f.Status(ctx, req.DepositAddress, req.Memo)
}

var _ intents.Provider = (*Fake)(nil)
