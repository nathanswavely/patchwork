package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// CreateReport handles POST /api/v1/reports.
func CreateReport(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			EntityType string `json:"entity_type"`
			EntityID   string `json:"entity_id"`
			Reason     string `json:"reason"`
			Details    string `json:"details"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.EntityType == "" || req.EntityID == "" || req.Reason == "" {
			http.Error(w, `{"error":"entity_type, entity_id, and reason are required"}`, http.StatusBadRequest)
			return
		}

		// Validate entity_type.
		switch req.EntityType {
		case "node", "event", "user", "notice", "reply":
		default:
			http.Error(w, `{"error":"entity_type must be node, event, user, notice, or reply"}`, http.StatusBadRequest)
			return
		}

		// Validate the target exists. A notice or a reply also resolves to
		// its patch: the report goes to that patch's admins, not the
		// instance's, who cannot read the room (docs/adr/081) — and only
		// someone in the room can have seen the thing they are reporting.
		var exists int
		var roomNodeID string
		switch req.EntityType {
		case "node":
			db.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = ?", req.EntityID).Scan(&exists)
		case "event":
			db.QueryRow("SELECT COUNT(*) FROM events WHERE id = ?", req.EntityID).Scan(&exists)
		case "user":
			db.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", req.EntityID).Scan(&exists)
		case "notice":
			db.QueryRow("SELECT node_id FROM notices WHERE id = ?", req.EntityID).Scan(&roomNodeID)
		case "reply":
			db.QueryRow("SELECT n.node_id FROM notice_replies r JOIN notices n ON n.id = r.notice_id WHERE r.id = ?", req.EntityID).Scan(&roomNodeID)
		}
		if roomNodeID != "" && inRoom(db, user, roomNodeID) {
			exists = 1
		}
		if exists == 0 {
			http.Error(w, `{"error":"target entity not found"}`, http.StatusNotFound)
			return
		}

		id := auth.NewUUIDv7()
		var nodeIDArg interface{}
		if roomNodeID != "" {
			nodeIDArg = roomNodeID
		}
		_, err := db.Exec(
			`INSERT INTO content_reports (id, reporter_id, entity_type, entity_id, reason, details, node_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, user.ID, req.EntityType, req.EntityID, req.Reason, req.Details, nodeIDArg,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to create report"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "report.create", "report", id, fmt.Sprintf(`{"entity_type":"%s","entity_id":"%s"}`, req.EntityType, req.EntityID), clientIP(r))

		if roomNodeID != "" {
			var slug, name string
			db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", roomNodeID).Scan(&slug, &name)
			notify(notifications.Event{
				Type:     notifications.NoticeReported,
				NodeID:   roomNodeID,
				NodeSlug: slug,
				NodeName: name,
				ActorID:  user.ID,
				EntityID: req.EntityID,
				Title:    "A " + req.EntityType + " on the noticeboard was reported",
				Body:     req.Reason,
				Link:     weblink.PatchNoticeboardReports(slug),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "ok"})
	}
}

// reportWithPreview is a report enriched with reporter and target preview info.
type reportWithPreview struct {
	model.ContentReport
	ReporterName string `json:"reporter_name"`
	TargetName   string `json:"target_name"`
}

// ListReports handles GET /api/v1/admin/reports.
func ListReports(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		after, limit := parsePaginationParams(r)
		status := r.URL.Query().Get("status")

		query := `SELECT id, reporter_id, entity_type, entity_id, reason, details, status, reviewed_by, resolution_note, created_at, updated_at FROM content_reports`
		// A report routed to a patch's admins is theirs, not the instance's
		// (docs/adr/081): the instance panel cannot read the room it is about.
		conditions := []string{"node_id IS NULL"}
		var args []interface{}

		if status != "" {
			conditions = append(conditions, "status = ?")
			args = append(args, status)
		}
		if after != "" {
			conditions = append(conditions, "id > ?")
			args = append(args, after)
		}

		if len(conditions) > 0 {
			query += " WHERE " + strings.Join(conditions, " AND ")
		}
		query += " ORDER BY id ASC LIMIT ?"
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list reports"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var reports []reportWithPreview
		for rows.Next() {
			var rpt reportWithPreview
			if err := rows.Scan(&rpt.ID, &rpt.ReporterID, &rpt.EntityType, &rpt.EntityID, &rpt.Reason, &rpt.Details, &rpt.Status, &rpt.ReviewedBy, &rpt.ResolutionNote, &rpt.CreatedAt, &rpt.UpdatedAt); err != nil {
				continue
			}

			// Look up reporter name.
			var reporterName string
			db.QueryRow("SELECT COALESCE(display_name, username) FROM users WHERE id = ?", rpt.ReporterID).Scan(&reporterName)
			rpt.ReporterName = reporterName

			// Look up target preview name.
			switch rpt.EntityType {
			case "node":
				db.QueryRow("SELECT name FROM nodes WHERE id = ?", rpt.EntityID).Scan(&rpt.TargetName)
			case "event":
				db.QueryRow("SELECT title FROM events WHERE id = ?", rpt.EntityID).Scan(&rpt.TargetName)
			case "user":
				db.QueryRow("SELECT COALESCE(display_name, username) FROM users WHERE id = ?", rpt.EntityID).Scan(&rpt.TargetName)
			case "notice":
				db.QueryRow("SELECT title FROM notices WHERE id = ?", rpt.EntityID).Scan(&rpt.TargetName)
			case "reply":
				db.QueryRow("SELECT body FROM notice_replies WHERE id = ?", rpt.EntityID).Scan(&rpt.TargetName)
				rpt.TargetName = excerpt(rpt.TargetName, 120)
			}

			reports = append(reports, rpt)
		}

		var nextCursor string
		if len(reports) > limit {
			nextCursor = reports[limit-1].ID
			reports = reports[:limit]
		}
		if reports == nil {
			reports = []reportWithPreview{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":       reports,
			"next_cursor": nextCursor,
		})
	}
}

// UpdateReport handles PATCH /api/v1/admin/reports/{id}.
func UpdateReport(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		reportID := r.PathValue("id")

		var req struct {
			Status         *string `json:"status"`
			ResolutionNote *string `json:"resolution_note"`
			Action         *string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Load the report to get entity info for action execution.
		var rpt model.ContentReport
		err := db.QueryRow(
			`SELECT id, reporter_id, entity_type, entity_id, status FROM content_reports WHERE id = ?`, reportID,
		).Scan(&rpt.ID, &rpt.ReporterID, &rpt.EntityType, &rpt.EntityID, &rpt.Status)
		if err != nil {
			http.Error(w, `{"error":"report not found"}`, http.StatusNotFound)
			return
		}

		var setClauses []string
		var args []interface{}

		if req.Status != nil {
			setClauses = append(setClauses, "status = ?")
			args = append(args, *req.Status)
		}
		if req.ResolutionNote != nil {
			setClauses = append(setClauses, "resolution_note = ?")
			args = append(args, *req.ResolutionNote)
		}
		setClauses = append(setClauses, "reviewed_by = ?")
		args = append(args, user.ID)
		setClauses = append(setClauses, "updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')")

		args = append(args, reportID)
		_, err = db.Exec(
			fmt.Sprintf("UPDATE content_reports SET %s WHERE id = ?", strings.Join(setClauses, ", ")),
			args...,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to update report"}`, http.StatusInternalServerError)
			return
		}

		// Execute action if provided.
		if req.Action != nil {
			switch *req.Action {
			case "suspend_user":
				// For user reports, suspend the target user.
				// For node/event reports, find the owner/creator and suspend them.
				var targetUserID string
				switch rpt.EntityType {
				case "user":
					targetUserID = rpt.EntityID
				case "node":
					db.QueryRow("SELECT owner_id FROM nodes WHERE id = ?", rpt.EntityID).Scan(&targetUserID)
				case "event":
					db.QueryRow("SELECT created_by FROM events WHERE id = ?", rpt.EntityID).Scan(&targetUserID)
				}
				if targetUserID != "" {
					db.Exec(
						`UPDATE users SET suspended_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
						targetUserID,
					)
					// Revoke live sessions so the suspension takes effect now.
					if err := auth.DestroyUserSessions(db, targetUserID); err != nil {
						log.Printf("reports: revoke sessions for suspended user %s: %v", targetUserID, err)
					}
					CreateNotification(db, targetUserID, "account.suspended", "Account Suspended",
						"Your account has been suspended due to a policy violation.", "/settings")
					auth.LogAuditEvent(db, user.ID, "admin.user_update", "user", targetUserID, `{"action":"suspend"}`, clientIP(r))
				}

			case "remove_content":
				switch rpt.EntityType {
				case "node":
					db.Exec(
						`UPDATE nodes SET removed_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
						rpt.EntityID,
					)
				case "event":
					db.Exec(
						`UPDATE events SET removed_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
						rpt.EntityID,
					)
				}

			case "reset_appearance":
				// The proportionate response to an offensive tile (docs/adr/029):
				// null the appearance so the quilt decides again. Touches only the
				// patch's face on the shared quilt — never its content.
				if rpt.EntityType == "node" {
					db.Exec(
						`UPDATE nodes SET appearance = NULL, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
						rpt.EntityID,
					)
					auth.LogAuditEvent(db, user.ID, "admin.node_update", "node", rpt.EntityID, `{"action":"reset_appearance"}`, clientIP(r))
				}

			case "remove_image":
				// The instance embeds an image it does not host (docs/adr/007),
				// so removing the reference is the whole of the remedy
				// available here — the bytes stay wherever the patch put them.
				// Proportionate in the same way reset_appearance is: it takes
				// down one picture rather than the patch or the event behind
				// it, and the antifascist baseline is unenforceable against
				// media without it.
				switch rpt.EntityType {
				case "node":
					db.Exec(`UPDATE nodes SET image_url = '', image_alt = '', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, rpt.EntityID)
					auth.LogAuditEvent(db, user.ID, "admin.node_update", "node", rpt.EntityID, `{"action":"remove_image"}`, clientIP(r))
				case "event":
					db.Exec(`UPDATE events SET image_url = '', image_alt = '', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, rpt.EntityID)
					auth.LogAuditEvent(db, user.ID, "admin.event_update", "event", rpt.EntityID, `{"action":"remove_image"}`, clientIP(r))
				}
			}
		}

		// Notify the reporter that their report was reviewed.
		CreateNotification(db, rpt.ReporterID, "report.resolved", "Report Reviewed",
			"Your report has been reviewed by an admin.", "")

		auth.LogAuditEvent(db, user.ID, "report.resolve", "report", reportID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
