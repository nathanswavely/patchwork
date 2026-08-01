package handler

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// What a patch has decided, in order (docs/adr/055).
//
// Patchwork already held every decision this returns. Proposals carried their
// tallies, attestations carried what a meeting settled, elections carried who
// they seated. None of it was readable as a sequence: the hub showed counts,
// the proposals list showed one status at a time, and a member asking "when
// did we change the quorum, and who voted?" had to go looking.
//
// So this assembles rather than stores. No table, no write path, nothing to
// keep in step — every entry is a view of a row that some other feature owns,
// which is also why the record federates and travels without being taught to.
//
// Only settled things appear. An open proposal is an argument in progress, and
// putting it here would make the record a to-do list.

type recordEntry struct {
	// Kind is what happened, not which table it came from: 'vote' where the
	// electorate decided, 'direct' where an admin did under rules that allow
	// it (docs/adr/041), 'election', 'council' and 'adoption' for the two
	// kinds of attestation (docs/adr/052, docs/adr/053).
	Kind string `json:"kind"`
	// At is when the decision was made. For an attestation that is the day the
	// community decided, which is not the day it was typed in.
	At      string `json:"at"`
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
	// Link is the entry's own page, so the record is a way in rather than a
	// dead end. Empty where the thing has no page of its own.
	Link    string `json:"link,omitempty"`
	Outcome string `json:"outcome,omitempty"`
	Actor   string `json:"actor,omitempty"`
	// Names, on a council attestation: who the meeting seated.
	Names []string `json:"names,omitempty"`

	// Deliberately no tally. `status` is written when a vote resolves and never
	// moves; a tally is recomputed on every read and drops ballots from people
	// who have since left (docs/adr/044), so the two drift. A seeded proposal
	// already rendered "Did not carry. 2 for, 1 against." The counts that
	// decided it were never stored, only the votes — and the proposal's own
	// page shows those in full, with which ones still count. One click away
	// beats a number on the record that contradicts the outcome beside it.
}

// GovernanceRecord handles GET /api/v1/nodes/{slug}/governance/record.
//
// Public, like the proposals it draws from. A patch's governance being
// readable by the people it affects is the whole premise, and nothing here
// discloses more than the pages it links to already do.
func GovernanceRecord(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		entries := []recordEntry{}
		entries = append(entries, settledProposals(db, nodeID, slug)...)
		entries = append(entries, recordedDecisions(db, nodeID)...)

		// Newest first. Sorted here rather than in SQL because the entries come
		// from three queries with different date columns, and an attestation's
		// date is the meeting's rather than the row's.
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].At > entries[j].At })

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"items": entries})
	}
}

// settledProposals covers everything decided in Patchwork: votes that carried
// or failed, direct changes, and elections.
func settledProposals(db *database.DB, nodeID, slug string) []recordEntry {
	out := []recordEntry{}
	rows, err := db.Query(`
		SELECT p.id, p.title, p.status, COALESCE(p.state,''), p.seats_contested,
		       COALESCE(p.applied_at, p.updated_at) AS decided_at,
		       COALESCE(u.display_name, u.username, '') AS author_name,
		       (SELECT COUNT(*) FROM votes v WHERE v.proposal_id = p.id) AS any_votes
		FROM proposals p
		LEFT JOIN users u ON u.id = p.author_id
		WHERE p.node_id = ? AND p.status IN ('approved','rejected')
		ORDER BY decided_at DESC`, nodeID)
	if err != nil {
		return out
	}
	defer rows.Close()

	for rows.Next() {
		var id, title, status, state, decidedAt, author string
		var seats, anyVotes int
		if rows.Scan(&id, &title, &status, &state, &seats, &decidedAt, &author, &anyVotes) != nil {
			continue
		}

		e := recordEntry{At: decidedAt, Title: title, Link: "/patches/" + slug + "/governance/" + id}
		switch {
		case seats > 0:
			e.Kind = "election"
			// A rejected election settled nothing; holdover kept the council
			// (docs/adr/051), and the record has to say that rather than
			// implying the community turned somebody down.
			if status == "approved" {
				e.Outcome = "seated"
			} else {
				e.Outcome = "unsettled"
			}
		case anyVotes == 0 && state == "in_effect":
			// Born applied under admin-decides rules (docs/adr/041). No vote
			// happened, so the record names who applied it instead of a tally.
			e.Kind = "direct"
			e.Outcome = "applied"
			e.Actor = author
		default:
			e.Kind = "vote"
			if status == "approved" {
				e.Outcome = "carried"
			} else {
				e.Outcome = "failed"
			}
		}
		out = append(out, e)
	}
	return out
}

// recordedDecisions covers what the community settled somewhere Patchwork was
// not, and came back to record (docs/adr/052, docs/adr/053).
func recordedDecisions(db *database.DB, nodeID string) []recordEntry {
	out := []recordEntry{}

	// Councils. Superseded records stay readable on the governance hub, where
	// the correction sits beside what it corrects; here they would read as two
	// councils seated on one day.
	rows, err := db.Query(`
		SELECT a.id, a.decided_at, a.summary, COALESCE(u.display_name, u.username, '')
		FROM attestations a
		LEFT JOIN users u ON u.id = a.recorded_by
		WHERE a.node_id = ? AND a.kind = 'leadership'
		  AND NOT EXISTS (SELECT 1 FROM attestations s WHERE s.supersedes_id = a.id)`, nodeID)
	if err == nil {
		for rows.Next() {
			var id, decidedAt, summary, recorder string
			if rows.Scan(&id, &decidedAt, &summary, &recorder) != nil {
				continue
			}
			e := recordEntry{
				Kind: "council", At: decidedAt, Title: "The council changed",
				Summary: summary, Actor: recorder,
			}
			nameRows, nerr := db.Query(
				`SELECT display_name FROM attestation_names WHERE attestation_id = ? ORDER BY position ASC`, id)
			if nerr == nil {
				for nameRows.Next() {
					var n string
					if nameRows.Scan(&n) == nil {
						e.Names = append(e.Names, n)
					}
				}
				nameRows.Close()
			}
			out = append(out, e)
		}
		rows.Close()
	}

	// Texts a meeting adopted.
	arows, aerr := db.Query(`
		SELECT a.doc_title, a.decided_at, a.summary, COALESCE(u.display_name, u.username, '')
		FROM amendment_attestations a
		LEFT JOIN users u ON u.id = a.recorded_by
		WHERE a.node_id = ?`, nodeID)
	if aerr == nil {
		defer arows.Close()
		for arows.Next() {
			var docTitle, decidedAt, summary, recorder string
			if arows.Scan(&docTitle, &decidedAt, &summary, &recorder) != nil {
				continue
			}
			out = append(out, recordEntry{
				Kind: "adoption", At: decidedAt, Title: docTitle,
				Summary: summary, Actor: recorder, Outcome: "adopted",
			})
		}
	}

	return out
}
