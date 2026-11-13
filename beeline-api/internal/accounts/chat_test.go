package accounts

import (
	"strings"
	"testing"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/invoices"
)

func TestParseInvoiceUnderstandsTheDocumentedPhrasings(t *testing.T) {
	cases := []struct {
		msg  string
		want []invoices.Item
	}{
		{"invoice 25 for logo design", []invoices.Item{{Name: "logo design", Quantity: 1, UnitPrice: "25000000"}}},
		{"Invoice $0.5 USDC for shirt", []invoices.Item{{Name: "shirt", Quantity: 1, UnitPrice: "500000"}}},
		{"bill 2 x linen shirt 0.15, delivery 0.05", []invoices.Item{
			{Name: "linen shirt", Quantity: 2, UnitPrice: "150000"},
			{Name: "delivery", Quantity: 1, UnitPrice: "50000"},
		}},
		{"please create an invoice website work 3.5 usdc", []invoices.Item{{Name: "website work", Quantity: 1, UnitPrice: "3500000"}}},
		{"charge 1", []invoices.Item{{Name: "Payment", Quantity: 1, UnitPrice: "1000000"}}},
		{"invoice 0.000001 for dust", []invoices.Item{{Name: "dust", Quantity: 1, UnitPrice: "1"}}},
	}
	for _, c := range cases {
		got, err := ParseInvoice(c.msg)
		if err != nil {
			t.Errorf("%q: %v", c.msg, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("%q: got %+v", c.msg, got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q item %d: got %+v want %+v", c.msg, i, got[i], c.want[i])
			}
		}
	}
}

func TestParseInvoiceRefusesWhatItDoesNotUnderstand(t *testing.T) {
	for _, msg := range []string{
		"", "   ", "hello there", "what can you do", "send all my money to bob",
		"invoice for logo design", // no price
		"invoice 0 for nothing",   // zero
		"invoice 1234567 for x",   // absurd whole part
	} {
		if items, err := ParseInvoice(msg); err == nil {
			t.Errorf("%q should be refused, got %+v", msg, items)
		}
	}
}

func TestParseInvoiceNeverUsesFloatingPoint(t *testing.T) {
	// 0.1 + 0.2 style traps: the price must be the exact decimal the user typed.
	got, err := ParseInvoice("invoice 0.3 for x")
	if err != nil || got[0].UnitPrice != "300000" {
		t.Fatalf("got %+v err %v", got, err)
	}
	got, err = ParseInvoice("invoice 19.999999 for x")
	if err != nil || got[0].UnitPrice != "19999999" {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestValidateProfile(t *testing.T) {
	if _, near, err := ValidateProfile("  Ada Design ", " Ada.Testnet "); err != nil || near != "ada.testnet" {
		t.Fatalf("near=%q err=%v", near, err)
	}
	for _, c := range [][2]string{{"", "ada.testnet"}, {"Ada", ""}, {"Ada", "NOT AN ACCOUNT!"}, {"Ada", "a"}} {
		if _, _, err := ValidateProfile(c[0], c[1]); err == nil {
			t.Errorf("accepted %v", c)
		}
	}
}

func TestParserHandlesPoliteAndNairaPhrasings(t *testing.T) {
	// The exact messages that failed on the live site.
	for _, msg := range []string{"make me an invoice for 5 inks for 100 naira per ink", "yes so create the invoice for 5 inks for me at 100 naira"} {
		items, err := ParseInvoiceNGN(msg, "1000")
		if err != nil {
			t.Fatalf("%q: %v", msg, err)
		}
		if len(items) != 1 || items[0].Quantity != 5 || items[0].UnitPrice != "100000" || !strings.HasPrefix(items[0].Name, "inks") || !strings.Contains(items[0].Name, "₦100") {
			t.Fatalf("%q -> %+v", msg, items)
		}
	}
	items, err := ParseInvoiceNGN("invoice 2 x laptops 600,000 naira", "1000")
	if err != nil || items[0].Quantity != 2 || items[0].UnitPrice != "600000000" {
		t.Fatalf("commas and x quantity: %+v %v", items, err)
	}
	// The USDC phrasing is unchanged, including "25 for 3 logos" not reading 25 as a quantity.
	if items, err := ParseInvoice("invoice 25 for logo design"); err != nil || items[0].UnitPrice != "25000000" || items[0].Quantity != 1 {
		t.Fatalf("usdc: %+v %v", items, err)
	}
}
