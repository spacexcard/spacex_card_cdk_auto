package handler

import "testing"

func TestEvaluateCardFailVerdict(t *testing.T) {
	cases := []struct {
		name              string
		fail, distinct, th int
		requireKnown      bool
		want              string
	}{
		{"under threshold", 1, 1, 2, true, verdictNeedMore},
		{"same email twice", 2, 1, 2, true, verdictEmailSuspect},
		{"two emails", 2, 2, 2, true, verdictCardSuspect},
		{"three fails one email", 3, 1, 2, true, verdictEmailSuspect},
		{"three fails two emails", 3, 2, 2, true, verdictCardSuspect},
		{"unknown emails guarded", 2, 0, 2, true, verdictUnknownEmails},
		{"unknown emails unguarded", 2, 0, 2, false, verdictCardSuspect},
		{"threshold 3", 2, 2, 3, true, verdictNeedMore},
		{"threshold 3 hit", 3, 2, 3, true, verdictCardSuspect},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateCardFailVerdict(tc.fail, tc.distinct, tc.th, tc.requireKnown)
			if got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestNormalizeAccountEmail(t *testing.T) {
	if normalizeAccountEmail("  A@B.com ") != "a@b.com" {
		t.Fatal("normalize")
	}
	if normalizeAccountEmail("") != "unknown" {
		t.Fatal("empty")
	}
}
