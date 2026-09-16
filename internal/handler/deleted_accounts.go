package handler

import (
	"database/sql"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// Rendering a deleted person (docs/adr/086).
//
// Deletion keeps the users row as a tombstone so the community's record
// survives: every vote still counts and every proposal still has a proposer.
// What must not survive is the person. The identity columns are emptied at
// deletion time, but `username` deliberately stays on the row — it is the
// UNIQUE constraint that retires the handle so nobody can register it later
// and inherit the inbound links to a stranger's old profile.
//
// That leaves one handle sitting in a column that a couple of dozen queries
// join to. The substitution therefore happens in SQL, at the API layer, not
// in the Svelte components: a frontend can only leak a name the API sent it,
// and there are far more render sites than query sites.

// DeletedAccountName is the neutral label every surface shows in place of a
// deleted person. Not a username, not a link — there is nothing to link to,
// since the profile route 404s for a tombstone.
const DeletedAccountName = "Deleted account"

// accountDeleted reports whether a user id names a tombstone. Used by the
// surfaces that have to answer differently rather than merely rename — the
// ActivityPub actor, which answers 410 Gone.
func accountDeleted(db *database.DB, userID string) bool {
	var deleted sql.NullString
	if err := db.QueryRow(`SELECT deleted_at FROM users WHERE id = ?`, userID).Scan(&deleted); err != nil {
		return false
	}
	return deleted.Valid && deleted.String != ""
}

// displayNameExpr renders the display name for a user joined under `alias`,
// substituting the neutral label for a tombstone.
//
// The ELSE branch is the expression these queries already carried
// (`COALESCE(display_name, username, '')`), kept identical so this changes
// nothing for a live account. A LEFT JOIN that matched nothing falls to the
// ELSE and still yields '', because `NULL IS NOT NULL` is false.
func displayNameExpr(alias string) string {
	return "CASE WHEN " + alias + ".deleted_at IS NOT NULL THEN '" + DeletedAccountName + "'" +
		" ELSE COALESCE(" + alias + ".display_name, " + alias + ".username, '') END"
}

// usernameExpr renders the username, blank for a tombstone. Blank rather
// than the label because this value is what profile links are built from:
// a surface that renders a handle at all has to be able to tell that there
// is nowhere to point it.
func usernameExpr(alias string) string {
	return "CASE WHEN " + alias + ".deleted_at IS NOT NULL THEN ''" +
		" ELSE COALESCE(" + alias + ".username, '') END"
}

// Rendering a hidden membership (docs/adr/006).
//
// A sibling of the substitution above, and a different shape of the same
// problem: a person whose acts must survive a read that their name must not.
// The difference is what the rule turns on. A tombstone is a fact about the
// row, so it can be decided in SQL once; a hidden membership is a fact about
// the row *and* who is asking, so the caller decides and this file supplies
// only the label and the question.

// HiddenMemberName is what stands in for a member who switched their
// membership out of sight (docs/adr/006) on a surface someone outside the
// room is reading. It names the state of the membership, not the person,
// the way DeletedAccountName names the state of the account.
const HiddenMemberName = "Hidden member"

// viewerIsInPatchRoom reports whether the request's viewer is inside a
// patch: an active admin or member of it, or an instance admin. This is the
// line ADR 006 draws for hidden memberships — "still seen by that patch's
// admins and members inside the workspace" — and it is deliberately
// narrower than canReadPatchDocs, which admits a follower holding the
// charters permission. A follower is an observer, not a member, and the
// people in a patch are not theirs to enumerate.
func viewerIsInPatchRoom(db *database.DB, r *http.Request, nodeID string) bool {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		return false
	}
	if user.Role == "admin" {
		return true
	}
	var role string
	if err := db.QueryRow(
		"SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active' AND role IN ('member','admin')",
		user.ID, nodeID,
	).Scan(&role); err != nil {
		return false
	}
	return role != ""
}
