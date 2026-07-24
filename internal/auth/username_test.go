package auth

import "testing"

func TestNormalizeUsername(t *testing.T) {
	cases := map[string]string{
		"  Nathan  ": "nathan",
		"UPPER":      "upper",
		"already":    "already",
	}
	for in, want := range cases {
		if got := NormalizeUsername(in); got != want {
			t.Errorf("NormalizeUsername(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateUsername(t *testing.T) {
	valid := []string{"abc", "nathan", "gallery-row", "a1b", "x0-9z", "abcdefghijklmnopqrstuvwxyz1230"}
	for _, u := range valid {
		if err := ValidateUsername(u); err != nil {
			t.Errorf("ValidateUsername(%q) unexpectedly failed: %v", u, err)
		}
	}

	invalid := []string{
		"",                                // empty
		"ab",                              // too short
		"abcdefghijklmnopqrstuvwxyz12345", // 31 chars
		"-leading",                        // leading hyphen
		"trailing-",                       // trailing hyphen
		"has_underscore",                  // charset
		"_system",                         // sentinel is unspellable
		"Has Upper",                       // caller must normalize first
		"dots.oh.no",                      // email-prefix style
		"ünïcode",                         // non-ascii
		"admin",                           // reserved
		"patchwork",                       // reserved
		"me",                              // reserved (and too short anyway)
	}
	for _, u := range invalid {
		if err := ValidateUsername(u); err == nil {
			t.Errorf("ValidateUsername(%q) should have failed", u)
		}
	}
}
