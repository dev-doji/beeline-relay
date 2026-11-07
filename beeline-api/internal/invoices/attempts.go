package invoices

import (
	"context"
	"fmt"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/ids"
)

// Attempt is one try at paying a pay link: an honest payment or a visitor's attack, with the
// vault's on-chain decision. Attempts are public in the attack log.
type Attempt struct {
	ID           string    `json:"id"`
	InvoiceID    string    `json:"invoice_id"`
	VendorName   string    `json:"vendor_name,omitempty"`
	Kind         string    `json:"kind"` // pay | attack
	Message      string    `json:"message"`
	EditedPayout string    `json:"edited_payout"`
	EditedAmount string    `json:"edited_amount"` // base units
	Outcome      string    `json:"outcome"`       // paid | blocked | reply | error
	Layer        string    `json:"layer"`
	Reason       string    `json:"reason"`
	Recipient    string    `json:"recipient"` // whom the agent tried to pay
	Amount       string    `json:"amount"`    // base units the agent tried to pay
	TxHash       string    `json:"tx_hash"`
	CreatedAt    time.Time `json:"created_at"`
}

// Stats summarises every attempt. Misdirected counts payments the vault accepted that went to
// anyone other than the invoice's payee or for any other amount: the number Beeline exists to
// keep at zero. It is computed from the records, not asserted.
type Stats struct {
	Attempts    int `json:"attempts"`
	Attacks     int `json:"attacks"`
	Blocked     int `json:"blocked"`
	Paid        int `json:"paid"`
	Misdirected int `json:"misdirected"`
	Invoices    int `json:"invoices"`
}

func (s *Service) RecordAttempt(ctx context.Context, a Attempt) (*Attempt, error) {
	a.ID = ids.New(ids.Attempt)
	a.CreatedAt = s.Now().UTC()
	_, err := s.DB.Exec(ctx, `INSERT INTO invoice_attempts (id, invoice_id, kind, message, edited_payout, edited_amount, outcome, layer, reason, recipient, amount, tx_hash, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		a.ID, a.InvoiceID, a.Kind, a.Message, a.EditedPayout, a.EditedAmount, a.Outcome, a.Layer, a.Reason, a.Recipient, a.Amount, a.TxHash, a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("record attempt: %w", err)
	}
	return &a, nil
}

// Attempts lists the newest attempts, for one invoice or (invoiceID "") for all.
func (s *Service) Attempts(ctx context.Context, invoiceID string, limit int) ([]Attempt, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.DB.Query(ctx, `SELECT a.id, a.invoice_id, i.vendor_name, a.kind, a.message, a.edited_payout, a.edited_amount, a.outcome,
		a.layer, a.reason, a.recipient, a.amount, a.tx_hash, a.created_at
		FROM invoice_attempts a JOIN invoices i ON i.id = a.invoice_id
		WHERE ($1 = '' OR a.invoice_id = $1) ORDER BY a.created_at DESC LIMIT $2`, invoiceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attempt{}
	for rows.Next() {
		var a Attempt
		if err := rows.Scan(&a.ID, &a.InvoiceID, &a.VendorName, &a.Kind, &a.Message, &a.EditedPayout, &a.EditedAmount, &a.Outcome,
			&a.Layer, &a.Reason, &a.Recipient, &a.Amount, &a.TxHash, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Service) Stats(ctx context.Context) (*Stats, error) {
	var st Stats
	err := s.DB.QueryRow(ctx, `SELECT
		count(*),
		count(*) FILTER (WHERE a.kind = 'attack'),
		count(*) FILTER (WHERE a.outcome = 'blocked'),
		count(*) FILTER (WHERE a.outcome = 'paid'),
		count(*) FILTER (WHERE a.outcome = 'paid' AND (a.recipient <> i.payout OR a.amount <> i.amount)),
		(SELECT count(*) FROM invoices)
		FROM invoice_attempts a JOIN invoices i ON i.id = a.invoice_id`).Scan(
		&st.Attempts, &st.Attacks, &st.Blocked, &st.Paid, &st.Misdirected, &st.Invoices)
	if err != nil {
		return nil, err
	}
	return &st, nil
}
