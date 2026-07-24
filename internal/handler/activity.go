package handler

import (
	"encoding/json"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
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
		// No visibility gate here: the feed is scoped to the viewer's own
		// memberships, and a private patch you belong to is yours to see.
		query := `
			SELECT id, type, title, body, link, patch_slug, patch_name, actor_name, created_at
			FROM (
				SELECT p.id, 'proposal' AS type, p.title,
					COALESCE(p.proposal_type, '') AS body,
					'/patches/' || n.slug || '/governance/' || p.id AS link,
					n.slug AS patch_slug, n.name AS patch_name,
					COALESCE(u.display_name, u.username, '') AS actor_name,
					p.created_at
				FROM proposals p
				JOIN nodes n ON n.id = p.node_id
					AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL
				LEFT JOIN users u ON u.id = p.author_id
				WHERE p.node_id IN (SELECT node_id FROM memberships WHERE user_id = ? AND status = 'active')

				UNION ALL

				SELECT e.id, 'event' AS type, e.title,
					'' AS body,
					'/patches/' || n.slug || '/events/' || e.id AS link,
					n.slug AS patch_slug, n.name AS patch_name,
					'' AS actor_name,
					e.created_at
				FROM events e
				JOIN nodes n ON n.id = e.node_id
					AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL
				WHERE e.removed_at IS NULL
					AND e.status = 'active'
					AND e.node_id IN (SELECT node_id FROM memberships WHERE user_id = ? AND status = 'active')

				UNION ALL

				SELECT g.id, 'governance' AS type, g.title,
					'' AS body,
					'/patches/' || n.slug || '/governance/docs/' || g.id AS link,
					n.slug AS patch_slug, n.name AS patch_name,
					'' AS actor_name,
					g.updated_at AS created_at
				FROM governance_docs g
				JOIN nodes n ON n.id = g.node_id
					AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL
				WHERE g.node_id IN (SELECT node_id FROM memberships WHERE user_id = ? AND status = 'active')
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
			if err := rows.Scan(&item.ID, &item.Type, &item.Title, &item.Body, &item.Link, &item.PatchSlug, &item.PatchName, &item.ActorName, &item.CreatedAt); err != nil {
				continue
			}
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
