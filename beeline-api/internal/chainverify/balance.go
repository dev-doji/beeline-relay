package chainverify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// IntentsContract is the NEAR Intents verifier on mainnet. Balances held inside it are multi-token (NEP-245) balances.
const IntentsContract = "intents.near"

// BalanceReader reads balances from NEAR. Read-only.
type BalanceReader struct{ RPC RPC }

// IntentsBalance returns account's balance of tokenID (for example "nep141:<usdc contract>") inside NEAR Intents,
// in base units. Money delivered to an account's Intents balance needs no token registration and no NEAR for gas.
func (b *BalanceReader) IntentsBalance(ctx context.Context, account, tokenID string) (*big.Int, error) {
	args, _ := json.Marshal(map[string]string{"account_id": account, "token_id": tokenID})
	var out struct {
		Result []byte `json:"result"`
		Error  string `json:"error"`
	}
	err := b.RPC.Call(ctx, "query", map[string]any{
		"request_type": "call_function", "finality": "final", "account_id": IntentsContract,
		"method_name": "mt_balance_of", "args_base64": base64.StdEncoding.EncodeToString(args),
	}, &out)
	if err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, errors.New("mt_balance_of: " + out.Error)
	}
	s := strings.Trim(strings.TrimSpace(string(out.Result)), `"`)
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() < 0 {
		return nil, fmt.Errorf("mt_balance_of returned %q", s)
	}
	return n, nil
}
