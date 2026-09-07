package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// Personal export (docs/adr/012, affordance 1): everything about the person
// asking, in one JSON document, with no admin involved. It is the data-rights
// baseline every member holds — the admin seamrip
// (GET /api/v1/admin/export) is a custody transfer of the whole community,
// and a member should never have to ask for their own record.
//
// The boundary rule of ADR 012 reads, here, as: this file carries the rows a
// person WROTE or that describe THEM, and nothing else. Content authored by
// other people is out of scope even when it names them — a reply under their
// notice is the replier's, and it is not in this file.
//
// # What is deliberately absent, and why
//
// Authentication material never travels. `credentials` (passkey public keys
// and sign counts), `sessions`, `recovery_codes`, `magic_links`,
// `invite_links`, `signup_tokens`, and the `users` columns
// `feed_secret_hash`, `private_key`, `public_key` are all either secrets or
// the material for presenting as this account. An export is a file that gets
// copied to a laptop, a USB stick, and an email; nothing in it should help
// anybody get in. This is the same line ADR 002 draws for the admin seamrip,
// drawn tighter: the seamrip at least carries public keys so a fork can keep
// an actor, and a person carrying their own record needs none of it.
//
// Moderation the person PERFORMED is absent for a different reason:
// `content_reports.reviewed_by` and `claim_requests.reviewed_by` record a
// decision about somebody else's content or claim. The judgement is the
// instance's record of how it handled a third party, not a fact about the
// reviewer, and the reported material certainly is not theirs to carry.
// Reports the person FILED are here — those they wrote.
//
// There is no RSVP table in this schema. Attendance is not recorded by
// Patchwork; `events.event_url` points out at whatever form the organizer
// uses (docs/adr/079), so there is nothing here to export.
//
// ActivityPub plumbing (`ap_followers`, `ap_following`, `ap_outbox_queue`)
// keys on actor URLs rather than user ids and holds delivery state rather
// than anybody's record, so it has no personal rows to find.

// personalExportSection is one top-level key of the export document: a name
// and the query that fills it. Every `?` in Query binds the requesting user's
// id, so a section can never be written to reach another person's rows —
// Args says how many times, and that is the only parameter a section has.
type personalExportSection struct {
	Key   string
	Args  int
	Query string
}

// personalExportSections lists every block of the document, in the order it
// is written. Each query names its columns explicitly rather than using
// SELECT *, so a column added later (a token, a hash, an internal flag)
// cannot join the export by accident.
func personalExportSections() []personalExportSection {
	return []personalExportSection{
		{
			// Every membership, INCLUDING the ones hidden from the public
			// profile and member list (docs/adr/006). The visibility switch
			// is the person's own, so the hidden ones are theirs to carry —
			// ADR 012 says so in as many words.
			Key: "memberships", Args: 1,
			Query: `SELECT m.id, m.node_id, n.slug AS node_slug, n.name AS node_name,
				m.role, m.status, m.visible, m.share_contact, m.join_message, m.joined_at
				FROM memberships m JOIN nodes n ON n.id = m.node_id
				WHERE m.user_id = ? ORDER BY m.joined_at`,
		},
		{
			// A patch is the community's record, not one person's, so only
			// the reference travels: enough to name what they hold, not a
			// copy of everybody's patch.
			Key: "patches_owned", Args: 1,
			Query: `SELECT id, slug, name, status, visibility, created_at
				FROM nodes WHERE owner_id = ? ORDER BY created_at`,
		},
		{
			Key: "patches_submitted", Args: 1,
			Query: `SELECT id, slug, name, status, submission_source, created_at
				FROM nodes WHERE submitted_by = ? ORDER BY created_at`,
		},
		{
			// Being named somebody's successor is a fact about this person
			// (docs/adr/051), and one they may not have been told twice.
			Key: "successor_for_patches", Args: 1,
			Query: `SELECT id, slug, name FROM nodes
				WHERE designated_successor_id = ? ORDER BY name`,
		},
		{
			Key: "seats_held", Args: 1,
			Query: `SELECT s.id, s.node_id, n.slug AS node_slug, s.term_ends_at, s.created_at
				FROM seats s JOIN nodes n ON n.id = s.node_id
				WHERE s.holder_id = ? ORDER BY s.created_at`,
		},
		{
			// proposed_body is the amendment text this person wrote, so it
			// travels with the proposal that carries it. git_sha / base_sha /
			// proposed_branch name commits in a repo that is not this file's
			// to describe, and ap_id names an object on this domain.
			Key: "proposals_authored", Args: 1,
			Query: `SELECT p.id, p.node_id, n.slug AS node_slug, p.title, p.body, p.status, p.state,
				p.proposal_type, p.duration_hours, p.voting_ends_at, p.voting_terms,
				p.target_doc, p.target_user_id, p.seats_contested, p.nominations_close_at,
				p.proposed_title, p.proposed_body, p.applied_at, p.created_at, p.updated_at
				FROM proposals p JOIN nodes n ON n.id = p.node_id
				WHERE p.author_id = ? ORDER BY p.created_at`,
		},
		{
			Key: "proposals_applied", Args: 1,
			Query: `SELECT id, node_id, title, applied_at FROM proposals
				WHERE applied_by = ? ORDER BY applied_at`,
		},
		{
			// A vote is the plainest thing a person owns here.
			Key: "votes", Args: 1,
			Query: `SELECT v.id, v.proposal_id, p.title AS proposal_title, v.value, v.created_at
				FROM votes v JOIN proposals p ON p.id = v.proposal_id
				WHERE v.user_id = ? ORDER BY v.created_at`,
		},
		{
			Key: "election_candidacies", Args: 1,
			Query: `SELECT c.id, c.proposal_id, p.title AS proposal_title, c.created_at
				FROM election_candidates c JOIN proposals p ON p.id = c.proposal_id
				WHERE c.user_id = ? ORDER BY c.created_at`,
		},
		{
			// An approval ballot is rows rather than a value (docs/adr/051),
			// so the person's ballot is the set of candidates they approved.
			Key: "election_ballots", Args: 1,
			Query: `SELECT b.id, b.proposal_id, b.candidate_id, b.created_at
				FROM election_ballots b WHERE b.voter_id = ? ORDER BY b.created_at`,
		},
		{
			Key: "proposal_comments", Args: 1,
			Query: `SELECT id, proposal_id, parent_id, body, created_at, updated_at
				FROM proposal_comments WHERE author_id = ? ORDER BY created_at`,
		},
		{
			Key: "comment_reactions", Args: 1,
			Query: `SELECT id, comment_id, emoji, created_at
				FROM comment_reactions WHERE user_id = ? ORDER BY created_at`,
		},
		{
			Key: "proposal_revisions", Args: 1,
			Query: `SELECT id, proposal_id, title, body, proposed_body, revision_number,
				change_note, created_at
				FROM proposal_revisions WHERE author_id = ? ORDER BY created_at`,
		},
		{
			// The noticeboard (docs/adr/081). A notice is readable only
			// inside its patch, and this is its author carrying their own
			// copy out — not a way to read the room from elsewhere, since
			// the query never leaves author_id.
			Key: "notices", Args: 1,
			Query: `SELECT nt.id, nt.node_id, n.slug AS node_slug, nt.title, nt.body,
				nt.image_url, nt.image_alt, nt.replies_open, nt.members_told,
				nt.created_at, nt.updated_at
				FROM notices nt JOIN nodes n ON n.id = nt.node_id
				WHERE nt.author_id = ? ORDER BY nt.created_at`,
		},
		{
			Key: "notice_replies", Args: 1,
			Query: `SELECT id, notice_id, body, created_at, updated_at
				FROM notice_replies WHERE author_id = ? ORDER BY created_at`,
		},
		{
			// Events they put up. Imported rows carry their source ids so
			// the person can tell what they typed from what a feed filled in
			// under their name (docs/adr/031).
			Key: "events_created", Args: 1,
			Query: `SELECT e.id, e.node_id, n.slug AS node_slug, e.title, e.description,
				e.location, e.latitude, e.longitude, e.starts_at, e.ends_at, e.timezone,
				e.recurrence, e.visibility, e.status, e.event_url, e.image_url, e.image_alt,
				e.source_id, e.source_uid, e.created_at, e.updated_at
				FROM events e JOIN nodes n ON n.id = e.node_id
				WHERE e.created_by = ? ORDER BY e.starts_at`,
		},
		{
			// Links they asked for between an event and another patch
			// (docs/adr/032), confirmed or not: the request was theirs.
			Key: "event_links_requested", Args: 1,
			Query: `SELECT id, event_id, node_id, status, initiated_by, created_at, confirmed_at
				FROM event_links WHERE requested_by = ? ORDER BY created_at`,
		},
		{
			// What they recorded about a decision taken somewhere Patchwork
			// was not (docs/adr/052, docs/adr/053).
			Key: "attestations_recorded", Args: 1,
			Query: `SELECT id, node_id, kind, decided_at, term_ends_at, summary, created_at
				FROM attestations WHERE recorded_by = ? ORDER BY created_at`,
		},
		{
			Key: "amendment_attestations_recorded", Args: 1,
			Query: `SELECT id, node_id, doc_id, target_doc, doc_title, decided_at,
				summary, adopted_body, git_sha, created_at
				FROM amendment_attestations WHERE recorded_by = ? ORDER BY created_at`,
		},
		{
			// Where an attestation names them as one of the people a meeting
			// seated. Somebody else wrote the record; being in it is a fact
			// about this person, and the row is only the name and its place.
			Key: "attestation_names", Args: 1,
			Query: `SELECT an.id, an.attestation_id, a.node_id, an.display_name, an.position
				FROM attestation_names an JOIN attestations a ON a.id = an.attestation_id
				WHERE an.user_id = ? ORDER BY an.position`,
		},
		{
			// Claims they filed (docs/adr/030). verification_token and the
			// email-token expiry stay behind: they are live secrets the
			// instance issued, not a record of what the person asked for.
			Key: "claims_filed", Args: 1,
			Query: `SELECT c.id, c.node_id, n.slug AS node_slug, c.method, c.evidence,
				c.status, c.email, c.review_note, c.created_at, c.updated_at
				FROM claim_requests c JOIN nodes n ON n.id = c.node_id
				WHERE c.user_id = ? ORDER BY c.created_at`,
		},
		{
			// Calendar feeds they attached (docs/adr/031). The URL is a
			// quasi-secret, and it is one the person themselves supplied to
			// a patch they administer.
			Key: "event_sources_added", Args: 1,
			Query: `SELECT s.id, s.node_id, n.slug AS node_slug, s.type, s.url, s.name_key,
				s.suggests, s.created_at
				FROM event_sources s JOIN nodes n ON n.id = s.node_id
				WHERE s.added_by = ? ORDER BY s.created_at`,
		},
		{
			Key: "aggregators_added", Args: 1,
			Query: `SELECT id, name, type, url, created_at
				FROM aggregators WHERE added_by = ? ORDER BY created_at`,
		},
		{
			// Crosswalk curation is hours of somebody's judgement
			// (docs/adr/056, docs/adr/063), and the somebody is a person.
			Key: "aggregator_names_ignored", Args: 1,
			Query: `SELECT aggregator_id, name_key, created_at
				FROM aggregator_ignored_names WHERE ignored_by = ? ORDER BY created_at`,
		},
		{
			Key: "aggregator_programs_credited", Args: 1,
			Query: `SELECT id, aggregator_id, name_key, title_key, display_title, node_id, created_at
				FROM aggregator_programs WHERE credited_by = ? ORDER BY created_at`,
		},
		{
			Key: "aggregator_offers_dismissed", Args: 1,
			Query: `SELECT program_id, event_id, created_at
				FROM aggregator_offer_dismissals WHERE dismissed_by = ? ORDER BY created_at`,
		},
		{
			Key: "notification_preferences", Args: 1,
			Query: `SELECT id, notification_type, channel, enabled, created_at, updated_at
				FROM notification_preferences WHERE user_id = ?
				ORDER BY notification_type, channel`,
		},
		{
			// In-app notifications they received. ADR 002 leaves these behind
			// on a seamrip because a fork rebuilds them from what travels;
			// here there is nothing to rebuild them from, and they are the
			// record of what this person was actually told.
			Key: "notifications", Args: 1,
			Query: `SELECT id, type, title, body, link, read_at, created_at
				FROM notifications WHERE user_id = ? ORDER BY created_at`,
		},
		{
			// Connected quilts and cross-quilt follows are the person's own
			// relationships rather than the community's (docs/adr/024) —
			// which is exactly why they stay behind on a seamrip and belong
			// here.
			Key: "connected_quilts", Args: 1,
			Query: `SELECT id, url, name, created_at FROM user_quilts
				WHERE user_id = ? ORDER BY created_at`,
		},
		{
			Key: "remote_follows", Args: 1,
			Query: `SELECT id, quilt_url, node_ap_id, node_slug, node_name, created_at
				FROM remote_follows WHERE user_id = ? ORDER BY quilt_url, created_at`,
		},
		{
			// Reports they filed. reviewed_by and resolution_note are the
			// instance's handling of somebody else's content, not the
			// reporter's record, so neither is here.
			Key: "reports_filed", Args: 1,
			Query: `SELECT id, entity_type, entity_id, node_id, reason, details, status, created_at
				FROM content_reports WHERE reporter_id = ? ORDER BY created_at`,
		},
		{
			// Their steward listing and the words they wrote for it
			// (docs/adr/023).
			Key: "steward_listing", Args: 1,
			Query: `SELECT id, blurb, listed, position, created_at
				FROM label_stewards WHERE user_id = ?`,
		},
		{
			// The audit log rows this person is the ACTOR of: their own
			// sign-ins, their own admin acts. It carries their own IP
			// addresses and nobody else's, and it is the one record of their
			// activity they cannot otherwise read — the admin panel shows it
			// and they do not.
			Key: "activity_log", Args: 1,
			Query: `SELECT id, action, entity_type, entity_id, metadata, ip_address, created_at
				FROM audit_log WHERE user_id = ? ORDER BY created_at`,
		},
	}
}

// personalExportUserColumns is every `users` column a person may see about
// themselves. Named one by one on purpose: the three that are missing —
// private_key, public_key, feed_secret_hash — are secrets, and ap_id/ap_type
// name an actor on this domain rather than telling the person anything.
const personalExportUserColumns = `id, username, display_name, email, bio, avatar_url,
	links, role, trusted_contributor, start_on_my_quilt, hide_amended_linings,
	contact_phone, contact_email, contact_note, suspended_at, created_at, updated_at`

// PersonalExport handles GET /api/v1/users/me/export.
func PersonalExport(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		if !middleware.PersonalExportRateLimit(r) {
			w.Header().Set("Retry-After", "600")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"you have downloaded your data several times just now — try again in a few minutes"}`))
			return
		}

		doc := map[string]any{
			"exported_at": time.Now().UTC().Format(time.RFC3339),
			"instance": map[string]any{
				"name":   settings.EffectiveName(db, cfg),
				"domain": cfg.Instance.Domain,
			},
		}

		profile, err := personalExportProfile(db, user.ID)
		if err != nil {
			http.Error(w, `{"error":"failed to build your export"}`, http.StatusInternalServerError)
			return
		}
		doc["user"] = profile

		for _, section := range personalExportSections() {
			args := make([]any, section.Args)
			for i := range args {
				args[i] = user.ID
			}
			rows, err := queryRowMaps(db, section.Query, args...)
			if err != nil {
				http.Error(w, `{"error":"failed to build your export"}`, http.StatusInternalServerError)
				return
			}
			doc[section.Key] = rows
		}

		auth.LogAuditEvent(db, user.ID, "user.export", "user", user.ID, "{}", clientIP(r))

		filename := fmt.Sprintf("patchwork-%s-%s.json",
			exportFilenameSafe(user.Username), time.Now().UTC().Format("2006-01-02"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(doc)
	}
}

// personalExportProfile reads the person's own `users` row and folds the
// three contact columns into the one card shape the API uses elsewhere
// (docs/adr/080), so the export reads like the product rather than like the
// schema.
func personalExportProfile(db *database.DB, userID string) (map[string]any, error) {
	rows, err := queryRowMaps(db,
		`SELECT `+personalExportUserColumns+` FROM users WHERE id = ?`, userID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("user %s not found", userID)
	}
	profile := rows[0]
	// The card is a set of typed items now (docs/adr/083), each with the
	// patches it is shared into. A personal export is the one place a person
	// can read back what they disclosed and to whom, so the shares travel
	// with the items rather than being flattened away.
	items, err := queryRowMaps(db,
		`SELECT id, kind, value, label, position FROM contact_items
		 WHERE user_id = ? ORDER BY position ASC, id ASC`, userID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		shares, err := queryRowMaps(db,
			`SELECT n.slug, n.name FROM contact_item_shares s
			 JOIN nodes n ON n.id = s.node_id
			 WHERE s.item_id = ? ORDER BY n.name`, item["id"])
		if err != nil {
			return nil, err
		}
		if shares == nil {
			shares = []map[string]any{}
		}
		item["shared_with"] = shares
	}
	if items == nil {
		items = []map[string]any{}
	}
	profile["contact_items"] = items
	delete(profile, "contact_phone")
	delete(profile, "contact_email")
	delete(profile, "contact_note")

	// `links` is stored as a JSON string and served as an array everywhere
	// else, so it arrives here as an array too. The block a person reads
	// first should look like the profile they edited, not like the column.
	if raw, ok := profile["links"].(string); ok && raw != "" {
		var parsed any
		if json.Unmarshal([]byte(raw), &parsed) == nil {
			profile["links"] = parsed
		}
	}
	// SQLite has no bool; these three are switches in the UI and read as
	// switches here.
	for _, flag := range []string{"trusted_contributor", "start_on_my_quilt", "hide_amended_linings"} {
		if n, ok := profile[flag].(int64); ok {
			profile[flag] = n != 0
		}
	}
	return profile, nil
}

// queryRowMaps runs a query and returns each row as a column-keyed map, so a
// section is a query and nothing else — no struct to keep in step with it.
// TEXT scans as []byte through the generic path and is converted back.
func queryRowMaps(db *database.DB, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	out := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(names))
		ptrs := make([]any, len(names))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		item := make(map[string]any, len(names))
		for i, name := range names {
			if b, ok := values[i].([]byte); ok {
				item[name] = string(b)
			} else {
				item[name] = values[i]
			}
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// exportFilenameSafe reduces a username to characters a Content-Disposition
// filename can carry unquoted on every platform.
func exportFilenameSafe(username string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, username)
	if safe == "" {
		return "export"
	}
	return safe
}
