package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// JoinNode handles POST /api/v1/nodes/{slug}/join.
func JoinNode(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// A patch that has moved takes no new relationships (docs/adr/090).
		// The old home stays readable and everybody already in it keeps
		// everything they had — this refuses only the two acts that would
		// start a relationship with a room the community has left, and it
		// answers with the address instead of a dead end.
		if moved := nodeMovedTo(db, nodeID); moved != "" {
			writeMovedAway(w, moved)
			return
		}

		// Check if this is a follow request.
		var reqBody struct {
			Role    string `json:"role"`
			Message string `json:"message"`
		}
		// Try to parse body for role; default to member.
		json.NewDecoder(r.Body).Decode(&reqBody)
		isFollow := reqBody.Role == "follower"

		// The join sheet's optional intro message (docs/adr/040). Trimmed
		// and length-checked regardless of path; only ever stored when
		// the join lands as a pending member request — see below.
		joinMessage := strings.TrimSpace(reqBody.Message)
		if len(joinMessage) > 500 {
			http.Error(w, `{"error":"message must be 500 characters or fewer"}`, http.StatusBadRequest)
			return
		}

		// Followers can follow any public patch regardless of membership policy.
		if isFollow {
			var vis string
			db.QueryRow("SELECT visibility FROM nodes WHERE id = ?", nodeID).Scan(&vis)
			if vis != "public" {
				http.Error(w, `{"error":"can only follow public patches"}`, http.StatusForbidden)
				return
			}
		}

		// Look up membership policy and node status.
		var membershipPolicy, nodeStatus string
		db.QueryRow("SELECT membership_policy, status FROM nodes WHERE id = ?", nodeID).Scan(&membershipPolicy, &nodeStatus)

		// Unclaimed patches only accept followers, not members. Callers are
		// expected to render no member rung here at all (docs/adr/042), so
		// anyone reaching this message is looking at a stale page: it says
		// what the patch is and what would change it, not what the reader
		// could already have done.
		if nodeStatus == "unclaimed" && !isFollow {
			http.Error(w, `{"error":"Patch must be claimed to accept members"}`, http.StatusForbidden)
			return
		}

		// Check for existing membership.
		var existingID, existingStatus, existingRole string
		err := db.QueryRow("SELECT id, status, role FROM memberships WHERE user_id = ? AND node_id = ?", user.ID, nodeID).Scan(&existingID, &existingStatus, &existingRole)

		// An invited person pressing the ordinary button is accepting
		// (docs/adr/098). Checked ahead of the policy gate on purpose: the
		// patch is invite_only precisely when invitations are the door, and
		// refusing the person who was asked in for having been asked in is
		// the bug this route exists to close.
		if err == nil && existingStatus == "invited" {
			if isFollow {
				// Following would overwrite the invitation with a lesser
				// relationship, silently. The page never offers it here; the
				// API says what the two answers are.
				http.Error(w, `{"error":"you have been invited to join this patch: accept or decline the invitation"}`, http.StatusConflict)
				return
			}
			if err := acceptInvitedRow(db, r, user, existingID, nodeID); err != nil {
				http.Error(w, `{"error":"failed to accept invitation"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"status": "active", "membership_id": existingID})
			return
		}

		if !isFollow && membershipPolicy == "invite_only" {
			http.Error(w, `{"error":"this node is invite only"}`, http.StatusForbidden)
			return
		}

		if err == nil {
			// Membership row exists.
			if existingStatus == "banned" {
				http.Error(w, `{"error":"You have been removed from this community"}`, http.StatusForbidden)
				return
			}
			if existingStatus == "active" {
				if isFollow && existingRole == "follower" {
					http.Error(w, `{"error":"already following"}`, http.StatusConflict)
					return
				}
				if isFollow && existingRole != "follower" {
					// Already a member or admin — following is a downgrade, ignore.
					http.Error(w, `{"error":"already a member"}`, http.StatusConflict)
					return
				}
				if !isFollow && existingRole == "follower" {
					// Follower upgrading to member — this is the "Become Member" flow.
					newRole := "member"
					newStatus := "active"
					auditAction := "membership.join"
					if !isFollow && membershipPolicy == "approval_required" {
						newStatus = "pending"
						auditAction = "membership.request"
					}
					// Store the join sheet message only on a pending request (docs/adr/040).
					var storedMessage interface{}
					if newStatus == "pending" && joinMessage != "" {
						storedMessage = joinMessage
					}
					_, err = db.Exec(
						"UPDATE memberships SET role = ?, status = ?, join_message = ?, joined_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?",
						newRole, newStatus, storedMessage, existingID,
					)
					if err != nil {
						http.Error(w, `{"error":"failed to upgrade membership"}`, http.StatusInternalServerError)
						return
					}
					auth.LogAuditEvent(db, user.ID, auditAction, "membership", existingID, `{"from":"follower"}`, clientIP(r))

					var nodeSlugN, nodeNameN string
					db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlugN, &nodeNameN)
					notify(notifications.Event{
						Type:     notifications.MembershipJoined,
						NodeID:   nodeID,
						NodeSlug: nodeSlugN,
						NodeName: nodeNameN,
						ActorID:  user.ID,
						Title:    "New member joined " + nodeNameN,
						Link:     weblink.PatchMembers(nodeSlugN),
					})

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					json.NewEncoder(w).Encode(map[string]string{"status": newStatus, "membership_id": existingID})
					return
				}
				// Already an active member/admin trying to join again.
				http.Error(w, `{"error":"already a member"}`, http.StatusConflict)
				return
			}
			if existingStatus == "pending" {
				http.Error(w, `{"error":"membership request already pending"}`, http.StatusConflict)
				return
			}
			// Status is "left" — reactivate with the requested role.
			newRole := "member"
			newStatus := "active"
			auditAction := "membership.join"
			if isFollow {
				newRole = "follower"
				auditAction = "membership.follow"
			} else if membershipPolicy == "approval_required" {
				newStatus = "pending"
				auditAction = "membership.request"
			}
			// Store the join sheet message only on a pending request (docs/adr/040).
			var storedMessage interface{}
			if newStatus == "pending" && joinMessage != "" {
				storedMessage = joinMessage
			}
			_, err = db.Exec(
				"UPDATE memberships SET status = ?, role = ?, join_message = ?, joined_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?",
				newStatus, newRole, storedMessage, existingID,
			)
			if err != nil {
				http.Error(w, `{"error":"failed to rejoin node"}`, http.StatusInternalServerError)
				return
			}
			auth.LogAuditEvent(db, user.ID, auditAction, "membership", existingID, "{}", clientIP(r))

			// Notify admins about the join/request (not for follows).
			if !isFollow {
				var nodeSlugN, nodeNameN string
				db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlugN, &nodeNameN)
				if newStatus == "active" {
					notify(notifications.Event{
						Type:     notifications.MembershipJoined,
						NodeID:   nodeID,
						NodeSlug: nodeSlugN,
						NodeName: nodeNameN,
						ActorID:  user.ID,
						Title:    "New member joined " + nodeNameN,
						Link:     weblink.PatchMembers(nodeSlugN),
					})
				} else if newStatus == "pending" {
					notify(notifications.Event{
						Type:     notifications.MembershipRequest,
						NodeID:   nodeID,
						NodeSlug: nodeSlugN,
						NodeName: nodeNameN,
						ActorID:  user.ID,
						Title:    "Membership request for " + nodeNameN,
						Link:     weblink.PatchMembersPending(nodeSlugN),
					})
				}
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"status": newStatus, "membership_id": existingID})
			return
		}

		// No existing membership — create new one.
		role := "member"
		newStatus := "active"
		auditAction := "membership.join"

		if isFollow {
			role = "follower"
			auditAction = "membership.follow"
		} else if membershipPolicy == "approval_required" {
			newStatus = "pending"
			auditAction = "membership.request"
		}

		// Store the join sheet message only on a pending request (docs/adr/040).
		var storedMessage interface{}
		if newStatus == "pending" && joinMessage != "" {
			storedMessage = joinMessage
		}

		id := auth.NewUUIDv7()
		_, err = db.Exec(
			`INSERT INTO memberships (id, user_id, node_id, role, status, join_message) VALUES (?, ?, ?, ?, ?, ?)`,
			id, user.ID, nodeID, role, newStatus, storedMessage,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to join node"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, auditAction, "membership", id, "{}", clientIP(r))

		// Notify admins about member join/request (not for follows).
		if !isFollow {
			var nodeSlugN, nodeNameN string
			db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlugN, &nodeNameN)
			if newStatus == "active" {
				notify(notifications.Event{
					Type:     notifications.MembershipJoined,
					NodeID:   nodeID,
					NodeSlug: nodeSlugN,
					NodeName: nodeNameN,
					ActorID:  user.ID,
					Title:    "New member joined " + nodeNameN,
					Link:     weblink.PatchMembers(nodeSlugN),
				})
			} else if newStatus == "pending" {
				notify(notifications.Event{
					Type:     notifications.MembershipRequest,
					NodeID:   nodeID,
					NodeSlug: nodeSlugN,
					NodeName: nodeNameN,
					ActorID:  user.ID,
					Title:    "Membership request for " + nodeNameN,
					Link:     weblink.PatchMembersPending(nodeSlugN),
				})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": newStatus, "membership_id": id})
	}
}

// WithdrawMembershipRequest handles POST /api/v1/nodes/{slug}/withdraw.
//
// A requester rescinds their own unanswered membership request. Distinct
// from leaving, the way WithdrawClaim is distinct from rejection: nobody
// admitted this person, so there is no community to exit and no admin
// decision to undo. Keeping the two verbs apart is what lets the audit log
// tell "changed their mind before anyone answered" from "was here and
// left" — and stops a patch's record showing a departure by somebody who
// was never a member.
//
// Its own route rather than a relaxed LeaveNode for the same reason: that
// handler's only-admin floor and its 'active' precondition are about a
// standing this row does not have, and widening the status it accepts
// would have made both of those read as if they applied.
//
// join_message goes with the request, as it does on rejection: the intro
// was written for a request that no longer exists.
func WithdrawMembershipRequest(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// Scoped to this caller's own row: there is no admin path in here.
		// An admin turning a request down is UpdateMember's reject, which
		// is a decision and is logged as one.
		var memID, status string
		err := db.QueryRow(
			"SELECT id, status FROM memberships WHERE user_id = ? AND node_id = ?",
			user.ID, nodeID,
		).Scan(&memID, &status)
		if err != nil {
			http.Error(w, `{"error":"no membership request to withdraw"}`, http.StatusBadRequest)
			return
		}
		if status != "pending" {
			http.Error(w, `{"error":"no membership request to withdraw"}`, http.StatusBadRequest)
			return
		}

		_, err = db.Exec("UPDATE memberships SET status = 'left', join_message = NULL WHERE id = ?", memID)
		if err != nil {
			http.Error(w, `{"error":"failed to withdraw request"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "membership.withdraw", "membership", memID, "{}", clientIP(r))

		// Nobody is notified, matching WithdrawClaim and matching rejection:
		// the request simply leaves the pending queue. An admin who never
		// got round to it has nothing to act on and nothing to be told.

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "withdrawn"})
	}
}

// LeaveNode handles POST /api/v1/nodes/{slug}/leave.
func LeaveNode(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// Active rows only. Leaving exits a relationship, and a requester
		// holds none — retracting an unanswered request is
		// WithdrawMembershipRequest, a different verb on a different object
		// (docs/adr/088). The split is not tidiness: it is what makes a
		// stale page safe. If a request is approved between page load and
		// click, a Withdraw that reached this handler would resign a
		// membership the person did not yet know they had.
		var memberRole string
		err := db.QueryRow("SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active'", user.ID, nodeID).Scan(&memberRole)
		if err != nil {
			http.Error(w, `{"error":"not a member"}`, http.StatusBadRequest)
			return
		}

		// Cannot leave if you're the only admin — unless the patch runs on a
		// maintainer and has named a successor, in which case leaving is what
		// hands the patch over (docs/adr/051). docs/adr/012 says leaving is a
		// member right, and this floor has always made that untrue for the one
		// person nobody can replace; designation is how they earn the exit.
		// With no successor named the floor holds, because the alternative is
		// a patch nobody can administer.
		if memberRole == "admin" {
			var adminCount int
			db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'", nodeID).Scan(&adminCount)
			if adminCount <= 1 && !succeedOnDeparture(db, r, nodeID, slug, user.ID) {
				http.Error(w, `{"error":"cannot leave as the only admin"}`, http.StatusConflict)
				return
			}
		}

		_, err = db.Exec("UPDATE memberships SET status = 'left' WHERE user_id = ? AND node_id = ?", user.ID, nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to leave node"}`, http.StatusInternalServerError)
			return
		}

		// Leaving drops what this patch could reach you by (docs/adr/083).
		// Only here, not in WithdrawFromNode: sharing needs an active
		// member/admin row, so a pending request never had any to drop.
		DropContactSharesFor(db, user.ID, nodeID)

		auth.LogAuditEvent(db, user.ID, "membership.leave", "membership", nodeID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// ListMembers handles GET /api/v1/nodes/{slug}/members.
func ListMembers(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		after, limit := parsePaginationParams(r)

		// Default to active members only. Allow ?status=pending for node admins.
		statusFilter := r.URL.Query().Get("status")
		if statusFilter == "" {
			statusFilter = "active"
		}

		user := middleware.UserFromContext(r.Context())

		// The patch's own admins hold every management verb here, and the
		// one surface that is theirs alone: who has been invited and has not
		// answered (docs/adr/098). Not an instance admin with no role in the
		// patch — the room rule of docs/adr/081.
		patchAdmin := user != nil && userHasNodeRole(db, user.ID, nodeID, "admin")

		// Only allow pending/banned status filter for node admins or site
		// admins. Anything else — 'invited', 'left', a typo — lists as
		// active: an invited row is served under its own key below, to the
		// patch's admins only, and never as a page of members wearing the
		// role they would have.
		switch statusFilter {
		case "pending", "banned":
			if user == nil || (user.Role != "admin" && !patchAdmin) {
				// Non-admins just see active members.
				statusFilter = "active"
			}
		default:
			statusFilter = "active"
		}

		// The patch's admins and members see the full list, including hidden
		// memberships and followers. Everyone else gets the public view:
		// visible member/admin rows only — hidden memberships and follower
		// relationships are never public (docs/adr/006).
		insider := false
		if user != nil {
			if user.Role == "admin" {
				insider = true
			} else {
				var role string
				db.QueryRow(
					"SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active' AND role IN ('member','admin')",
					user.ID, nodeID,
				).Scan(&role)
				insider = role != ""
			}
		}

		// Who this patch publishes (docs/adr/095). Read for every viewer,
		// because the payload carries it either way: an endpoint that just
		// returned an empty array would leave the client unable to tell "no
		// members yet" from "this patch does not publish its list", and
		// those two want opposite copy.
		publicList := "everyone"
		db.QueryRow("SELECT public_member_list FROM nodes WHERE id = ?", nodeID).Scan(&publicList)

		// The roster gate, as a SQL clause, applied to outsiders only. The
		// states are a ladder (nodes.go, publicMemberListStates), so each is
		// one more conjunct rather than a different query. "nobody" is
		// written as a false clause instead of an early return so that the
		// counts below still run: the patch's size stays public at every
		// setting, because the quilt sizes its tile by member count and a
		// control claiming to hide the number would be contradicted by the
		// front page (docs/adr/095 decision 3).
		rosterClause := ""
		if !insider {
			switch publicList {
			case "admins":
				rosterClause = " AND m.role = 'admin'"
			case "nobody":
				rosterClause = " AND 0"
			}
		}

		// The pending queue is admin-only reachable (see the statusFilter gate
		// above), so join_message rides along only there — it never appears on
		// the active/public listing (docs/adr/040).
		includeMessage := statusFilter == "pending"

		// Contact items are shown to the people in the room: the patch's own
		// active admins and members, and nobody else (docs/adr/083). That is
		// narrower than `insider` — an instance admin with no role here
		// curates the quilt but was not who the person chose to be reachable
		// by. Only items shared into *this* patch appear, and only on the
		// active listing.
		inRoom := false
		if user != nil && statusFilter == "active" {
			var role string
			db.QueryRow(
				"SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active' AND role IN ('member','admin')",
				user.ID, nodeID,
			).Scan(&role)
			inRoom = role != ""
		}

		// Two facts about the room that no page of it can answer, both about
		// the offer to share (the insight is #231's, against the pre-083
		// card). Whether the viewer already shares something here is a fact
		// about their own row, which may sit on page 4; whether anyone here
		// shares is a fact about the whole room. Derived from the loaded
		// array, each really meant "…among the twenty who happened to load",
		// so a member far down the list was never offered anything.
		//
		// Simpler under docs/adr/083 than it was under the card: there is no
		// emptiness test, because an item cannot be empty — a blank value is
		// refused at write, and deleting is how an item goes away. The second
		// query re-checks the owner's membership for the same reason
		// sharedContactItemsForNode does, so the sentence cannot promise a
		// card the listing will not show.
		viewerShares := false
		anyContact := false
		if inRoom {
			db.QueryRow(`SELECT EXISTS(SELECT 1 FROM contact_item_shares s
				JOIN contact_items ci ON ci.id = s.item_id
				WHERE s.node_id = ? AND ci.user_id = ?)`, nodeID, user.ID).Scan(&viewerShares)
			db.QueryRow(`SELECT EXISTS(SELECT 1 FROM contact_item_shares s
				JOIN contact_items ci ON ci.id = s.item_id
				JOIN memberships om ON om.user_id = ci.user_id AND om.node_id = s.node_id
					AND om.status = 'active' AND om.role IN ('member','admin')
				WHERE s.node_id = ?)`, nodeID).Scan(&anyContact)
		}

		cols := "m.id, m.user_id, m.node_id, m.role, m.status, m.joined_at, u.username, u.display_name, u.avatar_url"
		if includeMessage {
			cols += ", m.join_message"
		}
		query := `SELECT ` + cols + `
			FROM memberships m JOIN users u ON m.user_id = u.id
			WHERE m.node_id = ? AND m.status = ?`
		args := []interface{}{nodeID, statusFilter}
		if !insider {
			query += " AND m.visible = 1 AND m.role IN ('member','admin')" + rosterClause
		}

		if after != "" {
			query += " AND m.id > ?"
			args = append(args, after)
		}
		query += " ORDER BY m.id ASC LIMIT ?"
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list members"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type memberResponse struct {
			ID          string `json:"id"`
			UserID      string `json:"user_id"`
			NodeID      string `json:"node_id"`
			Role        string `json:"role"`
			Status      string `json:"status"`
			JoinedAt    string `json:"joined_at"`
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			AvatarURL   string `json:"avatar_url"`
			// JoinMessage is the join sheet's intro note. Present only on the
			// admin-only pending listing (docs/adr/040).
			JoinMessage string `json:"join_message,omitempty"`
			// Contact is the items this member shares into this patch, for a
			// viewer who is an active admin or member of it (docs/adr/083).
			// Attached after the page loads, in one batched query.
			Contact []model.ContactItem `json:"contact,omitempty"`
		}
		var members []memberResponse
		for rows.Next() {
			var m memberResponse
			dest := []interface{}{&m.ID, &m.UserID, &m.NodeID, &m.Role, &m.Status, &m.JoinedAt, &m.Username, &m.DisplayName, &m.AvatarURL}
			var jm sql.NullString
			if includeMessage {
				dest = append(dest, &jm)
			}
			if err := rows.Scan(dest...); err != nil {
				continue
			}
			if jm.Valid {
				m.JoinMessage = jm.String
			}
			members = append(members, m)
		}

		var nextCursor string
		if len(members) > limit {
			nextCursor = members[limit-1].ID
			members = members[:limit]
		}
		if members == nil {
			members = []memberResponse{}
		}

		// Contact items ride along for a viewer in the room (docs/adr/083),
		// attached after the page is trimmed so the lookahead row nobody sees
		// is never asked about.
		if inRoom && len(members) > 0 {
			ids := make([]string, 0, len(members))
			for _, m := range members {
				ids = append(ids, m.UserID)
			}
			byUser := sharedContactItemsForNode(db, nodeID, ids)
			for i := range members {
				if items := byUser[members[i].UserID]; len(items) > 0 {
					members[i].Contact = items
				}
			}
		}

		// Totals for the header: how many people this patch holds, counted
		// exactly as GetNode and the tree endpoint count them — every active
		// member/admin row and every active follower row, under no visibility
		// gate at all. Deliberately *not* "how many rows this viewer's listing
		// would hand over", which is what these used to be and what made the
		// profile head say 40 Members over a members page saying 37.
		//
		// The listing and the count answer different questions, so one gate
		// cannot serve both. A count names nobody: docs/adr/006 gives a member
		// a say over being *listed*, and docs/adr/095 decision 3 already
		// settled that the patch's own roster gate hides who and never how
		// many — the quilt sizes a tile by member count, so a number the front
		// page publishes is not withheld by a second page declining to state
		// it. `m.visible` is the weaker of the two gates and cannot reach
		// further than the stronger one: counting under it would make a
		// patch's published size a function of its members' private choices,
		// and would have the quilt redraw itself per viewer.
		//
		// The cost, recorded rather than hidden: on a small patch a count of
		// five over a list of two says two people are not listed. That
		// inference came off the profile head regardless — it has always
		// stated the ungated number directly above this page — so gating here
		// concealed nothing and only made the two numbers fight. A patch that
		// cannot afford the inference sets public_member_list to nobody, where
		// no row is published and there is nothing to subtract from.
		//
		// Followers are counted apart from members and admins and never summed
		// with them (CONTEXT.md). Status still follows the listing, so the
		// admin-only pending queue counts pending rows: there the question
		// really is how long the queue this viewer is working through is.
		countQuery := `SELECT
				COALESCE(SUM(CASE WHEN m.role IN ('member','admin') THEN 1 ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN m.role = 'follower' THEN 1 ELSE 0 END), 0)
			FROM memberships m
			WHERE m.node_id = ? AND m.status = ?`
		var memberTotal, followerTotal int
		if err := db.QueryRow(countQuery, nodeID, statusFilter).Scan(&memberTotal, &followerTotal); err != nil {
			http.Error(w, `{"error":"failed to list members"}`, http.StatusInternalServerError)
			return
		}

		payload := map[string]interface{}{
			"items":          members,
			"next_cursor":    nextCursor,
			"member_count":   memberTotal,
			"follower_count": followerTotal,
			// What this patch publishes, so the page can tell an empty list
			// from a withheld one (docs/adr/095). Sent to every viewer: the
			// admin who sets it is looking at the very room it governs.
			"public_member_list": publicList,
		}
		// Only for a viewer in the room: outside it there is no offer to
		// make, and whether anyone here is reachable is not an outsider's
		// fact to learn.
		if inRoom {
			payload["viewer_shares_contact"] = viewerShares
			payload["any_contact_shared"] = anyContact
		}
		// Outstanding invitations, for the patch's own admins and nobody
		// else (docs/adr/098). A separate key rather than rows in `items`:
		// an invited person is not a member, and nothing that counts or
		// pages members should have to know the difference.
		if patchAdmin {
			payload["invited"] = invitedPeople(db, nodeID)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

// ListMyMemberships handles GET /api/v1/me/nodes.
func ListMyMemberships(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		after, limit := parsePaginationParams(r)

		// contact_items_shared is how many of the caller's own items this
		// patch can read (docs/adr/083). It answers the question this page
		// exists for — what does each patch know about me — without a request
		// per row.
		query := `SELECT m.id, m.user_id, m.node_id, m.role, m.status, m.visible, m.share_contact, m.joined_at,
			n.name, n.slug, n.description, n.visibility, n.membership_policy, n.status,
			(SELECT COUNT(*) FROM contact_item_shares s
				JOIN contact_items ci ON ci.id = s.item_id
				WHERE s.node_id = m.node_id AND ci.user_id = m.user_id)
			FROM memberships m JOIN nodes n ON m.node_id = n.id
			WHERE m.user_id = ? AND m.status IN ('active', 'pending') AND n.status IN ('active','unclaimed')`
		// Not 'invited' (docs/adr/098). Every client counts this list as
		// "my patches", and an invitation is not one; the caller's open
		// invitations are GET /users/me/invitations, read by name.
		args := []interface{}{user.ID}

		if after != "" {
			query += " AND m.id > ?"
			args = append(args, after)
		}
		query += " ORDER BY m.id ASC LIMIT ?"
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list memberships"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type membershipResponse struct {
			ID     string `json:"id"`
			UserID string `json:"user_id"`
			NodeID string `json:"node_id"`
			// Omitted for a row that holds no standing (docs/adr/088). A
			// pending request is stored on the membership row and carries
			// role='member' — always, since you cannot request admin and
			// following never goes through pending — so the column says
			// what the row would become, not what it is. Sending it made
			// this endpoint assert a membership that GetNode, the member
			// list, the member count and userHasNodeRole all deny, and
			// every client then had to defend itself against its own
			// server. Absent is the honest answer, and a loud one: a
			// caller reading it gets undefined rather than a wrong role.
			Role             string `json:"role,omitempty"`
			Status           string `json:"status"`
			Visible          bool   `json:"visible"`
			ShareContact     bool   `json:"share_contact"`
			JoinedAt         string `json:"joined_at"`
			NodeName         string `json:"node_name"`
			NodeSlug         string `json:"node_slug"`
			NodeDescription  string `json:"node_description"`
			NodeVisibility   string `json:"node_visibility"`
			MembershipPolicy string `json:"membership_policy"`
			// The patch's own status, because whether a member rung exists
			// at all depends on it: an unclaimed patch takes followers only
			// (JoinNode), and a caller who can't see that draws a door that
			// 403s (docs/adr/042).
			NodeStatus string `json:"node_status"`
			// ContactItemsShared counts this caller's items the patch can
			// read. Only ever about the caller's own card.
			ContactItemsShared int `json:"contact_items_shared"`
		}
		var memberships []membershipResponse
		for rows.Next() {
			var m membershipResponse
			if err := rows.Scan(&m.ID, &m.UserID, &m.NodeID, &m.Role, &m.Status, &m.Visible, &m.ShareContact, &m.JoinedAt,
				&m.NodeName, &m.NodeSlug, &m.NodeDescription, &m.NodeVisibility, &m.MembershipPolicy,
				&m.NodeStatus, &m.ContactItemsShared); err != nil {
				continue
			}
			// Standing is stated once, here, the way GetNode states it:
			// only an active row has a role.
			if m.Status != "active" {
				m.Role = ""
			}
			memberships = append(memberships, m)
		}

		var nextCursor string
		if len(memberships) > limit {
			nextCursor = memberships[limit-1].ID
			memberships = memberships[:limit]
		}
		if memberships == nil {
			memberships = []membershipResponse{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":       memberships,
			"next_cursor": nextCursor,
		})
	}
}

// UpdateMember handles PATCH /api/v1/nodes/{slug}/members/{userId}.
func UpdateMember(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")
		targetUserID := r.PathValue("userId")

		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// Check that the caller has admin role on the node, or is a site admin.
		if user.Role != "admin" && !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		// Look up the target membership.
		var memID, currentRole, currentStatus string
		err := db.QueryRow(
			"SELECT id, role, status FROM memberships WHERE user_id = ? AND node_id = ?",
			targetUserID, nodeID,
		).Scan(&memID, &currentRole, &currentStatus)
		if err != nil {
			http.Error(w, `{"error":"membership not found"}`, http.StatusNotFound)
			return
		}

		var req struct {
			Role   *string `json:"role"`
			Status *string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Handle status changes (approve/reject/ban/reinstate).
		if req.Status != nil {
			switch *req.Status {
			case "active":
				// Approve a pending member.
				if currentStatus != "pending" {
					http.Error(w, `{"error":"can only approve pending members"}`, http.StatusBadRequest)
					return
				}
				_, err = db.Exec("UPDATE memberships SET status = 'active', join_message = NULL WHERE id = ?", memID)
				if err != nil {
					http.Error(w, `{"error":"failed to approve member"}`, http.StatusInternalServerError)
					return
				}
				auth.LogAuditEvent(db, user.ID, "membership.approve", "membership", memID,
					fmt.Sprintf(`{"target_user_id":"%s"}`, targetUserID), clientIP(r))

				// Notify the approved user.
				var nodeSlugN, nodeNameN string
				db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlugN, &nodeNameN)
				notify(notifications.Event{
					Type:     notifications.MembershipApproved,
					NodeID:   nodeID,
					NodeSlug: nodeSlugN,
					NodeName: nodeNameN,
					ActorID:  user.ID,
					TargetID: targetUserID,
					Title:    "Your membership in " + nodeNameN + " was approved",
					Link:     weblink.Patch(nodeSlugN),
				})

			case "banned":
				// Ban an active member or follower.
				if currentStatus != "active" {
					http.Error(w, `{"error":"can only ban active members"}`, http.StatusBadRequest)
					return
				}
				// Cannot ban yourself.
				if targetUserID == user.ID {
					http.Error(w, `{"error":"cannot ban yourself"}`, http.StatusBadRequest)
					return
				}
				// Cannot ban the last admin.
				if currentRole == "admin" {
					var adminCount int
					db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'", nodeID).Scan(&adminCount)
					if adminCount <= 1 {
						http.Error(w, `{"error":"cannot ban the last admin"}`, http.StatusConflict)
						return
					}
				}
				_, err = db.Exec("UPDATE memberships SET status = 'banned' WHERE id = ?", memID)
				if err != nil {
					http.Error(w, `{"error":"failed to ban member"}`, http.StatusInternalServerError)
					return
				}
				// A banned person is out of the room, so the room stops
				// being able to reach them (docs/adr/083).
				DropContactSharesFor(db, targetUserID, nodeID)

				auth.LogAuditEvent(db, user.ID, "membership.ban", "membership", memID,
					fmt.Sprintf(`{"target_user_id":"%s"}`, targetUserID), clientIP(r))

				var banSlug, banName string
				db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&banSlug, &banName)
				notify(notifications.Event{
					Type:     notifications.MembershipBanned,
					NodeID:   nodeID,
					NodeSlug: banSlug,
					NodeName: banName,
					ActorID:  user.ID,
					TargetID: targetUserID,
					Title:    "You have been removed from " + banName,
					Body:     "A patch admin has removed you from this community.",
					Link:     weblink.Patch(banSlug),
				})

			case "left":
				// Reject a pending member OR reinstate a banned member.
				if currentStatus == "pending" {
					_, err = db.Exec("UPDATE memberships SET status = 'left', join_message = NULL WHERE id = ?", memID)
					if err != nil {
						http.Error(w, `{"error":"failed to reject member"}`, http.StatusInternalServerError)
						return
					}
					auth.LogAuditEvent(db, user.ID, "membership.reject", "membership", memID,
						fmt.Sprintf(`{"target_user_id":"%s"}`, targetUserID), clientIP(r))
				} else if currentStatus == "banned" {
					_, err = db.Exec("UPDATE memberships SET status = 'left' WHERE id = ?", memID)
					if err != nil {
						http.Error(w, `{"error":"failed to reinstate member"}`, http.StatusInternalServerError)
						return
					}
					auth.LogAuditEvent(db, user.ID, "membership.reinstate", "membership", memID,
						fmt.Sprintf(`{"target_user_id":"%s"}`, targetUserID), clientIP(r))

					var reinstateSlug, reinstateName string
					db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&reinstateSlug, &reinstateName)
					notify(notifications.Event{
						Type:     notifications.MembershipReinstated,
						NodeID:   nodeID,
						NodeSlug: reinstateSlug,
						NodeName: reinstateName,
						ActorID:  user.ID,
						TargetID: targetUserID,
						Title:    "You have been reinstated in " + reinstateName,
						Body:     "You can now rejoin this community.",
						Link:     weblink.Patch(reinstateSlug),
					})
				} else {
					http.Error(w, `{"error":"can only reject pending or reinstate banned members"}`, http.StatusBadRequest)
					return
				}

			default:
				http.Error(w, `{"error":"invalid status value"}`, http.StatusBadRequest)
				return
			}
		}

		// Handle role changes.
		if req.Role != nil {
			newRole := *req.Role
			if newRole != "member" && newRole != "follower" && newRole != "admin" {
				http.Error(w, `{"error":"invalid role value"}`, http.StatusBadRequest)
				return
			}

			// An invited row has no role to change (docs/adr/098): the
			// person has not said yes, and a role set here would be what
			// they became the moment they did — admin, without anyone
			// having asked them into that.
			if currentStatus == "invited" {
				http.Error(w, `{"error":"they have not accepted the invitation yet"}`, http.StatusBadRequest)
				return
			}

			// Cannot demote the last admin.
			if currentRole == "admin" && newRole != "admin" {
				var adminCount int
				db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'", nodeID).Scan(&adminCount)
				if adminCount <= 1 {
					http.Error(w, `{"error":"cannot demote the last admin"}`, http.StatusConflict)
					return
				}
			}

			// A patch with no admins at all has no mechanism left to outrank,
			// and the three refusals below are all of the form "use this
			// patch's own mechanism instead of the dropdown". Every one of
			// those mechanisms is started by an admin: an attestation is
			// recorded by one, a nomination is raised by one, a seat is added
			// by one. So on an empty council the refusals stop protecting a
			// mechanism and start sealing the door.
			//
			// Only an instance admin can be standing at that door — the other
			// key is the one that is missing — so the condition needs no role
			// check of its own: `userHasNodeRole(admin)` is false for
			// everybody here by construction. Inactivity is what empties a
			// council without anyone choosing to (docs/adr/051), and this is
			// the route back that does not depend on the patch's own
			// machinery still being startable.
			//
			// It is audited with its reason, so the record says this was a
			// repair and not a promotion the patch's rules allowed.
			var nodeAdmins int
			db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'", nodeID).Scan(&nodeAdmins)
			restoringEmptyCouncil := nodeAdmins == 0 && newRole == "admin"

			// Where admins are chosen elsewhere, Patchwork does not make them
			// (docs/adr/052). The record of that decision is what promotes,
			// so a hand-made admin here would be a change with no decision
			// behind it — and the next attestation would undo it anyway.
			if !restoringEmptyCouncil && newRole == "admin" && currentRole != "admin" && leadershipDecidedElsewhere(db, nodeID) {
				http.Error(w, `{"error":"this patch chooses its admins elsewhere: record that decision instead"}`, http.StatusConflict)
				return
			}

			// On a meritocratic patch the community ratifies admins, so an
			// admin cannot simply make one (docs/adr/051). Without this the
			// ratification vote is optional, and an optional vote is theatre —
			// the same "rules on screen are not the rules in force" failure
			// docs/adr/041 named. Demotion is untouched: nothing about
			// earning a role says the community must vote to end it, and the
			// last-admin floor above still applies.
			if !restoringEmptyCouncil && newRole == "admin" && currentRole != "admin" && leadershipModel(db, nodeID) == "meritocratic" {
				http.Error(w, `{"error":"this patch ratifies admins by proposal: nominate them instead"}`, http.StatusConflict)
				return
			}

			// On an elected patch the dropdown does not make an admin either
			// (docs/adr/100). Two founders used it, believing they were doing
			// the ordinary thing, and got three admins, zero new seats, no
			// election, and a contest for one seat while three people held
			// power. A control that silently outranks the mechanism the
			// governance page advertises is worse than no control, so this
			// says which mechanism runs and where it is: nominate into a
			// vacant seat, or wait for the contest that fills the full one.
			// Demotion is untouched, as it is for meritocratic, and the
			// last-admin floor above still applies.
			if !restoringEmptyCouncil && newRole == "admin" && currentRole != "admin" && leadershipModel(db, nodeID) == "elected" {
				http.Error(w, `{"error":"`+electedPromotionDenial(db, nodeID)+`"}`, http.StatusConflict)
				return
			}

			// No cap on promotion. docs/adr/049 enforced max_admins here and
			// docs/adr/051 retracts it: how many admins a patch has is a
			// function of how it governs, not a number it configures, and
			// max_admins was unreachable anyway — RulesProposalEditor is the
			// only writer of governance-rules.json and the structured editor
			// preserves the field without offering a control. Migration 041
			// had backfilled "max_admins": 3 into nearly every patch, so
			// enforcing it capped live patches with no way out.

			// A follower has no room to share into, so a demotion drops
			// this patch's shares (docs/adr/083). Promotion never adds any:
			// there is no standing rule for it to satisfy.
			if newRole == "follower" && currentRole != "follower" {
				DropContactSharesFor(db, targetUserID, nodeID)
			}

			_, err = db.Exec("UPDATE memberships SET role = ?, "+roleSinceNow+" WHERE id = ?", newRole, memID)
			if err != nil {
				http.Error(w, `{"error":"failed to update role"}`, http.StatusInternalServerError)
				return
			}
			// On an elected patch every admin sits in a chair the record can
			// name (docs/adr/100), so a restoration puts them in one where a
			// chair is free. The chair keeps its own term end — the clock
			// belongs to the seat (docs/adr/051), and a repair is the last
			// act that should hand out a fresh mandate.
			if restoringEmptyCouncil {
				if seatID := vacantSeat(db, nodeID); seatID != "" {
					db.Exec("UPDATE seats SET holder_id = ? WHERE id = ?", targetUserID, seatID)
					auth.LogAuditEvent(db, user.ID, "seat.filled", "seat", seatID,
						fmt.Sprintf(`{"node_id":"%s","holder_id":"%s","reason":"council_empty"}`, nodeID, targetUserID), clientIP(r))
				}
			}

			reason := ""
			if restoringEmptyCouncil {
				reason = `,"reason":"council_empty"`
			}
			auth.LogAuditEvent(db, user.ID, "membership.role_change", "membership", memID,
				fmt.Sprintf(`{"target_user_id":"%s","old_role":"%s","new_role":"%s"%s}`, targetUserID, currentRole, newRole, reason), clientIP(r))
		}

		// Return the updated membership.
		var updatedRole, updatedStatus, joinedAt string
		db.QueryRow("SELECT role, status, joined_at FROM memberships WHERE id = ?", memID).Scan(&updatedRole, &updatedStatus, &joinedAt)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"id":        memID,
			"user_id":   targetUserID,
			"node_id":   nodeID,
			"role":      updatedRole,
			"status":    updatedStatus,
			"joined_at": joinedAt,
		})
	}
}
