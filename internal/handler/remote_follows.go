package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// Cross-quilt following (docs/adr/024). A remote follow is a row on the
// follower's home instance; when federation is enabled on both ends the
// instance service actor relays a single AP Follow per remote patch —
// never the person's own actor, so no one is enumerable in a remote
// followers collection. Follows are never auto-deleted: a patch missing
// from remote public data may have been deleted or gone private, and the
// two are indistinguishable from outside.

const snapshotMaxBytes = 8 * 1024

// normalizeOrigin validates a quilt URL and reduces it to scheme://host.
func normalizeOrigin(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return u.Scheme + "://" + u.Host, true
}

// remoteFollowJSON shapes a RemoteFollow for API responses, inlining the
// raw snapshot JSON.
func remoteFollowJSON(f model.RemoteFollow) map[string]interface{} {
	var snapshot interface{} = map[string]interface{}{}
	if f.Snapshot != "" {
		var parsed interface{}
		if err := json.Unmarshal([]byte(f.Snapshot), &parsed); err == nil {
			snapshot = parsed
		}
	}
	return map[string]interface{}{
		"id":         f.ID,
		"quilt_url":  f.QuiltURL,
		"node_ap_id": f.NodeAPID,
		"node_slug":  f.NodeSlug,
		"node_name":  f.NodeName,
		"snapshot":   snapshot,
		"created_at": f.CreatedAt,
	}
}

// ListRemoteFollows handles GET /api/v1/users/me/remote-follows.
func ListRemoteFollows(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		rows, err := db.Query(
			`SELECT id, user_id, quilt_url, node_ap_id, node_slug, node_name, snapshot, created_at
			 FROM remote_follows WHERE user_id = ? ORDER BY quilt_url, created_at`,
			user.ID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to load remote follows"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		follows := []map[string]interface{}{}
		for rows.Next() {
			var f model.RemoteFollow
			if err := rows.Scan(&f.ID, &f.UserID, &f.QuiltURL, &f.NodeAPID, &f.NodeSlug, &f.NodeName, &f.Snapshot, &f.CreatedAt); err != nil {
				continue
			}
			follows = append(follows, remoteFollowJSON(f))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"remote_follows": follows})
	}
}

// CreateRemoteFollow handles POST /api/v1/users/me/remote-follows.
// Body: {quilt_url, node_ap_id, node_slug, node_name, snapshot}.
// Idempotent per (user, node_ap_id): re-following refreshes the snapshot.
func CreateRemoteFollow(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			QuiltURL string          `json:"quilt_url"`
			NodeAPID string          `json:"node_ap_id"`
			NodeSlug string          `json:"node_slug"`
			NodeName string          `json:"node_name"`
			Snapshot json.RawMessage `json:"snapshot"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		origin, ok := normalizeOrigin(req.QuiltURL)
		if !ok {
			http.Error(w, `{"error":"quilt_url must be an http(s) URL"}`, http.StatusBadRequest)
			return
		}
		apID, err := url.Parse(strings.TrimSpace(req.NodeAPID))
		if err != nil || (apID.Scheme != "http" && apID.Scheme != "https") || apID.Host == "" {
			http.Error(w, `{"error":"node_ap_id must be an http(s) URL"}`, http.StatusBadRequest)
			return
		}
		originHost := strings.TrimPrefix(origin, "https://")
		originHost = strings.TrimPrefix(originHost, "http://")
		if apID.Host != originHost {
			http.Error(w, `{"error":"node_ap_id host must match quilt_url"}`, http.StatusBadRequest)
			return
		}
		slug := strings.TrimSpace(req.NodeSlug)
		if slug == "" {
			http.Error(w, `{"error":"node_slug is required"}`, http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(req.NodeName)
		if len(name) > 200 {
			name = name[:200]
		}
		snapshot := "{}"
		if len(req.Snapshot) > 0 {
			if len(req.Snapshot) > snapshotMaxBytes {
				http.Error(w, `{"error":"snapshot too large"}`, http.StatusBadRequest)
				return
			}
			var parsed map[string]interface{}
			if err := json.Unmarshal(req.Snapshot, &parsed); err != nil {
				http.Error(w, `{"error":"snapshot must be a JSON object"}`, http.StatusBadRequest)
				return
			}
			snapshot = string(req.Snapshot)
		}

		id := auth.NewUUIDv7()
		_, err = db.Exec(
			`INSERT INTO remote_follows (id, user_id, quilt_url, node_ap_id, node_slug, node_name, snapshot)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(user_id, node_ap_id) DO UPDATE SET
			   quilt_url = excluded.quilt_url, node_slug = excluded.node_slug,
			   node_name = excluded.node_name, snapshot = excluded.snapshot`,
			id, user.ID, origin, apID.String(), slug, name, snapshot,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to save follow"}`, http.StatusInternalServerError)
			return
		}

		// Upgrade over federation: relay one Follow per remote patch from
		// the instance service actor (docs/adr/024). Fire-and-forget — the
		// row is the truth the UI renders either way.
		if cfg.Federation.Enabled {
			go relayFollow(db, apID.String())
		}

		var f model.RemoteFollow
		err = db.QueryRow(
			`SELECT id, user_id, quilt_url, node_ap_id, node_slug, node_name, snapshot, created_at
			 FROM remote_follows WHERE user_id = ? AND node_ap_id = ?`,
			user.ID, apID.String(),
		).Scan(&f.ID, &f.UserID, &f.QuiltURL, &f.NodeAPID, &f.NodeSlug, &f.NodeName, &f.Snapshot, &f.CreatedAt)
		if err != nil {
			http.Error(w, `{"error":"failed to load follow"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(remoteFollowJSON(f))
	}
}

// UpdateRemoteFollow handles PATCH /api/v1/users/me/remote-follows/{id}.
// Used by the SPA to refresh a follow's display snapshot opportunistically
// after a successful remote fetch.
func UpdateRemoteFollow(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		followID := r.PathValue("id")

		var req struct {
			NodeName *string         `json:"node_name"`
			Snapshot json.RawMessage `json:"snapshot"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		if len(req.Snapshot) > 0 {
			if len(req.Snapshot) > snapshotMaxBytes {
				http.Error(w, `{"error":"snapshot too large"}`, http.StatusBadRequest)
				return
			}
			var parsed map[string]interface{}
			if err := json.Unmarshal(req.Snapshot, &parsed); err != nil {
				http.Error(w, `{"error":"snapshot must be a JSON object"}`, http.StatusBadRequest)
				return
			}
			if _, err := db.Exec(
				"UPDATE remote_follows SET snapshot = ? WHERE id = ? AND user_id = ?",
				string(req.Snapshot), followID, user.ID,
			); err != nil {
				http.Error(w, `{"error":"failed to update follow"}`, http.StatusInternalServerError)
				return
			}
		}
		if req.NodeName != nil {
			name := strings.TrimSpace(*req.NodeName)
			if len(name) > 200 {
				name = name[:200]
			}
			if _, err := db.Exec(
				"UPDATE remote_follows SET node_name = ? WHERE id = ? AND user_id = ?",
				name, followID, user.ID,
			); err != nil {
				http.Error(w, `{"error":"failed to update follow"}`, http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// DeleteRemoteFollow handles DELETE /api/v1/users/me/remote-follows/{id}.
// Only the person ends a follow (docs/adr/024) — this is the one way a
// remote follow dies.
func DeleteRemoteFollow(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		followID := r.PathValue("id")

		var nodeAPID string
		err := db.QueryRow(
			"SELECT node_ap_id FROM remote_follows WHERE id = ? AND user_id = ?",
			followID, user.ID,
		).Scan(&nodeAPID)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"follow not found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"failed to load follow"}`, http.StatusInternalServerError)
			return
		}

		if _, err := db.Exec("DELETE FROM remote_follows WHERE id = ? AND user_id = ?", followID, user.ID); err != nil {
			http.Error(w, `{"error":"failed to delete follow"}`, http.StatusInternalServerError)
			return
		}

		// If nobody local follows this remote patch anymore, withdraw the
		// instance actor's relayed Follow.
		if cfg.Federation.Enabled {
			go relayUnfollow(db, nodeAPID)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// relayFollow sends one Follow from the instance service actor to a
// remote patch actor, unless one is already outstanding.
func relayFollow(db *database.DB, nodeAPID string) {
	var existing string
	err := db.QueryRow("SELECT id FROM ap_following WHERE remote_actor_id = ?", nodeAPID).Scan(&existing)
	if err == nil {
		return // already relayed (accepted or pending)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		log.Printf("ap: relay follow lookup for %s: %v", nodeAPID, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	inbox := nodeAPID + "/inbox"
	if remote, err := ap.FetchActor(ctx, nodeAPID); err == nil && remote.Inbox != "" {
		inbox = remote.Inbox
	}

	rowID := auth.NewUUIDv7()
	if _, err := db.Exec(
		"INSERT INTO ap_following (id, remote_actor_id, remote_inbox) VALUES (?, ?, ?)",
		rowID, nodeAPID, inbox,
	); err != nil {
		log.Printf("ap: record outbound follow for %s: %v", nodeAPID, err)
		return
	}

	instanceID := ap.InstanceAPID(ap.GetDomain())
	followAPID := instanceID + "/follows/" + rowID
	if err := ap.QueueActivity(db, ap.BuildFollow(followAPID, instanceID, nodeAPID), inbox); err != nil {
		log.Printf("ap: queue follow for %s: %v", nodeAPID, err)
	}
}

// relayUnfollow withdraws the instance actor's Follow of a remote patch
// once the last local follower is gone.
func relayUnfollow(db *database.DB, nodeAPID string) {
	var remaining int
	if err := db.QueryRow("SELECT COUNT(*) FROM remote_follows WHERE node_ap_id = ?", nodeAPID).Scan(&remaining); err != nil || remaining > 0 {
		return
	}

	var rowID, inbox string
	err := db.QueryRow("SELECT id, remote_inbox FROM ap_following WHERE remote_actor_id = ?", nodeAPID).Scan(&rowID, &inbox)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		log.Printf("ap: relay unfollow lookup for %s: %v", nodeAPID, err)
		return
	}

	instanceID := ap.InstanceAPID(ap.GetDomain())
	followAPID := instanceID + "/follows/" + rowID
	if inbox == "" {
		inbox = nodeAPID + "/inbox"
	}
	if err := ap.QueueActivity(db, ap.BuildUndoFollow(followAPID, instanceID, nodeAPID), inbox); err != nil {
		log.Printf("ap: queue undo follow for %s: %v", nodeAPID, err)
		return
	}
	if _, err := db.Exec("DELETE FROM ap_following WHERE id = ?", rowID); err != nil {
		log.Printf("ap: delete outbound follow row %s: %v", rowID, err)
	}
}

// ListUserQuilts handles GET /api/v1/users/me/quilts.
func ListUserQuilts(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		rows, err := db.Query(
			"SELECT id, user_id, url, name, created_at FROM user_quilts WHERE user_id = ? ORDER BY created_at",
			user.ID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to load quilts"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		quilts := []model.UserQuilt{}
		for rows.Next() {
			var q model.UserQuilt
			if err := rows.Scan(&q.ID, &q.UserID, &q.URL, &q.Name, &q.CreatedAt); err != nil {
				continue
			}
			quilts = append(quilts, q)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"quilts": quilts})
	}
}

// AddUserQuilt handles POST /api/v1/users/me/quilts. Body: {url, name}.
func AddUserQuilt(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			URL  string `json:"url"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		origin, ok := normalizeOrigin(req.URL)
		if !ok {
			http.Error(w, `{"error":"url must be an http(s) URL"}`, http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(req.Name)
		if len(name) > 200 {
			name = name[:200]
		}

		id := auth.NewUUIDv7()
		_, err := db.Exec(
			`INSERT INTO user_quilts (id, user_id, url, name) VALUES (?, ?, ?, ?)
			 ON CONFLICT(user_id, url) DO UPDATE SET name = excluded.name`,
			id, user.ID, origin, name,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to save quilt"}`, http.StatusInternalServerError)
			return
		}

		var q model.UserQuilt
		err = db.QueryRow(
			"SELECT id, user_id, url, name, created_at FROM user_quilts WHERE user_id = ? AND url = ?",
			user.ID, origin,
		).Scan(&q.ID, &q.UserID, &q.URL, &q.Name, &q.CreatedAt)
		if err != nil {
			http.Error(w, `{"error":"failed to load quilt"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(q)
	}
}

// DeleteUserQuilt handles DELETE /api/v1/users/me/quilts/{id}.
// Disconnecting a quilt never cascades to its follows (docs/adr/024):
// connection is a browsing convenience, follows are relationships.
func DeleteUserQuilt(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		quiltID := r.PathValue("id")

		res, err := db.Exec("DELETE FROM user_quilts WHERE id = ? AND user_id = ?", quiltID, user.ID)
		if err != nil {
			http.Error(w, `{"error":"failed to delete quilt"}`, http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			http.Error(w, `{"error":"quilt not found"}`, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// AdminListNeighborQuilts handles GET /api/v1/admin/neighbor-quilts.
func AdminListNeighborQuilts(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		quilts, err := loadNeighborQuilts(db)
		if err != nil {
			http.Error(w, `{"error":"failed to load neighbor quilts"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"neighbor_quilts": quilts})
	}
}

// AdminAddNeighborQuilt handles POST /api/v1/admin/neighbor-quilts.
func AdminAddNeighborQuilt(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())

		var req struct {
			URL  string `json:"url"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		origin, ok := normalizeOrigin(req.URL)
		if !ok {
			http.Error(w, `{"error":"url must be an http(s) URL"}`, http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(req.Name)
		if len(name) > 200 {
			name = name[:200]
		}

		var maxPos sql.NullInt64
		db.QueryRow("SELECT MAX(position) FROM neighbor_quilts").Scan(&maxPos)

		id := auth.NewUUIDv7()
		_, err := db.Exec(
			`INSERT INTO neighbor_quilts (id, url, name, position) VALUES (?, ?, ?, ?)
			 ON CONFLICT(url) DO UPDATE SET name = excluded.name`,
			id, origin, name, maxPos.Int64+1,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to save neighbor quilt"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, adminUser.ID, "admin.neighbor_quilt_add", "neighbor_quilt", origin, "{}", clientIP(r))

		var q model.NeighborQuilt
		err = db.QueryRow(
			"SELECT id, url, name, position, created_at FROM neighbor_quilts WHERE url = ?",
			origin,
		).Scan(&q.ID, &q.URL, &q.Name, &q.Position, &q.CreatedAt)
		if err != nil {
			http.Error(w, `{"error":"failed to load neighbor quilt"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(q)
	}
}

// AdminDeleteNeighborQuilt handles DELETE /api/v1/admin/neighbor-quilts/{id}.
func AdminDeleteNeighborQuilt(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())
		quiltID := r.PathValue("id")

		var origin string
		if err := db.QueryRow("SELECT url FROM neighbor_quilts WHERE id = ?", quiltID).Scan(&origin); errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"neighbor quilt not found"}`, http.StatusNotFound)
			return
		}

		if _, err := db.Exec("DELETE FROM neighbor_quilts WHERE id = ?", quiltID); err != nil {
			http.Error(w, `{"error":"failed to delete neighbor quilt"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, adminUser.ID, "admin.neighbor_quilt_remove", "neighbor_quilt", origin, "{}", clientIP(r))
		w.WriteHeader(http.StatusNoContent)
	}
}

// loadNeighborQuilts returns the admin-curated neighbor list in order.
// Shared by the admin endpoint and the public instance document.
func loadNeighborQuilts(db *database.DB) ([]model.NeighborQuilt, error) {
	rows, err := db.Query("SELECT id, url, name, position, created_at FROM neighbor_quilts ORDER BY position, created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	quilts := []model.NeighborQuilt{}
	for rows.Next() {
		var q model.NeighborQuilt
		if err := rows.Scan(&q.ID, &q.URL, &q.Name, &q.Position, &q.CreatedAt); err != nil {
			continue
		}
		quilts = append(quilts, q)
	}
	return quilts, rows.Err()
}
