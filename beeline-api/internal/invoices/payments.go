package invoices

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Payment is a payment that was read from the chain and matched the invoice.
type Payment struct {
	TxHash    string    `json:"tx_hash"`
	Sender    string    `json:"sender"`
	Receiver  string    `json:"receiver"`
	Token     string    `json:"token"`
	Amount    string    `json:"amount"`
	Memo      string    `json:"memo"`
	BlockHash string    `json:"block_hash"`
	BlockTime time.Time `json:"block_time"`
	// Verification says how it was checked: "chain-verified" (a direct NEAR transfer read from the chain) or
	// "intents-delivered" (NEAR Intents reported delivery and the vendor's own USDC balance rose by the amount).
	Verification string    `json:"verification"`
	VerifiedAt   time.Time `json:"verified_at"`
}

// Event is something around an invoice that did not match (a refused claim).
type Event struct {
	Kind      string    `json:"kind"`
	Reason    string    `json:"reason"`
	TxHash    string    `json:"tx_hash"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	// ErrAlreadyPaid: the invoice is no longer open.
	ErrAlreadyPaid = errors.New("invoice already paid")
	// ErrTxReused: the transaction already paid an invoice.
	ErrTxReused = errors.New("transaction already used")
)

// RecordPayment stores a verified payment and marks the invoice paid, in one database transaction.
// It only moves open -> paid, and a transaction hash can be used once.
func (s *Service) RecordPayment(ctx context.Context, invoiceID string, p Payment) (*Payment, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op
	now := s.Now().UTC()
	ct, err := tx.Exec(ctx, `UPDATE invoices SET status = 'paid', paid_tx = $2, paid_at = $3 WHERE id = $1 AND status = 'open'`, invoiceID, p.TxHash, now)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() != 1 {
		return nil, ErrAlreadyPaid
	}
	if p.Verification == "" {
		p.Verification = "chain-verified"
	}
	_, err = tx.Exec(ctx, `INSERT INTO invoice_payments (tx_hash, invoice_id, sender, receiver, token, amount, memo, block_hash, block_time, verification, verified_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, p.TxHash, invoiceID, p.Sender, p.Receiver, p.Token, p.Amount, p.Memo, p.BlockHash, p.BlockTime, p.Verification, now)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" { // unique violation: this hash or this invoice already has a payment
			return nil, ErrTxReused
		}
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	p.VerifiedAt = now
	return &p, nil
}

// PaymentFor returns the verified payment for an invoice, or nil.
func (s *Service) PaymentFor(ctx context.Context, invoiceID string) (*Payment, error) {
	var p Payment
	err := s.DB.QueryRow(ctx, `SELECT tx_hash, sender, receiver, token, amount, memo, block_hash, block_time, verification, verified_at
		FROM invoice_payments WHERE invoice_id = $1`, invoiceID).Scan(&p.TxHash, &p.Sender, &p.Receiver, &p.Token, &p.Amount, &p.Memo, &p.BlockHash, &p.BlockTime, &p.Verification, &p.VerifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}

// AddEvent records a refused claim against an invoice.
func (s *Service) AddEvent(ctx context.Context, invoiceID, kind, reason, txHash string) error {
	if len(reason) > 300 {
		reason = reason[:300]
	}
	_, err := s.DB.Exec(ctx, `INSERT INTO invoice_events (invoice_id, kind, reason, tx_hash) VALUES ($1,$2,$3,$4)`, invoiceID, kind, reason, txHash)
	return err
}

// Events lists an invoice's refused claims, newest first.
func (s *Service) Events(ctx context.Context, invoiceID string, limit int) ([]Event, error) {
	rows, err := s.DB.Query(ctx, `SELECT kind, reason, tx_hash, created_at FROM invoice_events WHERE invoice_id = $1 ORDER BY id DESC LIMIT $2`, invoiceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Kind, &e.Reason, &e.TxHash, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
