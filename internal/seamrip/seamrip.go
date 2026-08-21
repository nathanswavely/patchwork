// Package seamrip defines the data-portability boundary of a Patchwork
// instance: which tables leave in an export and how they come back in an
// import. It is the single source of truth used by the admin zip endpoint
// (GET /api/v1/admin/export), the export CLI, and the import CLI.
//
// Scope: community data travels; instance identity does not.
//
//   - Included: user profiles (with email and instance role, so a fork can
//     re-authenticate its people), patches, memberships (the shared-member
//     overlap that threads and the quilt are inferred from), council seats
//     and their term ends, tags, events with their provenance, event sources
//     and their skip lists (feed URLs are quasi-secrets, but the admin
//     seamrip is already a custody transfer), proposals with the terms they
//     opened under and their raw votes — approval ballots and candidates
//     included, since an election's votes are no less raw for being shaped
//     differently — attestations of what was decided elsewhere, governance
//     docs, proposal comments / reactions / revisions, claim requests, and
//     notification preferences.
//   - Excluded: credentials, sessions, recovery codes, personal feed
//     secrets, magic/invite/signup links, ActivityPub
//     keypairs and ap_ids, remote followers and the delivery queue, audit
//     log, content reports, in-app notification rows, and reminder-dedup
//     state. A fresh instance regenerates its federation identity on first
//     boot (PopulateAPIds / BackfillKeypairs).
package seamrip

import (
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Column describes one exported field. Remap marks ID columns whose values
// must be rewritten through the old→new ID map on import. Nullable remap
// columns (parent_id, reviewed_by, ...) import as NULL when absent.
// Default fills the value when an archive row lacks the key entirely —
// how columns added after an archive was written stay importable (the
// INSERT names every column, so the table's own DEFAULT never applies).
type Column struct {
	Name    string
	Remap   bool
	Default any
}

// Table binds an export file to the query that fills it and the insert that
// restores it. Import inserts happen in Tables() order, so parents precede
// children.
type Table struct {
	File    string
	Name    string // SQL table name for import
	Query   string
	Columns []Column
}

func cols(spec ...Column) []Column { return spec }
func c(name string) Column         { return Column{Name: name} }
func id(name string) Column        { return Column{Name: name, Remap: true} }

// def is c() with a fallback for rows from archives older than the column.
func def(name string, fallback any) Column {
	return Column{Name: name, Default: fallback}
}

// Tables returns the full export/import specification in dependency order.
func Tables() []Table {
	return []Table{
		{
			File: "users.json",
			Name: "users",
			Query: `SELECT id, email, username, display_name, bio, avatar_url, links, role,
				suspended_at, created_at, updated_at FROM users WHERE username != '_system'`,
			// `links` is the same shape as a patch's, and a patch's travelled
			// while a person's did not (docs/adr/006). A profile arrived on
			// the fork with its bio and no way to reach anybody.
			Columns: cols(id("id"), c("email"), c("username"), c("display_name"),
				c("bio"), c("avatar_url"), def("links", "[]"), c("role"), c("suspended_at"),
				c("created_at"), c("updated_at")),
		},
		{
			File:    "tags.json",
			Name:    "tags",
			Query:   `SELECT id, name, motif, created_at FROM tags`,
			Columns: cols(id("id"), c("name"), c("motif"), c("created_at")),
		},
		{
			File: "nodes.json",
			Name: "nodes",
			Query: `SELECT id, owner_id, name, slug, description, latitude, longitude,
				address, website, image_url, image_alt, links, visibility, membership_policy, status, archived_from, appearance,
				follower_permissions, governance_config, governance_setup_complete,
				designated_successor_id, accept_event_suggestions,
				submitted_by, submission_source, did, created_at, updated_at
				FROM nodes WHERE removed_at IS NULL`,
			Columns: cols(id("id"), id("owner_id"), c("name"), c("slug"),
				c("description"), c("latitude"), c("longitude"), c("address"),
				c("website"),
				// A patch's image is a URL it owns, so it travels like any
				// other field (docs/adr/007). The bytes were never ours to
				// move, which is what makes this the easy case.
				def("image_url", ""), def("image_alt", ""),
				c("links"), c("visibility"), c("membership_policy"),
				c("status"), c("archived_from"), c("appearance"), c("follower_permissions"),
				c("governance_config"), c("governance_setup_complete"),
				// A named successor is a governance fact about the patch and
				// travels with its memberships (docs/adr/051). id() so it is
				// remapped like every other user reference — a raw user id
				// from the old instance would point at a stranger here.
				id("designated_successor_id"),
				// Whether non-members may suggest events here (docs/adr/026):
				// an admin's choice about their own door, and a patch that had
				// closed it found it open again on the fork.
				def("accept_event_suggestions", 1),
				// The patch's did:web identity (docs/adr/062). It travels while
				// verification_domain beside it does not, and the asymmetry is
				// the point: verification_domain is the OLD instance's vetting
				// judgement, which a fork has no business inheriting, whereas a
				// did:web value names its own document's location. The fork can
				// re-prove it in one request against the community's own domain,
				// trusting nobody here. Being self-locating is what makes a DID
				// portable, and it is the second reason 062 refused did:plc:
				// that one needs plc.directory to say what it means.
				id("submitted_by"), c("submission_source"),
				def("did", ""),
				c("created_at"), c("updated_at")),
		},
		{
			File:    "node_tags.json",
			Name:    "node_tags",
			Query:   `SELECT node_id, tag_id, position FROM node_tags`,
			Columns: cols(id("node_id"), id("tag_id"), c("position")),
		},
		{
			File: "memberships.json",
			Name: "memberships",
			Query: `SELECT id, user_id, node_id, role, status, visible, joined_at
				FROM memberships`,
			// `visible` is the member's own switch (docs/adr/006), and it
			// defaults to 1. Leaving it behind meant a fork re-exposed every
			// membership somebody had chosen to hide — on their profile and
			// in the patch's public member list at once, since one switch
			// drives both. A seamrip is the moment that choice matters most:
			// it is what a community does when its leadership goes sideways.
			Columns: cols(id("id"), id("user_id"), id("node_id"), c("role"),
				c("status"), def("visible", 1), c("joined_at")),
		},
		{
			// The council's chairs (docs/adr/051). A seat outlives its holder,
			// and `term_ends_at` is the patch's election calendar: dueness is
			// derived from it rather than stored, so a fork arriving with no
			// seats never schedules another election — the safety valve would
			// have stripped the machinery that rotates leadership. Beside
			// memberships because that is what a seat is a fact about.
			//
			// A vacant seat travels too: holder_id NULL is a chair waiting to
			// be contested, which the next election needs to know about.
			File: "seats.json",
			Name: "seats",
			Query: `SELECT id, node_id, holder_id, term_ends_at, created_at
				FROM seats`,
			Columns: cols(id("id"), id("node_id"), id("holder_id"),
				c("term_ends_at"), c("created_at")),
		},
		{
			File: "aggregators.json",
			Name: "aggregators",
			// Ahead of event_sources: a crosswalk entry references its
			// aggregator, and Import retries only within a table.
			//
			// paused is exported as 1, not as it stands: a fork inherits
			// the crosswalk's labour but not the decision to let an
			// outside feed write onto its patches (docs/adr/056). The new
			// steward attaches it themselves, or it never fetches.
			Query: `SELECT id, name, type, url, added_by, 1 AS paused,
				created_at, updated_at FROM aggregators`,
			Columns: cols(id("id"), c("name"), c("type"), c("url"),
				id("added_by"), c("paused"), c("created_at"), c("updated_at")),
		},
		{
			File: "event_sources.json",
			Name: "event_sources",
			// Feed URLs (a Google Calendar secret address is one) are
			// quasi-secrets; the admin seamrip is already a custody
			// transfer (docs/adr/012), so they travel. Fetch state stays
			// behind — the fork re-syncs from scratch.
			Query: `SELECT id, node_id, type, url, added_by, aggregator_id, name_key,
				suggests, created_at, updated_at FROM event_sources`,
			// aggregator_id + name_key make a row a crosswalk entry
			// (docs/adr/056). They travel because the crosswalk is dozens
			// of names mapped by hand — community labour, and the reason
			// seamrip exists. What does not travel is the standing to act
			// on them: the aggregator arrives paused.
			//
			// suggests travels with them and must: losing it would turn
			// every suggesting entry into a publishing one, and the fork
			// would publish onto patches that only ever agreed to be
			// asked.
			Columns: cols(id("id"), id("node_id"), c("type"), c("url"),
				id("added_by"), id("aggregator_id"), c("name_key"),
				def("suggests", 0), c("created_at"), c("updated_at")),
		},
		{
			File: "aggregator_ignored_names.json",
			Name: "aggregator_ignored_names",
			// Judging that "PA" names no organization is the same act of
			// curation as judging that "Binns Park" does, so it travels
			// with the crosswalk (docs/adr/056). A fork that lost it
			// would re-derive the same list by hand.
			Query: `SELECT aggregator_id, name_key, ignored_by, created_at
				FROM aggregator_ignored_names`,
			Columns: cols(id("aggregator_id"), c("name_key"), id("ignored_by"),
				c("created_at")),
		},
		{
			File: "aggregator_programs.json",
			Name: "aggregator_programs",
			// Recognizing that a venue's listed tour is the historical
			// society's is knowledge the feed does not contain and a
			// machine cannot re-derive — the organization is named nowhere
			// in it (docs/adr/063). Losing this on a fork would mean
			// recognizing every program again from memory, which is the
			// same loss as losing the crosswalk and travels for the same
			// reason.
			//
			// backfilled_at stays behind, so the fork's first pass is
			// silent back-fill rather than a notification per offer. That
			// is docs/adr/056's rule read through a restore: nobody wants
			// the fork's opening act to be forty announcements.
			Query: `SELECT id, aggregator_id, name_key, title_key, display_title,
				node_id, credited_by, created_at FROM aggregator_programs`,
			Columns: cols(id("id"), id("aggregator_id"), c("name_key"),
				c("title_key"), c("display_title"), id("node_id"),
				id("credited_by"), c("created_at")),
		},
		{
			File: "event_source_skips.json",
			Name: "event_source_skips",
			Query: `SELECT source_id, uid, occurrence, created_at
				FROM event_source_skips`,
			Columns: cols(id("source_id"), c("uid"), c("occurrence"),
				c("created_at")),
		},
		{
			File: "events.json",
			Name: "events",
			Query: `SELECT id, node_id, created_by, title, description, location,
				latitude, longitude, starts_at, ends_at, recurrence, visibility,
				image_url, image_alt,
				source_id, source_uid, source_occurrence,
				created_at, updated_at FROM events
				WHERE removed_at IS NULL AND status = 'active'`,
			Columns: cols(id("id"), id("node_id"), id("created_by"), c("title"),
				c("description"), c("location"), c("latitude"), c("longitude"),
				c("starts_at"), c("ends_at"), c("recurrence"), c("visibility"),
				def("image_url", ""), def("image_alt", ""),
				id("source_id"), c("source_uid"), def("source_occurrence", ""),
				c("created_at"), c("updated_at")),
		},
		{
			// An event's other patches (docs/adr/032). A link is a mutual
			// association two sets of admins each consented to — community
			// data by the same argument memberships are, and both ends already
			// travel. Only confirmed links: a pending one is invisible
			// everywhere, and a fork cannot carry a handshake nobody finished.
			File: "event_links.json",
			Name: "event_links",
			Query: `SELECT id, event_id, node_id, status, initiated_by, requested_by,
				absorb_event_id, created_at, confirmed_at
				FROM event_links WHERE status = 'confirmed'`,
			Columns: cols(id("id"), id("event_id"), id("node_id"), c("status"),
				c("initiated_by"), id("requested_by"), id("absorb_event_id"),
				c("created_at"), c("confirmed_at")),
		},
		{
			// Offers a credited patch declined (docs/adr/063). After events,
			// because a dismissal points at one and Import retries only
			// within a table. A refusal is that patch's own judgement, and a
			// fork that dropped it would re-offer every declined event on its
			// first sync and owe the same refusal again.
			File: "aggregator_offer_dismissals.json",
			Name: "aggregator_offer_dismissals",
			// Only dismissals whose event survived the export cut — events
			// travel active-only, and a dismissal pointing at a dropped
			// event would arrive orphaned.
			Query: `SELECT d.program_id, d.event_id, d.dismissed_by, d.created_at
				FROM aggregator_offer_dismissals d
				JOIN events e ON e.id = d.event_id
				WHERE e.removed_at IS NULL AND e.status = 'active'`,
			Columns: cols(id("program_id"), id("event_id"), id("dismissed_by"),
				c("created_at")),
		},
		{
			// Doorways to patches on other quilts (docs/adr/032). Stored as
			// host and slug rather than ids, so they survive a fork pointing at
			// the same remote patches they always did.
			File: "event_mentions.json",
			Name: "event_mentions",
			Query: `SELECT id, event_id, host, slug, name, created_at
				FROM event_mentions`,
			Columns: cols(id("id"), id("event_id"), c("host"), c("slug"),
				c("name"), c("created_at")),
		},
		{
			File: "proposals.json",
			Name: "proposals",
			Query: `SELECT id, node_id, author_id, title, body, status, state,
				proposal_type, duration_hours, voting_ends_at, voting_terms,
				target_doc, target_user_id, seats_contested, nominations_close_at,
				proposed_title, proposed_body, applied_at, applied_by,
				created_at, updated_at FROM proposals`,
			Columns: cols(id("id"), id("node_id"), id("author_id"), c("title"),
				c("body"), c("status"), c("state"), c("proposal_type"),
				c("duration_hours"), c("voting_ends_at"),
				// The rules a vote is judged by, fixed when it opened
				// (docs/adr/047). Without it a fork's in-flight votes fall back
				// to the new instance's live config — a vote finishing under
				// different rules than the ones people cast ballots under,
				// which is the exact failure the photograph was added to stop.
				c("voting_terms"),
				c("target_doc"),
				// The person a nomination is about travels remapped, like
				// every other user reference (docs/adr/051).
				id("target_user_id"),
				// What makes an election an election (docs/adr/051). A proposal
				// carrying candidates but `seats_contested = 0` is not read as
				// an election by anything — electionPhase returns empty, the
				// panel does not render, and the ballot below it becomes
				// orphaned rows. def(), because archives written before
				// migration 050 have neither key.
				def("seats_contested", 0), c("nominations_close_at"),
				c("proposed_title"), c("proposed_body"), c("applied_at"),
				id("applied_by"), c("created_at"), c("updated_at")),
		},
		{
			// A community's record of what it decided elsewhere travels with
			// it (docs/adr/052) — it is community data in the plainest sense,
			// and a fork that lost its governance history would lose the only
			// account of how its leadership was chosen.
			File: "attestations.json",
			Name: "attestations",
			Query: `SELECT id, node_id, kind, decided_at, term_ends_at, summary,
				recorded_by, created_at, supersedes_id FROM attestations`,
			Columns: cols(id("id"), id("node_id"), c("kind"), c("decided_at"),
				c("term_ends_at"), c("summary"), id("recorded_by"), c("created_at"),
				id("supersedes_id")),
		},
		{
			// What a meeting adopted, and the text it adopted (docs/adr/053).
			// `adopted_body` travels because the fork's git repos do not: the
			// charter carries only its latest text, so without this the
			// record of an earlier adoption would arrive empty.
			File: "amendment_attestations.json",
			Name: "amendment_attestations",
			Query: `SELECT id, node_id, doc_id, target_doc, doc_title, decided_at,
				summary, adopted_body, git_sha, recorded_by, created_at
				FROM amendment_attestations`,
			// git_sha is plain, not remapped: it names a commit in the old
			// instance's repo, which the fork does not have. Kept as the
			// provenance it is rather than dropped.
			Columns: cols(id("id"), id("node_id"), id("doc_id"), c("target_doc"),
				c("doc_title"), c("decided_at"), c("summary"), c("adopted_body"),
				c("git_sha"), id("recorded_by"), c("created_at")),
		},
		{
			File: "attestation_names.json",
			Name: "attestation_names",
			Query: `SELECT id, attestation_id, user_id, display_name, position
				FROM attestation_names`,
			// user_id is nullable and remapped: an unrealized name has none,
			// and a realized one must point at the imported person rather than
			// a stranger with the old instance's id.
			Columns: cols(id("id"), id("attestation_id"), id("user_id"),
				c("display_name"), c("position")),
		},
		{
			File:  "votes.json",
			Name:  "votes",
			Query: `SELECT id, proposal_id, user_id, value, created_at FROM votes`,
			Columns: cols(id("id"), id("proposal_id"), id("user_id"), c("value"),
				c("created_at")),
		},
		{
			// Who is standing in an election (docs/adr/051). Before
			// `election_ballots` below, because a ballot names a candidate and
			// the FK is enforced at insert.
			File: "election_candidates.json",
			Name: "election_candidates",
			Query: `SELECT id, proposal_id, user_id, created_at
				FROM election_candidates`,
			Columns: cols(id("id"), id("proposal_id"), id("user_id"), c("created_at")),
		},
		{
			// An approval ballot is rows rather than a value (docs/adr/051), so
			// it lives here instead of in `votes`. Same rule either way: ADR 002
			// travels "proposals with raw votes", and an election's votes are
			// no less raw for being shaped differently. Without them a forked
			// election shows a slate and no tally — a governance record that
			// says a contest happened and cannot say what it decided.
			File: "election_ballots.json",
			Name: "election_ballots",
			Query: `SELECT id, proposal_id, voter_id, candidate_id, created_at
				FROM election_ballots`,
			Columns: cols(id("id"), id("proposal_id"), id("voter_id"),
				id("candidate_id"), c("created_at")),
		},
		{
			File: "proposal_comments.json",
			Name: "proposal_comments",
			Query: `SELECT id, proposal_id, parent_id, author_id, body, created_at,
				updated_at FROM proposal_comments`,
			Columns: cols(id("id"), id("proposal_id"), id("parent_id"),
				id("author_id"), c("body"), c("created_at"), c("updated_at")),
		},
		{
			File: "comment_reactions.json",
			Name: "comment_reactions",
			Query: `SELECT id, comment_id, user_id, emoji, created_at
				FROM comment_reactions`,
			Columns: cols(id("id"), id("comment_id"), id("user_id"), c("emoji"),
				c("created_at")),
		},
		{
			File: "proposal_revisions.json",
			Name: "proposal_revisions",
			Query: `SELECT id, proposal_id, title, body, proposed_body,
				revision_number, author_id, change_note, created_at
				FROM proposal_revisions`,
			Columns: cols(id("id"), id("proposal_id"), c("title"), c("body"),
				c("proposed_body"), c("revision_number"), id("author_id"),
				c("change_note"), c("created_at")),
		},
		{
			File: "governance.json",
			Name: "governance_docs",
			Query: `SELECT id, node_id, title, body, kind, visibility, version, created_by,
				created_at, updated_at FROM governance_docs`,
			Columns: cols(id("id"), id("node_id"), c("title"), c("body"),
				c("kind"), c("visibility"), c("version"), id("created_by"),
				c("created_at"), c("updated_at")),
		},
		{
			File: "claim_requests.json",
			Name: "claim_requests",
			// verification_token is a secret and stays behind.
			Query: `SELECT id, node_id, user_id, method, evidence, status,
				reviewed_by, review_note, created_at, updated_at FROM claim_requests`,
			Columns: cols(id("id"), id("node_id"), id("user_id"), c("method"),
				c("evidence"), c("status"), id("reviewed_by"), c("review_note"),
				c("created_at"), c("updated_at")),
		},
		{
			File: "notification_preferences.json",
			Name: "notification_preferences",
			Query: `SELECT id, user_id, notification_type, channel, enabled,
				created_at, updated_at FROM notification_preferences`,
			Columns: cols(id("id"), id("user_id"), c("notification_type"),
				c("channel"), c("enabled"), c("created_at"), c("updated_at")),
		},
		{
			File: "patch_notification_config.json",
			Name: "patch_notification_config",
			Query: `SELECT id, node_id, category, enabled, created_at, updated_at
				FROM patch_notification_config`,
			Columns: cols(id("id"), id("node_id"), c("category"), c("enabled"),
				c("created_at"), c("updated_at")),
		},
	}
}

// Export runs every table query and hands the rows to sink, one call per
// table, in Tables() order.
func Export(db *database.DB, sink func(t Table, items []map[string]any) error) error {
	for _, t := range Tables() {
		items, err := queryTable(db, t)
		if err != nil {
			return err
		}
		if err := sink(t, items); err != nil {
			return err
		}
	}
	return nil
}

func queryTable(db *database.DB, t Table) ([]map[string]any, error) {
	rows, err := db.Query(t.Query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(t.Columns))
		ptrs := make([]any, len(t.Columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		item := make(map[string]any, len(t.Columns))
		for i, col := range t.Columns {
			// SQLite TEXT scans as []byte through the generic path.
			if b, ok := values[i].([]byte); ok {
				item[col.Name] = string(b)
			} else {
				item[col.Name] = values[i]
			}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ReadmeText documents the archive layout for humans opening the export.
const ReadmeText = `Patchwork Data Export (Seamrip)
===============================

This archive contains the portable data of a Patchwork instance: patches,
people, memberships, council seats and their terms, events (with the
patches they are linked to and the calendar feeds they were pulled from),
proposals with votes — election candidates and approval ballots included
— records of what was decided elsewhere, governance documents and
discussion, claims, and notification preferences.

Deliberately NOT included: credentials, sessions, recovery codes,
invite/magic/signup links, ActivityPub keys and identifiers, remote
followers, audit log, content reports, and the Label (the stewardship
disclosure — docs/adr/023: the fork has different stewards, a different
server, and a different bill, so it writes its own). A new instance mints
its own identity on first boot; import prefills the new Label with a
removable "seamripped from" line pointing back here.

To import into a fresh Patchwork database:
  go run ./cmd/import -db ./new-patchwork.db -in ./export/

Import rewrites every ID (relationships are preserved) and writes the
old-to-new mapping to id_map.json. People sign in again on the new instance
via magic link (same email) or a fresh invite link.
`
