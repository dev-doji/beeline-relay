package chainverify

import (
	"context"
	"encoding/json"
	"testing"
)

// viewRPC is a TEST DOUBLE: it answers a NEAR view call with fixed result bytes (as the node returns them,
// an array of byte values) and records the call.
type viewRPC struct {
	result string
	seen   *map[string]any
}

func (f viewRPC) Call(_ context.Context, _ string, params any, out any) error {
	if f.seen != nil {
		*f.seen = params.(map[string]any)
	}
	bytes := []int{}
	for _, c := range []byte(f.result) {
		bytes = append(bytes, int(c))
	}
	raw, _ := json.Marshal(map[string]any{"result": bytes})
	return json.Unmarshal(raw, out)
}

func TestIntentsBalanceReadsTheVerifierContract(t *testing.T) {
	var seen map[string]any
	b := &BalanceReader{RPC: viewRPC{`"2500000"`, &seen}}
	n, err := b.IntentsBalance(context.Background(), "vendor.near", "nep141:usdc")
	if err != nil || n.String() != "2500000" {
		t.Fatalf("balance = %v, %v", n, err)
	}
	if seen["account_id"] != IntentsContract || seen["method_name"] != "mt_balance_of" {
		t.Fatalf("must read mt_balance_of on the Intents contract: %v", seen)
	}
	for _, bad := range []string{`"abc"`, `"-5"`, ``} {
		if _, err := (&BalanceReader{RPC: viewRPC{result: bad}}).IntentsBalance(context.Background(), "a", "t"); err == nil {
			t.Errorf("%q must be an error", bad)
		}
	}
}
