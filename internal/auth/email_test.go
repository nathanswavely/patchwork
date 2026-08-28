package auth

import "testing"

func TestNormalizeEmailFolds(t *testing.T) {
	// The whole point: what gets stored and what gets typed later must be
	// the same string, because every lookup is an exact match.
	for _, raw := range []string{
		"Someone@Example.com",
		"  someone@example.com  ",
		"SOMEONE@EXAMPLE.COM",
	} {
		got, err := NormalizeEmail(raw)
		if err != nil {
			t.Fatalf("NormalizeEmail(%q): %v", raw, err)
		}
		if got != "someone@example.com" {
			t.Errorf("NormalizeEmail(%q) = %q, want someone@example.com", raw, got)
		}
	}
}

func TestNormalizeEmailRejects(t *testing.T) {
	cases := map[string]string{
		"empty":         "",
		"blank":         "   ",
		"no at":         "someone",
		"no domain":     "someone@",
		"display form":  "Someone <someone@example.com>",
		"trailing junk": "someone@example.com, other@example.com",
	}
	for name, raw := range cases {
		if got, err := NormalizeEmail(raw); err == nil {
			t.Errorf("%s: NormalizeEmail(%q) = %q, want error", name, raw, got)
		}
	}
}

func TestNormalizeEmailLength(t *testing.T) {
	long := "a"
	for len(long) < maxEmailLen {
		long += "a"
	}
	if _, err := NormalizeEmail(long + "@example.com"); err == nil {
		t.Error("over-long address accepted")
	}
}
