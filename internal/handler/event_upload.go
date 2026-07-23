package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// maxBulkEvents bounds one upload. A season fits; a firehose doesn't.
const maxBulkEvents = 200

type bulkEventRow struct {
	Title       string  `json:"title"`
	StartsAt    string  `json:"starts_at"`
	EndsAt      *string `json:"ends_at"`
	Location    string  `json:"location"`
	Description string  `json:"description"`
	Visibility  string  `json:"visibility"`
}

// BulkCreateEvents handles POST /api/v1/nodes/{slug}/events/bulk — the
// spreadsheet door. Bulk upload is an admin act, deliberately narrower
// than single-event posting: patch admins on active patches, the
// instance admin and trusted contributors on unclaimed ones (their
// docs/adr/026 grant already lets them record events there directly).
// Members still post one event at a time; suggesters go through review.
//
// The batch is all-or-nothing on validation (fix row 7 and retry beats
// half a season imported), rows whose title+start already exist are
// skipped so re-uploading a corrected sheet can't duplicate it, and
// creation is silent — no notification or federation burst for forty
// season entries, mirroring an event source's first sync (docs/adr/031).
func BulkCreateEvents(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var nodeID, nodeStatus string
		err := db.QueryRow(
			`SELECT id, status FROM nodes WHERE slug = ? AND status IN ('active','unclaimed') AND removed_at IS NULL`,
			r.PathValue("slug"),
		).Scan(&nodeID, &nodeStatus)
		if err != nil {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		allowed := user.Role == "admin" ||
			(nodeStatus == "active" && userHasNodeRole(db, user.ID, nodeID, "admin")) ||
			(nodeStatus == "unclaimed" && user.TrustedContributor)
		if !allowed {
			http.Error(w, `{"error":"bulk upload is for patch admins"}`, http.StatusForbidden)
			return
		}

		var req struct {
			Events []bulkEventRow `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if len(req.Events) == 0 {
			http.Error(w, `{"error":"no events in upload"}`, http.StatusBadRequest)
			return
		}
		if len(req.Events) > maxBulkEvents {
			http.Error(w, fmt.Sprintf(`{"error":"an upload is at most %d events"}`, maxBulkEvents), http.StatusBadRequest)
			return
		}

		// Validate everything before creating anything.
		type rowError struct {
			Index int    `json:"index"`
			Error string `json:"error"`
		}
		var rowErrors []rowError
		fail := func(i int, msg string) { rowErrors = append(rowErrors, rowError{Index: i, Error: msg}) }
		for i := range req.Events {
			ev := &req.Events[i]
			if ev.Title == "" {
				fail(i, "title is required")
			}
			start, err := time.Parse(time.RFC3339, ev.StartsAt)
			if err != nil {
				fail(i, "starts_at must be an RFC 3339 timestamp")
			}
			if ev.EndsAt != nil {
				end, err := time.Parse(time.RFC3339, *ev.EndsAt)
				if err != nil {
					fail(i, "ends_at must be an RFC 3339 timestamp")
				} else if !start.IsZero() && !end.After(start) {
					fail(i, "ends_at must be after starts_at")
				}
			}
			switch ev.Visibility {
			case "":
				ev.Visibility = "public"
			case "public", "private", "unlisted":
			default:
				fail(i, "visibility must be public, private, or unlisted")
			}
		}
		if rowErrors != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"errors": rowErrors})
			return
		}

		// Existing title+start pairs make re-uploads idempotent.
		existing := map[string]bool{}
		rows, err := db.Query(
			`SELECT title, starts_at FROM events WHERE node_id = ? AND removed_at IS NULL`, nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to upload events"}`, http.StatusInternalServerError)
			return
		}
		for rows.Next() {
			var title, startsAt string
			if err := rows.Scan(&title, &startsAt); err != nil {
				rows.Close()
				http.Error(w, `{"error":"failed to upload events"}`, http.StatusInternalServerError)
				return
			}
			existing[title+"\x00"+startsAt] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			http.Error(w, `{"error":"failed to upload events"}`, http.StatusInternalServerError)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, `{"error":"failed to upload events"}`, http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		created, skipped := 0, 0
		for _, ev := range req.Events {
			key := ev.Title + "\x00" + ev.StartsAt
			if existing[key] {
				skipped++
				continue
			}
			existing[key] = true // dedupe within the sheet too

			id := auth.NewUUIDv7()
			apID := ap.EventAPID(ap.GetDomain(), id)
			if _, err := tx.Exec(
				`INSERT INTO events (id, node_id, created_by, title, description, location,
				 starts_at, ends_at, recurrence, visibility, status, ap_id)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', ?, 'active', ?)`,
				id, nodeID, user.ID, ev.Title, ev.Description, ev.Location,
				ev.StartsAt, ev.EndsAt, ev.Visibility, apID,
			); err != nil {
				http.Error(w, `{"error":"failed to upload events"}`, http.StatusInternalServerError)
				return
			}
			created++
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, `{"error":"failed to upload events"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "event.bulk_upload", "node", nodeID,
			fmt.Sprintf(`{"created":%d,"skipped":%d}`, created, skipped), clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"created": created, "skipped": skipped})
	}
}
