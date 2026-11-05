package playground

import "github.com/dev-doji/beeline-relay/beeline-api/internal/intents"

// quoteHash is the 1Click quote hash the vault computes (and the reviewer must approve).
func quoteHash(quoteJSON string) (string, error) {
	return intents.QuoteHash([]byte(quoteJSON))
}
