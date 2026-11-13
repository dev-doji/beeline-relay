package integration

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
)

func TestLedgerTotalsAreSeparateForTestAndRealMoneyAndOnlyCountPaymentsTheServerRecorded(t *testing.T) {
	ctx := context.Background()
	uid := "did:privy:ledger_" + strings.ToLower(strconv.FormatInt(time.Now().UnixNano(), 36))
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, shop_name, near_account) VALUES ($1, 'Ledger Shop', 'ledger.testnet')`, uid); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM guard_reviews WHERE invoice_id IN (SELECT id FROM invoices WHERE user_id = $1)`, uid)
		_, _ = pool.Exec(ctx, `DELETE FROM invoice_events WHERE invoice_id IN (SELECT id FROM invoices WHERE user_id = $1)`, uid)
		_, _ = pool.Exec(ctx, `DELETE FROM invoice_payments WHERE invoice_id IN (SELECT id FROM invoices WHERE user_id = $1)`, uid)
		_, _ = pool.Exec(ctx, `DELETE FROM invoices WHERE user_id = $1`, uid)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, uid)
	})
	_, issuer, _ := ed25519.GenerateKey(rand.Reader)
	svc := &invoices.Service{DB: pool, Issuer: issuer, Asset: "nep141:usdc.fakes.testnet", TTL: time.Hour, Now: time.Now, MainnetEnabled: true}
	item := func(units string) []invoices.Item {
		return []invoices.Item{{Name: "Thing", Quantity: 1, UnitPrice: units}}
	}
	mk := func(real bool, payout, units string) *invoices.Invoice {
		inv, err := svc.CreateFor(ctx, uid, invoices.CreateInput{VendorName: "Ledger Shop", Payout: payout, Items: item(units), Real: real})
		if err != nil {
			t.Fatal(err)
		}
		return inv
	}
	testPaid := mk(false, "ledger.testnet", "2000000")
	realPaid := mk(true, "ledger.near", "3000000")
	open := mk(false, "ledger.testnet", "1000000")
	_ = open
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	if _, err := svc.RecordPayment(ctx, testPaid.ID, invoices.Payment{TxHash: "LedgerTestTx" + suffix, Sender: "buyer.testnet", Receiver: "ledger.testnet", Token: "usdc.fakes.testnet", Amount: "2000000", Memo: "m", BlockHash: "b", BlockTime: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordPayment(ctx, realPaid.ID, invoices.Payment{TxHash: "LedgerRealTx" + suffix, Sender: "cross-chain:base", Receiver: "ledger.near", Token: "x", Amount: "3000000", Memo: "m", BlockHash: "intents", BlockTime: time.Now(), Verification: "intents-delivered"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddEvent(ctx, open.ID, "WRONG_AMOUNT", "short", "tx1"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO guard_reviews (invoice_id, subject, ref, verdict, reason, model) VALUES ($1,'claim_refused','tx1','fraud','looks deliberate','m'), ($2,'payment_confirmed','LedgerTestTx','clear','fine','m')`, open.ID, testPaid.ID); err != nil {
		t.Fatal(err)
	}

	entries, sum, err := svc.Ledger(ctx, uid, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || sum.PaidCount != 2 {
		t.Fatalf("two recorded payments expected: %d entries, %d paid", len(entries), sum.PaidCount)
	}
	if sum.CollectedTest != "2000000" || sum.CollectedReal != "3000000" {
		t.Fatalf("test and real money must stay separate: test=%s real=%s", sum.CollectedTest, sum.CollectedReal)
	}
	if sum.OpenCount != 1 || sum.RefusedClaims != 1 || sum.GuardReviews != 2 || sum.GuardFlagged != 1 || sum.GuardFraudVerdict != 1 {
		t.Fatalf("summary: %+v", sum)
	}
	gotReal := false
	for _, e := range entries {
		if e.InvoiceID == realPaid.ID && e.Network == "mainnet" && e.Verification == "intents-delivered" {
			gotReal = true
		}
	}
	if !gotReal {
		t.Fatalf("the real-money entry must be labelled mainnet and intents-delivered: %+v", entries)
	}
	// Another user's books are empty.
	other, osum, err := svc.Ledger(ctx, "did:privy:nobody", 50)
	if err != nil || len(other) != 0 || osum.PaidCount != 0 {
		t.Fatalf("another vendor must see none of this: %v %+v %v", other, osum, err)
	}
}
