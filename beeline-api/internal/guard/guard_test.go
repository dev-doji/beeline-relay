package guard

import "testing"

func TestParseAcceptsOnlyTheThreeVerdicts(t *testing.T) {
	v, r, err := Parse("Sure! {\"verdict\":\"Watch\",\"reason\":\"  Wrong amount, maybe a typo.  \"}")
	if err != nil || v != VerdictWatch || r != "Wrong amount, maybe a typo." {
		t.Fatalf("got %q %q %v", v, r, err)
	}
	for _, bad := range []string{
		"no json at all",
		`{"verdict":"safe","reason":"x"}`,
		`{"verdict":"fraud","reason":""}`,
		`{"verdict":"","reason":"x"}`,
	} {
		if _, _, err := Parse(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestParseClipsALongReason(t *testing.T) {
	long := make([]byte, 900)
	for i := range long {
		long[i] = 'a'
	}
	_, r, err := Parse(`{"verdict":"clear","reason":"` + string(long) + `"}`)
	if err != nil || len([]rune(r)) != 300 {
		t.Fatalf("reason length %d, err %v", len([]rune(r)), err)
	}
}

func TestARefusedClaimOrARefundIsNeverClear(t *testing.T) {
	for _, subject := range []string{SubjectClaimRefused, SubjectPayinEvent} {
		if got := floor(subject, VerdictClear); got != VerdictWatch {
			t.Errorf("%s: clear became %s, want watch", subject, got)
		}
		if got := floor(subject, VerdictFraud); got != VerdictFraud {
			t.Errorf("%s: fraud must stay fraud, got %s", subject, got)
		}
	}
	if got := floor(SubjectPaymentConfirmed, VerdictClear); got != VerdictClear {
		t.Errorf("a confirmed payment can be clear, got %s", got)
	}
}
