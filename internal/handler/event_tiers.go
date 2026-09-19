package handler

import (
	"encoding/json"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// Who an event is for, and what the patch allows
// (docs/adr/2026-09-19-an-event-says-who-it-is-for-within-what-the-patch-allows.md).
//
// An event's visibility is one of three tiers, each named for the role it
// reaches: `public` is anyone, `followers` is the patch's followers, members
// and admins, `members` stops at members and admins. Above it sits the
// patch's own `follower_permissions.events`, which is a ceiling rather than a
// default: where the switch is off, every read path treats a `followers`
// event as `members`. The row is never rewritten — turning the switch back on
// restores what the event always said about itself.
//
// This file is the one place that answers the question, in the two shapes the
// read paths need it: canReadEvent for a single event already loaded, and
// eventVisibleSQL for a listing that has to decide it row by row. They must
// agree, so they are written beside each other.
//
// Three things it deliberately never consults. It never looks at event_links:
// a confirmed link grants a patch presence on somebody else's event, not the
// right to widen who may read it. It never looks at nodes.visibility: a
// private patch is unlisted rather than locked (CONTEXT.md), which is a
// discovery rule and not this one. And it gives an instance admin holding no
// role on the patch exactly the answer it gives a stranger — there is no
// bypass here, and TestEveryInstanceAdminReachIsDeclared would want to know
// about one.

// followerEventsAllowedSQL renders `follower_permissions.events` as a boolean
// expression over the nodes row aliased `alias`.
//
// Absent means allowed, matching scanFollowerPermissions and
// followerMayJoinProposals: the shipped default is the box ticked, and a
// patch that has never opened the rules editor stores "{}" and has said
// nothing. Malformed JSON means allowed too, via the json_valid guard —
// json_extract would otherwise abort the whole query, and a listing that
// 500s because one patch's rules blob is garbage is worse than a listing
// that shows that patch's followers events.
func followerEventsAllowedSQL(alias string) string {
	return `CASE WHEN json_valid(COALESCE(` + alias + `.follower_permissions, '')) ` +
		`THEN COALESCE(json_extract(` + alias + `.follower_permissions, '$.events'), 1) ` +
		`ELSE 1 END = 1`
}

// followerMayReadEvents reports whether a follower of nodeID gets that
// patch's `followers` events — the Go half of followerEventsAllowedSQL, with
// the same defaults for the same reasons.
func followerMayReadEvents(db *database.DB, nodeID string) bool {
	var fpJSON string
	db.QueryRow("SELECT COALESCE(follower_permissions,'{}') FROM nodes WHERE id = ?", nodeID).Scan(&fpJSON)
	fp := model.FollowerPermissions{Events: true, Proposals: true, Charters: true, Members: true}
	// A bad blob leaves the permissive defaults above, deliberately.
	json.Unmarshal([]byte(fpJSON), &fp)
	return fp.Events
}

// canReadEvent reports whether user may read an event at tier `visibility`
// hosted at nodeID.
//
// Replaces canReadNonPublicEvent, which asked the same question of a binary
// column and so needed no tier argument. Its callers (GetEvent, EventICS) now
// hand it the tier and let it decide the public case too, so nothing outside
// this file compares a visibility string to "public" to work out whether to
// ask.
func canReadEvent(db *database.DB, user *model.User, nodeID, visibility string) bool {
	if visibility == "public" {
		return true
	}
	if user == nil {
		return false
	}
	var role string
	if err := db.QueryRow(
		"SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active'",
		user.ID, nodeID,
	).Scan(&role); err != nil {
		return false
	}
	switch role {
	case "admin", "member":
		return true
	case "follower":
		return visibility == "followers" && followerMayReadEvents(db, nodeID)
	}
	return false
}

// eventVisibleSQL is canReadEvent as a WHERE fragment, for the listings that
// decide a page of events at a time. `alias` is the events row; the fragment
// carries its own nodes join, so it composes with any query that has an
// events alias in scope and needs nothing else from the caller.
//
// Returns the args to append in the same call, because the conditions and the
// args of every caller here are appended in lockstep and splitting them is
// how a placeholder ends up bound to a timezone.
func eventVisibleSQL(alias string, user *model.User) (string, []interface{}) {
	if user == nil {
		return alias + ".visibility = 'public'", nil
	}
	return `(` + alias + `.visibility = 'public' OR EXISTS (
		SELECT 1 FROM memberships evm JOIN nodes evn ON evn.id = evm.node_id
		WHERE evm.user_id = ? AND evm.node_id = ` + alias + `.node_id
		  AND evm.status = 'active'
		  AND (evm.role IN ('member','admin')
		       OR (evm.role = 'follower' AND ` + alias + `.visibility = 'followers'
		           AND ` + followerEventsAllowedSQL("evn") + `))))`,
		[]interface{}{user.ID}
}

// eventVisibleToMatchedMembership is the same rule for a query that has
// already joined the membership row it matched on — scope=my and the personal
// ICS feed, which both admit an event through *any* active relationship and
// then have to ask what that particular relationship is worth.
//
// It cannot use eventVisibleSQL: the membership that let the row in may be on
// a patch the event only links to, and the tier is a statement about the
// event's own patch. Hence the `m.node_id = e.node_id` on every branch.
// `memberAlias` is the memberships row, `eventAlias` the events row, and
// `nodeAlias` the event's own patch.
func eventVisibleToMatchedMembership(memberAlias, eventAlias, nodeAlias string) string {
	return `(` + eventAlias + `.visibility = 'public'
		OR (` + memberAlias + `.node_id = ` + eventAlias + `.node_id
		    AND ` + memberAlias + `.role IN ('member','admin'))
		OR (` + memberAlias + `.node_id = ` + eventAlias + `.node_id
		    AND ` + memberAlias + `.role = 'follower'
		    AND ` + eventAlias + `.visibility = 'followers'
		    AND ` + followerEventsAllowedSQL(nodeAlias) + `))`
}
