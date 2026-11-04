// Package auth authenticates API keys and derives the caller's workspace. Workspace identity
// always comes from the credential, never from request bodies.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/ids"
)

type Kind string

const (
	KindAgent   Kind = "agent"
	KindConsole Kind = "console"

	keyPrefix = "blr_"
)

// Principal is the authenticated caller.
type Principal struct {
	WorkspaceID string
	KeyID       string
	Kind        Kind
}

type ctxKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

var ErrInvalidKey = errors.New("invalid or revoked API key")

func hash(key string) []byte {
	sum := sha256.Sum256([]byte(key))
	return sum[:]
}

// Store looks up and creates API keys. Only SHA-256 hashes are persisted.
type Store struct{ DB *pgxpool.Pool }

func (s *Store) Authenticate(ctx context.Context, key string) (Principal, error) {
	if !strings.HasPrefix(key, keyPrefix) || len(key) > 128 {
		return Principal{}, ErrInvalidKey
	}
	var p Principal
	err := s.DB.QueryRow(ctx,
		`SELECT id, workspace_id, kind FROM api_keys WHERE key_hash = $1 AND revoked_at IS NULL`,
		hash(key)).Scan(&p.KeyID, &p.WorkspaceID, &p.Kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrInvalidKey
	}
	return p, err
}

// CreateWorkspace creates a workspace and returns its ID.
func (s *Store) CreateWorkspace(ctx context.Context, name string, sandbox bool) (string, error) {
	id := ids.New(ids.Workspace)
	_, err := s.DB.Exec(ctx, `INSERT INTO workspaces (id, name, is_sandbox) VALUES ($1, $2, $3)`, id, name, sandbox)
	return id, err
}

// CreateKey issues a new API key. The plaintext key is returned once and never stored.
func (s *Store) CreateKey(ctx context.Context, workspaceID string, kind Kind, label string) (keyID, plaintext string, err error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", err
	}
	plaintext = keyPrefix + base64.RawURLEncoding.EncodeToString(b[:])
	keyID = ids.New(ids.APIKey)
	_, err = s.DB.Exec(ctx,
		`INSERT INTO api_keys (id, workspace_id, kind, label, key_hash) VALUES ($1, $2, $3, $4, $5)`,
		keyID, workspaceID, kind, label, hash(plaintext))
	return keyID, plaintext, err
}
