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

// reportedParty resolves the person a report is about, and names the thing of
// theirs that was reported. A report names content; moderation lands on the
// account behind it — the patch's owner, the event's creator, or, for a report
// about an account, that account.
//
// suspend_user resolved this inline first. The warning uses it too, and the
// two must agree: a warning that reaches somebody a suspension would not is a
// warning delivered to the wrong person.
//
// `what` and `link` describe only the reported party's own content, so they
// are safe to put in front of them. Nothing the reporter wrote — the reason,
// the details, the admin's resolution note — belongs in a message to the
// person reported: those are written for moderators, and can name the
// reporter.
func reportedParty(db *database.DB, rpt model.ContentReport) (userID, what, link string) {
	switch rpt.EntityType {
	case "user":
		return rpt.EntityID, "your account", "/settings"
	case "node":
		var owner, name, slug string
		db.QueryRow("SELECT owner_id, name, slug FROM nodes WHERE id = ?", rpt.EntityID).
			Scan(&owner, &name, &slug)
		if owner == "" {
			return "", "", ""
		}
		return owner, fmt.Sprintf("your patch %q", name), weblink.Patch(slug)
	case "event":
		var creator, title string
		db.QueryRow("SELECT created_by, title FROM events WHERE id = ?", rpt.EntityID).
			Scan(&creator, &title)
		if creator == "" {
			return "", "", ""
		}
		return creator, fmt.Sprintf("your event %q", title), weblink.Event(rpt.EntityID)
	}
	return "", "", ""
}

// The content_reports.status CHECK from migrations/001. A value added there
// must be added here.
var reportStatuses = []string{"pending", "reviewed", "resolved", "dismissed"}

// Every action this queue accepts. Two of them — dismiss and warn — carry no
// side effect beyond the status the caller sends with them, so they have no
// case in the switch below and are easy to mistake for typos. They are not:
// the admin panel's action menu offers both, and dismiss is its default. A
// value dropped from this list is an admin button that stops working.
var reportActions = []string{
	"dismiss", "warn", "remove_content", "reset_appearance", "suspend_user", "remove_image",
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

		// status is CHECK-constrained, so an unrecognized value is refused
		// here rather than by the database — the constraint would reject it
		// too, but as a 500 the caller cannot act on. The patch-side queue
		// (notice_reports.go) never had this hole: it maps an action to a
		// status itself and never takes one from the request.
		if req.Status != nil && !oneOf(*req.Status, reportStatuses) {
			http.Error(w, fmt.Sprintf(`{"error":"status must be one of %s"}`,
				strings.Join(reportStatuses, ", ")), http.StatusBadRequest)
			return
		}

		// An unrecognized action used to fall through the switch below in
		// silence: the report was marked resolved, the moderation the admin
		// asked for never happened, and the response said it had. Refused
		// here rather than in the switch because the status is written first,
		// and a refusal after that write would leave the half of the request
		// that did land in place.
		if req.Action != nil && !oneOf(*req.Action, reportActions) {
			http.Error(w, fmt.Sprintf(`{"error":"action must be one of %s"}`,
				strings.Join(reportActions, ", ")), http.StatusBadRequest)
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
			case "dismiss":
				// Nothing to carry out: dismissing is a decision about the
				// report rather than an action on what was reported, and the
				// status the caller sent alongside records it.

			case "warn":
				// A warning is the one moderation outcome the reported person
				// is supposed to hear about — it is the whole of the remedy.
				// Until now it did nothing at all: it set the status to
				// resolved, and the only notification went to the reporter, so
				// "Warn" warned nobody.
				//
				// It says what was reported and leaves the content alone. It
				// does not say who reported it, or repeat anything they or the
				// reviewing admin wrote.
				warnedID, what, link := reportedParty(db, rpt)
				if warnedID != "" {
					CreateNotification(db, warnedID, "account.warned",
						"A moderator reviewed a report about "+what,
						"An instance admin reviewed a report about "+what+" and issued a warning. "+
							"Nothing was changed or removed.", link)
					auth.LogAuditEvent(db, user.ID, "admin.user_warn", "user", warnedID,
						fmt.Sprintf(`{"report_id":%q,"entity_type":%q}`, reportID, rpt.EntityType), clientIP(r))
				}

			case "suspend_user":
				// The account behind the reported content: for a user report
				// the user, otherwise the patch's owner or the event's creator.
				targetUserID, _, _ := reportedParty(db, rpt)
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
