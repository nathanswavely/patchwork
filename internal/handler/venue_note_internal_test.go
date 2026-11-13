package handler

import (
	"strings"
	"testing"
)

// The venue note is a time and a place, on a patch that decides somewhere
// else (docs/adr/2026-11-11-a-patch-that-meets-elsewhere-says-where.md).
func TestVenueNoteProblem(t *testing.T) {
	long := strings.Repeat("a", venueNoteMaxRunes+1)
	cases := []struct {
		name, body string
		ok         bool
	}{
		{"no note", `{"proposal_venue":"patchwork"}`, true},
		{"note where proposals are decided elsewhere", `{"proposal_venue":"elsewhere","venue_note":"First Tuesday, 7pm, the church hall"}`, true},
		{"note where admins are chosen elsewhere", `{"leadership_venue":"elsewhere","venue_note":"The AGM"}`, true},
		{"note where everything is decided here", `{"leadership_venue":"patchwork","proposal_venue":"patchwork","venue_note":"The AGM"}`, false},
		{"a note that is minutes", `{"proposal_venue":"elsewhere","venue_note":"` + long + `"}`, false},
		{"whitespace is no note", `{"venue_note":"   "}`, true},
		{"not a rules file this can read", `not json`, true},
	}
	for _, c := range cases {
		got := venueNoteProblem(c.body)
		if (got == "") != c.ok {
			t.Errorf("%s: got %q, want ok=%v", c.name, got, c.ok)
		}
	}
}
