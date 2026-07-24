package handler

import (
	"encoding/json"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// ListRevisions handles GET /api/v1/proposals/{id}/revisions.
func ListRevisions(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proposalID := r.PathValue("id")

		// Revisions snapshot the proposed charter text, so they inherit the
		// target charter's visibility (docs/adr/036).
		var revNodeID, revTargetDoc string
		db.QueryRow("SELECT node_id, COALESCE(target_doc,'') FROM proposals WHERE id = ?", proposalID).Scan(&revNodeID, &revTargetDoc)
		docTextHidden := revNodeID != "" && hiddenDocRedactor(db, r, revNodeID)(revTargetDoc)

		rows, err := db.Query(
			`SELECT r.id, r.proposal_id, r.title, r.body, COALESCE(r.proposed_body,''), r.revision_number, r.author_id, r.change_note, r.created_at,
			 COALESCE(u.display_name, u.username) as author_name
			 FROM proposal_revisions r
			 LEFT JOIN users u ON u.id = r.author_id
			 WHERE r.proposal_id = ?
			 ORDER BY r.revision_number ASC`, proposalID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to list revisions"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type revisionItem struct {
			ID             string `json:"id"`
			ProposalID     string `json:"proposal_id"`
			Title          string `json:"title"`
			Body           string `json:"body"`
			ProposedBody   string `json:"proposed_body,omitempty"`
			RevisionNumber int    `json:"revision_number"`
			AuthorID       string `json:"author_id"`
			AuthorName     string `json:"author_name"`
			ChangeNote     string `json:"change_note"`
			CreatedAt      string `json:"created_at"`
		}

		var revisions []revisionItem
		for rows.Next() {
			var rev revisionItem
			if err := rows.Scan(&rev.ID, &rev.ProposalID, &rev.Title, &rev.Body, &rev.ProposedBody, &rev.RevisionNumber, &rev.AuthorID, &rev.ChangeNote, &rev.CreatedAt, &rev.AuthorName); err != nil {
				continue
			}
			if docTextHidden {
				rev.ProposedBody = ""
			}
			revisions = append(revisions, rev)
		}
		if revisions == nil {
			revisions = []revisionItem{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": revisions,
		})
	}
}

// CreateRevision handles POST /api/v1/proposals/{id}/revisions.
func CreateRevision(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		proposalID := r.PathValue("id")

		// Get proposal details.
		var authorID, nodeID, status, currentTitle, currentBody string
		var currentProposedBody, targetDoc, proposedBranch string
		err := db.QueryRow(
			`SELECT author_id, node_id, status, title, body, COALESCE(proposed_body,''), COALESCE(target_doc,''), COALESCE(proposed_branch,'')
			 FROM proposals WHERE id = ?`, proposalID,
		).Scan(&authorID, &nodeID, &status, &currentTitle, &currentBody, &currentProposedBody, &targetDoc, &proposedBranch)
		if err != nil {
			http.Error(w, `{"error":"proposal not found"}`, http.StatusNotFound)
			return
		}

		if user.ID != authorID {
			http.Error(w, `{"error":"only the author can create revisions"}`, http.StatusForbidden)
			return
		}

		if status != "open" {
			http.Error(w, `{"error":"can only revise open proposals"}`, http.StatusBadRequest)
			return
		}

		var req struct {
			Title        *string `json:"title"`
			Body         *string `json:"body"`
			ProposedBody *string `json:"proposed_body"`
			ChangeNote   string  `json:"change_note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.ChangeNote == "" {
			http.Error(w, `{"error":"change_note is required"}`, http.StatusBadRequest)
			return
		}

		// Get current max revision number.
		var maxRevision int
		db.QueryRow("SELECT COALESCE(MAX(revision_number), 0) FROM proposal_revisions WHERE proposal_id = ?", proposalID).Scan(&maxRevision)
		nextRevision := maxRevision + 1

		// Snapshot the current proposal state as a revision.
		revID := auth.NewUUIDv7()
		_, err = db.Exec(
			`INSERT INTO proposal_revisions (id, proposal_id, title, body, proposed_body, revision_number, author_id, change_note)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			revID, proposalID, currentTitle, currentBody, currentProposedBody, nextRevision, user.ID, req.ChangeNote,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to create revision"}`, http.StatusInternalServerError)
			return
		}

		// Update the proposal with new values.
		var setClauses []string
		var args []interface{}
		if req.Title != nil {
			setClauses = append(setClauses, "title = ?")
			args = append(args, *req.Title)
		}
		if req.Body != nil {
			setClauses = append(setClauses, "body = ?")
			args = append(args, *req.Body)
		}
		if req.ProposedBody != nil {
			setClauses = append(setClauses, "proposed_body = ?")
			args = append(args, *req.ProposedBody)
		}

		if len(setClauses) > 0 {
			setClauses = append(setClauses, "updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')")
			args = append(args, proposalID)
			_, err = db.Exec(
				"UPDATE proposals SET "+join(setClauses, ", ")+" WHERE id = ?",
				args...,
			)
			if err != nil {
				http.Error(w, `{"error":"failed to update proposal"}`, http.StatusInternalServerError)
				return
			}
		}

		// If this is an amendment with a git branch, update the branch.
		if req.ProposedBody != nil && targetDoc != "" && proposedBranch != "" {
			governance.DeleteBranch(governance.GetDataDir(), nodeID, proposedBranch)
			governance.CreateBranch(governance.GetDataDir(), nodeID, proposedBranch, targetDoc, *req.ProposedBody, user.DisplayName, user.Email, req.ChangeNote)
		}

		auth.LogAuditEvent(db, user.ID, "proposal.revise", "proposal", proposalID, `{"revision_number":`+json.Number(itoa(nextRevision)).String()+`}`, clientIP(r))

		// Return the updated proposal.
		var p map[string]interface{}
		var title, body, proposedBody, updatedAt string
		db.QueryRow(
			`SELECT title, body, COALESCE(proposed_body,''), updated_at FROM proposals WHERE id = ?`, proposalID,
		).Scan(&title, &body, &proposedBody, &updatedAt)

		p = map[string]interface{}{
			"id":              proposalID,
			"title":           title,
			"body":            body,
			"proposed_body":   proposedBody,
			"updated_at":      updatedAt,
			"revision_number": nextRevision,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	}
}

// itoa converts an int to a string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}
