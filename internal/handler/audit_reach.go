package handler

import (
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// Recording the reach (docs/adr/2026-10-20-an-act-the-instance-role-let-through-says-so.md).
//
// docs/adr/115 decided an instance admin's reach into a patch is custody, not
// rank, and deferred removing it. It also named what the deferral left
// behind: an undocumented break-glass. An instance admin holding no role in a
// patch could rename it, rewrite its charters or drop its events, and the
// audit entry was indistinguishable from the patch's own admin doing it —
// `node.update` with the field names and nothing else.
//
// So every audit entry for an act on a patch says, when it is true, that the
// instance role is what let the act through: `"acting_without_role": true` in
// the entry's metadata. Nothing is granted or refused here. The reach stays
// exactly as wide as it was until issue #286; it just stops being silent.
//
// The key is absent rather than false on every other entry, so the existing
// shape of the log does not change for the overwhelming majority of rows,
// and a query for the reach is a query for the key.

// actingWithoutRole reports whether an act on this patch is being let through
// by the caller's instance role rather than by standing in the patch: an
// instance admin who holds none of roles there. With no roles given, the act
// is a patch admin's, which is what nearly every reach stands in for.
//
// Ask it before the act, not after. UpdateMember can make the caller an admin
// of the patch they were reaching into, and asking afterwards would record
// the reach as the standing it just created.
func actingWithoutRole(db *database.DB, user *model.User, nodeID string, roles ...string) bool {
	if user == nil || user.Role != "admin" || nodeID == "" {
		return false
	}
	if len(roles) == 0 {
		roles = []string{"admin"}
	}
	return !userHasNodeRole(db, user.ID, nodeID, roles...)
}

// reachDetail returns an audit payload carrying the acting_without_role mark
// when without is true, and payload unchanged otherwise. It copies rather
// than writes into payload, so a caller's map is never altered behind it.
func reachDetail(payload map[string]any, without bool) map[string]any {
	if !without {
		return payload
	}
	out := make(map[string]any, len(payload)+1)
	for k, v := range payload {
		out[k] = v
	}
	out["acting_without_role"] = true
	return out
}
