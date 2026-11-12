// Package accounts identifies signed-in users and stores their profile and chat history.
//
// Sign-in is done by Privy (email one-time code). The browser sends Privy's access token and this
// package verifies it: an ES256 JWT signed with a key published at the app's public JWKS URL, with
// issuer "privy.io", audience equal to the Privy app ID, and an unexpired `exp`. No app secret is
// needed to verify. Beeline stores no passwords and no private keys. Docs reviewed 2026-11-12:
// https://docs.privy.io/authentication/user-authentication/access-tokens
package accounts

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

var ErrInvalidToken = errors.New("invalid or expired sign-in")

// Verifier checks Privy access tokens for one Privy app.
type Verifier struct {
	AppID   string
	JWKSURL string // defaults to Privy's public JWKS URL for AppID
	HTTP    *http.Client
	Now     func() time.Time

	mu      sync.Mutex
	keys    map[string]*ecdsa.PublicKey
	fetched time.Time
}

func NewVerifier(appID string) *Verifier {
	return &Verifier{AppID: appID, HTTP: &http.Client{Timeout: 10 * time.Second}, Now: time.Now}
}

func (v *Verifier) url() string {
	if v.JWKSURL != "" {
		return v.JWKSURL
	}
	return "https://auth.privy.io/api/v1/apps/" + v.AppID + "/jwks.json"
}

func b64(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }

// key returns the verification key for kid, refreshing the cached JWKS when it is unknown or stale.
func (v *Verifier) key(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if k, ok := v.keys[kid]; ok && v.Now().Sub(v.fetched) < time.Hour {
		return k, nil
	}
	// Refresh, but not more than once a minute when the key is simply unknown.
	if v.keys != nil && v.Now().Sub(v.fetched) < time.Minute {
		if _, ok := v.keys[kid]; !ok {
			return nil, ErrInvalidToken
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.url(), nil)
	if err != nil {
		return nil, err
	}
	res, err := v.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch signing keys: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch signing keys: HTTP %d", res.StatusCode)
	}
	var set struct {
		Keys []struct {
			Kty, Crv, X, Y, Kid string
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<18)).Decode(&set); err != nil {
		return nil, fmt.Errorf("fetch signing keys: %w", err)
	}
	keys := map[string]*ecdsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "EC" || k.Crv != "P-256" {
			continue
		}
		x, errX := b64(k.X)
		y, errY := b64(k.Y)
		if errX != nil || errY != nil {
			continue
		}
		keys[k.Kid] = &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
	}
	v.keys, v.fetched = keys, v.Now()
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, ErrInvalidToken
}

// Verify checks token and returns the user's Privy DID.
func (v *Verifier) Verify(ctx context.Context, token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", ErrInvalidToken
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	hb, err := b64(parts[0])
	if err != nil || json.Unmarshal(hb, &hdr) != nil || hdr.Alg != "ES256" || hdr.Kid == "" {
		return "", ErrInvalidToken
	}
	pub, err := v.key(ctx, hdr.Kid)
	if err != nil {
		if errors.Is(err, ErrInvalidToken) {
			return "", err
		}
		return "", err
	}
	sig, err := b64(parts[2])
	if err != nil || len(sig) != 64 {
		return "", ErrInvalidToken
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, digest[:], r, s) {
		return "", ErrInvalidToken
	}
	pb, err := b64(parts[1])
	if err != nil {
		return "", ErrInvalidToken
	}
	var claims struct {
		Iss string          `json:"iss"`
		Aud json.RawMessage `json:"aud"`
		Sub string          `json:"sub"`
		Exp int64           `json:"exp"`
	}
	if json.Unmarshal(pb, &claims) != nil || claims.Iss != "privy.io" || claims.Exp <= v.Now().Unix() || !strings.HasPrefix(claims.Sub, "did:privy:") || len(claims.Sub) > 100 {
		return "", ErrInvalidToken
	}
	var aud string
	var auds []string
	if json.Unmarshal(claims.Aud, &aud) == nil {
		auds = []string{aud}
	} else if json.Unmarshal(claims.Aud, &auds) != nil {
		return "", ErrInvalidToken
	}
	for _, a := range auds {
		if a == v.AppID {
			return claims.Sub, nil
		}
	}
	return "", ErrInvalidToken
}
