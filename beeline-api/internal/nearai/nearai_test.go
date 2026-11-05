package nearai

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeCloud is a TEST DOUBLE for NEAR AI Cloud. It signs completions the documented way with a
// test key, standing in for a model's TEE key.
type fakeCloud struct {
	key       ed25519.PrivateKey
	answer    string
	gateway   bool  // sign in the gateway format (no model prefix)
	notFound  int32 // signature 404s before it is "recorded"
	mu        sync.Mutex
	requests  map[string][2][]byte
	sigCalls  atomic.Int32
	completes atomic.Int32
}

func newFake(answer string) *fakeCloud {
	return &fakeCloud{
		key:      ed25519.NewKeyFromSeed(make([]byte, 32)),
		answer:   answer,
		requests: map[string][2][]byte{},
	}
}

func (f *fakeCloud) pub() string { return hex.EncodeToString(f.key.Public().(ed25519.PublicKey)) }

func hexSum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func (f *fakeCloud) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer sk-test" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key sk-test leaked?"}}`))
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
		n := f.completes.Add(1)
		req, _ := io.ReadAll(r.Body)
		id := fmt.Sprintf("chatcmpl-%d", n)
		resp, _ := json.Marshal(map[string]any{
			"id": id, "object": "chat.completion", "created": 1793793660, "model": "test/model",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": f.answer}}},
		})
		f.mu.Lock()
		f.requests[id] = [2][]byte{req, resp}
		f.mu.Unlock()
		_, _ = w.Write(resp)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/signature/"):
		if f.sigCalls.Add(1) <= f.notFound {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"Not found"}}`))
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/v1/signature/")
		f.mu.Lock()
		ex := f.requests[id]
		f.mu.Unlock()
		text := fmt.Sprintf("%s:%s:%s", r.URL.Query().Get("model"), hexSum(ex[0]), hexSum(ex[1]))
		if f.gateway {
			text = fmt.Sprintf("%s:%s", hexSum(ex[0]), hexSum(ex[1]))
		}
		_ = json.NewEncoder(w).Encode(Signature{
			Text: text, Signature: hex.EncodeToString(ed25519.Sign(f.key, []byte(text))),
			SigningAddress: f.pub(), SigningAlgo: r.URL.Query().Get("signing_algo"), SignatureKind: "provider_tee",
		})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

const quoteJSON = `{"signature":"ed25519:x","quote":{"depositAddress":"abc"}}`

func review(t *testing.T, f *fakeCloud) (*Review, error) {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, "sk-test", 5*time.Second)
	c.RetryDelay = time.Millisecond
	return c.Review(context.Background(), "test/model", "Pay invoice #42", "QHASH", quoteJSON)
}

func TestApprovedReviewVerifiesUnderTrustedKey(t *testing.T) {
	f := newFake("Matches the task.\nAPPROVE QHASH\n")
	r, err := review(t, f)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Approved || r.Verdict != "APPROVE QHASH" {
		t.Fatalf("verdict %q approved=%v", r.Verdict, r.Approved)
	}
	if err := VerifySignature(r, f.pub()); err != nil {
		t.Fatalf("verify: %v", err)
	}
	// What the vault checks in the request: the policy prompt and the exact quote.
	var req struct {
		Model    string `json:"model"`
		Messages []struct{ Role, Content string }
	}
	if err := json.Unmarshal([]byte(r.Request), &req); err != nil {
		t.Fatal(err)
	}
	if req.Model != "test/model" || len(req.Messages) != 2 || req.Messages[0].Content != ReviewPolicy ||
		!strings.Contains(req.Messages[1].Content, quoteJSON) {
		t.Fatalf("request does not match what the vault checks: %+v", req)
	}
	if hexSum([]byte(req.Messages[0].Content)) != PolicySHA256() {
		t.Fatal("policy hash mismatch")
	}
}

func TestRejectVerdictIsNotApproved(t *testing.T) {
	r, err := review(t, newFake("The task is an invoice but the quote pays a new party.\nREJECT recipient mismatch"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Approved || !strings.HasPrefix(r.Verdict, "REJECT") {
		t.Fatalf("verdict %q approved=%v", r.Verdict, r.Approved)
	}
}

func TestApprovalOfAnotherHashIsNotApproved(t *testing.T) {
	r, err := review(t, newFake("APPROVE OTHERHASH"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Approved {
		t.Fatal("approval of a different quote hash must not count")
	}
}

func TestVerifyRejectsUntrustedKeyAndTampering(t *testing.T) {
	f := newFake("APPROVE QHASH")
	r, err := review(t, f)
	if err != nil {
		t.Fatal(err)
	}
	other := hex.EncodeToString(ed25519.NewKeyFromSeed(bytesOf(9)).Public().(ed25519.PublicKey))
	if err := VerifySignature(r, other); err == nil {
		t.Fatal("expected failure for an untrusted key")
	}
	tampered := *r
	tampered.Response = strings.Replace(r.Response, "APPROVE QHASH", "APPROVE QHASH ", 1)
	if err := VerifySignature(&tampered, f.pub()); err == nil {
		t.Fatal("expected failure for an edited response")
	}
	forged := *r
	forged.Signature.Text = fmt.Sprintf("test/model:%s:%s", hexSum([]byte(forged.Request)), hexSum([]byte(forged.Response)))
	forged.Signature.Signature = hex.EncodeToString(ed25519.Sign(ed25519.NewKeyFromSeed(bytesOf(9)), []byte(forged.Signature.Text)))
	if err := VerifySignature(&forged, f.pub()); err == nil {
		t.Fatal("expected failure for a signature by another key")
	}
}

func TestGatewaySignatureIsNotEnoughForTheVault(t *testing.T) {
	f := newFake("APPROVE QHASH")
	f.gateway = true
	r, err := review(t, f)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifySignature(r, f.pub()); err == nil || !strings.Contains(err.Error(), "provider_tee") {
		t.Fatalf("expected a provider_tee error, got %v", err)
	}
}

func TestSignatureNotYetRecordedIsRetried(t *testing.T) {
	f := newFake("APPROVE QHASH")
	f.notFound = 2
	r, err := review(t, f)
	if err != nil {
		t.Fatal(err)
	}
	if f.sigCalls.Load() != 3 || VerifySignature(r, f.pub()) != nil {
		t.Fatalf("calls=%d", f.sigCalls.Load())
	}
}

func TestProviderErrorsAreTypedAndNeverEchoTheKey(t *testing.T) {
	srv := httptest.NewServer(newFake("x"))
	defer srv.Close()
	c := NewClient(srv.URL, "sk-wrong", time.Second)
	_, err := c.Review(context.Background(), "test/model", "t", "h", quoteJSON)
	apiErr, ok := err.(*Error)
	if !ok || apiErr.Status != http.StatusUnauthorized {
		t.Fatalf("expected a typed 401, got %v", err)
	}
	if strings.Contains(err.Error(), "sk-wrong") {
		t.Fatal("error must not contain the API key")
	}
}

func TestVerdictLine(t *testing.T) {
	for in, want := range map[string]string{
		"a\nAPPROVE x\n\n  ": "APPROVE x",
		"REJECT y":           "REJECT y",
		"":                   "",
	} {
		if got := VerdictLine(in); got != want {
			t.Errorf("VerdictLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func bytesOf(b byte) []byte {
	s := make([]byte, 32)
	for i := range s {
		s[i] = b
	}
	return s
}

// vaultFixture is shared with the Rust contract (src/testdata/go_review.json), which verifies it
// with the same code the vault runs on-chain. It proves that a review built by this client
// passes the vault's checks byte for byte.
const vaultFixture = "../../../contracts/agent-vault/src/testdata/go_review.json"

func TestVaultFixtureIsCurrent(t *testing.T) {
	f := newFake("The recipient and amount match the task.\nAPPROVE QHASH")
	r, err := review(t, f)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.MarshalIndent(map[string]any{
		"note":          "TEST FIXTURE generated by beeline-api/internal/nearai tests with a test key; not a real NEAR AI signature",
		"model":         r.Model,
		"signing_key":   f.pub(),
		"policy_sha256": PolicySHA256(),
		"quote_json":    quoteJSON,
		"quote_hash":    "QHASH",
		"created":       1793793660,
		"review":        r.VaultArgs(),
	}, "", "  ")
	got = append(got, '\n')
	if os.Getenv("UPDATE_FIXTURES") == "1" {
		if err := os.MkdirAll(filepath.Dir(vaultFixture), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(vaultFixture, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(vaultFixture)
	if err != nil {
		t.Fatalf("%v (run with UPDATE_FIXTURES=1)", err)
	}
	if string(want) != string(got) {
		t.Fatal("vault fixture is stale; run with UPDATE_FIXTURES=1 and re-run the contract tests")
	}
}
