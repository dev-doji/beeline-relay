// Package invoices issues signed vendor invoices ("pay links") for the playground.
//
// The issuer signs the exact invoice JSON with ed25519. The vault verifies that signature
// on-chain (contracts/agent-vault/src/invoice.rs) and pays a non-allowlisted vendor only for
// the signed payee, amount and asset, once, before expiry. The issuer key here is a TESTNET key.
package invoices

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/ids"
)

// MaxAmount caps playground invoices at the testnet vault's per-payment limit (5 test USDC).
const MaxAmount = 5_000_000

var (
	ErrNotFound = errors.New("invoice not found")
	// NEAR account IDs: named (ada.testnet, shop.ada.testnet) or implicit (64 hex).
	nearAccountRe = regexp.MustCompile(`^(([a-z\d]+[\-_])*[a-z\d]+\.)*([a-z\d]+[\-_])*[a-z\d]+$`)
	unitsRe       = regexp.MustCompile(`^[1-9][0-9]{0,11}$`)
)

type Item struct {
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
	UnitPrice string `json:"unit_price"` // base units (6 decimals)
}

// CreateInput is what a vendor submits.
type CreateInput struct {
	VendorName   string `json:"vendor_name"`
	VendorHandle string `json:"vendor_handle"`
	Payout       string `json:"payout"`
	Memo         string `json:"memo"`
	Items        []Item `json:"items"`
}

// Payload is the signed invoice body. Field order is fixed: the signature covers these bytes.
type Payload struct {
	V         int    `json:"v"`
	ID        string `json:"id"`
	Vendor    string `json:"vendor"`
	Payout    string `json:"payout"`
	Asset     string `json:"asset"`
	Amount    string `json:"amount"`
	ExpiresAt int64  `json:"expires_at"`
	Memo      string `json:"memo"`
}

type Invoice struct {
	ID           string     `json:"id"`
	VendorName   string     `json:"vendor_name"`
	VendorHandle string     `json:"vendor_handle"`
	Payout       string     `json:"payout"`
	Asset        string     `json:"asset"`
	Amount       string     `json:"amount"`
	Memo         string     `json:"memo"`
	Items        []Item     `json:"items"`
	Payload      string     `json:"payload"`
	Signature    string     `json:"signature"`
	Status       string     `json:"status"` // open | paid
	PaidTx       *string    `json:"paid_tx"`
	PaidAt       *time.Time `json:"paid_at"`
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    time.Time  `json:"expires_at"`
}

type Service struct {
	DB     *pgxpool.Pool
	Issuer ed25519.PrivateKey
	Asset  string // destination asset the vault pays out in
	TTL    time.Duration
	Now    func() time.Time
}

// IssuerPublicKeyHex is the key the vault owner registers with set_invoice_issuer.
func (s *Service) IssuerPublicKeyHex() string {
	return hex.EncodeToString(s.Issuer.Public().(ed25519.PublicKey))
}

// Validate checks the input and returns the total in base units.
func Validate(in *CreateInput) (string, error) {
	in.VendorName = strings.TrimSpace(in.VendorName)
	in.VendorHandle = strings.TrimSpace(in.VendorHandle)
	in.Memo = strings.TrimSpace(in.Memo)
	in.Payout = strings.TrimSpace(in.Payout)
	if l := len(in.VendorName); l == 0 || l > 60 {
		return "", errors.New("vendor_name must be 1-60 characters")
	}
	if len(in.VendorHandle) > 60 || len(in.Memo) > 200 {
		return "", errors.New("vendor_handle must be at most 60 and memo at most 200 characters")
	}
	in.Payout = strings.ToLower(in.Payout)
	if !nearAccountRe.MatchString(in.Payout) || len(in.Payout) < 2 || len(in.Payout) > 64 {
		return "", errors.New("payout must be a NEAR account, e.g. ada-fabrics.yourname.testnet")
	}
	if len(in.Items) == 0 || len(in.Items) > 10 {
		return "", errors.New("an invoice needs 1-10 items")
	}
	total := new(big.Int)
	for i := range in.Items {
		it := &in.Items[i]
		it.Name = strings.TrimSpace(it.Name)
		if l := len(it.Name); l == 0 || l > 80 {
			return "", fmt.Errorf("item %d: name must be 1-80 characters", i+1)
		}
		if it.Quantity < 1 || it.Quantity > 999 {
			return "", fmt.Errorf("item %d: quantity must be 1-999", i+1)
		}
		if !unitsRe.MatchString(it.UnitPrice) {
			return "", fmt.Errorf("item %d: unit_price must be a positive integer in base units", i+1)
		}
		p, _ := new(big.Int).SetString(it.UnitPrice, 10)
		total.Add(total, p.Mul(p, big.NewInt(int64(it.Quantity))))
	}
	if total.Cmp(big.NewInt(MaxAmount)) > 0 {
		return "", errors.New("playground invoices are capped at 5 test USDC (the testnet vault's per-payment limit)")
	}
	return total.String(), nil
}

// Create validates, signs and stores an anonymous invoice (the public playground).
func (s *Service) Create(ctx context.Context, in CreateInput) (*Invoice, error) {
	return s.CreateFor(ctx, "", in)
}

// CreateFor is Create for a signed-in user: the invoice is recorded as theirs. userID "" means anonymous.
func (s *Service) CreateFor(ctx context.Context, userID string, in CreateInput) (*Invoice, error) {
	amount, err := Validate(&in)
	if err != nil {
		return nil, err
	}
	now := s.Now().UTC()
	expires := now.Add(s.TTL).Truncate(time.Second)
	p := Payload{V: 1, ID: ids.New(ids.Invoice), Vendor: in.VendorName, Payout: in.Payout,
		Asset: s.Asset, Amount: amount, ExpiresAt: expires.Unix(), Memo: in.Memo}
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	sig := hex.EncodeToString(ed25519.Sign(s.Issuer, payload))
	items, _ := json.Marshal(in.Items)
	var owner *string
	if userID != "" {
		owner = &userID
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO invoices (id, vendor_name, vendor_handle, payout, asset, amount, memo, items, payload, signature, created_at, expires_at, user_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		p.ID, p.Vendor, in.VendorHandle, p.Payout, p.Asset, p.Amount, p.Memo, items, string(payload), sig, now, expires, owner)
	if err != nil {
		return nil, fmt.Errorf("store invoice: %w", err)
	}
	return s.Get(ctx, p.ID)
}

func (s *Service) Get(ctx context.Context, id string) (*Invoice, error) {
	var inv Invoice
	var items []byte
	err := s.DB.QueryRow(ctx, `SELECT id, vendor_name, vendor_handle, payout, asset, amount, memo, items, payload, signature,
		status, paid_tx, paid_at, created_at, expires_at FROM invoices WHERE id = $1`, id).Scan(
		&inv.ID, &inv.VendorName, &inv.VendorHandle, &inv.Payout, &inv.Asset, &inv.Amount, &inv.Memo, &items,
		&inv.Payload, &inv.Signature, &inv.Status, &inv.PaidTx, &inv.PaidAt, &inv.CreatedAt, &inv.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(items, &inv.Items)
	return &inv, nil
}

// ListByUser returns a user's invoices, newest first.
func (s *Service) ListByUser(ctx context.Context, userID string, limit int) ([]Invoice, error) {
	rows, err := s.DB.Query(ctx, `SELECT id FROM invoices WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Invoice, 0, len(ids))
	for _, id := range ids {
		inv, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *inv)
	}
	return out, nil
}

// Owner reports which user an invoice belongs to ("" for anonymous playground invoices).
func (s *Service) Owner(ctx context.Context, id string) (string, error) {
	var owner *string
	err := s.DB.QueryRow(ctx, `SELECT user_id FROM invoices WHERE id = $1`, id).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil || owner == nil {
		return "", err
	}
	return *owner, nil
}

// MarkPaid records the testnet transaction that paid the invoice. It only moves open -> paid.
func (s *Service) MarkPaid(ctx context.Context, id, tx string) error {
	_, err := s.DB.Exec(ctx, `UPDATE invoices SET status = 'paid', paid_tx = $2, paid_at = $3 WHERE id = $1 AND status = 'open'`,
		id, tx, s.Now().UTC())
	return err
}
