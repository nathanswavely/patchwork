package handler

import (
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// The trusted-contributor grant, at either scope
// (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar.md,
// amending docs/adr/026).
//
// ADR 026 put the grant on a boolean column, which made it quilt-wide or
// nothing: the person who suggested one listing and wanted to keep its
// calendar had to be handed every unclaimed calendar on the instance. So they
// were handed none, and every event they added queued again for the admin who
// had just approved the patch. The grant now has a second scope — one patch —
// and `node_trusted_contributors` is where a per-patch one lives.

// userTrustedOn reports whether a user holds the trusted-contributor grant
// reaching one patch, at either scope: the quilt-wide flag, or a per-patch row
// for exactly this patch.
//
// It deliberately does **not** check the patch's status. Every gate that calls
// it has already established `unclaimed` — that is the condition the grant is
// about, and duplicating it here would put the rule in two places and let the
// two disagree. The one gate that is about no patch (deriving a verification
// domain from a new suggestion's website, unclaimed.go) still reads the
// quilt-wide flag directly, because a suggestion is not the patch it names.
//
// A nil user is never trusted: an anonymous caller holds no grant.
func userTrustedOn(db *database.DB, user *model.User, nodeID string) bool {
	if user == nil || nodeID == "" {
		return false
	}
	if user.TrustedContributor {
		return true
	}
	var count int
	db.QueryRow(
		`SELECT COUNT(*) FROM node_trusted_contributors WHERE user_id = ? AND node_id = ?`,
		user.ID, nodeID,
	).Scan(&count)
	return count > 0
}
