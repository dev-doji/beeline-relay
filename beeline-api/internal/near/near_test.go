package near

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/mr-tron/base58"
)

func testKey() Key {
	return Key{AccountID: "vault.test.near", Private: ed25519.NewKeyFromSeed(make([]byte, 32))}
}

// The encoding is checked field by field against NEAR's borsh schema for Transaction V0.
func TestSignFunctionCallEncoding(t *testing.T) {
	k := testKey()
	bh := make([]byte, 32)
	bh[0] = 7
	signed, hash, err := SignFunctionCall(k, 42, bh, FunctionCall{
		Receiver: "vault.test.near", Method: "pay_quote", Args: []byte(`{"a":1}`),
		Gas: 150_000_000_000_000, Deposit: big.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	b := signed
	read := func(n int) []byte { p := b[:n]; b = b[n:]; return p }
	str := func() string { n := binary.LittleEndian.Uint32(read(4)); return string(read(int(n))) }

	txStart := len(signed)
	if got := str(); got != "vault.test.near" {
		t.Fatalf("signer %q", got)
	}
	if read(1)[0] != 0 || string(read(32)) != string(k.Private.Public().(ed25519.PublicKey)) {
		t.Fatal("public key")
	}
	if binary.LittleEndian.Uint64(read(8)) != 42 {
		t.Fatal("nonce")
	}
	if str() != "vault.test.near" || read(32)[0] != 7 {
		t.Fatal("receiver / block hash")
	}
	if binary.LittleEndian.Uint32(read(4)) != 1 || read(1)[0] != 2 {
		t.Fatal("one FunctionCall action")
	}
	if str() != "pay_quote" || str() != `{"a":1}` {
		t.Fatal("method / args")
	}
	if binary.LittleEndian.Uint64(read(8)) != 150_000_000_000_000 {
		t.Fatal("gas")
	}
	dep := read(16)
	if dep[0] != 1 || dep[15] != 0 {
		t.Fatal("deposit u128 little-endian")
	}
	txLen := txStart - len(b)
	sum := sha256.Sum256(signed[:txLen])
	if string(sum[:]) != string(hash) {
		t.Fatal("hash must be sha256 of the borsh transaction")
	}
	if read(1)[0] != 0 || !ed25519.Verify(k.Private.Public().(ed25519.PublicKey), hash, read(64)) || len(b) != 0 {
		t.Fatal("signature")
	}
}

func TestParseKeyRoundTrip(t *testing.T) {
	k := testKey()
	parsed, err := ParseKey("a.near", "ed25519:"+base58.Encode(k.Private))
	if err != nil || parsed.PublicKey() != k.PublicKey() {
		t.Fatalf("round trip failed: %v", err)
	}
	if _, err := ParseKey("a.near", "ed25519:abc"); err == nil {
		t.Fatal("short key must be rejected")
	}
}

func TestFailureMessage(t *testing.T) {
	st := json.RawMessage(`{"Failure":{"ActionError":{"index":0,"kind":{"FunctionCallError":{"ExecutionError":"Smart contract panicked: BEELINE_VAULT_REJECTED: x"}}}}}`)
	if got := failureMessage(st); got != "Smart contract panicked: BEELINE_VAULT_REJECTED: x" {
		t.Fatalf("got %q", got)
	}
	if failureMessage(json.RawMessage(`{"SuccessValue":""}`)) != "" {
		t.Fatal("success has no failure")
	}
}

// TestLiveTestnetReplayIsRejected sends a REAL testnet transaction: it replays the rehearsal's
// already-paid quote with the agent key, which the vault must reject. It moves no funds and
// runs only when NEAR_LIVE_TEST=1 and the local .testnet files exist.
func TestLiveTestnetReplayIsRejected(t *testing.T) {
	if os.Getenv("NEAR_LIVE_TEST") != "1" {
		t.Skip("set NEAR_LIVE_TEST=1 to send a real testnet transaction")
	}
	var kf struct{ AccountID, PrivateKey string }
	raw, err := os.ReadFile("../../.testnet/agent-key.json")
	if err != nil {
		t.Skip("no .testnet/agent-key.json")
	}
	var m map[string]string
	_ = json.Unmarshal(raw, &m)
	kf.AccountID, kf.PrivateKey = m["account_id"], m["private_key"]
	key, err := ParseKey(kf.AccountID, kf.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile("../../.testnet/pay-allowed.json")
	if err != nil {
		t.Skip("no .testnet/pay-allowed.json")
	}
	c := NewClient("https://rpc.testnet.fastnear.com", 60*time.Second)
	out, err := c.Send(context.Background(), key, FunctionCall{
		Receiver: kf.AccountID, Method: "pay_quote", Args: args, Gas: 100_000_000_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("tx %s failure=%q", out.Hash, out.Failure)
	if out.Failure == "" {
		t.Fatal("replay must be rejected by the vault")
	}
}
