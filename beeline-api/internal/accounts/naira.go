package accounts

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

// Naira pricing: a vendor can price in naira ("5 computers at 600000 naira") and Bee converts to USDC with
// the rate Switch quotes for a Nigerian bank payout. The invoice itself is always in USDC and fixed at the
// moment it is made; the rate is a snapshot, not a promise about later.

var (
	nairaWordRe  = regexp.MustCompile(`(?i)(₦|\bngn\b|\bnaira\b|\bn\s?\d)`)
	ngnAmountRe  = regexp.MustCompile(`^\d{1,12}(\.\d{1,2})?$`)
	rateStringRe = regexp.MustCompile(`^\d{1,7}(\.\d{1,8})?$`)
)

// wantsNaira is true only when the user's own message says the prices are in naira. The model cannot decide that.
func wantsNaira(userMsg string) bool { return nairaWordRe.MatchString(userMsg) }

// ngnToUnits converts a naira amount to USDC base units (6 decimals), rounding UP so the vendor is never
// short of the naira value. rate is naira per 1 USDC as a decimal string. No floating point.
func ngnToUnits(ngn, rate string) (string, error) {
	if !ngnAmountRe.MatchString(ngn) || !rateStringRe.MatchString(rate) {
		return "", fmt.Errorf("bad naira amount or rate")
	}
	n, _ := new(big.Rat).SetString(ngn)
	r, _ := new(big.Rat).SetString(rate)
	if n.Sign() <= 0 || r.Sign() <= 0 {
		return "", fmt.Errorf("amount and rate must be positive")
	}
	// units = ceil(n * 1e6 / r)
	q := new(big.Rat).Mul(n, big.NewRat(1_000_000, 1))
	q.Quo(q, r)
	num, den := q.Num(), q.Denom()
	units := new(big.Int)
	rem := new(big.Int)
	units.DivMod(num, den, rem)
	if rem.Sign() != 0 {
		units.Add(units, big.NewInt(1))
	}
	if units.Sign() <= 0 {
		return "", fmt.Errorf("that is less than the smallest USDC amount")
	}
	return units.String(), nil
}

// niceNaira formats "600000" as "600,000".
func niceNaira(s string) string {
	whole, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, c := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if frac != "" {
		return b.String() + "." + frac
	}
	return b.String()
}
