// Package payment owns payment sessions: idempotent creation bound to a stored quote,
// backend-enforced state transitions and the append-only event log.
package payment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/ids"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/quote"
)

const (
	idempotencyTTL       = 24 * time.Hour
	maxIdempotencyKeyLen = 255
	maxClientRefLen      = 128
)

type Payment struct {
	ID               string     `json:"payment_id"`
	QuoteID          string     `json:"quote_id"`
	ClientReference  *string    `json:"client_reference"`
	AgentKeyID       *string    `json:"agent_key_id"`
	State            State      `json:"state"`
	Fingerprint      string     `json:"fingerprint"`
	SourceTxHash     *string    `json:"source_tx_hash"`
	ProviderStatus   *string    `json:"provider_status"`
	LastReconciledAt *time.Time `json:"last_reconciled_at"`
	IsSimulated      bool       `json:"is_simulated"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`

	WorkspaceID string `json:"-"`
}

type Event struct {
	ID            string          `json:"event_id"`
	PaymentID     string          `json:"payment_id"`
	Sequence      int             `json:"sequence"`
	Type          string          `json:"type"`
	Source        string          `json:"source"`
	StateBefore   *State          `json:"state_before"`
	StateAfter    *State          `json:"state_after"`
	ObservedAt    time.Time       `json:"occurred_at"`
	RecordedAt    time.Time       `json:"recorded_at"`
	CorrelationID *string         `json:"correlation_id"`
	Metadata      json.RawMessage `json:"metadata"`
}

type CreateInput struct {
	QuoteID         string  `json:"quote_id"`
	ClientReference *string `json:"client_reference"`
}

type CreateResult struct {
	Payment  *Payment
	Replayed bool
}

type Service struct {
	DB  *pgxpool.Pool
	Now func() time.Time
}

// RequestFingerprint identifies the logical request behind an idempotency key.
func RequestFingerprint(in CreateInput) string {
	ref := ""
	if in.ClientReference != nil {
		ref = *in.ClientReference
	}
	b, _ := json.Marshal(struct {
		V   int    `json:"v"`
		Q   string `json:"quote_id"`
		Ref string `json:"client_reference"`
	}{1, in.QuoteID, ref})
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ValidateIdempotencyKey(key string) error {
	if key == "" {
		return apperr.New(http.StatusBadRequest, apperr.CodeIdempotencyKeyRequired, "The Idempotency-Key header is required.")
	}
	if len(key) > maxIdempotencyKeyLen {
		return apperr.Validation("Idempotency-Key must be at most 255 characters.")
	}
	for _, r := range key {
		if r > unicode.MaxASCII || !unicode.IsPrint(r) {
			return apperr.Validation("Idempotency-Key must be printable ASCII.")
		}
	}
	return nil
}

// Create creates a payment session from a stored quote. The same key and request returns
// the original payment; the same key with a different request is rejected.
func (s *Service) Create(ctx context.Context, workspaceID string, agentKeyID *string, idemKey string, in CreateInput) (*CreateResult, error) {
	if err := ValidateIdempotencyKey(idemKey); err != nil {
		return nil, err
	}
	if in.QuoteID == "" {
		return nil, apperr.Validation("quote_id is required.")
	}
	if in.ClientReference != nil && (len(*in.ClientReference) == 0 || len(*in.ClientReference) > maxClientRefLen) {
		return nil, apperr.Validation("client_reference must be 1-128 characters when provided.")
	}

	reqFP := RequestFingerprint(in)
	// A concurrent request with the same key can win the race; then we read its result.
	for attempt := 0; attempt < 2; attempt++ {
		res, err := s.create(ctx, workspaceID, agentKeyID, idemKey, reqFP, in)
		if isUniqueViolation(err, "idempotency_records_pkey") && attempt == 0 {
			continue
		}
		return res, err
	}
	return nil, errors.New("unreachable")
}

func (s *Service) create(ctx context.Context, workspaceID string, agentKeyID *string, idemKey, reqFP string, in CreateInput) (*CreateResult, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	now := s.Now().UTC()

	// Check the key before touching the quote, so a conflicting reuse is reported as a
	// conflict even when it names a quote that does not exist.
	if res, err := s.checkIdempotency(ctx, tx, workspaceID, idemKey, reqFP, now); res != nil || err != nil {
		return res, err
	}

	// Lock the quote. Concurrent requests for the same quote serialize here, so the second
	// idempotency check sees whatever an earlier request committed.
	q, err := quote.Scan(tx.QueryRow(ctx,
		`SELECT `+quote.SelectColumns()+` FROM quotes WHERE id = $1 AND workspace_id = $2 FOR UPDATE`,
		in.QuoteID, workspaceID))
	if err != nil {
		return nil, err
	}
	if res, err := s.checkIdempotency(ctx, tx, workspaceID, idemKey, reqFP, now); res != nil || err != nil {
		return res, err
	}

	// Expired records may be replaced by a new request with the same key.
	if _, err := tx.Exec(ctx, `DELETE FROM idempotency_records WHERE workspace_id = $1 AND idempotency_key = $2`, workspaceID, idemKey); err != nil {
		return nil, err
	}

	// Replays above succeed even after the quote expires; only new payments are refused.
	if q.Expired(now) {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.CodeQuoteExpired,
			"The quote has expired. Request a fresh quote before continuing.")
	}
	var used bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM payment_sessions WHERE quote_id = $1)`, q.ID).Scan(&used); err != nil {
		return nil, err
	}
	if used {
		return nil, apperr.New(http.StatusConflict, apperr.CodeQuoteAlreadyUsed,
			"This quote already backs another payment. Request a new quote.")
	}

	var sandbox bool
	if err := tx.QueryRow(ctx, `SELECT is_sandbox FROM workspaces WHERE id = $1`, workspaceID).Scan(&sandbox); err != nil {
		return nil, fmt.Errorf("read workspace: %w", err)
	}

	p := &Payment{
		ID:              ids.New(ids.Payment),
		WorkspaceID:     workspaceID,
		QuoteID:         q.ID,
		ClientReference: in.ClientReference,
		AgentKeyID:      agentKeyID,
		State:           StateAwaitingAuthorization,
		Fingerprint:     q.Fingerprint,
		IsSimulated:     sandbox,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO payment_sessions (id, workspace_id, quote_id, agent_key_id, client_reference, state,
			fingerprint, is_simulated, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)`,
		p.ID, p.WorkspaceID, p.QuoteID, p.AgentKeyID, p.ClientReference, p.State, p.Fingerprint, p.IsSimulated, now); err != nil {
		return nil, fmt.Errorf("insert payment: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO idempotency_records (workspace_id, idempotency_key, request_fingerprint, payment_id, created_at, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		workspaceID, idemKey, reqFP, p.ID, now, now.Add(idempotencyTTL)); err != nil {
		return nil, err
	}

	after := p.State
	meta, _ := json.Marshal(map[string]any{
		"quote_id":         q.ID,
		"fingerprint":      q.Fingerprint,
		"quote_expires_at": q.ExpiresAt,
		"deposit_address":  q.DepositAddress,
		"idempotency_key":  idemKey,
		"simulated":        sandbox,
	})
	if err := appendEvent(ctx, tx, &Event{
		PaymentID: p.ID, Type: "payment_created", Source: "application",
		StateAfter: &after, ObservedAt: now, Metadata: meta,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &CreateResult{Payment: p}, nil
}

// checkIdempotency returns the original payment for a replay, a conflict error for a reused
// key with a different request, or (nil, nil) when the key is unused.
func (s *Service) checkIdempotency(ctx context.Context, tx pgx.Tx, workspaceID, idemKey, reqFP string, now time.Time) (*CreateResult, error) {
	var storedFP, storedPaymentID string
	err := tx.QueryRow(ctx, `
		SELECT request_fingerprint, payment_id FROM idempotency_records
		WHERE workspace_id = $1 AND idempotency_key = $2 AND expires_at > $3`,
		workspaceID, idemKey, now).Scan(&storedFP, &storedPaymentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read idempotency record: %w", err)
	}
	if storedFP != reqFP {
		return nil, apperr.New(http.StatusConflict, apperr.CodeIdempotencyConflict,
			"This Idempotency-Key was already used with a different request.")
	}
	p, err := s.get(ctx, tx, workspaceID, storedPaymentID)
	if err != nil {
		return nil, err
	}
	return &CreateResult{Payment: p, Replayed: true}, nil
}

// appendEvent assigns the next sequence number and inserts the event. The caller must hold
// the payment row lock (or have just inserted the payment) inside tx.
func appendEvent(ctx context.Context, tx pgx.Tx, e *Event) error {
	e.ID = ids.New(ids.Event)
	if e.Metadata == nil {
		e.Metadata = json.RawMessage(`{}`)
	}
	if e.RecordedAt.IsZero() {
		e.RecordedAt = e.ObservedAt
	}
	return tx.QueryRow(ctx, `
		INSERT INTO payment_events (id, payment_id, sequence, type, source, state_before, state_after,
			observed_at, recorded_at, correlation_id, metadata)
		VALUES ($1, $2, (SELECT COALESCE(MAX(sequence), 0) + 1 FROM payment_events WHERE payment_id = $2),
			$3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING sequence`,
		e.ID, e.PaymentID, e.Type, e.Source, e.StateBefore, e.StateAfter, e.ObservedAt, e.RecordedAt, e.CorrelationID, []byte(e.Metadata),
	).Scan(&e.Sequence)
}

const paymentCols = `id, workspace_id, quote_id, client_reference, agent_key_id, state, fingerprint,
	source_tx_hash, provider_status, last_reconciled_at, is_simulated, created_at, updated_at`

func scanPayment(row pgx.Row) (*Payment, error) {
	var p Payment
	err := row.Scan(&p.ID, &p.WorkspaceID, &p.QuoteID, &p.ClientReference, &p.AgentKeyID, &p.State,
		&p.Fingerprint, &p.SourceTxHash, &p.ProviderStatus, &p.LastReconciledAt, &p.IsSimulated,
		&p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.New(http.StatusNotFound, apperr.CodePaymentNotFound, "Payment not found.")
	}
	if err != nil {
		return nil, fmt.Errorf("scan payment: %w", err)
	}
	return &p, nil
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (s *Service) get(ctx context.Context, q querier, workspaceID, id string) (*Payment, error) {
	return scanPayment(q.QueryRow(ctx, `SELECT `+paymentCols+` FROM payment_sessions WHERE id = $1 AND workspace_id = $2`, id, workspaceID))
}

func (s *Service) Get(ctx context.Context, workspaceID, id string) (*Payment, error) {
	return s.get(ctx, s.DB, workspaceID, id)
}

// List returns the workspace's most recent payments, newest first.
func (s *Service) List(ctx context.Context, workspaceID string, limit int) ([]*Payment, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.DB.Query(ctx, `SELECT `+paymentCols+` FROM payment_sessions WHERE workspace_id = $1 ORDER BY created_at DESC, id LIMIT $2`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Payment{}
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Events returns the payment's append-only timeline in sequence order.
func (s *Service) Events(ctx context.Context, workspaceID, paymentID string) ([]*Event, error) {
	if _, err := s.Get(ctx, workspaceID, paymentID); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `
		SELECT id, payment_id, sequence, type, source, state_before, state_after, observed_at, recorded_at,
			correlation_id, metadata
		FROM payment_events WHERE payment_id = $1 ORDER BY sequence`, paymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Event{}
	for rows.Next() {
		var e Event
		var meta []byte
		if err := rows.Scan(&e.ID, &e.PaymentID, &e.Sequence, &e.Type, &e.Source, &e.StateBefore, &e.StateAfter,
			&e.ObservedAt, &e.RecordedAt, &e.CorrelationID, &meta); err != nil {
			return nil, err
		}
		e.Metadata = meta
		out = append(out, &e)
	}
	return out, rows.Err()
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
