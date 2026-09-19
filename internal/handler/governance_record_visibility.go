package handler

import (
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Who may read a patch's deliberation — its proposals, the discussion under
// them, and the attestations recording what was decided elsewhere
// (docs/adr/2026-09-18-the-default-should-match-the-assumption.md).
//
// This is the sibling of the public member list's gate and deliberately not
// the same shape. That one is a ladder applied per row, because a roster is a
// list of names and `admins` is a coherent subset of it. This one is a door,
// because a deliberation record is prose that names people inside itself: a
// nomination carries target_user_id and usually names its subject in its own
// title, so there is no state between open and closed that says what it means.
//
// It does not govern charters. Those carry their own per-document visibility
// (docs/adr/036, canReadPatchDocs), and the lining is pinned public
// (docs/adr/037). A patch's finished statements and its unfinished arguments
// are different things and are gated separately.

// canReadGovernanceRecord reports whether this request's viewer may read the
// patch's deliberation. Open records are readable by anyone, signed out
// included; a closed one is the room's — active admins and members, plus
// instance admins.
//
// A follower is an outsider here, exactly as they are for the member list
// (docs/adr/095). Following needs nobody's approval, so counting a follower as
// an insider would let any signed-in stranger admit themselves to a closed
// record by clicking Follow.
func canReadGovernanceRecord(db *database.DB, r *http.Request, nodeID string) bool {
	if governanceRecordIsPublic(db, nodeID) {
		return true
	}
	return viewerIsInPatchRoom(db, r, nodeID)
}

// governanceRecordIsPublic reports whether the patch publishes its
// deliberation at all. Separate from the function above because the delivery
// worker has no request to carry a viewer: federation asks only whether this
// may leave the instance, and the answer for a closed record is no, whoever
// the remote follower is.
//
// A missing row reads closed. The alternative — treating an unreadable node as
// open — would publish on exactly the paths where something has already gone
// wrong.
func governanceRecordIsPublic(db *database.DB, nodeID string) bool {
	var setting string
	if err := db.QueryRow(
		"SELECT public_governance_record FROM nodes WHERE id = ?", nodeID,
	).Scan(&setting); err != nil {
		return false
	}
	return setting == "everyone"
}
