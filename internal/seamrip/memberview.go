package seamrip

import (
	"fmt"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// The member seamrip (docs/adr/089, docs/adr/012 affordance 2).
//
// The portability boundary has two axes, and this file is the second one.
// Tables() says what TRAVELS — community data leaves, instance identity does
// not (docs/adr/002). memberViews() says, for each travelling table, WHICH
// ROWS A GIVEN VIEWER MAY CARRY OUT. It is one boundary with two questions
// asked of every table, not two boundaries:
// TestEveryTableHasAMemberViewRule fails the build when a table answers the
// first and not the second, the way TestEveryTableHasABoundaryDecision fails
// when a table answers neither.
//
// The rule the two axes implement together is ADR 012's: a member can take
// what they can already see. Nothing here widens a read — every predicate
// below is the SQL form of a check that already gates an API response, and
// where the two could drift the narrower answer wins.
//
// The export runs the SAME queries the admin seamrip runs, wrapped:
//
//	SELECT <columns, some replaced by an expression> FROM ( <the table's own
//	query> ) WHERE <the member-view predicate>
//
// so a column added to Tables() is exported by both paths or by neither.
// There is no second set of queries to keep in step, which is the failure
// mode ADR 002 was written about.

// MemberView states which rows of a travelling table a member may carry out
// of the instance, and which of its columns are emptied when they do.
//
// Rule is prose and is required: it is what the boundary test checks for,
// and what a reader comes here to find.
type MemberView struct {
	// Rule says, in one sentence, which rows this viewer may carry.
	Rule string

	// Never marks a table that travels in the admin seamrip and never in a
	// member's. The file is still written, empty, so the archive has the
	// same shape either way and the absence is visible rather than implied.
	Never bool

	// Where is a SQL predicate over the columns the table's own query
	// selects. Every `?` in it binds the viewer's user id.
	Where string

	// Cols replaces a column's value with a SQL expression, for the fields
	// that are readable in some rows and not others — a charter's mirrored
	// text, a foreign key whose target did not travel. Keys must name
	// columns the table exports.
	Cols map[string]string
}

// The four sets every rule is written in terms of. Each is the SQL form of a
// check that already gates a read path, named here once so the rules below
// read as sentences rather than as joins.

// sqlVisibleNodes: the patches this viewer may carry. Public ones, plus
// private ones they hold an active membership on — private is unlisted, not
// locked, and this is the same set ListNodes serves under scope=my.
const sqlVisibleNodes = `SELECT vn.id FROM nodes vn
	WHERE vn.removed_at IS NULL
	AND (vn.visibility = 'public'
		OR EXISTS (SELECT 1 FROM memberships vm
			WHERE vm.node_id = vn.id AND vm.user_id = ? AND vm.status = 'active'))`

// sqlInsiderNodes: the patches this viewer is inside — an active admin or
// member. The line ADR 006 draws for hidden memberships, ADR 036 for
// members-only charters, and ListEvents for members-only events.
const sqlInsiderNodes = `SELECT im.node_id FROM memberships im
	WHERE im.user_id = ? AND im.status = 'active' AND im.role IN ('member','admin')`

// sqlAdminNodes: the patches this viewer administers. Only an admin sees a
// calendar feed's URL, which can carry a token.
const sqlAdminNodes = `SELECT am.node_id FROM memberships am
	WHERE am.user_id = ? AND am.status = 'active' AND am.role = 'admin'`

// sqlVisibleEvents: the events on those patches this viewer may read.
// Members-only events only inside their own patch — a confirmed link to
// another patch never widens visibility, which is the rule ListEvents
// enforces in the same shape.
const sqlVisibleEvents = `SELECT ve.id FROM events ve
	WHERE ve.removed_at IS NULL AND ve.status = 'active'
	AND ve.node_id IN (` + sqlVisibleNodes + `)
	AND (ve.visibility = 'public' OR ve.node_id IN (` + sqlInsiderNodes + `))`

// sqlVisibleProposals: proposals on a patch this viewer may carry. A
// proposal is public deliberation; only the charter text mirrored into an
// amendment follows the charter's own visibility (docs/adr/036).
const sqlVisibleProposals = `SELECT vp.id FROM proposals vp
	WHERE vp.node_id IN (` + sqlVisibleNodes + `)`

// sqlVisibleSources: the calendar feeds this viewer may carry — theirs to
// see because they administer the patch, and never a crosswalk entry, whose
// aggregator is instance-level curation that does not travel at all.
const sqlVisibleSources = `SELECT vs.id FROM event_sources vs
	WHERE vs.aggregator_id IS NULL AND vs.node_id IN (` + sqlAdminNodes + `)`

// sqlVisibleDocs: the charters this viewer may read (docs/adr/036).
const sqlVisibleDocs = `SELECT vd.id FROM governance_docs vd
	WHERE vd.node_id IN (` + sqlVisibleNodes + `)
	AND (vd.visibility = 'public' OR vd.node_id IN (` + sqlInsiderNodes + `))`

// charterText wraps a column holding a charter's mirrored body so it empties
// for a viewer who could not open that charter. Deliberately coarser than
// the API's per-document check (hiddenDocRedactor joins target_doc to a
// filename derived in Go): here a non-member of a patch that keeps ANY
// members-only charter loses the mirrored text on all of them. Reproducing
// the filename function in SQL would be a second copy of a rule that only
// has to be right once, and the cost of the coarse version falls on the
// safe side — text withheld, never text leaked.
func charterText(col string) string {
	return `CASE WHEN COALESCE(` + col + `,'') = ''
		OR node_id IN (` + sqlInsiderNodes + `)
		OR NOT EXISTS (SELECT 1 FROM governance_docs hg
			WHERE hg.node_id = mv.node_id AND hg.visibility = 'members')
		THEN ` + col + ` ELSE '' END`
}

// memberViews maps a table name to its member-view rule. It lives beside
// Tables() rather than inside it only because the travel decisions are
// already long enough to read; the boundary test treats the two as one
// specification and fails on a table missing from either.
func memberViews() map[string]MemberView {
	return map[string]MemberView{
		"users": {
			// The stub rule, and the one decision in this file that is
			// derived rather than declared. The row set is the closure of
			// everything else that travelled: MemberExport reads the
			// schema's own foreign keys into users and carries exactly the
			// people the other tables name — which is authorship, a
			// membership the viewer can read, a seat, an attestation. It has
			// to be the closure, or a row arrives on the fork pointing at
			// nobody and the import drops it.
			//
			// What each person carries is four fields: id, username,
			// display name, avatar. Everything else is emptied here.
			// Emails never travel — that is the line between this and the
			// admin seamrip (docs/adr/012), and the reason the fork
			// re-invites people out of band. Bio, links, contact card and
			// instance role are absent for a plainer reason: nobody asked
			// this person whether their profile should be copied to a new
			// server, and a stub is enough for the fork to re-invite them
			// and for the threads to survive.
			//
			// A tombstone travels as the tombstone it is (docs/adr/086):
			// deleted_at comes along, and the identity columns it already
			// emptied stay empty. Losing deleted_at would resurrect the
			// account on the fork with its handle free to sign in under.
			Rule: "every person another travelling row names, as a stub: id, username, display name, avatar, and a tombstone's deleted_at. Never an email.",
			Cols: map[string]string{
				"email":         "NULL",
				"bio":           "''",
				"links":         "'[]'",
				"role":          "'member'",
				"contact_phone": "''",
				"contact_email": "''",
				"contact_note":  "''",
				// The old instance's moderation judgement, not a fact the
				// fork inherits. A suspension is a decision its stewards
				// made, and they are not the fork's stewards.
				"suspended_at": "NULL",
			},
		},
		"tags": {
			Rule:  "all of them: a tag is the quilt's shared vocabulary and the tag list is a public read.",
			Where: "",
		},
		"nodes": {
			Rule:  "public patches, plus private ones the viewer holds an active membership on.",
			Where: "id IN (" + sqlVisibleNodes + ")",
			Cols: map[string]string{
				// Who put an unclaimed listing up is the instance's review
				// record (docs/adr/026), not the patch's. No read path
				// shows it outside the admin queue.
				"submitted_by": "NULL",
			},
		},
		"node_tags": {
			Rule:  "the tags of a patch that travelled.",
			Where: "node_id IN (" + sqlVisibleNodes + ")",
		},
		"memberships": {
			// The public member list rule of docs/adr/006, read as an
			// export: visible member and admin rows are public, hidden ones
			// and every follower row belong to the room. The viewer's own
			// rows come along whatever they say, because those are theirs.
			Rule:  "on a patch that travelled: the viewer's own rows, every row on a patch they are inside, and otherwise only visible member/admin rows.",
			Where: `node_id IN (` + sqlVisibleNodes + `) AND (user_id = ? OR node_id IN (` + sqlInsiderNodes + `) OR (visible = 1 AND role IN ('member','admin')))`,
		},
		"seats": {
			// A council is public: the governance overview names the
			// admins and the next term end to anybody (docs/adr/051).
			Rule:  "the council of a patch that travelled.",
			Where: "node_id IN (" + sqlVisibleNodes + ")",
		},
		"aggregators": {
			Rule:  "never: an aggregator is instance-level crosswalk curation read only in the admin panel (docs/adr/056), and its URL is a quasi-secret.",
			Never: true,
		},
		"event_sources": {
			Rule:  "only feeds on a patch the viewer administers, and never a crosswalk entry — a feed URL can carry a token, and the aggregator a crosswalk row belongs to does not travel.",
			Where: "id IN (" + sqlVisibleSources + ")",
		},
		"aggregator_ignored_names": {
			Rule:  "never, with its aggregator.",
			Never: true,
		},
		"aggregator_programs": {
			Rule:  "never, with its aggregator.",
			Never: true,
		},
		"event_source_skips": {
			Rule:  "the skip list of a feed that travelled.",
			Where: "source_id IN (" + sqlVisibleSources + ")",
		},
		"events": {
			Rule:  "on a patch that travelled: public events, plus members-only ones where the viewer is a member or admin of the event's own patch.",
			Where: "id IN (" + sqlVisibleEvents + ")",
			Cols: map[string]string{
				// Provenance points at a feed row, and most viewers carry
				// no feeds. Blanked rather than dropped: the event is the
				// community's, the feed it came through is the admin's.
				"source_id": "CASE WHEN source_id IN (" + sqlVisibleSources + ") THEN source_id ELSE NULL END",
			},
		},
		"event_links": {
			Rule:  "confirmed links where both ends travelled — the event and the patch it is linked to.",
			Where: "event_id IN (" + sqlVisibleEvents + ") AND node_id IN (" + sqlVisibleNodes + ")",
			Cols: map[string]string{
				"absorb_event_id": "CASE WHEN absorb_event_id IN (" + sqlVisibleEvents + ") THEN absorb_event_id ELSE NULL END",
			},
		},
		"aggregator_offer_dismissals": {
			Rule:  "never, with its aggregator.",
			Never: true,
		},
		"event_mentions": {
			Rule:  "the cross-quilt doorways of an event that travelled.",
			Where: "event_id IN (" + sqlVisibleEvents + ")",
		},
		"proposals": {
			Rule:  "on a patch that travelled; the mirrored charter text follows the charter's visibility (docs/adr/036).",
			Where: "node_id IN (" + sqlVisibleNodes + ")",
			Cols:  map[string]string{"proposed_body": charterText("proposed_body")},
		},
		"attestations": {
			Rule:  "on a patch that travelled: what a community decided elsewhere is a public read (docs/adr/052).",
			Where: "node_id IN (" + sqlVisibleNodes + ")",
		},
		"amendment_attestations": {
			Rule:  "on a patch that travelled; the adopted text follows the charter's visibility.",
			Where: "node_id IN (" + sqlVisibleNodes + ")",
			Cols: map[string]string{
				"adopted_body": charterText("adopted_body"),
				// The charter this adopted, when that charter travelled.
				// Nullable already, because a record of what a meeting
				// adopted outlives the document (migration 051).
				"doc_id": "CASE WHEN doc_id IN (" + sqlVisibleDocs + ") THEN doc_id ELSE NULL END",
			},
		},
		"attestation_names": {
			Rule:  "the names on an attestation that travelled.",
			Where: "attestation_id IN (SELECT va.id FROM attestations va WHERE va.node_id IN (" + sqlVisibleNodes + "))",
		},
		"votes": {
			Rule:  "the votes on a proposal that travelled: a tally is what makes the record a decision.",
			Where: "proposal_id IN (" + sqlVisibleProposals + ")",
		},
		"election_candidates": {
			Rule:  "the slate of a proposal that travelled.",
			Where: "proposal_id IN (" + sqlVisibleProposals + ")",
		},
		"election_ballots": {
			Rule:  "the approvals cast on a proposal that travelled.",
			Where: "proposal_id IN (" + sqlVisibleProposals + ")",
		},
		"proposal_comments": {
			Rule:  "the discussion under a proposal that travelled.",
			Where: "proposal_id IN (" + sqlVisibleProposals + ")",
		},
		"comment_reactions": {
			Rule:  "the reactions on a comment that travelled.",
			Where: "comment_id IN (SELECT vc.id FROM proposal_comments vc WHERE vc.proposal_id IN (" + sqlVisibleProposals + "))",
		},
		"notices": {
			Rule:  "never: the noticeboard is the room's, and a member carries their own notices out through the personal export instead (docs/adr/081).",
			Never: true,
		},
		"notice_replies": {
			Rule:  "never, with the noticeboard.",
			Never: true,
		},
		"proposal_revisions": {
			Rule:  "the revision history of a proposal that travelled; mirrored charter text follows the charter's visibility.",
			Where: "proposal_id IN (" + sqlVisibleProposals + ")",
			Cols: map[string]string{
				// proposal_revisions has no node_id of its own, so the
				// charter-text test is asked through the proposal.
				"proposed_body": `CASE WHEN COALESCE(proposed_body,'') = ''
					OR EXISTS (SELECT 1 FROM proposals rp WHERE rp.id = mv.proposal_id
						AND rp.node_id IN (` + sqlInsiderNodes + `))
					OR NOT EXISTS (SELECT 1 FROM proposals rp JOIN governance_docs rg ON rg.node_id = rp.node_id
						WHERE rp.id = mv.proposal_id AND rg.visibility = 'members')
					THEN proposed_body ELSE '' END`,
			},
		},
		"governance_docs": {
			Rule:  "on a patch that travelled: public charters, plus members-only ones where the viewer is a member or admin of that patch (docs/adr/036).",
			Where: "id IN (" + sqlVisibleDocs + ")",
		},
		"claim_requests": {
			Rule:  "never: a claim is a person's evidence of who they are, handled by the instance's review (docs/adr/030), not the patch's record.",
			Never: true,
		},
		"notification_preferences": {
			Rule:  "never: a person's own settings travel in their own export (docs/adr/012, affordance 1), not in somebody else's fork.",
			Never: true,
		},
		"patch_notification_config": {
			Rule:  "the notification categories of a patch the viewer is inside.",
			Where: "node_id IN (" + sqlInsiderNodes + ")",
		},
	}
}

// View returns the member-view rule for a table, and whether one is stated.
func (t Table) View() (MemberView, bool) {
	v, ok := memberViews()[t.Name]
	return v, ok
}

// memberQuery wraps a table's own query in the viewer's member view. The
// column list is the table's, in order, so the rows this returns are the
// same shape the admin export writes — a member seamrip is the same archive
// with fewer rows in it and some fields emptied.
func memberQuery(t Table, v MemberView) string {
	cols := make([]string, len(t.Columns))
	for i, col := range t.Columns {
		if expr, replaced := v.Cols[col.Name]; replaced {
			cols[i] = "(" + expr + ") AS " + col.Name
			continue
		}
		cols[i] = col.Name
	}
	q := fmt.Sprintf("SELECT %s FROM (%s) AS mv", strings.Join(cols, ", "), t.Query)
	if v.Where != "" {
		q += " WHERE " + v.Where
	}
	return q
}

// MemberExport writes the member seamrip: the same tables the admin export
// writes, narrowed to the rows viewerID may already read, with people
// reduced to stubs. sink is called once per table in Tables() order, so a
// consumer writing a zip or a directory of files needs no other knowledge.
//
// Two passes, and the second one is why. Every table but `users` is queried
// through its member view; while that happens, MemberExport collects the
// user ids those rows name, reading the columns from the schema's own
// foreign keys into `users` rather than from a hand-kept list. Then `users`
// is exported restricted to exactly that set.
//
// The closure is not a nicety. The import mints a new id for every value in
// a remapped column, so a membership naming a person the archive left out
// would arrive pointing at a users row that does not exist, and SQLite would
// refuse the insert — a silent row loss in the mechanism whose whole job is
// not to lose rows. Deriving the set from the FKs means a new
// user-referencing column joins the closure the day it is added.
func MemberExport(db *database.DB, viewerID string, sink func(t Table, items []map[string]any) error) error {
	tables := Tables()
	rows := make(map[string][]map[string]any, len(tables))

	// The viewer is always in the bundle: they are the person seeding the
	// fork, and a bundle whose own author is missing reads as a mistake.
	people := map[string]bool{viewerID: true}

	for _, t := range tables {
		if t.Name == "users" {
			continue
		}
		v, ok := t.View()
		if !ok {
			return fmt.Errorf("table %s has no member-view rule", t.Name)
		}
		if v.Never {
			rows[t.Name] = []map[string]any{}
			continue
		}
		items, err := queryMemberTable(db, t, v, viewerID)
		if err != nil {
			return fmt.Errorf("member export %s: %w", t.Name, err)
		}
		rows[t.Name] = items

		refs, err := userRefColumns(db, t.Name)
		if err != nil {
			return fmt.Errorf("member export %s: %w", t.Name, err)
		}
		for _, col := range refs {
			for _, item := range items {
				if s, ok := item[col].(string); ok && s != "" && s != SentinelUserID {
					people[s] = true
				}
			}
		}
	}

	for _, t := range tables {
		if t.Name != "users" {
			continue
		}
		v, _ := t.View()
		all, err := queryMemberTable(db, t, v, viewerID)
		if err != nil {
			return fmt.Errorf("member export users: %w", err)
		}
		stubs := []map[string]any{}
		for _, item := range all {
			if s, ok := item["id"].(string); ok && people[s] {
				stubs = append(stubs, item)
			}
		}
		rows["users"] = stubs
	}

	for _, t := range tables {
		if err := sink(t, rows[t.Name]); err != nil {
			return err
		}
	}
	return nil
}

// queryMemberTable runs one table's member-view query. Every `?` in the
// wrapped SQL binds the viewer's id — the only parameter a member view has,
// which is what stops one from being written to reach another person's rows.
func queryMemberTable(db *database.DB, t Table, v MemberView, viewerID string) ([]map[string]any, error) {
	q := memberQuery(t, v)
	args := make([]any, strings.Count(q, "?"))
	for i := range args {
		args[i] = viewerID
	}
	return scanRows(db, q, t.Columns, args)
}

// scanRows reads a result set into column-keyed maps, the shape Import
// reads back.
func scanRows(db *database.DB, query string, columns []Column, args []any) ([]map[string]any, error) {
	rs, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()

	items := []map[string]any{}
	for rs.Next() {
		values := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, err
		}
		item := make(map[string]any, len(columns))
		for i, col := range columns {
			if b, ok := values[i].([]byte); ok {
				item[col.Name] = string(b)
			} else {
				item[col.Name] = values[i]
			}
		}
		items = append(items, item)
	}
	return items, rs.Err()
}

// userRefColumns returns the columns of a table whose foreign key points at
// `users`. Read from the schema so the closure above cannot fall behind a
// migration: a column added with `REFERENCES users(id)` is in the answer the
// moment it exists.
func userRefColumns(db *database.DB, table string) ([]string, error) {
	rs, err := db.Query(`SELECT "from" FROM pragma_foreign_key_list(?) WHERE "table" = 'users'`, table)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var cols []string
	for rs.Next() {
		var col string
		if err := rs.Scan(&col); err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	return cols, rs.Err()
}

// MemberReadmeText documents a member seamrip for whoever opens the archive.
// It differs from ReadmeText where the bundles differ, and says so: this one
// is a view, it carries no addresses, and the fork re-invites its people.
const MemberReadmeText = `Patchwork Member Seamrip
========================

This archive is one member's view of a Patchwork quilt, in the same format
as a full seamrip, so a community can stand itself up again elsewhere
without waiting for an administrator. manifest.json says who took it, when,
and from where.

It carries what that person could already read: public patches and the
private ones they belong to, the member lists they can see, events,
charters, proposals with their votes and discussion, seats and the records
of what was decided elsewhere.

It does NOT carry other people's secrets. There are no email addresses in
it, so nobody can be signed in on the new quilt from this file alone.
People travel as stubs — a username, a display name, an avatar — which is
enough for the memberships to keep their shape, so the threads between
patches survive the move. The new quilt invites people back the way the
old one did, and each person re-joins by choice and sets their own
visibility again.

Also absent: hidden memberships outside the patches the person belongs to,
members-only charters and events from patches they are not in, every
patch's noticeboard, contact cards, calendar feed URLs for patches they do
not administer, claims, reports, and the audit log.

To import into a fresh Patchwork database:
  go run ./cmd/import -db ./new-patchwork.db -in ./export/

Import rewrites every ID and writes the old-to-new mapping to id_map.json.
`
