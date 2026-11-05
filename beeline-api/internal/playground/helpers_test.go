package playground

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/mr-tron/base58"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/nearai"
)

// signForTest signs like NEAR AI's provider_tee signatures: {model}:{sha256(req)}:{sha256(resp)}.
func signForTest(key ed25519.PrivateKey, model, req, resp string) nearai.Signature {
	sum := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
	text := model + ":" + sum(req) + ":" + sum(resp)
	return nearai.Signature{
		Text: text, Signature: hex.EncodeToString(ed25519.Sign(key, []byte(text))),
		SigningAddress: hex.EncodeToString(key.Public().(ed25519.PublicKey)), SigningAlgo: "ed25519", SignatureKind: "provider_tee",
	}
}

func hexOf(b []byte) string { return hex.EncodeToString(b) }

// verifyWith reports whether quoteJSON's signature verifies under key's public key.
func verifyWith(quoteJSON string, key ed25519.PrivateKey) (bool, error) {
	var q struct {
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal([]byte(quoteJSON), &q); err != nil {
		return false, err
	}
	pub := "ed25519:" + base58.Encode(key.Public().(ed25519.PublicKey))
	return intents.VerifyQuoteSignature([]byte(quoteJSON), q.Signature, pub)
}
