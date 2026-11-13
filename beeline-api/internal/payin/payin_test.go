package payin

import "testing"

func TestValidateRefund(t *testing.T) {
	ok := map[string]string{
		"base": "0x1111111111111111111111111111111111111111",
		"eth":  "0xAbCdEf0123456789aBcDeF0123456789abcdef01",
		"sol":  "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin",
		"tron": "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
		"ton":  "EQCD39VS5jcptHL8vMjEXrzGaRcCVYto7HUn4bpAOg8xqB2N",
	}
	for chain, a := range ok {
		if err := ValidateRefund(chain, a); err != nil {
			t.Errorf("%s %q should be valid: %v", chain, a, err)
		}
	}
	bad := map[string]string{
		"base": "0x123",
		"eth":  "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8", // a Tron address is not an Ethereum one
		"sol":  "0x1111111111111111111111111111111111111111",
		"tron": "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin",
		"doge": "D8vFz4p1L37jdg47HXKn3mPVq7yWmz9n1B", // not on the shortlist
		"ton":  "",
	}
	for chain, a := range bad {
		if err := ValidateRefund(chain, a); err == nil {
			t.Errorf("%s %q must be refused", chain, a)
		}
	}
}

func TestHumanAmounts(t *testing.T) {
	for in, want := range map[string]string{
		"2004000": "2.004", "2000000": "2", "1": "0.000001", "10500000": "10.5", "0": "0",
	} {
		if got := human(in, 6); got != want {
			t.Errorf("human(%s) = %s, want %s", in, got, want)
		}
	}
	if got := human("1000000000000000000", 18); got != "1" {
		t.Errorf("18 decimals: %s", got)
	}
}

func TestOnlyTerminalStatesAreFinal(t *testing.T) {
	for _, s := range []string{StatusPaid, StatusRefunded, StatusFailed, StatusExpired, StatusIncomplete} {
		if !terminal(s) {
			t.Errorf("%s should be terminal", s)
		}
	}
	for _, s := range []string{StatusWaiting, StatusProcessing, StatusConfirming} {
		if terminal(s) {
			t.Errorf("%s must not be terminal", s)
		}
	}
}
