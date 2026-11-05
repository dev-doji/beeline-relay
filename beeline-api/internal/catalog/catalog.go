// Package catalog holds the networks, assets and routes Beeline Relay is configured to support.
//
// Asset IDs are 1Click canonical asset IDs taken from GET /v0/tokens on 2026-10-28
// (see docs/NEAR_INTENTS_ROUTE.md). A route listed here is *configured*, not verified:
// verification status is reported separately by /capabilities.
package catalog

import (
	"fmt"
	"regexp"
)

type Network struct {
	ID   string `json:"id"` // 1Click "blockchain" value
	Name string `json:"name"`
}

type Asset struct {
	ID       string `json:"id"` // 1Click canonical assetId
	Network  string `json:"network"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
	Contract string `json:"contract"`
}

type Route struct {
	ID          string `json:"id"`
	Source      Asset  `json:"source"`
	Destination Asset  `json:"destination"`
	// 1Click rails for this route. NEAR-side legs go through the NEAR Intents verifier
	// (INTENTS): on 2026-11-05 1Click stopped quoting NEAR ORIGIN_CHAIN deposits and NEAR
	// DESTINATION_CHAIN delivery (docs/NEAR_INTENTS_ROUTE.md section 6b).
	DepositType   string `json:"deposit_type"`
	RefundType    string `json:"refund_type"`
	RecipientType string `json:"recipient_type"`
}

type Catalog struct {
	networks []Network
	assets   []Asset
	routes   []Route
}

const (
	NetworkNEAR = "near"
	NetworkBase = "base"
)

var (
	usdcNEAR = Asset{
		ID:       "nep141:17208628f84f5d6ad33f0da3bbbeb27ffcb398eac501a31bd6ad2011e36133a1",
		Network:  NetworkNEAR,
		Symbol:   "USDC",
		Decimals: 6,
		Contract: "17208628f84f5d6ad33f0da3bbbeb27ffcb398eac501a31bd6ad2011e36133a1",
	}
	usdcBase = Asset{
		ID:       "nep141:base-0x833589fcd6edb6e08f4c7c32d4f71b54bda02913.omft.near",
		Network:  NetworkBase,
		Symbol:   "USDC",
		Decimals: 6,
		Contract: "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
	}
)

// Default returns the hackathon catalogue: USDC between NEAR and Base, in both directions.
func Default() *Catalog {
	return &Catalog{
		networks: []Network{{ID: NetworkNEAR, Name: "NEAR"}, {ID: NetworkBase, Name: "Base"}},
		assets:   []Asset{usdcNEAR, usdcBase},
		routes: []Route{
			{
				ID: "usdc-near-to-usdc-base", Source: usdcNEAR, Destination: usdcBase,
				DepositType: "INTENTS", RefundType: "INTENTS", RecipientType: "DESTINATION_CHAIN",
			},
			{
				ID: "usdc-base-to-usdc-near", Source: usdcBase, Destination: usdcNEAR,
				DepositType: "ORIGIN_CHAIN", RefundType: "ORIGIN_CHAIN", RecipientType: "INTENTS",
			},
		},
	}
}

func (c *Catalog) Networks() []Network { return c.networks }
func (c *Catalog) Routes() []Route     { return c.routes }

// Assets returns assets on the given network, or all assets when network is empty.
func (c *Catalog) Assets(network string) []Asset {
	out := []Asset{}
	for _, a := range c.assets {
		if network == "" || a.Network == network {
			out = append(out, a)
		}
	}
	return out
}

func (c *Catalog) HasNetwork(id string) bool {
	for _, n := range c.networks {
		if n.ID == id {
			return true
		}
	}
	return false
}

func (c *Catalog) HasAsset(network, assetID string) bool {
	for _, a := range c.assets {
		if a.Network == network && a.ID == assetID {
			return true
		}
	}
	return false
}

func (c *Catalog) FindRoute(srcNetwork, srcAsset, dstNetwork, dstAsset string) (Route, bool) {
	for _, r := range c.routes {
		if r.Source.Network == srcNetwork && r.Source.ID == srcAsset &&
			r.Destination.Network == dstNetwork && r.Destination.ID == dstAsset {
			return r, true
		}
	}
	return Route{}, false
}

func (c *Catalog) RouteByID(id string) (Route, bool) {
	for _, r := range c.routes {
		if r.ID == id {
			return r, true
		}
	}
	return Route{}, false
}

var (
	// NEAR account IDs: named (alice.near, sub.alice.near) or implicit (64 lowercase hex).
	nearAccountRe = regexp.MustCompile(`^(([a-z\d]+[\-_])*[a-z\d]+\.)*([a-z\d]+[\-_])*[a-z\d]+$`)
	evmAddressRe  = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
)

// ValidateAddress checks that addr is well-formed for the network. It does not check
// that the account exists or can receive the asset.
func ValidateAddress(network, addr string) error {
	switch network {
	case NetworkNEAR:
		if len(addr) < 2 || len(addr) > 64 || !nearAccountRe.MatchString(addr) {
			return fmt.Errorf("%q is not a valid NEAR account ID", addr)
		}
	case NetworkBase:
		if !evmAddressRe.MatchString(addr) {
			return fmt.Errorf("%q is not a valid EVM address", addr)
		}
	default:
		return fmt.Errorf("unsupported network %q", network)
	}
	return nil
}
