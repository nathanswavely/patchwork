package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// ListEvents handles GET /api/v1/events.
func ListEvents(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		after, limit := parsePaginationParams(r)
		nodeID := r.URL.Query().Get("node_id")
		nodeSlug := r.URL.Query().Get("node_slug")
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")

		// Resolve node_slug to node_id if provided.
		if nodeSlug != "" && nodeID == "" {
			nodeID = NodeIDFromSlug(db, nodeSlug)
			if nodeID == "" {
				// Slug didn't match any node — return empty results.
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"items":       []model.Event{},
					"next_cursor": "",
				})
				return
			}
		}

		query := `SELECT e.id, e.node_id, e.created_by, e.title, e.description, e.location, e.latitude, e.longitude, e.starts_at, e.ends_at, e.recurrence, e.visibility, e.created_at, e.updated_at, n.name AS node_name, n.slug AS node_slug, n.status AS node_status FROM events e JOIN nodes n ON e.node_id = n.id`
		var conditions []string
		var args []interface{}

		conditions = append(conditions, "e.visibility = 'public'")
		conditions = append(conditions, "e.removed_at IS NULL")
		conditions = append(conditions, "e.status = 'active'")

		// The hosting patch gates its events: an archived or removed patch's
		// calendar is gone from every listing. Visibility gates discovery
		// only — a private patch is unlisted, not locked, so its own page
		// (reached by slug, matching GetNode) still lists its events, but
		// the instance-wide feed and map never surface them.
		conditions = append(conditions, "n.status IN ('active','unclaimed')")
		conditions = append(conditions, "n.removed_at IS NULL")
		if nodeID == "" {
			conditions = append(conditions, "n.visibility = 'public'")

			// Amended-lining discovery filter (docs/adr/036): the instance-wide
			// feed drops a diverged patch's events, except for viewers who
			// knowingly joined or followed it — the filter protects browsers,
			// not participants. A patch's own page (nodeID != "") is a direct
			// link, and direct links always work.
			if hideAmendedLinings(db, r) {
				viewer := middleware.UserFromContext(r.Context())
				for divergedID, status := range NodeLiningStatuses(db) {
					if status != governance.LiningDiverged {
						continue
					}
					if viewer != nil && userHasNodeRole(db, viewer.ID, divergedID, "admin", "member", "follower") {
						continue
					}
					conditions = append(conditions, "e.node_id != ?")
					args = append(args, divergedID)
				}
			}
		}

		if nodeID != "" {
			// A patch's calendar carries its own events plus confirmed
			// event links (docs/adr/032). A private patch's events never
			// blend onto another patch's page — its calendar stays its own.
			conditions = append(conditions, `(e.node_id = ? OR EXISTS (
				SELECT 1 FROM event_links el WHERE el.event_id = e.id
				AND el.node_id = ? AND el.status = 'confirmed'))`)
			args = append(args, nodeID, nodeID)
			conditions = append(conditions, "(n.visibility = 'public' OR e.node_id = ?)")
			args = append(args, nodeID)
		}
		if from != "" {
			conditions = append(conditions, "e.starts_at >= ?")
			args = append(args, from)
		}
		if to != "" {
			conditions = append(conditions, "e.starts_at <= ?")
			args = append(args, to)
		}
		if sortKey, id, ok := decodeCursor(after); after != "" && ok {
			conditions = append(conditions, keysetCondition("e.starts_at", "e.id", false))
			args = append(args, sortKey, sortKey, id)
		}

		query += " WHERE " + strings.Join(conditions, " AND ")
		query += " ORDER BY e.starts_at ASC, e.id ASC LIMIT ?"
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list events"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		// node_status travels with each event so feeds can derive the
		// community-submitted label (docs/adr/026) where the event renders
		// away from its patch.
		type eventWithNode struct {
			model.Event
			NodeName   string `json:"node_name"`
			NodeSlug   string `json:"node_slug"`
			NodeStatus string `json:"node_status"`
		}

		var events []eventWithNode
		for rows.Next() {
			var e eventWithNode
			if err := rows.Scan(&e.ID, &e.NodeID, &e.CreatedBy, &e.Title, &e.Description, &e.Location, &e.Latitude, &e.Longitude, &e.StartsAt, &e.EndsAt, &e.Recurrence, &e.Visibility, &e.CreatedAt, &e.UpdatedAt, &e.NodeName, &e.NodeSlug, &e.NodeStatus); err != nil {
				continue
			}
			events = append(events, e)
		}

		var nextCursor string
		if len(events) > limit {
			nextCursor = encodeCursor(events[limit-1].StartsAt, events[limit-1].ID)
			events = events[:limit]
		}
		if events == nil {
			events = []eventWithNode{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":       events,
			"next_cursor": nextCursor,
		})
	}
}

// GetEvent handles GET /api/v1/events/{id}.
func GetEvent(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		eventID := r.PathValue("id")
		if eventID == "" {
			http.Error(w, `{"error":"event id required"}`, http.StatusBadRequest)
			return
		}

		var e struct {
			model.Event
			NodeName   string               `json:"node_name"`
			NodeSlug   string               `json:"node_slug"`
			NodeStatus string               `json:"node_status"`
			Links      []model.EventLink    `json:"links"`
			Mentions   []model.EventMention `json:"mentions"`
		}
		// An archived or removed patch takes its events with it — same gate
		// as GetNode, so an event link doesn't outlive its patch page.
		err := db.QueryRow(
			`SELECT e.id, e.node_id, e.created_by, e.title, e.description, e.location, e.latitude, e.longitude, e.starts_at, e.ends_at, e.recurrence, e.visibility, e.status, e.source_id, e.created_at, e.updated_at, n.name AS node_name, n.slug AS node_slug, n.status AS node_status
			 FROM events e JOIN nodes n ON e.node_id = n.id
			 WHERE e.id = ? AND e.removed_at IS NULL
			   AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL`, eventID,
		).Scan(&e.ID, &e.NodeID, &e.CreatedBy, &e.Title, &e.Description, &e.Location, &e.Latitude, &e.Longitude, &e.StartsAt, &e.EndsAt, &e.Recurrence, &e.Visibility, &e.Status, &e.SourceID, &e.CreatedAt, &e.UpdatedAt, &e.NodeName, &e.NodeSlug, &e.NodeStatus)
		if err != nil {
			http.Error(w, `{"error":"event not found"}`, http.StatusNotFound)
			return
		}

		// A pending submission is visible only to its submitter and its
		// reviewers (docs/adr/026) — to everyone else it doesn't exist yet.
		user := middleware.UserFromContext(r.Context())
		if e.Status == "pending_review" {
			if user == nil || (user.ID != e.CreatedBy && user.Role != "admin" && !userHasNodeRole(db, user.ID, e.NodeID, "admin")) {
				http.Error(w, `{"error":"event not found"}`, http.StatusNotFound)
				return
			}
		}

		// Event links and cross-quilt mentions (docs/adr/032): confirmed
		// links and mentions are public; pending links only reach the
		// admins who could act on them.
		e.Links = eventLinksForViewer(db, user, e.ID, e.NodeID)
		e.Mentions = eventMentions(db, e.ID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(e)
	}
}

// broadcastEventCreate federates a public event to the patch's AP
// followers (docs/adr/024) — this is what makes a cross-quilt follow more
// than a bookmark. Private/unlisted and pending events never federate.
func broadcastEventCreate(db *database.DB, e model.Event, nodeID string) {
	if e.Visibility != "public" {
		return
	}
	go func() {
		obj := ap.EventToObject(e, ap.GetDomain())
		activity := map[string]interface{}{
			"@context": ap.Context,
			"type":     "Create",
			"id":       obj.ID + "/activity",
			"actor":    ap.NodeAPID(ap.GetDomain(), nodeID),
			"object":   obj,
		}
		ap.BroadcastToFollowers(db, "node", nodeID, activity)
	}()
}

// CreateEvent handles POST /api/v1/events.
//
// Who may post directly and who lands in review follows docs/adr/026:
// review is owed to whoever owns the calendar. Members and admins post
// directly to their own patch; trusted contributors post directly to
// unclaimed patches; everyone else's event enters pending_review for the
// calendar's owner (instance admin for unclaimed, patch admins for
// active patches).
func CreateEvent(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			NodeID      string   `json:"node_id"`
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Location    string   `json:"location"`
			Latitude    *float64 `json:"latitude"`
			Longitude   *float64 `json:"longitude"`
			StartsAt    string   `json:"starts_at"`
			EndsAt      *string  `json:"ends_at"`
			Recurrence  string   `json:"recurrence"`
			Visibility  string   `json:"visibility"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.NodeID == "" || req.Title == "" || req.StartsAt == "" {
			http.Error(w, `{"error":"node_id, title, and starts_at are required"}`, http.StatusBadRequest)
			return
		}
		if req.Visibility == "" {
			req.Visibility = "public"
		}

		// Verify node exists and load what the authz decision needs.
		var nodeStatus string
		var acceptSuggestions bool
		err := db.QueryRow(
			"SELECT status, accept_event_suggestions FROM nodes WHERE id = ? AND status IN ('active','unclaimed') AND removed_at IS NULL",
			req.NodeID,
		).Scan(&nodeStatus, &acceptSuggestions)
		if err != nil {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// Direct posting rights; everyone else may still submit for review.
		direct := user.Role == "admin"
		switch nodeStatus {
		case "active":
			direct = direct || userHasMembership(db, user.ID, req.NodeID)
		case "unclaimed":
			direct = direct || user.TrustedContributor
		}

		status := "active"
		if !direct {
			if !cfg.Submissions.Enabled {
				http.Error(w, `{"error":"community submissions are disabled on this instance"}`, http.StatusForbidden)
				return
			}
			if nodeStatus == "active" && !acceptSuggestions {
				http.Error(w, `{"error":"this patch does not accept event suggestions"}`, http.StatusForbidden)
				return
			}
			status = "pending_review"
		}

		id := auth.NewUUIDv7()
		apID := ap.EventAPID(ap.GetDomain(), id)
		_, err = db.Exec(
			`INSERT INTO events (id, node_id, created_by, title, description, location, latitude, longitude, starts_at, ends_at, recurrence, visibility, status, ap_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, req.NodeID, user.ID, req.Title, req.Description, req.Location, req.Latitude, req.Longitude, req.StartsAt, req.EndsAt, req.Recurrence, req.Visibility, status, apID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to create event"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "event.create", "event", id, fmt.Sprintf(`{"status":"%s"}`, status), clientIP(r))

		var e model.Event
		db.QueryRow(
			`SELECT id, node_id, created_by, title, description, location, latitude, longitude, starts_at, ends_at, recurrence, visibility, status, created_at, updated_at
			 FROM events WHERE id = ?`, id,
		).Scan(&e.ID, &e.NodeID, &e.CreatedBy, &e.Title, &e.Description, &e.Location, &e.Latitude, &e.Longitude, &e.StartsAt, &e.EndsAt, &e.Recurrence, &e.Visibility, &e.Status, &e.CreatedAt, &e.UpdatedAt)

		var nodeSlugN, nodeNameN string
		db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", req.NodeID).Scan(&nodeSlugN, &nodeNameN)

		if status == "pending_review" {
			// Route the submission to whoever owns the calendar.
			if nodeStatus == "unclaimed" {
				notify(notifications.Event{
					Type:     notifications.AdminEventSubmission,
					NodeID:   req.NodeID,
					NodeSlug: nodeSlugN,
					NodeName: nodeNameN,
					ActorID:  user.ID,
					EntityID: id,
					Title:    "New event submission: " + req.Title,
					Link:     "/admin/event-submissions",
				})
			} else {
				notify(notifications.Event{
					Type:     notifications.EventSuggested,
					NodeID:   req.NodeID,
					NodeSlug: nodeSlugN,
					NodeName: nodeNameN,
					ActorID:  user.ID,
					EntityID: id,
					Title:    "Event suggested: " + req.Title,
					Link:     "/patches/" + nodeSlugN + "/events",
				})
			}
		} else {
			// Notify members about the new event.
			notify(notifications.Event{
				Type:     notifications.EventCreated,
				NodeID:   req.NodeID,
				NodeSlug: nodeSlugN,
				NodeName: nodeNameN,
				ActorID:  user.ID,
				EntityID: id,
				Title:    "New event: " + req.Title,
				Link:     "/patches/" + nodeSlugN + "/events/" + id,
			})
			broadcastEventCreate(db, e, req.NodeID)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(e)
	}
}

// UpdateEvent handles PATCH /api/v1/events/{id}.
func UpdateEvent(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		eventID := r.PathValue("id")

		// Get event to check permissions.
		var nodeID, createdBy, eventStatus string
		var sourceID *string
		err := db.QueryRow("SELECT node_id, created_by, status, source_id FROM events WHERE id = ?", eventID).Scan(&nodeID, &createdBy, &eventStatus, &sourceID)
		if err != nil {
			http.Error(w, `{"error":"event not found"}`, http.StatusNotFound)
			return
		}

		// The source is authoritative for imported events (docs/adr/031):
		// they are read-only here — change them in the source calendar,
		// or detach this one to make it an ordinary local event.
		if sourceID != nil {
			http.Error(w, `{"error":"this event comes from an event source — edit it in the source calendar, or detach it first"}`, http.StatusForbidden)
			return
		}
		var nodeStatus string
		db.QueryRow("SELECT status FROM nodes WHERE id = ?", nodeID).Scan(&nodeStatus)

		// Changes follow the same door the event came through (docs/adr/026):
		// whoever may post directly edits directly; an ordinary submitter may
		// edit their own event, but the edit re-enters review. On active
		// patches an adopted suggestion belongs to the patch — the suggester
		// keeps no residual rights once it is approved.
		isCreator := user.ID == createdBy
		direct := user.Role == "admin" ||
			(nodeStatus == "active" && userHasNodeRole(db, user.ID, nodeID, "admin")) ||
			(nodeStatus == "unclaimed" && user.TrustedContributor && isCreator)
		reReview := false
		switch {
		case direct:
			// Full edit, status untouched.
		case isCreator && eventStatus == "pending_review":
			// Editing your own submission while it waits is free.
		case isCreator && nodeStatus == "unclaimed":
			reReview = true
		default:
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		allowedFields := map[string]bool{
			"title": true, "description": true, "location": true,
			"latitude": true, "longitude": true, "starts_at": true,
			"ends_at": true, "recurrence": true, "visibility": true,
		}

		var setClauses []string
		var args []interface{}
		for field, val := range req {
			if !allowedFields[field] {
				continue
			}
			setClauses = append(setClauses, field+" = ?")
			args = append(args, val)
		}

		if len(setClauses) == 0 {
			http.Error(w, `{"error":"no valid fields to update"}`, http.StatusBadRequest)
			return
		}

		if reReview {
			setClauses = append(setClauses, "status = 'pending_review'")
		}
		setClauses = append(setClauses, "updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')")
		args = append(args, eventID)

		_, err = db.Exec(
			fmt.Sprintf("UPDATE events SET %s WHERE id = ?", strings.Join(setClauses, ", ")),
			args...,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to update event"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "event.update", "event", eventID, "{}", clientIP(r))

		var e model.Event
		db.QueryRow(
			`SELECT id, node_id, created_by, title, description, location, latitude, longitude, starts_at, ends_at, recurrence, visibility, status, created_at, updated_at
			 FROM events WHERE id = ?`, eventID,
		).Scan(&e.ID, &e.NodeID, &e.CreatedBy, &e.Title, &e.Description, &e.Location, &e.Latitude, &e.Longitude, &e.StartsAt, &e.EndsAt, &e.Recurrence, &e.Visibility, &e.Status, &e.CreatedAt, &e.UpdatedAt)

		var nodeSlugN, nodeNameN string
		db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlugN, &nodeNameN)

		if reReview {
			// The edit pulled the event back into the queue.
			notify(notifications.Event{
				Type:     notifications.AdminEventSubmission,
				NodeID:   nodeID,
				NodeSlug: nodeSlugN,
				NodeName: nodeNameN,
				ActorID:  user.ID,
				EntityID: eventID,
				Title:    "Event edit awaiting review: " + e.Title,
				Link:     "/admin/event-submissions",
			})
		} else if e.Status == "active" {
			// Notify members about the event update.
			notify(notifications.Event{
				Type:     notifications.EventUpdated,
				NodeID:   nodeID,
				NodeSlug: nodeSlugN,
				NodeName: nodeNameN,
				ActorID:  user.ID,
				EntityID: eventID,
				Title:    "Event updated: " + e.Title,
				Link:     "/patches/" + nodeSlugN + "/events/" + eventID,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(e)
	}
}

// DeleteEvent handles DELETE /api/v1/events/{id}.
func DeleteEvent(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		eventID := r.PathValue("id")
		if eventID == "" {
			http.Error(w, `{"error":"event id required"}`, http.StatusBadRequest)
			return
		}

		// Get event to check permissions and capture info for notification.
		var nodeID, eventTitle, createdBy, eventStatus string
		var sourceID, sourceUID *string
		var sourceOccurrence string
		err := db.QueryRow("SELECT node_id, title, created_by, status, source_id, source_uid, source_occurrence FROM events WHERE id = ?", eventID).
			Scan(&nodeID, &eventTitle, &createdBy, &eventStatus, &sourceID, &sourceUID, &sourceOccurrence)
		if err != nil {
			http.Error(w, `{"error":"event not found"}`, http.StatusNotFound)
			return
		}

		// Admin on hosting node, global admin — or the event's own creator:
		// removing your own contribution is always free (docs/adr/026),
		// whether it is still pending or already published.
		if user.Role != "admin" && !userHasNodeRole(db, user.ID, nodeID, "admin") && user.ID != createdBy {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		// Deleting an imported event skip-lists its feed item first, so
		// the next sync doesn't resurrect it (docs/adr/031).
		if sourceID != nil && sourceUID != nil {
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO event_source_skips (source_id, uid, occurrence) VALUES (?, ?, ?)`,
				*sourceID, *sourceUID, sourceOccurrence,
			); err != nil {
				http.Error(w, `{"error":"failed to delete event"}`, http.StatusInternalServerError)
				return
			}
		}

		result, err := db.Exec("DELETE FROM events WHERE id = ?", eventID)
		if err != nil {
			http.Error(w, `{"error":"failed to delete event"}`, http.StatusInternalServerError)
			return
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			http.Error(w, `{"error":"event not found"}`, http.StatusNotFound)
			return
		}

		auth.LogAuditEvent(db, user.ID, "event.delete", "event", eventID, "{}", clientIP(r))

		// Notify members about the cancellation — but a withdrawn pending
		// submission was never announced, so its removal isn't either.
		if eventStatus != "active" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		var nodeSlugN, nodeNameN string
		db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlugN, &nodeNameN)
		notify(notifications.Event{
			Type:     notifications.EventCancelled,
			NodeID:   nodeID,
			NodeSlug: nodeSlugN,
			NodeName: nodeNameN,
			ActorID:  user.ID,
			EntityID: eventID,
			Title:    "Event cancelled: " + eventTitle,
			Link:     "/patches/" + nodeSlugN,
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
