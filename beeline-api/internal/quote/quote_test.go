package quote

import (
	"testing"
	"time"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/apperr"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/catalog"
)

func validInput() CreateInput {
	c := catalog.Default()
	r := c.Routes()[0] // NEAR -> Base
	return CreateInput{
		SourceNetwork:      r.Source.Network,
		SourceAssetID:      r.Source.ID,
		SourceAmount:       "1000000",
		DestinationNetwork: r.Destination.Network,
		DestinationAssetID: r.Destination.ID,
		DestinationAddress: "0x000000000000000000000000000000000000dEaD",
		RefundAddress:      "vault.beeline-test.near",
	}
}

func TestFingerprintChangesWithEveryMaterialField(t *testing.T) {
	base := validInput()
	fp := Fingerprint("r", base)
	if fp != Fingerprint("r", base) {
		t.Fatal("fingerprint is not deterministic")
	}
	mutations := map[string]func(*CreateInput){
		"amount":      func(in *CreateInput) { in.SourceAmount = "1000001" },
		"recipient":   func(in *CreateInput) { in.DestinationAddress = "0x000000000000000000000000000000000000bEEF" },
		"refund":      func(in *CreateInput) { in.RefundAddress = "other.near" },
		"src asset":   func(in *CreateInput) { in.SourceAssetID = "x" },
		"dst asset":   func(in *CreateInput) { in.DestinationAssetID = "x" },
		"src network": func(in *CreateInput) { in.SourceNetwork = "x" },
		"dst network": func(in *CreateInput) { in.DestinationNetwork = "x" },
	}
	for name, mutate := range mutations {
		in := base
		mutate(&in)
		if Fingerprint("r", in) == fp {
			t.Errorf("changing %s did not change the fingerprint", name)
		}
	}
	if Fingerprint("other-route", base) == fp {
		t.Error("changing route did not change the fingerprint")
	}
}

func TestValidate(t *testing.T) {
	s := &Service{Catalog: catalog.Default()}
	if _, err := s.Validate(validInput()); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}

	cases := map[string]struct {
		mutate func(*CreateInput)
		code   string
	}{
		"unknown network": {func(in *CreateInput) { in.SourceNetwork = "sol" }, apperr.CodeUnsupportedNetwork},
		"symbol not id":   {func(in *CreateInput) { in.SourceAssetID = "USDC" }, apperr.CodeUnsupportedAsset},
		"same asset route": {func(in *CreateInput) {
			in.DestinationNetwork, in.DestinationAssetID = in.SourceNetwork, in.SourceAssetID
		}, apperr.CodeRouteUnavailable},
		"float amount":       {func(in *CreateInput) { in.SourceAmount = "1.5" }, apperr.CodeValidation},
		"zero amount":        {func(in *CreateInput) { in.SourceAmount = "0" }, apperr.CodeValidation},
		"bad recipient":      {func(in *CreateInput) { in.DestinationAddress = "alice.near" }, apperr.CodeValidation},
		"bad refund address": {func(in *CreateInput) { in.RefundAddress = "Not A NEAR Account" }, apperr.CodeValidation},
	}
	for name, c := range cases {
		in := validInput()
		c.mutate(&in)
		_, err := s.Validate(in)
		ae, ok := apperr.As(err)
		if !ok || ae.Code != c.code {
			t.Errorf("%s: got %v, want code %s", name, err, c.code)
		}
	}
}

func TestExpired(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	q := &Quote{ExpiresAt: now}
	if !q.Expired(now) {
		t.Error("quote must be expired exactly at its deadline")
	}
	if q.Expired(now.Add(-time.Second)) {
		t.Error("quote must be valid before its deadline")
	}
}
