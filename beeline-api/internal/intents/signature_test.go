package intents

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// A real non-dry 1Click quote fetched through the Beeline API on 2026-11-04 (no funds were
// sent to its deposit address). Used to prove signature verification against live data.
const liveQuote = `{"quote": {"amountIn": "1000000", "deadline": "2026-11-07T13:32:05.506Z", "amountOut": "997398", "refundFee": "0", "amountInUsd": "0.999515000000", "minAmountIn": "1000000", "withdrawFee": "2400", "amountOutUsd": "0.996914261970", "minAmountOut": "987424", "timeEstimate": 22, "depositAddress": "b2a200b2bdabdbca8544ea2847e2e4af24779ec274a89be2d7e53d187758991b", "timeWhenInactive": "2026-11-07T13:32:05.506Z", "amountInFormatted": "1.0", "amountOutFormatted": "0.997398"}, "signature": "ed25519:1Kf341fWaooBVUWYfxt8RiTq6WWpGV11cVSr8nFTznG7mUQNb9zdqdzC6y54hM4jMP8Kv4GUhkAaUiw8o3eDs7p", "timestamp": "2026-11-04T13:22:33.625Z", "quoteRequest": {"dry": false, "amount": "1000000", "appFees": [{"fee": 1, "recipient": "5880ad2b362620fadf759cbceb1cd5737ce8c6ed7fb8e9942881e6731f9247dd", "limitOrderId": null}], "insured": false, "deadline": "2026-11-04T13:32:05.506Z", "refundTo": "intents.near", "swapType": "EXACT_INPUT", "recipient": "0x2222222222222222222222222222222222222222", "refundType": "ORIGIN_CHAIN", "depositMode": "SIMPLE", "depositType": "ORIGIN_CHAIN", "originAsset": "nep141:17208628f84f5d6ad33f0da3bbbeb27ffcb398eac501a31bd6ad2011e36133a1", "recipientType": "DESTINATION_CHAIN", "confidentiality": "public", "destinationAsset": "nep141:base-0x833589fcd6edb6e08f4c7c32d4f71b54bda02913.omft.near", "slippageTolerance": 100, "quoteWaitingTimeMs": 0}, "correlationId": "ccc4674d-f500-4a92-b5c9-61cb05643390"}`

const liveSignature = "ed25519:1Kf341fWaooBVUWYfxt8RiTq6WWpGV11cVSr8nFTznG7mUQNb9zdqdzC6y54hM4jMP8Kv4GUhkAaUiw8o3eDs7p"

func TestVerifyLiveQuoteSignature(t *testing.T) {
	ok, err := VerifyQuoteSignature([]byte(liveQuote), liveSignature, OneClickManagerPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		h, _ := QuoteHash([]byte(liveQuote))
		t.Fatalf("live 1Click quote signature did not verify (hash %s)", h)
	}
}

func TestTamperedQuoteFailsVerification(t *testing.T) {
	tampered := map[string]string{
		"deposit address": strings.Replace(liveQuote, "b2a200b2bdab", "aaaa00b2bdab", 1),
		"recipient":       strings.Replace(liveQuote, "0x2222222222222222222222222222222222222222", "0x3333333333333333333333333333333333333333", 1),
		"amount out":      strings.Replace(liveQuote, `"amountOut": "997398"`, `"amountOut": "997399"`, 1),
		"timestamp":       strings.Replace(liveQuote, "13:22:33.625Z", "13:22:34.625Z", 1),
	}
	for name, raw := range tampered {
		ok, err := VerifyQuoteSignature([]byte(raw), liveSignature, OneClickManagerPublicKey)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if ok {
			t.Errorf("tampered %s still verified", name)
		}
	}
}

func TestUnsignedFieldsDoNotAffectHash(t *testing.T) {
	// appFees, insured, depositMode and correlationId are not part of the signed payload.
	changed := strings.Replace(liveQuote, `"insured": false`, `"insured": true`, 1)
	changed = strings.Replace(changed, `"depositMode": "SIMPLE"`, `"depositMode": "MEMO"`, 1)
	ok, err := VerifyQuoteSignature([]byte(changed), liveSignature, OneClickManagerPublicKey)
	if err != nil || !ok {
		t.Fatalf("unsigned fields changed the result: ok=%v err=%v", ok, err)
	}
}

// A real INTENTS-deposit quote fetched on 2026-11-05 (shared with the vault's Rust tests).
func TestLiveIntentsQuoteSignatureVerifies(t *testing.T) {
	raw, err := os.ReadFile("../../../contracts/agent-vault/src/testdata/live_intents_quote.json")
	if err != nil {
		t.Fatal(err)
	}
	var q struct {
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(raw, &q); err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyQuoteSignature(raw, q.Signature, OneClickManagerPublicKey)
	if err != nil || !ok {
		t.Fatalf("live INTENTS quote must verify: ok=%v err=%v", ok, err)
	}
}
