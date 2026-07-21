// Package seamrip defines the data-portability boundary of a Patchwork
// instance: which tables leave in an export and how they come back in an
// import. It is the single source of truth used by the admin zip endpoint
// (GET /api/v1/admin/export), the export CLI, and the import CLI.
//
// Scope: community data travels; instance identity does not.
//
//   - Included: user profiles (with email and instance role, so a fork can
//     re-authenticate its people), patches, memberships (the shared-member
//     overlap that threads and the quilt are inferred from), tags, events
//     with their provenance, event sources and their skip lists (feed URLs
//     are quasi-secrets, but the admin seamrip is already a custody
//     transfer), proposals with raw votes, governance docs, proposal
//     comments / reactions / revisions, claim requests, and notification
//     preferences.
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
type Column struct {
	Name  string
	Remap bool
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

// Tables returns the full export/import specification in dependency order.
func Tables() []Table {
	return []Table{
		{
			File: "users.json",
			Name: "users",
			Query: `SELECT id, email, username, display_name, bio, avatar_url, role,
				suspended_at, created_at, updated_at FROM users WHERE username != '_system'`,
			Columns: cols(id("id"), c("email"), c("username"), c("display_name"),
				c("bio"), c("avatar_url"), c("role"), c("suspended_at"),
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
				address, website, links, visibility, membership_policy, status, appearance,
				follower_permissions, governance_config, governance_setup_complete,
				submitted_by, submission_source, created_at, updated_at
				FROM nodes WHERE removed_at IS NULL`,
			Columns: cols(id("id"), id("owner_id"), c("name"), c("slug"),
				c("description"), c("latitude"), c("longitude"), c("address"),
				c("website"), c("links"), c("visibility"), c("membership_policy"),
				c("status"), c("appearance"), c("follower_permissions"),
				c("governance_config"), c("governance_setup_complete"),
				id("submitted_by"), c("submission_source"),
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
			Query: `SELECT id, user_id, node_id, role, status, joined_at
				FROM memberships`,
			Columns: cols(id("id"), id("user_id"), id("node_id"), c("role"),
				c("status"), c("joined_at")),
		},
		{
			File: "event_sources.json",
			Name: "event_sources",
			// Feed URLs (a Google Calendar secret address is one) are
			// quasi-secrets; the admin seamrip is already a custody
			// transfer (docs/adr/012), so they travel. Fetch state stays
			// behind — the fork re-syncs from scratch.
			Query: `SELECT id, node_id, type, url, added_by, created_at,
				updated_at FROM event_sources`,
			Columns: cols(id("id"), id("node_id"), c("type"), c("url"),
				id("added_by"), c("created_at"), c("updated_at")),
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
				source_id, source_uid, source_occurrence,
				created_at, updated_at FROM events
				WHERE removed_at IS NULL AND status = 'active'`,
			Columns: cols(id("id"), id("node_id"), id("created_by"), c("title"),
				c("description"), c("location"), c("latitude"), c("longitude"),
				c("starts_at"), c("ends_at"), c("recurrence"), c("visibility"),
				id("source_id"), c("source_uid"), c("source_occurrence"),
				c("created_at"), c("updated_at")),
		},
		{
			File: "proposals.json",
			Name: "proposals",
			Query: `SELECT id, node_id, author_id, title, body, status, state,
				proposal_type, duration_hours, voting_ends_at, target_doc,
				proposed_title, proposed_body, applied_at, applied_by,
				created_at, updated_at FROM proposals`,
			Columns: cols(id("id"), id("node_id"), id("author_id"), c("title"),
				c("body"), c("status"), c("state"), c("proposal_type"),
				c("duration_hours"), c("voting_ends_at"), c("target_doc"),
				c("proposed_title"), c("proposed_body"), c("applied_at"),
				id("applied_by"), c("created_at"), c("updated_at")),
		},
		{
			File:  "votes.json",
			Name:  "votes",
			Query: `SELECT id, proposal_id, user_id, value, created_at FROM votes`,
			Columns: cols(id("id"), id("proposal_id"), id("user_id"), c("value"),
				c("created_at")),
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
			Query: `SELECT id, node_id, title, body, version, created_by,
				created_at, updated_at FROM governance_docs`,
			Columns: cols(id("id"), id("node_id"), c("title"), c("body"),
				c("version"), id("created_by"), c("created_at"), c("updated_at")),
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
people, memberships, events, event sources (the calendar feeds patches
pull from), proposals with votes, governance documents and discussion,
claims, and notification preferences.

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
