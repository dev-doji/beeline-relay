package invoices

import (
	"context"
	"time"
)

// LedgerEntry is one confirmed payment, as the vendor's books show it. It is read from invoice_payments, which is
// written once per payment by the server after it verified the payment (never from anything a customer sent).
type LedgerEntry struct {
	InvoiceID    string    `json:"invoice_id"`
	Network      string    `json:"network"`
	Amount       string    `json:"amount"` // USDC base units
	Payer        string    `json:"payer"`
	Verification string    `json:"verification"`
	TxHash       string    `json:"tx_hash"`
	PaidAt       time.Time `json:"paid_at"`
}

// Summary is the vendor's totals. Test money and real money are never added together.
type Summary struct {
	CollectedReal     string `json:"collected_real"` // USDC base units
	CollectedTest     string `json:"collected_test"`
	PaidCount         int    `json:"paid_count"`
	OpenCount         int    `json:"open_count"`
	RefusedClaims     int    `json:"refused_claims"` // payment attempts the server refused
	GuardFlagged      int    `json:"guard_flagged"`  // Guard verdicts that are not "clear"
	GuardFraudVerdict int    `json:"guard_fraud"`    // of those, rated "fraud"
	GuardReviews      int    `json:"guard_reviews"`  // all Guard verdicts
}

// Ledger returns a vendor's recent confirmed payments, newest first, and their totals.
func (s *Service) Ledger(ctx context.Context, userID string, limit int) ([]LedgerEntry, *Summary, error) {
	rows, err := s.DB.Query(ctx, `SELECT i.id, i.asset, p.amount, p.sender, p.verification, p.tx_hash, p.verified_at
		FROM invoices i JOIN invoice_payments p ON p.invoice_id = i.id
		WHERE i.user_id = $1 ORDER BY p.verified_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	entries := []LedgerEntry{}
	for rows.Next() {
		var e LedgerEntry
		var asset string
		if err := rows.Scan(&e.InvoiceID, &asset, &e.Amount, &e.Payer, &e.Verification, &e.TxHash, &e.PaidAt); err != nil {
			return nil, nil, err
		}
		e.Network = NetworkOf("nep141:" + trimNEP141(asset))
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	sum := &Summary{}
	err = s.DB.QueryRow(ctx, `SELECT
		COALESCE(SUM(p.amount::numeric) FILTER (WHERE i.asset = $2), 0)::text,
		COALESCE(SUM(p.amount::numeric) FILTER (WHERE i.asset <> $2), 0)::text,
		COUNT(p.tx_hash)::int
		FROM invoices i JOIN invoice_payments p ON p.invoice_id = i.id WHERE i.user_id = $1`, userID, MainnetUSDC).
		Scan(&sum.CollectedReal, &sum.CollectedTest, &sum.PaidCount)
	if err != nil {
		return nil, nil, err
	}
	if err := s.DB.QueryRow(ctx, `SELECT COUNT(*)::int FROM invoices WHERE user_id = $1 AND status = 'open' AND expires_at > now()`, userID).Scan(&sum.OpenCount); err != nil {
		return nil, nil, err
	}
	if err := s.DB.QueryRow(ctx, `SELECT COUNT(*)::int FROM invoice_events e JOIN invoices i ON i.id = e.invoice_id WHERE i.user_id = $1`, userID).Scan(&sum.RefusedClaims); err != nil {
		return nil, nil, err
	}
	if err := s.DB.QueryRow(ctx, `SELECT COUNT(*)::int, COUNT(*) FILTER (WHERE g.verdict <> 'clear')::int, COUNT(*) FILTER (WHERE g.verdict = 'fraud')::int
		FROM guard_reviews g JOIN invoices i ON i.id = g.invoice_id WHERE i.user_id = $1`, userID).
		Scan(&sum.GuardReviews, &sum.GuardFlagged, &sum.GuardFraudVerdict); err != nil {
		return nil, nil, err
	}
	return entries, sum, nil
}

func trimNEP141(a string) string {
	if len(a) > 7 && a[:7] == "nep141:" {
		return a[7:]
	}
	return a
}
