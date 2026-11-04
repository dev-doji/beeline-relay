// Package amount parses token amounts expressed as integer base units (wei, yoctoNEAR, …).
// Floating point is never used for money.
package amount

import (
	"errors"
	"math/big"
)

// MaxDigits matches the NUMERIC(78, 0) database columns (enough for uint256).
const MaxDigits = 78

var (
	ErrEmpty    = errors.New("amount is required")
	ErrFormat   = errors.New("amount must be a positive integer in base units, as a string of digits")
	ErrZero     = errors.New("amount must be greater than zero")
	ErrTooLarge = errors.New("amount is too large")
)

// ParseBaseUnits parses a strictly formatted positive base-unit amount: digits only,
// no sign, no decimal point, no exponent, no leading zeros.
func ParseBaseUnits(s string) (*big.Int, error) {
	if s == "" {
		return nil, ErrEmpty
	}
	if len(s) > MaxDigits {
		return nil, ErrTooLarge
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return nil, ErrFormat
		}
	}
	if s == "0" {
		return nil, ErrZero
	}
	if s[0] == '0' {
		return nil, ErrFormat
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, ErrFormat
	}
	return v, nil
}
