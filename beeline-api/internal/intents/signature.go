package intents

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mr-tron/base58"
)

// OneClickManagerPublicKey is the key 1Click signs quotes with, as published in the official
// TypeScript SDK (defuse-protocol/one-click-sdk-typescript, src/quote-signature.ts).
const OneClickManagerPublicKey = "ed25519:reYaWhvwu8Jzo3WUM3zhn6VrhuMEF4eADL17qtRVifc"

// Fields of quoteRequest included in the signed payload, always present.
var signedRequestFields = []string{
	"dry", "swapType", "slippageTolerance", "originAsset", "depositType", "destinationAsset",
	"amount", "refundTo", "refundType", "recipient", "recipientType", "deadline",
}

// Optional quoteRequest fields, included only when truthy (the SDK maps falsy to undefined).
var optionalRequestFields = []string{
	"quoteWaitingTimeMs", "referral", "virtualChainRecipient", "virtualChainRefundRecipient", "customRecipientMsg",
}

// Fields of quote included for every quote (dropped by the serializer only when absent).
var signedQuoteFields = []string{
	"amountIn", "amountInFormatted", "amountInUsd", "minAmountIn",
	"amountOut", "amountOutFormatted", "amountOutUsd", "minAmountOut",
}

// Extra quote fields for non-dry quotes, included only when truthy.
var optionalQuoteFields = []string{
	"depositAddress", "depositMemo", "deadline", "timeWhenInactive", "timeEstimate", "refundFee", "withdrawFee",
}

// QuoteHash reproduces the 1Click SDK's quoteHash: base58(sha256(stableStringify(
// {...signedRequest, ...signedQuote, timestamp}))). Quote fields override request fields of
// the same name (notably "deadline").
func QuoteHash(raw []byte) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // keep numbers exactly as the provider wrote them
	var resp struct {
		Timestamp    string         `json:"timestamp"`
		QuoteRequest map[string]any `json:"quoteRequest"`
		Quote        map[string]any `json:"quote"`
	}
	if err := dec.Decode(&resp); err != nil {
		return "", fmt.Errorf("decode quote response: %w", err)
	}
	if resp.QuoteRequest == nil || resp.Quote == nil || resp.Timestamp == "" {
		return "", errors.New("quote response is missing quoteRequest, quote or timestamp")
	}

	payload := map[string]any{}
	for _, k := range signedRequestFields {
		if v, ok := resp.QuoteRequest[k]; ok && v != nil {
			payload[k] = v
		}
	}
	for _, k := range optionalRequestFields {
		if v, ok := resp.QuoteRequest[k]; ok && truthy(v) {
			payload[k] = v
		}
	}
	for _, k := range signedQuoteFields {
		if v, ok := resp.Quote[k]; ok && v != nil {
			payload[k] = v
		}
	}
	if dry, _ := resp.QuoteRequest["dry"].(bool); !dry {
		for _, k := range optionalQuoteFields {
			if v, ok := resp.Quote[k]; ok && truthy(v) {
				payload[k] = v
			}
		}
	}
	payload["timestamp"] = resp.Timestamp

	canonical, err := stableStringify(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return base58.Encode(sum[:]), nil
}

// VerifyQuoteSignature checks that a raw 1Click quote response was signed by publicKey.
// The signed message is the UTF-8 bytes of the base58 quote hash.
func VerifyQuoteSignature(raw []byte, signature, publicKey string) (bool, error) {
	pub, err := decodeEd25519(publicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false, fmt.Errorf("invalid public key")
	}
	sig, err := decodeEd25519(signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false, fmt.Errorf("invalid signature encoding")
	}
	hash, err := QuoteHash(raw)
	if err != nil {
		return false, err
	}
	return ed25519.Verify(pub, []byte(hash), sig), nil
}

func decodeEd25519(s string) ([]byte, error) {
	return base58.Decode(strings.TrimPrefix(s, "ed25519:"))
}

// truthy mirrors JavaScript truthiness for JSON values.
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case json.Number:
		f, err := t.Float64()
		return err == nil && f != 0
	default:
		return true
	}
}

// stableStringify matches json-stable-stringify: object keys sorted, no whitespace, and no
// HTML escaping (encoding/json escapes <, > and & by default; JavaScript does not).
func stableStringify(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil { // maps are encoded with sorted keys
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
