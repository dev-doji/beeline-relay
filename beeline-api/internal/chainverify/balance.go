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

// BalanceReader reads NEP-141 balances from NEAR (view call ft_balance_of). Read-only.
type BalanceReader struct{ RPC RPC }

// FTBalance returns account's balance of token in base units.
func (b *BalanceReader) FTBalance(ctx context.Context, token, account string) (*big.Int, error) {
	args, _ := json.Marshal(map[string]string{"account_id": account})
	var out struct {
		Result []byte `json:"result"`
		Error  string `json:"error"`
	}
	err := b.RPC.Call(ctx, "query", map[string]any{
		"request_type": "call_function", "finality": "final", "account_id": token,
		"method_name": "ft_balance_of", "args_base64": base64.StdEncoding.EncodeToString(args),
	}, &out)
	if err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, errors.New("ft_balance_of: " + out.Error)
	}
	s := strings.Trim(strings.TrimSpace(string(out.Result)), `"`)
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() < 0 {
		return nil, fmt.Errorf("ft_balance_of returned %q", s)
	}
	return n, nil
}

// StorageRegistered reports whether account is registered with the token contract (NEP-145 storage_balance_of).
// A NEP-141 token cannot be held by an unregistered account, so a transfer to one can fail.
func (b *BalanceReader) StorageRegistered(ctx context.Context, token, account string) (bool, error) {
	args, _ := json.Marshal(map[string]string{"account_id": account})
	var out struct {
		Result []byte `json:"result"`
		Error  string `json:"error"`
	}
	err := b.RPC.Call(ctx, "query", map[string]any{
		"request_type": "call_function", "finality": "final", "account_id": token,
		"method_name": "storage_balance_of", "args_base64": base64.StdEncoding.EncodeToString(args),
	}, &out)
	if err != nil {
		return false, err
	}
	if out.Error != "" {
		return false, errors.New("storage_balance_of: " + out.Error)
	}
	s := strings.TrimSpace(string(out.Result))
	return s != "" && s != "null", nil
}
