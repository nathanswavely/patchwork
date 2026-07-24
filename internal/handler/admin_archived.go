package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// archivedNode is one row of the instance admin's archived-patches list.
type archivedNode struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	ArchivedFrom  string `json:"archived_from,omitempty"`
	RestoresTo    string `json:"restores_to"`
	MemberCount   int    `json:"member_count"`
	FollowerCount int    `json:"follower_count"`
	ArchivedAt    string `json:"archived_at"`
}

// restoreTarget resolves the status a restore would return the node to.
// archived_from is authoritative when present; rows archived before the
// column existed fall back to inference — any active admin membership
// means the patch was active, otherwise it was unclaimed (docs/adr/034).
func restoreTarget(db *database.DB, nodeID, archivedFrom string) string {
	if archivedFrom == "active" || archivedFrom == "unclaimed" {
		return archivedFrom
	}
	var adminCount int
	db.QueryRow(
		"SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'", nodeID,
	).Scan(&adminCount)
	if adminCount > 0 {
		return "active"
	}
	return "unclaimed"
}

// AdminListNodes handles GET /api/v1/admin/nodes. Only status=archived is
// supported — the admin panel has no general patch inventory yet, and if
// one is ever built this list collapses into it as a filter (docs/adr/034).
// Rejected community submissions also carry status='archived' but with
// removed_at set; they are refuse, not archives, and never appear here.
func AdminListNodes(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("status") != "archived" {
			http.Error(w, `{"error":"only status=archived is supported"}`, http.StatusBadRequest)
			return
		}
		after, limit := parsePaginationParams(r)

		query := `SELECT id, name, slug, COALESCE(archived_from, ''), updated_at,
			COALESCE((SELECT COUNT(*) FROM memberships m WHERE m.node_id = nodes.id AND m.status = 'active' AND m.role IN ('admin','member')), 0),
			COALESCE((SELECT COUNT(*) FROM memberships m WHERE m.node_id = nodes.id AND m.status = 'active' AND m.role = 'follower'), 0)
			FROM nodes WHERE status = 'archived' AND removed_at IS NULL`
		args := []interface{}{}
		if after != "" {
			query += " AND id > ?"
			args = append(args, after)
		}
		query += " ORDER BY id ASC LIMIT ?"
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list archived nodes"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		nodes := []archivedNode{}
		for rows.Next() {
			var n archivedNode
			if err := rows.Scan(&n.ID, &n.Name, &n.Slug, &n.ArchivedFrom, &n.ArchivedAt, &n.MemberCount, &n.FollowerCount); err != nil {
				continue
			}
			n.RestoresTo = restoreTarget(db, n.ID, n.ArchivedFrom)
			nodes = append(nodes, n)
		}

		hasMore := false
		if len(nodes) > limit {
			hasMore = true
			nodes = nodes[:limit]
		}
		nextCursor := ""
		if hasMore {
			nextCursor = nodes[len(nodes)-1].ID
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"nodes":       nodes,
			"has_more":    hasMore,
			"next_cursor": nextCursor,
		})
	}
}

// AdminRestoreNode handles POST /api/v1/admin/nodes/{id}/restore — the only
// way back from archived, deliberately instance-admin-only and keyed by ID:
// every slug-based route refuses archived patches, and that refusal stays
// absolute (docs/adr/034). Restore is silent — no notifications, no AP
// activity; the patch reappearing is the announcement.
func AdminRestoreNode(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID := r.PathValue("id")

		var status, archivedFrom string
		var removedAt *string
		err := db.QueryRow(
			"SELECT status, COALESCE(archived_from, ''), removed_at FROM nodes WHERE id = ?", nodeID,
		).Scan(&status, &archivedFrom, &removedAt)
		if err != nil || removedAt != nil {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if status != "archived" {
			http.Error(w, `{"error":"node is not archived"}`, http.StatusConflict)
			return
		}

		target := restoreTarget(db, nodeID, archivedFrom)
		_, err = db.Exec(
			"UPDATE nodes SET status = ?, archived_from = NULL, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?",
			target, nodeID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to restore node"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "node.restore", "node", nodeID,
			fmt.Sprintf(`{"restored_to":%q}`, target), clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": target})
	}
}
