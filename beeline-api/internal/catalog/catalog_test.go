package catalog

import "testing"

func TestValidateAddress(t *testing.T) {
	cases := []struct {
		network, addr string
		ok            bool
	}{
		{NetworkNEAR, "alice.near", true},
		{NetworkNEAR, "vault.beeline-relay.near", true},
		{NetworkNEAR, "a_b-c.testnet", true},
		{NetworkNEAR, "98793cd91a3f870fb126f66285808c7e094afcfc4eda8a970f6648cdf0dbd6de", true},
		{NetworkNEAR, "Alice.near", false},
		{NetworkNEAR, "alice..near", false},
		{NetworkNEAR, "-alice.near", false},
		{NetworkNEAR, "a", false},
		{NetworkNEAR, "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913", true}, // ETH-implicit account (NEP-518)
		{NetworkNEAR, "0x833589FCD6EDB6E08F4C7C32D4F71B54BDA02913", false},
		{NetworkBase, "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913", true},
		{NetworkBase, "0x833589FCD6EDB6E08F4C7C32D4F71B54BDA02913", true},
		{NetworkBase, "833589fcd6edb6e08f4c7c32d4f71b54bda02913", false},
		{NetworkBase, "0x833589fcd6edb6e08f4c7c32d4f71b54bda0291", false},
		{NetworkBase, "alice.near", false},
		{"sol", "anything", false},
	}
	for _, c := range cases {
		if err := ValidateAddress(c.network, c.addr); (err == nil) != c.ok {
			t.Errorf("ValidateAddress(%s, %q) err=%v, want ok=%v", c.network, c.addr, err, c.ok)
		}
	}
}

func TestFindRoute(t *testing.T) {
	c := Default()
	r, ok := c.FindRoute(NetworkNEAR, usdcNEAR.ID, NetworkBase, usdcBase.ID)
	if !ok || r.ID != "usdc-near-to-usdc-base" {
		t.Fatalf("expected NEAR→Base route, got %+v ok=%v", r, ok)
	}
	if _, ok := c.FindRoute(NetworkNEAR, usdcNEAR.ID, NetworkNEAR, usdcNEAR.ID); ok {
		t.Fatal("same-asset route should not exist")
	}
	if _, ok := c.FindRoute(NetworkBase, usdcNEAR.ID, NetworkBase, usdcBase.ID); ok {
		t.Fatal("asset on the wrong network must not match")
	}
}

func TestAssetsFilter(t *testing.T) {
	c := Default()
	if got := c.Assets(NetworkBase); len(got) != 1 || got[0].ID != usdcBase.ID {
		t.Fatalf("unexpected base assets %+v", got)
	}
	if got := c.Assets("sol"); len(got) != 0 {
		t.Fatalf("expected no assets for unknown network, got %+v", got)
	}
}
