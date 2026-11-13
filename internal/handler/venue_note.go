package handler

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Where a patch that decides elsewhere actually meets
// (docs/adr/2026-11-11-a-patch-that-meets-elsewhere-says-where.md).
//
// A visitor to a coalition that decides at its meetings read, on its
// Governance tab, that proposals and admins are decided "outside Patchwork",
// and never where. The meeting existed only in the founder's description of
// the patch (F-034). The venue fields say *whether* the decision happens
// here; this is the one sentence saying where it does instead.

// venueNoteMaxRunes keeps the note to what it is for: a time and a place.
// Minutes and longer accounts are documents (docs/adr/053).
const venueNoteMaxRunes = 200

// venueNoteProblem checks the venue note in a proposed rules file. Empty
// when it is fine, including when the body is not a rules file this can
// read: the rules editor is not the only thing that writes one, and refusing
// on a parse error here would be a second, stricter parser nobody else runs.
func venueNoteProblem(proposedBody string) string {
	var rules struct {
		LeadershipVenue string `json:"leadership_venue"`
		ProposalVenue   string `json:"proposal_venue"`
		VenueNote       string `json:"venue_note"`
	}
	if json.Unmarshal([]byte(proposedBody), &rules) != nil {
		return ""
	}
	note := strings.TrimSpace(rules.VenueNote)
	if note == "" {
		return ""
	}
	if utf8.RuneCountInString(note) > venueNoteMaxRunes {
		return "where this patch decides has to fit in 200 characters: a time and a place. Minutes and longer accounts go in a document"
	}
	// A note on a patch that decides everything here would be read under a
	// heading that says nothing happens elsewhere, which is the field nobody
	// reads, docs/adr/049's failure, in a new place.
	if rules.LeadershipVenue != "elsewhere" && rules.ProposalVenue != "elsewhere" {
		return "this patch decides everything in Patchwork, so there is no other place to name"
	}
	return ""
}

// meetsElsewhere reports whether a patch decides anything outside Patchwork:
// its admins, its proposals, or both. It is the patch that keeps minutes
// somewhere else and brings them back, which is what lets its members add
// documents (docs/adr/2026-11-11-a-member-can-file-the-minutes.md).
func meetsElsewhere(db *database.DB, nodeID string) bool {
	return leadershipVenue(db, nodeID) == "elsewhere" || proposalVenue(db, nodeID) == "elsewhere"
}
