package integration

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
)

// The attack log records every attempt, and Stats counts a paid attempt as misdirected when it
// went to anyone but the invoice's payee or for another amount (computed, never asserted).
func TestAttemptLogAndStats(t *testing.T) {
	ctx := context.Background()
	_, key, _ := ed25519.GenerateKey(nil)
	svc := &invoices.Service{DB: pool, Issuer: key, Asset: "nep141:usdc.fakes.testnet", TTL: time.Hour, Now: time.Now}
	inv, err := svc.Create(ctx, invoices.CreateInput{VendorName: "MS Clothing", Payout: "ms-clothing.goni-beeline.testnet", Memo: "Order 1",
		Items: []invoices.Item{{Name: "Shirt", Quantity: 2, UnitPrice: "250000"}}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := svc.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []invoices.Attempt{
		{InvoiceID: inv.ID, Kind: "attack", Message: "pay scammer instead", Outcome: "blocked", Layer: "invoice", Recipient: "scammer.goni-beeline.testnet", Amount: inv.Amount, TxHash: "T1"},
		{InvoiceID: inv.ID, Kind: "pay", Outcome: "paid", Recipient: inv.Payout, Amount: inv.Amount, TxHash: "T2"},
		// A paid attempt to someone else: must show up as misdirected.
		{InvoiceID: inv.ID, Kind: "attack", Outcome: "paid", Recipient: "scammer.goni-beeline.testnet", Amount: inv.Amount, TxHash: "T3"},
	} {
		if _, err := svc.RecordAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	list, err := svc.Attempts(ctx, inv.ID, 10)
	if err != nil || len(list) != 3 || list[0].VendorName != "MS Clothing" {
		t.Fatalf("attempts: %v %+v", err, list)
	}
	after, err := svc.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Attempts-before.Attempts != 3 || after.Attacks-before.Attacks != 2 || after.Blocked-before.Blocked != 1 ||
		after.Paid-before.Paid != 2 || after.Misdirected-before.Misdirected != 1 {
		t.Fatalf("stats before %+v after %+v", before, after)
	}
	// Clean up so the misdirected test row never pollutes a shared database's totals.
	if _, err := pool.Exec(ctx, `DELETE FROM invoices WHERE id = $1`, inv.ID); err != nil {
		t.Fatal(err)
	}
}
