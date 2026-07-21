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
		if req.EntityType != "node" && req.EntityType != "event" && req.EntityType != "user" {
			http.Error(w, `{"error":"entity_type must be node, event, or user"}`, http.StatusBadRequest)
			return
		}

		// Validate the target exists.
		var exists int
		switch req.EntityType {
		case "node":
			db.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = ?", req.EntityID).Scan(&exists)
		case "event":
			db.QueryRow("SELECT COUNT(*) FROM events WHERE id = ?", req.EntityID).Scan(&exists)
		case "user":
			db.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", req.EntityID).Scan(&exists)
		}
		if exists == 0 {
			http.Error(w, `{"error":"target entity not found"}`, http.StatusNotFound)
			return
		}

		id := auth.NewUUIDv7()
		_, err := db.Exec(
			`INSERT INTO content_reports (id, reporter_id, entity_type, entity_id, reason, details) VALUES (?, ?, ?, ?, ?, ?)`,
			id, user.ID, req.EntityType, req.EntityID, req.Reason, req.Details,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to create report"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "report.create", "report", id, fmt.Sprintf(`{"entity_type":"%s","entity_id":"%s"}`, req.EntityType, req.EntityID), clientIP(r))

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
		var conditions []string
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
