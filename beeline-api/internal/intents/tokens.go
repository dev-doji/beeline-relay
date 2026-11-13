package intents

import "context"

// Token is one asset NEAR Intents can move, from GET /v0/tokens (checked 2026-11-13: 230 tokens on 39 chains).
type Token struct {
	AssetID         string `json:"assetId"`
	Blockchain      string `json:"blockchain"`
	Symbol          string `json:"symbol"`
	Decimals        int    `json:"decimals"`
	ContractAddress string `json:"contractAddress"`
}

// Tokens lists the assets NEAR Intents supports.
func (c *Client) Tokens(ctx context.Context) ([]Token, error) {
	var out []Token
	if _, err := c.do(ctx, "GET", "/v0/tokens", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
