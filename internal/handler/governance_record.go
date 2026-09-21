package handler

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
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

		// The record endpoint is the deliberation by name — every entry
		// carries its author, and a settled one its applier or decliner
		// (docs/adr/2026-09-18-the-default-should-match-the-assumption.md).
		if !canReadGovernanceRecord(db, r, nodeID) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"items":                    []recordEntry{},
				"public_governance_record": "nobody",
			})
			return
		}

		// Whether this reader is owed the names. A public record read from
		// outside the room withholds them the way the roster does; inside,
		// a hidden membership is still visible to the patch's own people.
		namesWithheld := !viewerIsInPatchRoom(db, r, nodeID)

		entries := []recordEntry{}
		entries = append(entries, settledProposals(db, nodeID, slug)...)
		entries = append(entries, recordedDecisions(db, nodeID)...)
		entries = append(entries, councilChanges(db, nodeID, namesWithheld)...)

		// Newest first. Sorted here rather than in SQL because the entries come
		// from three queries with different date columns, and an attestation's
		// date is the meeting's rather than the row's.
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].At > entries[j].At })

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"items": entries})
	}
}

// settledProposals covers everything decided in Patchwork: votes that carried
// or failed, direct changes, and elections — plus the two that decided nothing
// and are here because their window closed, a lapse and an unsettled contest.
func settledProposals(db *database.DB, nodeID, slug string) []recordEntry {
	out := []recordEntry{}
	rows, err := db.Query(`
		SELECT p.id, p.title, p.status, COALESCE(p.state,''), p.seats_contested,
		       COALESCE(p.applied_at, p.updated_at) AS decided_at,
		       `+displayNameExpr("u")+` AS author_name,
		       COALESCE(`+displayNameExpr("ap")+`, '') AS applier_name,
		       COALESCE(`+displayNameExpr("dc")+`, '') AS decliner_name,
		       COALESCE(p.voting_terms,'') AS terms,
		       (SELECT COUNT(*) FROM votes v WHERE v.proposal_id = p.id) AS any_votes
		FROM proposals p
		LEFT JOIN users u ON u.id = p.author_id
		LEFT JOIN users ap ON ap.id = p.applied_by
		LEFT JOIN users dc ON dc.id = p.declined_by
		WHERE p.node_id = ? AND p.status IN ('approved','rejected')
		ORDER BY decided_at DESC`, nodeID)
	if err != nil {
		return out
	}
	defer rows.Close()

	for rows.Next() {
		var id, title, status, state, decidedAt, author, applier, decliner, termsJSON string
		var seats, anyVotes int
		if rows.Scan(&id, &title, &status, &state, &seats, &decidedAt, &author, &applier, &decliner, &termsJSON, &anyVotes) != nil {
			continue
		}
		var terms model.GovernanceConfig
		json.Unmarshal([]byte(termsJSON), &terms)

		e := recordEntry{At: decidedAt, Title: title, Link: "/patches/" + slug + "/governance/" + id}
		switch {
		case seats > 0:
			e.Kind = "election"
			// A rejected election settled nothing; holdover kept the council
			// (docs/adr/051), and the record has to say that rather than
			// implying the community turned somebody down.
			if status == "approved" {
				e.Outcome = "seated"
				// Who, by name. The line read "The electorate seated a
				// council." on a contest that filled one chair of three,
				// and a former chair of that co-op read it four times
				// before accepting it named nobody: "If I had skimmed the
				// record and gone away, I would have gone away with the
				// wrong idea of my own co-op." The same `Names` field a
				// council attestation already uses, so the record has one
				// way of saying who was seated rather than two.
				//
				// Empty on contests that resolved before the outcome was
				// stored, and the page falls back to the unnamed sentence.
				e.Names = seatedNames(db, id)
			} else {
				e.Outcome = "unsettled"
			}
		case decliner != "":
			// The maintainer said no (docs/adr/092). Never "failed": a
			// tally, if there was one, was advice, and the record must not
			// say the members turned it down when one person did.
			e.Kind = "direct"
			e.Outcome = "declined"
			e.Actor = decliner
		case terms.DecisionMethod == "admin" && state == "in_effect", anyVotes == 0 && state == "in_effect":
			// Born applied under admin-decides rules (docs/adr/041), or
			// approved by the maintainer after asking the members
			// (docs/adr/092). Either way a person decided, and the record
			// names them instead of a tally. Proposals from before terms
			// were photographed carry none, so the vote-less in_effect
			// case still recognises them.
			e.Kind = "direct"
			e.Outcome = "applied"
			e.Actor = author
			if applier != "" {
				e.Actor = applier
			}
		default:
			e.Kind = "vote"
			switch {
			case status == "approved":
				e.Outcome = "carried"
			case state == "lapsed":
				// The window closed under quorum (docs/adr/097). Nobody
				// decided it either way, so it is not a vote that failed —
				// and the `rejected` it carries is the schema's only
				// terminal "no", not the community's answer. Read off the
				// state for the same reason the election branch above reads
				// off the seats: the status column cannot tell them apart.
				e.Outcome = "lapsed"
			default:
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
		SELECT a.id, a.decided_at, a.summary, `+displayNameExpr("u")+`
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
		SELECT a.doc_title, a.decided_at, a.summary, `+displayNameExpr("u")+`
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

// seatedNames is who a contest put in the chairs, in the order the chairs
// were filled.
//
// Read from the stored outcome rather than recomputed from the tally, for
// the reason recordEntry gives for carrying no tally at all: a tally drops
// ballots from people who have since left the patch, so it moves after the
// fact, and a governance record that renames who was elected because a voter
// left is worse than one that stays quiet.
//
// Empty for contests that resolved before that column existed; the page has
// a sentence for that.
func seatedNames(db *database.DB, proposalID string) []string {
	rows, err := db.Query(
		`SELECT `+displayNameExpr("u")+`
		   FROM election_candidates c LEFT JOIN users u ON u.id = c.user_id
		  WHERE c.proposal_id = ? AND c.seated = 1
		  ORDER BY c.id ASC`, proposalID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil && name != "" {
			out = append(out, name)
		}
	}
	return out
}

// councilChanges is who came off the council, who went on, and when.
//
// The Record is headed as this patch's decisions and carried none of these.
// A founder came back specifically for them, twice, and left without them
// both times: "Nothing on this site tells me when Devon and Ana came off, or
// why. The Record is headed 'Everything this patch has settled' and it does
// not contain the single most important event in the co-op's year... That is
// the one thing I came back to find out and I had to work it out from a
// notice I wrote myself last October."
//
// She could not have found it anywhere else. A seat vacated for inactivity
// and an interim promotion are written to the audit log, which is
// instance-admin only, so the largest governance event a patch can have —
// its council emptying — reached no member-facing surface at all.
//
// Drawn from the audit log rather than a new table, because the rows already
// exist and are already the authority. What changes is who may read these
// three actions about their own patch, which is the same set the rest of
// this endpoint answers to.
func councilChanges(db *database.DB, nodeID string, namesWithheld bool) []recordEntry {
	out := []recordEntry{}
	// node_id is in the metadata for the two the sweep writes and absent from
	// the one a person writes, whose entity is the membership row; COALESCE
	// covers both, and the LEFT JOIN keeps an entry readable after the
	// membership itself is gone.
	rows, err := db.Query(`
		SELECT a.action, a.created_at, COALESCE(a.metadata, ''),
		       COALESCE(`+displayNameExpr("su")+`, ''),
		       COALESCE(`+displayNameExpr("au")+`, ''),
		       COALESCE(sm.visible, 1)
		FROM audit_log a
		LEFT JOIN memberships m ON m.id = a.entity_id
		LEFT JOIN users su ON su.id = COALESCE(json_extract(a.metadata, '$.target_user_id'), m.user_id)
		LEFT JOIN users au ON au.id = a.user_id
		LEFT JOIN memberships sm
		       ON sm.user_id = COALESCE(json_extract(a.metadata, '$.target_user_id'), m.user_id)
		      AND sm.node_id = ?
		WHERE a.entity_type = 'membership'
		  AND a.action IN ('membership.seat_vacated','membership.succession','membership.role_change')
		  AND COALESCE(json_extract(a.metadata, '$.node_id'), m.node_id) = ?
		ORDER BY a.created_at DESC`, nodeID, nodeID)
	if err != nil {
		return out
	}
	defer rows.Close()

	for rows.Next() {
		var action, at, metaJSON, subject, actor string
		var visible int
		if rows.Scan(&action, &at, &metaJSON, &subject, &actor, &visible) != nil {
			continue
		}
		var meta struct {
			OldRole string `json:"old_role"`
			NewRole string `json:"new_role"`
			Reason  string `json:"reason"`
		}
		json.Unmarshal([]byte(metaJSON), &meta)

		// A role change that never touched the council is not a council
		// event: member to follower is a relationship, not a seat.
		if action == "membership.role_change" && meta.OldRole != "admin" && meta.NewRole != "admin" {
			continue
		}

		// The same rule the roster and the council run (docs/adr/006,
		// docs/adr/095). Inside the room a hidden membership is still
		// visible; outside it, the record says what happened without
		// saying who it happened to.
		if subject == "" {
			subject = DeletedAccountName
		} else if namesWithheld || visible == 0 {
			subject = HiddenMemberName
		}
		if namesWithheld {
			actor = ""
		}

		e := recordEntry{Kind: "seat", At: at, Title: subject}
		switch {
		case action == "membership.seat_vacated":
			// The reason rides in the outcome rather than in Summary, which
			// the page renders as a line of its own: a bare "inactivity"
			// under a sentence that already says it reads as machine
			// wreckage rather than a record.
			e.Outcome = "vacated"
			if meta.Reason == "inactivity" {
				e.Outcome = "vacated_inactivity"
			}
		case action == "membership.succession":
			e.Outcome = "stepped_in"
		case meta.NewRole == "admin":
			e.Outcome = "made_admin"
			e.Actor = actor
		default:
			e.Outcome = "stood_down"
			e.Actor = actor
		}
		out = append(out, e)
	}
	return out
}
