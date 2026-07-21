package config

import (
	"testing"
	"time"
)

// An empty session block must reproduce the pre-ADR-017 behaviour exactly,
// so upgrading an instance that never touched patchwork.yaml changes nothing.
func TestSessionDefaultsMatchPreviousBehaviour(t *testing.T) {
	max, idle, err := Session{}.Durations()
	if err != nil {
		t.Fatalf("defaults: %v", err)
	}
	if max != 30*24*time.Hour {
		t.Fatalf("default max_lifetime = %s, want 720h", max)
	}
	if idle != 14*24*time.Hour {
		t.Fatalf("default idle_timeout = %s, want 336h", idle)
	}
}

func TestSessionDurationParsing(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"7d", 7 * 24 * time.Hour},
		{"12h", 12 * time.Hour},
		{"90m", 90 * time.Minute},
		{"1.5d", 36 * time.Hour},
	}
	for _, c := range cases {
		max, _, err := Session{MaxLifetime: c.in, IdleTimeout: "1m"}.Durations()
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if max != c.want {
			t.Fatalf("%q parsed as %s, want %s", c.in, max, c.want)
		}
	}
}

func TestSessionDurationRejectsNonsense(t *testing.T) {
	for _, in := range []string{"soon", "30 days", "-5d", "0h"} {
		if _, _, err := (Session{MaxLifetime: in}).Durations(); err == nil {
			t.Fatalf("%q was accepted as a session lifetime", in)
		}
	}
}

// An idle timeout above the ceiling can never fire. Silently ignoring it
// would leave a config that says one thing and does another.
func TestIdleTimeoutCannotExceedCeiling(t *testing.T) {
	_, _, err := Session{MaxLifetime: "7d", IdleTimeout: "14d"}.Durations()
	if err == nil {
		t.Fatal("idle_timeout longer than max_lifetime was accepted")
	}
}
