package accounts

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testApp = "app_test_123"

// TestKey is a throwaway signing key and a JWKS server standing in for Privy. TEST DOUBLE.
type testIssuer struct {
	key *ecdsa.PrivateKey
	srv *httptest.Server
}

func newIssuer(t *testing.T) *testIssuer {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	enc := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	pad := func(b []byte) []byte { out := make([]byte, 32); copy(out[32-len(b):], b); return out }
	jwks := map[string]any{"keys": []map[string]string{{
		"kty": "EC", "crv": "P-256", "alg": "ES256", "use": "sig", "kid": "k1",
		"x": enc(pad(key.X.Bytes())), "y": enc(pad(key.Y.Bytes())),
	}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(jwks) }))
	t.Cleanup(srv.Close)
	return &testIssuer{key: key, srv: srv}
}

func (i *testIssuer) token(t *testing.T, alg, kid string, claims map[string]any) string {
	t.Helper()
	enc := func(v any) string { b, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(b) }
	signing := enc(map[string]string{"alg": alg, "kid": kid, "typ": "JWT"}) + "." + enc(claims)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, i.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	rb, sb := r.Bytes(), s.Bytes()
	copy(sig[32-len(rb):32], rb)
	copy(sig[64-len(sb):], sb)
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (i *testIssuer) verifier() *Verifier {
	v := NewVerifier(testApp)
	v.JWKSURL = i.srv.URL
	return v
}

func good() map[string]any {
	return map[string]any{"iss": "privy.io", "aud": testApp, "sub": "did:privy:abc123", "exp": time.Now().Add(time.Hour).Unix()}
}

func TestVerifyAcceptsAValidToken(t *testing.T) {
	iss := newIssuer(t)
	id, err := iss.verifier().Verify(context.Background(), iss.token(t, "ES256", "k1", good()))
	if err != nil || id != "did:privy:abc123" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}

func TestVerifyRejectsAnythingElse(t *testing.T) {
	iss := newIssuer(t)
	other := newIssuer(t) // a different key: signature must not verify
	cases := map[string]string{
		"wrong audience": iss.token(t, "ES256", "k1", with(good(), "aud", "someone-elses-app")),
		"wrong issuer":   iss.token(t, "ES256", "k1", with(good(), "iss", "evil.example")),
		"expired":        iss.token(t, "ES256", "k1", with(good(), "exp", time.Now().Add(-time.Minute).Unix())),
		"not a privy id": iss.token(t, "ES256", "k1", with(good(), "sub", "admin")),
		"unknown key id": iss.token(t, "ES256", "nope", good()),
		"alg none":       iss.token(t, "none", "k1", good()),
		"alg HS256":      iss.token(t, "HS256", "k1", good()),
		"foreign key":    other.token(t, "ES256", "k1", good()),
		"garbage":        "not.a.jwt",
		"empty":          "",
	}
	for name, tok := range cases {
		if id, err := iss.verifier().Verify(context.Background(), tok); !errors.Is(err, ErrInvalidToken) || id != "" {
			t.Errorf("%s: want ErrInvalidToken, got id=%q err=%v", name, id, err)
		}
	}
}

func TestVerifyTamperedPayloadFails(t *testing.T) {
	iss := newIssuer(t)
	tok := iss.token(t, "ES256", "k1", good())
	parts := splitDots(tok)
	forged, _ := json.Marshal(with(good(), "sub", "did:privy:someone-else"))
	bad := parts[0] + "." + base64.RawURLEncoding.EncodeToString(forged) + "." + parts[2]
	if _, err := iss.verifier().Verify(context.Background(), bad); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("a payload edited after signing must fail: %v", err)
	}
}

func with(m map[string]any, k string, v any) map[string]any {
	out := map[string]any{}
	for kk, vv := range m {
		out[kk] = vv
	}
	out[k] = v
	return out
}

func splitDots(s string) []string {
	var out []string
	cur := ""
	for _, c := range s {
		if c == '.' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(c)
	}
	return append(out, cur)
}
