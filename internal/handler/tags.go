package handler

import (
	"encoding/json"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

type tagResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Motif is the optional motif slug this tag contributes to patches that
	// chose no explicit motif (docs/adr/021). Slugs name entries in the
	// frontend motif registry; unknown slugs fall through there (ADR 004),
	// so the server treats this as an opaque string.
	Motif     string `json:"motif,omitempty"`
	CreatedAt string `json:"created_at"`
	NodeCount int    `json:"node_count"`
}

// ListTags handles GET /api/v1/tags.
func ListTags(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// node_count only counts patches the public tree would show —
		// this endpoint is unauthenticated, so private patches must not
		// leak into the totals.
		rows, err := db.Query(`
			SELECT t.id, t.name, COALESCE(t.motif,''), t.created_at,
				(SELECT COUNT(*) FROM node_tags nt
				 JOIN nodes n ON n.id = nt.node_id
				 WHERE nt.tag_id = t.id
				   AND n.status IN ('active','unclaimed')
				   AND n.removed_at IS NULL
				   AND n.visibility = 'public') AS node_count
			FROM tags t
			ORDER BY t.name ASC`)
		if err != nil {
			http.Error(w, `{"error":"failed to list tags"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var tags []tagResponse
		for rows.Next() {
			var t tagResponse
			if err := rows.Scan(&t.ID, &t.Name, &t.Motif, &t.CreatedAt, &t.NodeCount); err != nil {
				continue
			}
			tags = append(tags, t)
		}
		if tags == nil {
			tags = []tagResponse{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tags)
	}
}

// CreateTag handles POST /api/v1/admin/tags.
func CreateTag(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name  string `json:"name"`
			Motif string `json:"motif"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
			http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
			return
		}

		id := auth.NewUUIDv7()
		var motif interface{}
		if req.Motif != "" {
			motif = req.Motif
		}
		_, err := db.Exec("INSERT INTO tags (id, name, motif) VALUES (?, ?, ?)", id, req.Name, motif)
		if err != nil {
			http.Error(w, `{"error":"tag already exists or failed to create"}`, http.StatusConflict)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(tagResponse{ID: id, Name: req.Name, Motif: req.Motif})
	}
}

// UpdateTag handles PATCH /api/v1/admin/tags/{id}.
// Only the motif is mutable: renaming a tag would silently change what every
// patch wearing it says about itself, so a rename is delete + create.
func UpdateTag(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tagID := r.PathValue("id")
		if tagID == "" {
			http.Error(w, `{"error":"tag id required"}`, http.StatusBadRequest)
			return
		}

		var req struct {
			Motif *string `json:"motif"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Motif == nil {
			http.Error(w, `{"error":"motif is required (empty string clears it)"}`, http.StatusBadRequest)
			return
		}

		var motif interface{}
		if *req.Motif != "" {
			motif = *req.Motif
		}
		result, err := db.Exec("UPDATE tags SET motif = ? WHERE id = ?", motif, tagID)
		if err != nil {
			http.Error(w, `{"error":"failed to update tag"}`, http.StatusInternalServerError)
			return
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			http.Error(w, `{"error":"tag not found"}`, http.StatusNotFound)
			return
		}

		var t tagResponse
		db.QueryRow("SELECT id, name, COALESCE(motif,''), created_at FROM tags WHERE id = ?", tagID).
			Scan(&t.ID, &t.Name, &t.Motif, &t.CreatedAt)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(t)
	}
}

// DeleteTag handles DELETE /api/v1/admin/tags/{id}.
func DeleteTag(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tagID := r.PathValue("id")
		if tagID == "" {
			http.Error(w, `{"error":"tag id required"}`, http.StatusBadRequest)
			return
		}

		// Delete associations first, then tag.
		db.Exec("DELETE FROM node_tags WHERE tag_id = ?", tagID)
		result, err := db.Exec("DELETE FROM tags WHERE id = ?", tagID)
		if err != nil {
			http.Error(w, `{"error":"failed to delete tag"}`, http.StatusInternalServerError)
			return
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			http.Error(w, `{"error":"tag not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// resolveTagIDs maps tag names to IDs, preserving order. Unknown names are
// rejected — patch admins pick from the curated vocabulary, they don't extend
// it (docs/adr/021).
func resolveTagIDs(db *database.DB, names []string) ([]string, string) {
	ids := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		var id string
		if err := db.QueryRow("SELECT id FROM tags WHERE name = ?", name).Scan(&id); err != nil {
			return nil, name
		}
		ids = append(ids, id)
	}
	return ids, ""
}

// setNodeTags replaces a node's tags with the given tag IDs, storing the
// array order as position — the patch admin's priority order. The first
// motif-bearing tag wins motif derivation on the frontend.
func setNodeTags(db *database.DB, nodeID string, tagIDs []string) error {
	if _, err := db.Exec("DELETE FROM node_tags WHERE node_id = ?", nodeID); err != nil {
		return err
	}
	for i, tagID := range tagIDs {
		if _, err := db.Exec(
			"INSERT INTO node_tags (node_id, tag_id, position) VALUES (?, ?, ?)",
			nodeID, tagID, i,
		); err != nil {
			return err
		}
	}
	return nil
}

// nodeTagNames returns a node's tag names in stored (priority) order.
func nodeTagNames(db *database.DB, nodeID string) []string {
	tags := []string{}
	rows, err := db.Query(
		`SELECT t.name FROM node_tags nt JOIN tags t ON nt.tag_id = t.id
		 WHERE nt.node_id = ? ORDER BY COALESCE(nt.position, 1000000), t.name`, nodeID,
	)
	if err != nil {
		return tags
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			tags = append(tags, name)
		}
	}
	return tags
}
