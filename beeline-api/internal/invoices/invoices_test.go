package invoices

import (
	"strings"
	"testing"
)

func valid() CreateInput {
	return CreateInput{
		VendorName: "MS Clothing", VendorHandle: "@msclothing", Payout: "ms-clothing.goni-beeline.testnet",
		Memo: "Order #18", Items: []Item{{Name: "Linen shirt", Quantity: 3, UnitPrice: "150000"}, {Name: "Delivery", Quantity: 1, UnitPrice: "50000"}},
	}
}

func TestValidateSumsItemsInBaseUnits(t *testing.T) {
	in := valid()
	total, err := Validate(&in)
	if err != nil || total != "500000" {
		t.Fatalf("total=%s err=%v", total, err)
	}
}

func TestValidateRejectsBadInput(t *testing.T) {
	cases := map[string]func(*CreateInput){
		"vendor":   func(in *CreateInput) { in.VendorName = " " },
		"payout":   func(in *CreateInput) { in.Payout = "0x7777777777777777777777777777777777777777!" },
		"no items": func(in *CreateInput) { in.Items = nil },
		"price":    func(in *CreateInput) { in.Items[0].UnitPrice = "1.5" },
		"qty":      func(in *CreateInput) { in.Items[0].Quantity = 0 },
		"cap":      func(in *CreateInput) { in.Items[0].UnitPrice = "1000000001" },
	}
	for name, mutate := range cases {
		in := valid()
		mutate(&in)
		if _, err := Validate(&in); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	in := valid()
	in.Items[0].UnitPrice = "1000000001"
	_, err := Validate(&in)
	if err == nil || !strings.Contains(err.Error(), "1000 test USDC") {
		t.Fatalf("cap message: %v", err)
	}
}

func TestRealInvoiceNeedsAMainnetPayout(t *testing.T) {
	svc := &Service{MainnetEnabled: true}
	in := valid()
	in.Real = true // valid() pays out to a .testnet account
	if _, err := svc.Check(&in); err == nil || !strings.Contains(err.Error(), "mainnet") {
		t.Fatalf("a real invoice to a testnet account must be refused: %v", err)
	}
	in.Payout = "ms-clothing.near"
	if _, err := svc.Check(&in); err != nil {
		t.Fatalf("a mainnet payout is fine: %v", err)
	}
}

func TestRealInvoiceCapIsOnePointFiveUSDCAndSaysSo(t *testing.T) {
	svc := &Service{MainnetEnabled: true}
	in := valid()
	in.Real = true
	in.Payout = "ms-clothing.near"
	in.Items = []Item{{Name: "Thing", Quantity: 1, UnitPrice: "1500000"}}
	if _, err := svc.Check(&in); err != nil {
		t.Fatalf("exactly 1.5 USDC is allowed: %v", err)
	}
	in.Items[0].UnitPrice = "1500001"
	_, err := svc.Check(&in)
	if err == nil || !strings.Contains(err.Error(), "1.5 USDC") {
		t.Fatalf("one unit over must be refused and name the cap: %v", err)
	}
}
