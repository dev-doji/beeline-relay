// Package testnetquote builds TESTNET-SIMULATED 1Click-style quotes signed with a local test key.
//
// 1Click has no testnet, so the testnet vault is initialised with this test key as its quote
// signer. Quotes are marked with correlationId "TESTNET-SIMULATED-QUOTE" and can never be paid
// by a mainnet vault, which trusts only 1Click's real key.
package testnetquote

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/mr-tron/base58"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/intents"
)

const CorrelationID = "TESTNET-SIMULATED-QUOTE"

// Params describes one simulated quote.
type Params struct {
	Vault            string // refundTo (the vault itself)
	Token            string // NEP-141 token the vault spends, e.g. usdc.fakes.testnet
	DestinationAsset string
	Recipient        string
	Amount           string // base units
	Now              time.Time
}

// Quote is a built, signed quote.
type Quote struct {
	JSON           string
	Hash           string
	DepositAddress string
}

// Build signs a quote with signer, using the same hash 1Click signs (intents.QuoteHash).
func Build(signer ed25519.PrivateKey, p Params) (*Quote, error) {
	var dep [32]byte
	if _, err := rand.Read(dep[:]); err != nil {
		return nil, err
	}
	now := p.Now.UTC()
	resp := map[string]any{
		"correlationId": CorrelationID,
		"timestamp":     now.Format("2006-01-02T15:04:05.000Z"),
		"signature":     "",
		"quoteRequest": map[string]any{
			"dry": false, "swapType": "EXACT_INPUT", "slippageTolerance": 100,
			"originAsset": "nep141:" + p.Token, "depositType": "ORIGIN_CHAIN",
			"destinationAsset": p.DestinationAsset, "amount": p.Amount,
			"refundTo": p.Vault, "refundType": "ORIGIN_CHAIN",
			"recipient": p.Recipient, "recipientType": "DESTINATION_CHAIN",
			"deadline": now.Add(10 * time.Minute).Format("2006-01-02T15:04:05.000Z"),
		},
		"quote": map[string]any{
			"amountIn": p.Amount, "minAmountIn": p.Amount, "amountOut": p.Amount, "minAmountOut": p.Amount,
			"depositAddress": hex.EncodeToString(dep[:]),
			"deadline":       now.Add(24 * time.Hour).Format("2006-01-02T15:04:05.000Z"),
			"timeEstimate":   22,
		},
	}
	unsigned, _ := json.Marshal(resp)
	hash, err := intents.QuoteHash(unsigned)
	if err != nil {
		return nil, err
	}
	resp["signature"] = "ed25519:" + base58.Encode(ed25519.Sign(signer, []byte(hash)))
	out, _ := json.Marshal(resp)
	return &Quote{JSON: string(out), Hash: hash, DepositAddress: hex.EncodeToString(dep[:])}, nil
}
