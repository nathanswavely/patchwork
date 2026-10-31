package handler

import (
	"encoding/json"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// UserActivityFeed handles GET /api/v1/activity.
// Returns a reverse-chronological feed of recent activity across all patches the user belongs to.
func UserActivityFeed(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		after, limit := parsePaginationParams(r)
		if limit > 50 {
			limit = 50
		}

		// Each branch gates on the hosting node's status: an archived patch
		// disappears from the member's patch list (memberships handler), so
		// its activity has to leave the feed too — the links would 404.
		// No *node* visibility gate here: the feed is scoped to the viewer's
		// own memberships, and a private patch you belong to is yours to
		// see. What each row is *about* is a different question, and every
		// branch asks it — every active relationship lands rows in this
		// feed, follower included, and following costs nothing and needs
		// nobody's approval (docs/adr/116). So each branch asks exactly what
		// the REST read of the same thing asks, as SQL beside its Go twin:
		//
		//   - an event: its own tier (eventVisibleToMatchedMembership);
		//   - a proposal: the patch's governance record, open or the room's
		//     (governanceRecordReadableSQL ↔ canReadGovernanceRecord). A
		//     follower is an outsider to a closed record; the proposals key
		//     gates taking part, not reading (docs/adr/050);
		//   - a charter: published, or the members-only shelf for the room
		//     and a follower the patch grants charters
		//     (patchDocReadableSQL ↔ canReadPatchDocs).
		//
		// Before issue #398 the proposal and charter branches asked nothing,
		// so any signed-in account could read the titles of an invite-only
		// patch's closed proposals and unpublished charters by following it.
		// Every branch is dated by when the thing came to exist, its
		// created_at, so one timestamp means one thing across the feed.
		// A charter used to be dated by updated_at, which put it back at
		// the top as if new on every edit while the item said nothing about
		// what had happened (issue #398). A charter's text changes through
		// a proposal, and that proposal is its own row here; a direct edit
		// is a different event this feed does not yet name, and should
		// come back as its own type if it comes back at all.
		query := `
			SELECT id, type, title, body, patch_slug, patch_name, actor_name, created_at
			FROM (
				SELECT p.id, 'proposal' AS type, p.title,
					COALESCE(p.proposal_type, '') AS body,
					n.slug AS patch_slug, n.name AS patch_name,
					`+displayNameExpr("u")+` AS actor_name,
					p.created_at
				FROM proposals p
				JOIN nodes n ON n.id = p.node_id
					AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL
				JOIN memberships m ON m.node_id = p.node_id AND m.user_id = ? AND m.status = 'active'
				LEFT JOIN users u ON u.id = p.author_id
				WHERE ` + governanceRecordReadableSQL("m", "n") + `

				UNION ALL

				SELECT e.id, 'event' AS type, e.title,
					'' AS body,
					n.slug AS patch_slug, n.name AS patch_name,
					'' AS actor_name,
					e.created_at
				FROM events e
				JOIN nodes n ON n.id = e.node_id
					AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL
				JOIN memberships m ON m.node_id = e.node_id AND m.user_id = ? AND m.status = 'active'
				WHERE e.removed_at IS NULL
					AND e.status = 'active'
					AND ` + eventVisibleToMatchedMembership("m", "e", "n") + `

				UNION ALL

				SELECT g.id, 'governance' AS type, g.title,
					'' AS body,
					n.slug AS patch_slug, n.name AS patch_name,
					'' AS actor_name,
					g.created_at
				FROM governance_docs g
				JOIN nodes n ON n.id = g.node_id
					AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL
				JOIN memberships m ON m.node_id = g.node_id AND m.user_id = ? AND m.status = 'active'
				WHERE ` + patchDocReadableSQL("m", "g", "n") + `
			)
			`

		args := []interface{}{user.ID, user.ID, user.ID}

		// created_at is not unique across the three unioned sources, so the cursor
		// carries id as a tiebreaker — a bare timestamp would drop unserved rows
		// sharing the boundary row's timestamp.
		if sortKey, id, ok := decodeCursor(after); after != "" && ok {
			query += ` WHERE ` + keysetCondition("created_at", "id", true)
			args = append(args, sortKey, sortKey, id)
		}

		query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to load activity"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type feedItem struct {
			ID        string `json:"id"`
			Type      string `json:"type"`
			Title     string `json:"title"`
			Body      string `json:"body,omitempty"`
			Link      string `json:"link,omitempty"`
			PatchSlug string `json:"patch_slug"`
			PatchName string `json:"patch_name"`
			ActorName string `json:"actor_name,omitempty"`
			CreatedAt string `json:"created_at"`
		}

		var items []feedItem
		for rows.Next() {
			var item feedItem
			if err := rows.Scan(&item.ID, &item.Type, &item.Title, &item.Body, &item.PatchSlug, &item.PatchName, &item.ActorName, &item.CreatedAt); err != nil {
				continue
			}
			item.Link = activityLink(item.Type, item.PatchSlug, item.ID)
			items = append(items, item)
		}

		var nextCursor string
		if len(items) > limit {
			nextCursor = encodeCursor(items[limit-1].CreatedAt, items[limit-1].ID)
			items = items[:limit]
		}
		if items == nil {
			items = []feedItem{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":       items,
			"next_cursor": nextCursor,
		})
	}
}

// activityLink is where a feed item points, built by internal/weblink rather
// than concatenated in SQL. The SQL copies had drifted: an event pointed at
// /patches/{slug}/events/{id}, which the SPA serves only as a redirect and
// which no other client knows (issue #398), the same drift weblink was
// written to end for notifications (issue #56).
func activityLink(itemType, slug, id string) string {
	switch itemType {
	case "proposal":
		return weblink.Proposal(slug, id)
	case "event":
		return weblink.Event(id)
	case "governance":
		return weblink.GovernanceDoc(slug, id)
	}
	return ""
}
