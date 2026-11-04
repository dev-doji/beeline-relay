package amount

import (
	"errors"
	"strings"
	"testing"
)

func TestParseBaseUnits(t *testing.T) {
	valid := map[string]string{
		"1":                            "1",
		"1000000":                      "1000000",
		strings.Repeat("9", MaxDigits): strings.Repeat("9", MaxDigits),
	}
	for in, want := range valid {
		got, err := ParseBaseUnits(in)
		if err != nil || got.String() != want {
			t.Errorf("ParseBaseUnits(%q) = %v, %v; want %s", in, got, err, want)
		}
	}

	invalid := map[string]error{
		"":                               ErrEmpty,
		"0":                              ErrZero,
		"01":                             ErrFormat,
		"-1":                             ErrFormat,
		"+1":                             ErrFormat,
		"1.5":                            ErrFormat,
		"1e6":                            ErrFormat,
		" 1":                             ErrFormat,
		"1,000":                          ErrFormat,
		"0x10":                           ErrFormat,
		"١":                              ErrFormat, // non-ASCII digit
		strings.Repeat("9", MaxDigits+1): ErrTooLarge,
	}
	for in, want := range invalid {
		if _, err := ParseBaseUnits(in); !errors.Is(err, want) {
			t.Errorf("ParseBaseUnits(%q) error = %v; want %v", in, err, want)
		}
	}
}
