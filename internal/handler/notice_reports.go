package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// The patch's own report queue (docs/adr/081, tool 3): reports about
// notices and replies go to the patch's admins, who are the only people
// besides its members who can read the room. This is the first patch-level
// report queue in the codebase; the instance panel never lists these rows
// (ListReports filters node_id IS NULL), and these handlers never list
// anything else.
//
// Both routes are mounted behind RequireNodeRole(admin), which lets an
// instance admin through; patchAdminOnly below does not. An instance admin
// with no role in the patch cannot read the room (docs/adr/080 drew the
// same line for the contact card), and a queue that quotes the room is the
// room. The queries also scope by node_id so a report id from another
// patch is not found rather than acted on.

// patchAdminOnly resolves the slug and insists the caller holds an active
// admin role in that patch. Writes 404 and returns "" otherwise.
func patchAdminOnly(db *database.DB, w http.ResponseWriter, r *http.Request) string {
	user := middleware.UserFromContext(r.Context())
	nodeID := NodeIDFromSlug(db, r.PathValue("slug"))
	if nodeID == "" || user == nil || !userHasNodeRole(db, user.ID, nodeID, "admin") {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return ""
	}
	return nodeID
}

// patchReport is a queue row, with the reported thing quoted so the admin
// can judge it without leaving the queue.
type patchReport struct {
	model.ContentReport
	ReporterName string `json:"reporter_name"`
	// Target is what was reported: a notice's title or a reply's body.
	Target string `json:"target"`
	// NoticeID is the notice the report is about, or the notice the
	// reported reply sits under — the page the admin opens to see it.
	NoticeID string `json:"notice_id"`
	// Gone is set when the reported thing no longer exists: its author took
	// it down, or another admin did, before this report was reviewed.
	Gone bool `json:"gone"`
}

// ListPatchReports handles GET /api/v1/nodes/{slug}/reports.
func ListPatchReports(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := patchAdminOnly(db, w, r)
		if nodeID == "" {
			return
		}
		after, limit := parsePaginationParams(r)
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "pending"
		}

		query := `SELECT id, reporter_id, entity_type, entity_id, reason, details, status, reviewed_by, resolution_note, created_at, updated_at
			FROM content_reports WHERE node_id = ? AND status = ?`
		args := []interface{}{nodeID, status}
		if after != "" {
			query += " AND id > ?"
			args = append(args, after)
		}
		query += " ORDER BY id ASC LIMIT ?"
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list reports"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		items := []patchReport{}
		for rows.Next() {
			var rpt patchReport
			if err := rows.Scan(&rpt.ID, &rpt.ReporterID, &rpt.EntityType, &rpt.EntityID, &rpt.Reason, &rpt.Details,
				&rpt.Status, &rpt.ReviewedBy, &rpt.ResolutionNote, &rpt.CreatedAt, &rpt.UpdatedAt); err != nil {
				continue
			}
			db.QueryRow("SELECT COALESCE(display_name, username) FROM users WHERE id = ?", rpt.ReporterID).Scan(&rpt.ReporterName)
			switch rpt.EntityType {
			case "notice":
				rpt.NoticeID = rpt.EntityID
				if db.QueryRow("SELECT title FROM notices WHERE id = ?", rpt.EntityID).Scan(&rpt.Target) != nil {
					rpt.Gone = true
				}
			case "reply":
				if db.QueryRow("SELECT notice_id, body FROM notice_replies WHERE id = ?", rpt.EntityID).Scan(&rpt.NoticeID, &rpt.Target) != nil {
					rpt.Gone = true
				}
			}
			items = append(items, rpt)
		}

		var next string
		if len(items) > limit {
			next = items[limit-1].ID
			items = items[:limit]
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"items": items, "next_cursor": next})
	}
}

// UpdatePatchReport handles PATCH /api/v1/nodes/{slug}/reports/{id}. Three
// actions, and they are the moderation kit (docs/adr/081, decision 2):
// `dismiss` leaves the thing up; `remove` takes it down (hard delete,
// audited); `close_replies` switches replies off on the notice — the
// report's own, or the one the reported reply sits under. Anything else is
// a forum tool and is refused.
func UpdatePatchReport(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID := patchAdminOnly(db, w, r)
		reportID := r.PathValue("id")
		if nodeID == "" {
			return
		}

		var req struct {
			Action         string `json:"action"`
			ResolutionNote string `json:"resolution_note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		var rpt model.ContentReport
		err := db.QueryRow(
			"SELECT id, reporter_id, entity_type, entity_id, status FROM content_reports WHERE id = ? AND node_id = ?",
			reportID, nodeID,
		).Scan(&rpt.ID, &rpt.ReporterID, &rpt.EntityType, &rpt.EntityID, &rpt.Status)
		if err != nil {
			http.Error(w, `{"error":"report not found"}`, http.StatusNotFound)
			return
		}
		if rpt.Status != "pending" {
			http.Error(w, `{"error":"this report has already been reviewed"}`, http.StatusBadRequest)
			return
		}

		newStatus := "resolved"
		switch req.Action {
		case "dismiss":
			newStatus = "dismissed"
		case "remove":
			switch rpt.EntityType {
			case "notice":
				db.Exec("DELETE FROM notices WHERE id = ? AND node_id = ?", rpt.EntityID, nodeID)
				auth.LogAuditEvent(db, user.ID, "notice.delete", "notice", rpt.EntityID,
					fmt.Sprintf(`{"node_id":"%s","report_id":"%s"}`, nodeID, reportID), clientIP(r))
			case "reply":
				db.Exec("DELETE FROM notice_replies WHERE id = ? AND notice_id IN (SELECT id FROM notices WHERE node_id = ?)", rpt.EntityID, nodeID)
				auth.LogAuditEvent(db, user.ID, "notice.reply_delete", "notice_reply", rpt.EntityID,
					fmt.Sprintf(`{"node_id":"%s","report_id":"%s"}`, nodeID, reportID), clientIP(r))
			}
		case "close_replies":
			noticeID := rpt.EntityID
			if rpt.EntityType == "reply" {
				db.QueryRow("SELECT notice_id FROM notice_replies WHERE id = ?", rpt.EntityID).Scan(&noticeID)
			}
			db.Exec("UPDATE notices SET replies_open = 0 WHERE id = ? AND node_id = ?", noticeID, nodeID)
			auth.LogAuditEvent(db, user.ID, "notice.replies", "notice", noticeID,
				fmt.Sprintf(`{"replies_open":false,"report_id":"%s"}`, reportID), clientIP(r))
		default:
			http.Error(w, `{"error":"action must be dismiss, remove, or close_replies"}`, http.StatusBadRequest)
			return
		}

		if _, err := db.Exec(
			`UPDATE content_reports SET status = ?, resolution_note = ?, reviewed_by = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
			newStatus, req.ResolutionNote, user.ID, reportID,
		); err != nil {
			http.Error(w, `{"error":"failed to update report"}`, http.StatusInternalServerError)
			return
		}

		// The reporter hears the same thing they would from the instance
		// panel: reviewed, and no more. What was decided is the room's.
		CreateNotification(db, rpt.ReporterID, "report.resolved", "Report reviewed",
			"Your report was reviewed by the patch's admins.", "")
		auth.LogAuditEvent(db, user.ID, "report.resolve", "report", reportID,
			fmt.Sprintf(`{"action":"%s","node_id":"%s"}`, req.Action, nodeID), clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": newStatus})
	}
}
