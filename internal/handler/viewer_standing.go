package handler

import "github.com/patchwork-toolkit/patchwork/internal/model"

// The viewer's standing on each patch of a discovery list (the quilt tree
// and GET /nodes): admin, member or follower for an active row, pending or
// invited for a row that is not standing yet, and nothing at all for no row,
// a row the person left or was banned from, and an anonymous caller.
//
// A card's relationship chip used to look this up in the membership store,
// which was built from the first page of me/nodes, so a person in more than
// twenty patches saw "Follow" on the ones they followed last. The patch page
// never had that bug because GetNode states membership_role on the payload;
// this is the same answer for a list, in one LEFT JOIN rather than a query
// per row (memberships is UNIQUE(user_id, node_id), so the join adds no
// rows).
//
// `status` is read before `role`: a pending row carries role='member', which
// is what it would become, not what it is (docs/adr/088). A hidden membership
// still reports its standing, because the switch hides a person from others
// and this answers only the person themselves (docs/adr/006).
//
// The join's placeholder binds the caller's id, or "" for nobody, which
// matches no row. It must be bound in the order the text appears.
const viewerStandingJoin = " LEFT JOIN memberships vs ON vs.node_id = n.id AND vs.user_id = ?"

const viewerStandingColumn = `CASE WHEN vs.status = 'active' THEN vs.role
	WHEN vs.status IN ('pending','invited') THEN vs.status
	ELSE '' END`

func viewerStandingArg(u *model.User) string {
	if u == nil {
		return ""
	}
	return u.ID
}
