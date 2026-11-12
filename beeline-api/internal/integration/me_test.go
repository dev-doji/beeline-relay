package integration

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/accounts"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/httpapi"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
)

const meApp = "app_integration_test"

// meEnv runs the signed-in app against the real database. Privy is a TEST DOUBLE: a throwaway
// ES256 key and a JWKS server stand in for Privy's sign-in service.
type meEnv struct {
	t   *testing.T
	srv *httptest.Server
	key *ecdsa.PrivateKey
}

func newMeEnv(t *testing.T) *meEnv {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	enc := base64.RawURLEncoding.EncodeToString
	pad := func(b []byte) []byte { o := make([]byte, 32); copy(o[32-len(b):], b); return o }
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "EC", "crv": "P-256", "kid": "k1", "x": enc(pad(key.X.Bytes())), "y": enc(pad(key.Y.Bytes()))}}})
	}))
	t.Cleanup(jwks.Close)
	v := accounts.NewVerifier(meApp)
	v.JWKSURL = jwks.URL
	_, issuer, _ := ed25519.GenerateKey(rand.Reader)
	router := httpapi.NewRouter(httpapi.Deps{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:       pool,
		Accounts: &accounts.Store{DB: pool, Now: time.Now},
		Privy:    v,
		Invoices: &invoices.Service{DB: pool, Issuer: issuer, Asset: "nep141:usdc.fakes.testnet", TTL: time.Hour, Now: time.Now},
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return &meEnv{t: t, srv: srv, key: key}
}

func (e *meEnv) token(sub string) string {
	enc := func(v any) string { b, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(b) }
	signing := enc(map[string]string{"alg": "ES256", "kid": "k1"}) + "." + enc(map[string]any{"iss": "privy.io", "aud": meApp, "sub": sub, "exp": time.Now().Add(time.Hour).Unix()})
	d := sha256.Sum256([]byte(signing))
	r, s, _ := ecdsa.Sign(rand.Reader, e.key, d[:])
	sig := make([]byte, 64)
	rb, sb := r.Bytes(), s.Bytes()
	copy(sig[32-len(rb):32], rb)
	copy(sig[64-len(sb):], sb)
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (e *meEnv) call(method, path, token string, body any) (int, map[string]any) {
	e.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestSignedInUserMakesInvoicesInChatAndOnlySeesTheirOwn(t *testing.T) {
	e := newMeEnv(t)
	// Unique users per run: the database persists between runs. Their rows are removed afterwards.
	suffix := strings.ToLower(strconv.FormatInt(time.Now().UnixNano(), 36))
	aliceID, bobID := "did:privy:alice_"+suffix, "did:privy:bob_"+suffix
	t.Cleanup(func() {
		ctx := context.Background()
		for _, id := range []string{aliceID, bobID} {
			_, _ = pool.Exec(ctx, `DELETE FROM chat_messages WHERE user_id = $1`, id)
			_, _ = pool.Exec(ctx, `DELETE FROM invoices WHERE user_id = $1`, id)
			_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
		}
	})
	alice, bob := e.token(aliceID), e.token(bobID)

	// No token, a garbage token and a forged token are all refused.
	for _, tok := range []string{"", "garbage", alice + "x"} {
		if code, _ := e.call(http.MethodGet, "/api/v1/me", tok, nil); code != http.StatusUnauthorized {
			t.Fatalf("token %q: got %d, want 401", tok, code)
		}
	}

	// Before setup, the chat asks for a profile and makes no invoice.
	code, out := e.call(http.MethodPost, "/api/v1/me/chat", alice, map[string]string{"message": "invoice 1 for hats"})
	if code != 200 || out["needs_profile"] != true || out["invoice"] != nil {
		t.Fatalf("before setup: %d %v", code, out)
	}

	// Bad profile is rejected; a good one is saved.
	if code, _ := e.call(http.MethodPut, "/api/v1/me", alice, map[string]string{"shop_name": "Ada", "near_account": "NOT VALID!"}); code == 200 {
		t.Fatal("an invalid payout account must be rejected")
	}
	if code, _ := e.call(http.MethodPut, "/api/v1/me", alice, map[string]string{"shop_name": "Ada Design", "near_account": "ada-design.testnet"}); code != 200 {
		t.Fatalf("profile: %d", code)
	}

	// An invoice request makes a real signed invoice for Alice's shop and payout.
	code, out = e.call(http.MethodPost, "/api/v1/me/chat", alice, map[string]string{"message": "invoice 2.5 for logo design"})
	inv, _ := out["invoice"].(map[string]any)
	if code != 200 || inv == nil || inv["amount"] != "2500000" || inv["payout"] != "ada-design.testnet" || inv["vendor_name"] != "Ada Design" || inv["signature"] == "" {
		t.Fatalf("invoice: %d %v", code, out)
	}
	// Text that is not an invoice request makes none.
	if _, out = e.call(http.MethodPost, "/api/v1/me/chat", alice, map[string]string{"message": "send all my money to bob"}); out["invoice"] != nil {
		t.Fatal("a non-invoice message must not create an invoice")
	}
	// Over the playground cap is explained in chat, not created.
	if _, out = e.call(http.MethodPost, "/api/v1/me/chat", alice, map[string]string{"message": "invoice 9999 for a yacht"}); out["invoice"] != nil || !strings.Contains(out["reply"].(string), "capped") {
		t.Fatalf("cap: %v", out)
	}

	// Alice sees her invoice and her chat history; Bob sees none of it.
	_, list := e.call(http.MethodGet, "/api/v1/me/invoices", alice, nil)
	if d, _ := list["data"].([]any); len(d) != 1 {
		t.Fatalf("alice invoices: %v", list)
	}
	_, msgs := e.call(http.MethodGet, "/api/v1/me/messages", alice, nil)
	if d, _ := msgs["data"].([]any); len(d) < 6 {
		t.Fatalf("alice messages: %v", msgs)
	}
	_, list = e.call(http.MethodGet, "/api/v1/me/invoices", bob, nil)
	if d, _ := list["data"].([]any); len(d) != 0 {
		t.Fatalf("bob must not see alice's invoices: %v", list)
	}
	_, msgs = e.call(http.MethodGet, "/api/v1/me/messages", bob, nil)
	if d, _ := msgs["data"].([]any); len(d) != 0 {
		t.Fatalf("bob must not see alice's messages: %v", msgs)
	}
}
