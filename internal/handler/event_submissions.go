package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// eventSubmission is a pending event plus enough context to review it.
type eventSubmission struct {
	model.Event
	NodeName          string `json:"node_name"`
	NodeSlug          string `json:"node_slug"`
	NodeStatus        string `json:"node_status"`
	SubmitterUsername string `json:"submitter_username"`
	SubmitterDisplay  string `json:"submitter_display_name"`
}

const eventSubmissionSelect = `SELECT e.id, e.node_id, e.created_by, e.title, e.description, e.location,
	e.latitude, e.longitude, e.starts_at, e.ends_at, e.recurrence, e.visibility, e.status,
	e.created_at, e.updated_at, n.name, n.slug, n.status,
	COALESCE(u.username,''), COALESCE(u.display_name,'')
	FROM events e
	JOIN nodes n ON n.id = e.node_id
	LEFT JOIN users u ON u.id = e.created_by
	WHERE e.status = 'pending_review' AND e.removed_at IS NULL`

func scanEventSubmissions(db *database.DB, query string, args ...interface{}) ([]eventSubmission, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []eventSubmission
	for rows.Next() {
		var s eventSubmission
		if err := rows.Scan(&s.ID, &s.NodeID, &s.CreatedBy, &s.Title, &s.Description, &s.Location,
			&s.Latitude, &s.Longitude, &s.StartsAt, &s.EndsAt, &s.Recurrence, &s.Visibility, &s.Status,
			&s.CreatedAt, &s.UpdatedAt, &s.NodeName, &s.NodeSlug, &s.NodeStatus,
			&s.SubmitterUsername, &s.SubmitterDisplay); err != nil {
			continue
		}
		items = append(items, s)
	}
	if items == nil {
		items = []eventSubmission{}
	}
	return items, nil
}

// ListAdminEventSubmissions handles GET /api/v1/admin/event-submissions.
// The instance admin's queue: pending events on unclaimed patches, whose
// calendars the instance admin holds in trust (docs/adr/026).
func ListAdminEventSubmissions(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		after, limit := parsePaginationParams(r)

		query := eventSubmissionSelect + " AND n.status = 'unclaimed'"
		args := []interface{}{}
		if sortKey, id, ok := decodeCursor(after); after != "" && ok {
			query += " AND " + keysetCondition("e.created_at", "e.id", true)
			args = append(args, sortKey, sortKey, id)
		}
		query += " ORDER BY e.created_at DESC, e.id DESC LIMIT ?"
		args = append(args, limit+1)

		items, err := scanEventSubmissions(db, query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to query event submissions"}`, http.StatusInternalServerError)
			return
		}

		hasMore := len(items) > limit
		if hasMore {
			items = items[:limit]
		}
		resp := map[string]interface{}{"items": items}
		if hasMore && len(items) > 0 {
			last := items[len(items)-1]
			resp["next_cursor"] = encodeCursor(last.CreatedAt, last.ID)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// ListNodeEventSubmissions handles GET /api/v1/nodes/{slug}/event-submissions.
// A patch admin's queue: pending suggestions to their own patch.
func ListNodeEventSubmissions(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		var nodeID string
		err := db.QueryRow("SELECT id FROM nodes WHERE slug = ? AND removed_at IS NULL", slug).Scan(&nodeID)
		if err != nil {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if user.Role != "admin" && !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		items, err := scanEventSubmissions(db,
			eventSubmissionSelect+" AND e.node_id = ? ORDER BY e.created_at DESC, e.id DESC", nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to query event submissions"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"items": items})
	}
}

// ReviewEventSubmission handles PATCH /api/v1/events/{id}/review.
// Review is owed to whoever owns the calendar (docs/adr/026): the
// instance admin for unclaimed patches, the patch's admins for active
// ones. Approval publishes the event — notifications and federation fire
// exactly as if it had just been posted directly.
func ReviewEventSubmission(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		eventID := r.PathValue("id")

		var req struct {
			Action string `json:"action"` // "approve" or "reject"
			Note   string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Action != "approve" && req.Action != "reject" {
			http.Error(w, `{"error":"action must be 'approve' or 'reject'"}`, http.StatusBadRequest)
			return
		}

		var e model.Event
		var nodeStatus, nodeSlug, nodeName string
		err := db.QueryRow(
			`SELECT e.id, e.node_id, e.created_by, e.title, e.visibility, e.status, n.status, n.slug, n.name
			 FROM events e JOIN nodes n ON n.id = e.node_id
			 WHERE e.id = ? AND e.removed_at IS NULL`, eventID,
		).Scan(&e.ID, &e.NodeID, &e.CreatedBy, &e.Title, &e.Visibility, &e.Status, &nodeStatus, &nodeSlug, &nodeName)
		if err != nil {
			http.Error(w, `{"error":"event not found"}`, http.StatusNotFound)
			return
		}
		if e.Status != "pending_review" {
			http.Error(w, `{"error":"event is not awaiting review"}`, http.StatusBadRequest)
			return
		}

		// Unclaimed calendars are reviewed by the instance admin alone;
		// active calendars by that patch's admins (global admin override
		// applies, as on every node endpoint).
		if nodeStatus == "unclaimed" {
			if user.Role != "admin" {
				http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
				return
			}
		} else {
			if user.Role != "admin" && !userHasNodeRole(db, user.ID, e.NodeID, "admin") {
				http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
				return
			}
		}

		switch req.Action {
		case "approve":
			_, err := db.Exec(
				"UPDATE events SET status = 'active', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?",
				eventID,
			)
			if err != nil {
				http.Error(w, `{"error":"failed to approve event"}`, http.StatusInternalServerError)
				return
			}
			auth.LogAuditEvent(db, user.ID, "event.submission_approved", "event", eventID, "{}", clientIP(r))

			notify(notifications.Event{
				Type:     notifications.EventSubmissionApproved,
				NodeID:   e.NodeID,
				NodeSlug: nodeSlug,
				NodeName: nodeName,
				ActorID:  user.ID,
				TargetID: e.CreatedBy,
				EntityID: eventID,
				Title:    "Your event was approved: " + e.Title,
				Link:     "/patches/" + nodeSlug + "/events/" + eventID,
			})
			notify(notifications.Event{
				Type:     notifications.EventCreated,
				NodeID:   e.NodeID,
				NodeSlug: nodeSlug,
				NodeName: nodeName,
				ActorID:  user.ID,
				EntityID: eventID,
				Title:    "New event: " + e.Title,
				Link:     "/patches/" + nodeSlug + "/events/" + eventID,
			})

			var full model.Event
			db.QueryRow(
				`SELECT id, node_id, created_by, title, description, location, latitude, longitude, starts_at, ends_at, recurrence, visibility, status, created_at, updated_at
				 FROM events WHERE id = ?`, eventID,
			).Scan(&full.ID, &full.NodeID, &full.CreatedBy, &full.Title, &full.Description, &full.Location, &full.Latitude, &full.Longitude, &full.StartsAt, &full.EndsAt, &full.Recurrence, &full.Visibility, &full.Status, &full.CreatedAt, &full.UpdatedAt)
			broadcastEventCreate(db, full, e.NodeID)

		case "reject":
			if _, err := db.Exec("DELETE FROM events WHERE id = ?", eventID); err != nil {
				http.Error(w, `{"error":"failed to reject event"}`, http.StatusInternalServerError)
				return
			}
			auth.LogAuditEvent(db, user.ID, "event.submission_rejected", "event", eventID, fmt.Sprintf(`{"note":%q}`, req.Note), clientIP(r))

			notify(notifications.Event{
				Type:     notifications.EventSubmissionRejected,
				NodeID:   e.NodeID,
				NodeSlug: nodeSlug,
				NodeName: nodeName,
				ActorID:  user.ID,
				TargetID: e.CreatedBy,
				EntityID: eventID,
				Title:    "Your event was declined: " + e.Title,
				Body:     req.Note,
				Link:     "/patches/" + nodeSlug,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
