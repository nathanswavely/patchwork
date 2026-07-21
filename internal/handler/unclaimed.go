package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// SubmitPatch handles POST /api/v1/submissions.
// Community members submit places/orgs to add to the quilt.
func SubmitPatch(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !cfg.Submissions.Enabled {
			http.Error(w, `{"error":"community submissions are disabled on this instance"}`, http.StatusForbidden)
			return
		}
		user := middleware.UserFromContext(r.Context())

		var req struct {
			Name        string           `json:"name"`
			Description string           `json:"description"`
			Website     string           `json:"website"`
			Links       []model.NodeLink `json:"links"`
			Address     string           `json:"address"`
			Latitude    *float64         `json:"latitude"`
			Longitude   *float64         `json:"longitude"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
			return
		}

		// Check for duplicate by slug.
		slug := uniqueSlug(db, generateSlug(req.Name))
		baseSlug := generateSlug(req.Name)
		var existingSlug string
		db.QueryRow("SELECT slug FROM nodes WHERE slug = ? AND status IN ('active','unclaimed','pending_review')", baseSlug).Scan(&existingSlug)
		if existingSlug != "" {
			http.Error(w, fmt.Sprintf(`{"error":"a patch with a similar name already exists","existing_slug":"%s"}`, existingSlug), http.StatusConflict)
			return
		}

		status := "pending_review"
		if cfg.Submissions.AutoApprove {
			status = "unclaimed"
		}

		id := auth.NewUUIDv7()
		linksStr := "[]"
		if len(req.Links) > 0 {
			lb, _ := json.Marshal(req.Links)
			linksStr = string(lb)
		}

		apID := ap.NodeAPID(ap.GetDomain(), id)
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

		// The verification domain is a trust anchor (docs/adr/030): only a
		// trusted contributor's website auto-derives one. Ordinary community
		// submissions get none — the admin sets it at approval time.
		verificationDomain := ""
		if user.TrustedContributor {
			verificationDomain = deriveVerificationDomain(req.Website)
		}

		_, err := db.Exec(
			`INSERT INTO nodes (id, owner_id, name, slug, description, latitude, longitude, address, website, links, visibility, membership_policy, status, submitted_by, submission_source, verification_domain, ap_id, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'public', 'open', ?, ?, 'community', ?, ?, ?, ?)`,
			id, model.SystemUserID, req.Name, slug, req.Description, req.Latitude, req.Longitude, req.Address, req.Website, linksStr, status, user.ID, verificationDomain, apID, now, now,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to create submission"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "node.submit", "node", id, r.RemoteAddr, fmt.Sprintf(`{"status":"%s"}`, status))

		// Notify site admins about the new submission.
		notify(notifications.Event{
			Type:     notifications.AdminSubmission,
			ActorID:  user.ID,
			EntityID: id,
			Title:    "New patch submission: " + req.Name,
			Link:     "/admin/submissions",
		})

		resp := map[string]interface{}{
			"status": status,
		}
		if status == "unclaimed" {
			resp["node"] = map[string]string{"id": id, "slug": slug, "name": req.Name}
		} else {
			resp["message"] = "Submission sent for review"
			resp["id"] = id
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}
}

// CreateUnclaimedPatch handles POST /api/v1/admin/unclaimed.
// Admin directly creates an unclaimed patch.
func CreateUnclaimedPatch(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			Name               string           `json:"name"`
			Description        string           `json:"description"`
			Website            string           `json:"website"`
			VerificationDomain string           `json:"verification_domain"`
			Links              []model.NodeLink `json:"links"`
			Address            string           `json:"address"`
			Latitude           *float64         `json:"latitude"`
			Longitude          *float64         `json:"longitude"`
			Tags               []string         `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
			return
		}

		// Explicit domain wins; otherwise derive from the website the admin
		// supplied (shared platforms yield none — docs/adr/030).
		verificationDomain := deriveVerificationDomain(req.Website)
		if req.VerificationDomain != "" {
			d, err := validateExplicitDomain(req.VerificationDomain)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
				return
			}
			verificationDomain = d
		}

		id := auth.NewUUIDv7()
		slug := uniqueSlug(db, generateSlug(req.Name))

		linksStr := "[]"
		if len(req.Links) > 0 {
			lb, _ := json.Marshal(req.Links)
			linksStr = string(lb)
		}

		apID := ap.NodeAPID(ap.GetDomain(), id)
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

		_, err := db.Exec(
			`INSERT INTO nodes (id, owner_id, name, slug, description, latitude, longitude, address, website, links, visibility, membership_policy, status, submitted_by, submission_source, verification_domain, ap_id, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'public', 'open', 'unclaimed', ?, 'admin', ?, ?, ?, ?)`,
			id, model.SystemUserID, req.Name, slug, req.Description, req.Latitude, req.Longitude, req.Address, req.Website, linksStr, user.ID, verificationDomain, apID, now, now,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to create unclaimed patch"}`, http.StatusInternalServerError)
			return
		}

		// Assign tags if provided.
		for _, tagName := range req.Tags {
			var tagID string
			err := db.QueryRow("SELECT id FROM tags WHERE name = ?", tagName).Scan(&tagID)
			if err != nil {
				continue
			}
			db.Exec("INSERT OR IGNORE INTO node_tags (node_id, tag_id) VALUES (?, ?)", id, tagID)
		}

		auth.LogAuditEvent(db, user.ID, "node.create_unclaimed", "node", id, r.RemoteAddr, "")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": id, "slug": slug, "name": req.Name, "status": "unclaimed"})
	}
}

// BulkCreateUnclaimed handles POST /api/v1/admin/unclaimed/bulk.
func BulkCreateUnclaimed(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			Nodes []struct {
				Name               string           `json:"name"`
				Description        string           `json:"description"`
				Website            string           `json:"website"`
				VerificationDomain string           `json:"verification_domain"`
				Links              []model.NodeLink `json:"links"`
				Address            string           `json:"address"`
				Latitude           *float64         `json:"latitude"`
				Longitude          *float64         `json:"longitude"`
				Tags               []string         `json:"tags"`
			} `json:"nodes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		created := 0
		var errors []string
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

		for i, n := range req.Nodes {
			if n.Name == "" {
				errors = append(errors, fmt.Sprintf("item %d: name is required", i))
				continue
			}

			id := auth.NewUUIDv7()
			slug := uniqueSlug(db, generateSlug(n.Name))

			linksStr := "[]"
			if len(n.Links) > 0 {
				lb, _ := json.Marshal(n.Links)
				linksStr = string(lb)
			}

			verificationDomain := deriveVerificationDomain(n.Website)
			if n.VerificationDomain != "" {
				d, err := validateExplicitDomain(n.VerificationDomain)
				if err != nil {
					errors = append(errors, fmt.Sprintf("item %d (%s): %v", i, n.Name, err))
					continue
				}
				verificationDomain = d
			}

			apID := ap.NodeAPID(ap.GetDomain(), id)
			_, err := db.Exec(
				`INSERT INTO nodes (id, owner_id, name, slug, description, latitude, longitude, address, website, links, visibility, membership_policy, status, submitted_by, submission_source, verification_domain, ap_id, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'public', 'open', 'unclaimed', ?, 'admin', ?, ?, ?, ?)`,
				id, model.SystemUserID, n.Name, slug, n.Description, n.Latitude, n.Longitude, n.Address, n.Website, linksStr, user.ID, verificationDomain, apID, now, now,
			)
			if err != nil {
				errors = append(errors, fmt.Sprintf("item %d (%s): %v", i, n.Name, err))
				continue
			}

			for _, tagName := range n.Tags {
				var tagID string
				if db.QueryRow("SELECT id FROM tags WHERE name = ?", tagName).Scan(&tagID) == nil {
					db.Exec("INSERT OR IGNORE INTO node_tags (node_id, tag_id) VALUES (?, ?)", id, tagID)
				}
			}
			created++
		}

		auth.LogAuditEvent(db, user.ID, "node.bulk_create_unclaimed", "", "", r.RemoteAddr, fmt.Sprintf(`{"created":%d}`, created))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"created": created,
			"errors":  errors,
		})
	}
}

// ListSubmissions handles GET /api/v1/admin/submissions.
// Returns nodes with status='pending_review' for admin moderation.
func ListSubmissions(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		after, limit := parsePaginationParams(r)

		query := `SELECT n.id, n.name, n.slug, n.description, n.website, COALESCE(n.links,'[]'), n.address, n.submitted_by, n.created_at,
			COALESCE(u.username,''), COALESCE(u.display_name,'')
			FROM nodes n LEFT JOIN users u ON n.submitted_by = u.id
			WHERE n.status = 'pending_review' AND n.removed_at IS NULL`
		args := []interface{}{}

		if sortKey, id, ok := decodeCursor(after); after != "" && ok {
			query += " AND " + keysetCondition("n.created_at", "n.id", true)
			args = append(args, sortKey, sortKey, id)
		}
		query += " ORDER BY n.created_at DESC, n.id DESC LIMIT ?"
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to query submissions"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type submission struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			Slug            string `json:"slug"`
			// What the website would derive as a trust anchor — shown to the
			// reviewing admin as a suggestion, never applied automatically.
			SuggestedVerificationDomain string `json:"suggested_verification_domain"`
			Description     string `json:"description"`
			Website         string `json:"website"`
			Links           json.RawMessage `json:"links"`
			Address         string `json:"address"`
			SubmittedBy     string `json:"submitted_by"`
			CreatedAt       string `json:"created_at"`
			SubmitterName   string `json:"submitter_username"`
			SubmitterDisplay string `json:"submitter_display_name"`
		}

		var items []submission
		for rows.Next() {
			var s submission
			var linksStr string
			if err := rows.Scan(&s.ID, &s.Name, &s.Slug, &s.Description, &s.Website, &linksStr, &s.Address, &s.SubmittedBy, &s.CreatedAt, &s.SubmitterName, &s.SubmitterDisplay); err != nil {
				continue
			}
			s.Links = json.RawMessage(linksStr)
			s.SuggestedVerificationDomain = deriveVerificationDomain(s.Website)
			items = append(items, s)
		}

		hasMore := len(items) > limit
		if hasMore {
			items = items[:limit]
		}
		if items == nil {
			items = []submission{}
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

// ReviewSubmission handles PATCH /api/v1/admin/submissions/{id}.
func ReviewSubmission(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID := r.PathValue("id")

		var req struct {
			Action string `json:"action"` // "approve" or "reject"
			Note   string `json:"note"`
			// Trust anchor for self-service claims, vetted here by the
			// reviewing admin (docs/adr/030). The submitter's website never
			// becomes one on its own.
			VerificationDomain string `json:"verification_domain"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Verify node exists and is pending_review.
		var status string
		err := db.QueryRow("SELECT status FROM nodes WHERE id = ?", nodeID).Scan(&status)
		if err != nil || status != "pending_review" {
			http.Error(w, `{"error":"submission not found"}`, http.StatusNotFound)
			return
		}

		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

		switch req.Action {
		case "approve":
			verificationDomain, derr := validateExplicitDomain(req.VerificationDomain)
			if derr != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, derr.Error()), http.StatusBadRequest)
				return
			}
			db.Exec("UPDATE nodes SET status = 'unclaimed', verification_domain = ?, updated_at = ? WHERE id = ?", verificationDomain, now, nodeID)
			auth.LogAuditEvent(db, user.ID, "node.submission_approved", "node", nodeID, r.RemoteAddr, "")
		case "reject":
			db.Exec("UPDATE nodes SET status = 'archived', removed_at = ?, updated_at = ? WHERE id = ?", now, now, nodeID)
			auth.LogAuditEvent(db, user.ID, "node.submission_rejected", "node", nodeID, r.RemoteAddr, fmt.Sprintf(`{"note":"%s"}`, req.Note))
		default:
			http.Error(w, `{"error":"action must be 'approve' or 'reject'"}`, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
