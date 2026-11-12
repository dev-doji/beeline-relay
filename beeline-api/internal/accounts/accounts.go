package accounts

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// nearAccountRe matches NEAR account IDs: named (shop.near, ada.testnet) or implicit (64 hex).
var nearAccountRe = regexp.MustCompile(`^(([a-z\d]+[\-_])*[a-z\d]+\.)*([a-z\d]+[\-_])*[a-z\d]+$`)

type User struct {
	ID          string    `json:"id"`
	ShopName    string    `json:"shop_name"`
	NearAccount string    `json:"near_account"`
	CreatedAt   time.Time `json:"created_at"`
}

type Message struct {
	ID        int64     `json:"id"`
	Role      string    `json:"role"`
	Body      string    `json:"body"`
	InvoiceID *string   `json:"invoice_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	DB  *pgxpool.Pool
	Now func() time.Time
}

// ValidateProfile cleans and checks profile fields. The payout account is the user's own choice:
// Beeline cannot tell whether they control it, so it is shown back to them before every invoice.
func ValidateProfile(shop, near string) (string, string, error) {
	shop = strings.TrimSpace(shop)
	near = strings.ToLower(strings.TrimSpace(near))
	if l := len(shop); l == 0 || l > 60 {
		return "", "", errors.New("shop_name must be 1-60 characters")
	}
	if l := len(near); l < 2 || l > 64 || !nearAccountRe.MatchString(near) {
		return "", "", errors.New("near_account must be a NEAR account, e.g. yourshop.testnet")
	}
	return shop, near, nil
}

// Ensure creates the user on first sign-in and returns it.
func (s *Store) Ensure(ctx context.Context, id string) (*User, error) {
	if _, err := s.DB.Exec(ctx, `INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`, id); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *Store) Get(ctx context.Context, id string) (*User, error) {
	var u User
	err := s.DB.QueryRow(ctx, `SELECT id, shop_name, near_account, created_at FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.ShopName, &u.NearAccount, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errors.New("user not found")
	}
	return &u, err
}

func (s *Store) UpdateProfile(ctx context.Context, id, shop, near string) (*User, error) {
	shop, near, err := ValidateProfile(shop, near)
	if err != nil {
		return nil, err
	}
	if _, err := s.DB.Exec(ctx, `UPDATE users SET shop_name = $2, near_account = $3, updated_at = now() WHERE id = $1`, id, shop, near); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *Store) AddMessage(ctx context.Context, userID, role, body string, invoiceID *string) error {
	if len(body) > 2000 {
		body = body[:2000]
	}
	_, err := s.DB.Exec(ctx, `INSERT INTO chat_messages (user_id, role, body, invoice_id) VALUES ($1,$2,$3,$4)`, userID, role, body, invoiceID)
	return err
}

// Messages returns the user's most recent messages, oldest first.
func (s *Store) Messages(ctx context.Context, userID string, limit int) ([]Message, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, role, body, invoice_id, created_at FROM (
		SELECT id, role, body, invoice_id, created_at FROM chat_messages WHERE user_id = $1 ORDER BY id DESC LIMIT $2) t ORDER BY id`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Role, &m.Body, &m.InvoiceID, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
