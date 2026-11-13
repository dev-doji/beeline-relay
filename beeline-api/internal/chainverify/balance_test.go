package chainverify

import (
	"context"
	"encoding/json"
	"testing"
)

// viewRPC is a TEST DOUBLE: it answers a NEAR view call with fixed result bytes (as the node returns them,
// an array of byte values).
type viewRPC struct{ result string }

func (f viewRPC) Call(_ context.Context, _ string, _ any, out any) error {
	bytes := []int{}
	for _, c := range []byte(f.result) {
		bytes = append(bytes, int(c))
	}
	raw, _ := json.Marshal(map[string]any{"result": bytes})
	return json.Unmarshal(raw, out)
}

func TestBalanceReaderParsesAFungibleTokenBalance(t *testing.T) {
	b := &BalanceReader{RPC: viewRPC{`"2500000"`}}
	n, err := b.FTBalance(context.Background(), "usdc.example", "vendor.near")
	if err != nil || n.String() != "2500000" {
		t.Fatalf("balance = %v, %v", n, err)
	}
	if _, err := (&BalanceReader{RPC: viewRPC{`"abc"`}}).FTBalance(context.Background(), "t", "a"); err == nil {
		t.Fatal("a non-numeric balance must be an error")
	}
	if _, err := (&BalanceReader{RPC: viewRPC{`"-5"`}}).FTBalance(context.Background(), "t", "a"); err == nil {
		t.Fatal("a negative balance must be an error")
	}
}

func TestStorageRegisteredTellsRegisteredFromUnregistered(t *testing.T) {
	reg := &BalanceReader{RPC: viewRPC{`{"total":"1250000000000000000000","available":"0"}`}}
	if ok, err := reg.StorageRegistered(context.Background(), "t", "a"); err != nil || !ok {
		t.Fatalf("registered account: %v %v", ok, err)
	}
	none := &BalanceReader{RPC: viewRPC{`null`}}
	if ok, err := none.StorageRegistered(context.Background(), "t", "a"); err != nil || ok {
		t.Fatalf("unregistered account (null): %v %v", ok, err)
	}
}
