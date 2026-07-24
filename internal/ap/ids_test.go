package ap_test

import (
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
)

func TestAPID_WithDomain(t *testing.T) {
	got := ap.APID("example.com", "users", "abc123")
	want := "https://example.com/ap/users/abc123"
	if got != want {
		t.Errorf("APID() = %s, want %s", got, want)
	}
}

func TestAPID_EmptyDomain(t *testing.T) {
	got := ap.APID("", "users", "abc123")
	want := "https://localhost/ap/users/abc123"
	if got != want {
		t.Errorf("APID() with empty domain = %s, want %s", got, want)
	}
}

func TestUserAPID(t *testing.T) {
	got := ap.UserAPID("example.com", "user-1")
	want := "https://example.com/ap/users/user-1"
	if got != want {
		t.Errorf("UserAPID() = %s, want %s", got, want)
	}
}

func TestNodeAPID(t *testing.T) {
	got := ap.NodeAPID("example.com", "node-1")
	want := "https://example.com/ap/nodes/node-1"
	if got != want {
		t.Errorf("NodeAPID() = %s, want %s", got, want)
	}
}

func TestEventAPID(t *testing.T) {
	got := ap.EventAPID("example.com", "event-1")
	want := "https://example.com/ap/events/event-1"
	if got != want {
		t.Errorf("EventAPID() = %s, want %s", got, want)
	}
}

func TestProposalAPID(t *testing.T) {
	got := ap.ProposalAPID("example.com", "proposal-1")
	want := "https://example.com/ap/proposals/proposal-1"
	if got != want {
		t.Errorf("ProposalAPID() = %s, want %s", got, want)
	}
}

func TestSetAndGetDomain(t *testing.T) {
	ap.SetDomain("my-instance.org")
	got := ap.GetDomain()
	want := "my-instance.org"
	if got != want {
		t.Errorf("GetDomain() after SetDomain = %s, want %s", got, want)
	}
	// Reset to avoid polluting other tests.
	ap.SetDomain("")
}

func TestGetDomain_Default(t *testing.T) {
	ap.SetDomain("")
	got := ap.GetDomain()
	want := "localhost"
	if got != want {
		t.Errorf("GetDomain() default = %s, want %s", got, want)
	}
}
