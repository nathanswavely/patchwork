package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/eventsource"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// maxSourcesPerNode bounds how many feeds one patch may pull from.
const maxSourcesPerNode = 5

// sourceNodeAccess resolves a slug and answers whether the user may
// manage its event sources (docs/adr/031): patch admins on their own
// active patch, the instance admin on unclaimed patches (who holds those
// calendars in trust). Trusted contributors are deliberately excluded —
// their grant delegates the review queue, not standing feeds.
func sourceNodeAccess(db *database.DB, user *model.User, slug string) (nodeID string, ok bool) {
	var status string
	err := db.QueryRow(
		`SELECT id, status FROM nodes WHERE slug = ? AND status IN ('active','unclaimed') AND removed_at IS NULL`,
		slug,
	).Scan(&nodeID, &status)
	if err != nil {
		return "", false
	}
	if user.Role == "admin" {
		return nodeID, true
	}
	if status == "active" && userHasNodeRole(db, user.ID, nodeID, "admin") {
		return nodeID, true
	}
	return nodeID, false
}

func scanEventSources(db *database.DB, nodeID string) ([]model.EventSource, error) {
	rows, err := db.Query(
		`SELECT s.id, s.node_id, s.type, s.url, s.added_by, s.status,
		 s.last_fetch_at, s.last_success_at, s.last_error,
		 (SELECT COUNT(*) FROM events e WHERE e.source_id = s.id AND e.removed_at IS NULL),
		 s.created_at, s.updated_at
		 FROM event_sources s WHERE s.node_id = ? ORDER BY s.created_at`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sources := []model.EventSource{}
	for rows.Next() {
		var s model.EventSource
		if err := rows.Scan(&s.ID, &s.NodeID, &s.Type, &s.URL, &s.AddedBy, &s.Status,
			&s.LastFetchAt, &s.LastSuccessAt, &s.LastError, &s.EventCount,
			&s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		sources = append(sources, s)
	}
	return sources, rows.Err()
}

// ListEventSources handles GET /api/v1/nodes/{slug}/event-sources.
func ListEventSources(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := sourceNodeAccess(db, user, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !ok {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}
		sources, err := scanEventSources(db, nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to list event sources"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": sources})
	}
}

// CreateEventSource handles POST /api/v1/nodes/{slug}/event-sources.
// Attaching is vouching for the feed once (docs/adr/031); the first sync
// kicks off immediately in the background.
func CreateEventSource(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := sourceNodeAccess(db, user, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !ok {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		var req struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
			http.Error(w, `{"error":"url is required"}`, http.StatusBadRequest)
			return
		}
		if u, err := url.Parse(req.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			// webcal:// is what calendar apps hand people; accept the
			// intent rather than teaching everyone to rewrite it.
			if u, err2 := url.Parse(req.URL); err2 == nil && u.Scheme == "webcal" && u.Host != "" {
				u.Scheme = "https"
				req.URL = u.String()
			} else {
				http.Error(w, `{"error":"url must be http(s)"}`, http.StatusBadRequest)
				return
			}
		}

		var count int
		db.QueryRow(`SELECT COUNT(*) FROM event_sources WHERE node_id = ?`, nodeID).Scan(&count)
		if count >= maxSourcesPerNode {
			http.Error(w, `{"error":"this patch already has the maximum number of event sources"}`, http.StatusConflict)
			return
		}

		id := auth.NewUUIDv7()
		_, err := db.Exec(
			`INSERT INTO event_sources (id, node_id, type, url, added_by) VALUES (?, ?, 'ics', ?, ?)`,
			id, nodeID, req.URL, user.ID,
		)
		if err != nil {
			http.Error(w, `{"error":"this feed is already attached"}`, http.StatusConflict)
			return
		}
		auth.LogAuditEvent(db, user.ID, "event_source.create", "event_source", id, `{"url":"`+req.URL+`"}`, clientIP(r))

		// First sync in the background; the UI polls the source list.
		// Not the request context — the sync must outlive this response.
		go eventsource.Sync(context.Background(), db, pkgNotifier, id)

		sources, err := scanEventSources(db, nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to list event sources"}`, http.StatusInternalServerError)
			return
		}
		for _, s := range sources {
			if s.ID == id {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(s)
				return
			}
		}
		http.Error(w, `{"error":"failed to create event source"}`, http.StatusInternalServerError)
	}
}

// DeleteEventSource handles DELETE /api/v1/nodes/{slug}/event-sources/{id}.
// Past events stay with the patch as detached history; future imported
// events were the feed's promises and go with it (docs/adr/031).
func DeleteEventSource(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := sourceNodeAccess(db, user, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !ok {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}
		sourceID := r.PathValue("id")

		var exists int
		db.QueryRow(`SELECT COUNT(*) FROM event_sources WHERE id = ? AND node_id = ?`, sourceID, nodeID).Scan(&exists)
		if exists == 0 {
			http.Error(w, `{"error":"event source not found"}`, http.StatusNotFound)
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := db.Exec(`DELETE FROM events WHERE source_id = ? AND starts_at > ?`, sourceID, now); err != nil {
			http.Error(w, `{"error":"failed to remove event source"}`, http.StatusInternalServerError)
			return
		}
		if _, err := db.Exec(
			`UPDATE events SET source_id = NULL, source_uid = NULL, source_occurrence = '' WHERE source_id = ?`,
			sourceID,
		); err != nil {
			http.Error(w, `{"error":"failed to remove event source"}`, http.StatusInternalServerError)
			return
		}
		if _, err := db.Exec(`DELETE FROM event_sources WHERE id = ?`, sourceID); err != nil {
			http.Error(w, `{"error":"failed to remove event source"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "event_source.delete", "event_source", sourceID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// SyncEventSource handles POST /api/v1/nodes/{slug}/event-sources/{id}/sync —
// a manual "sync now", lightly rate-limited per source.
func SyncEventSource(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := sourceNodeAccess(db, user, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !ok {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}
		sourceID := r.PathValue("id")

		var lastFetch *string
		err := db.QueryRow(`SELECT last_fetch_at FROM event_sources WHERE id = ? AND node_id = ?`, sourceID, nodeID).Scan(&lastFetch)
		if err != nil {
			http.Error(w, `{"error":"event source not found"}`, http.StatusNotFound)
			return
		}
		if lastFetch != nil {
			if t, err := time.Parse("2006-01-02T15:04:05.000Z", *lastFetch); err == nil && time.Since(t) < time.Minute {
				http.Error(w, `{"error":"this source just synced — try again in a minute"}`, http.StatusTooManyRequests)
				return
			}
		}

		// Synchronous: the caller wants to see the outcome. Errors are
		// recorded on the source row, which is what we return.
		eventsource.Sync(r.Context(), db, pkgNotifier, sourceID)

		sources, err := scanEventSources(db, nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to sync"}`, http.StatusInternalServerError)
			return
		}
		for _, s := range sources {
			if s.ID == sourceID {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(s)
				return
			}
		}
		http.Error(w, `{"error":"event source not found"}`, http.StatusNotFound)
	}
}

// DetachEvent handles POST /api/v1/events/{id}/detach: cut one imported
// event loose from its source. It becomes an ordinary local event and
// the source ignores its feed item from then on (docs/adr/031).
func DetachEvent(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		eventID := r.PathValue("id")

		var nodeID string
		var sourceID, sourceUID *string
		var sourceOccurrence string
		err := db.QueryRow(
			`SELECT node_id, source_id, source_uid, source_occurrence FROM events WHERE id = ? AND removed_at IS NULL`,
			eventID,
		).Scan(&nodeID, &sourceID, &sourceUID, &sourceOccurrence)
		if err != nil {
			http.Error(w, `{"error":"event not found"}`, http.StatusNotFound)
			return
		}
		if sourceID == nil {
			http.Error(w, `{"error":"this event is not attached to an event source"}`, http.StatusBadRequest)
			return
		}
		if user.Role != "admin" && !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		if sourceUID != nil {
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO event_source_skips (source_id, uid, occurrence) VALUES (?, ?, ?)`,
				*sourceID, *sourceUID, sourceOccurrence,
			); err != nil {
				http.Error(w, `{"error":"failed to detach event"}`, http.StatusInternalServerError)
				return
			}
		}
		if _, err := db.Exec(
			`UPDATE events SET source_id = NULL, source_uid = NULL, source_occurrence = '' WHERE id = ?`,
			eventID,
		); err != nil {
			http.Error(w, `{"error":"failed to detach event"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "event.detach", "event", eventID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
