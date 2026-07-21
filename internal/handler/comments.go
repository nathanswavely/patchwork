package handler

import (
	"encoding/json"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// allowedEmoji is the set of valid reaction emoji.
var allowedEmoji = map[string]bool{
	"👍": true,
	"👎": true,
	"❤️": true,
	"🤔": true,
	"🎉": true,
	"👀": true,
}

// commentItem is the JSON shape returned by ListComments.
type commentItem struct {
	ID         string         `json:"id"`
	Body       string         `json:"body"`
	AuthorName string         `json:"author_name"`
	AuthorID   string         `json:"author_id"`
	CreatedAt  string         `json:"created_at"`
	UpdatedAt  string         `json:"updated_at"`
	ParentID   *string        `json:"parent_id"`
	Replies    []commentItem  `json:"replies"`
	Reactions  []reactionItem `json:"reactions"`
}

type reactionItem struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	Me    bool   `json:"me"`
}

// ListComments handles GET /api/v1/proposals/{id}/comments.
// Returns threaded comments with reactions.
func ListComments(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proposalID := r.PathValue("id")

		// Determine current user (optional, for reaction "me" flag).
		var currentUserID string
		cookie, _ := r.Cookie(auth.CookieName)
		if cookie != nil {
			if u, _ := auth.ValidateSession(db, cookie.Value); u != nil {
				currentUserID = u.ID
			}
		}

		// Fetch all comments for this proposal.
		rows, err := db.Query(
			`SELECT c.id, c.body, COALESCE(u.display_name, u.username) as author_name, c.author_id, c.created_at, c.updated_at, c.parent_id
			 FROM proposal_comments c
			 LEFT JOIN users u ON u.id = c.author_id
			 WHERE c.proposal_id = ?
			 ORDER BY c.created_at ASC`, proposalID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to list comments"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		// Build a flat list and index by ID.
		type flatComment struct {
			commentItem
			parentID *string
		}
		var all []flatComment
		byID := make(map[string]int) // id -> index in all

		for rows.Next() {
			var c flatComment
			if err := rows.Scan(&c.ID, &c.Body, &c.AuthorName, &c.AuthorID, &c.CreatedAt, &c.UpdatedAt, &c.parentID); err != nil {
				continue
			}
			c.ParentID = c.parentID
			c.Replies = []commentItem{}
			c.Reactions = []reactionItem{}
			byID[c.ID] = len(all)
			all = append(all, c)
		}

		// Fetch all reactions grouped by comment.
		reactionRows, err := db.Query(
			`SELECT cr.comment_id, cr.emoji, COUNT(*) as cnt,
			 MAX(CASE WHEN cr.user_id = ? THEN 1 ELSE 0 END) as me
			 FROM comment_reactions cr
			 JOIN proposal_comments pc ON pc.id = cr.comment_id
			 WHERE pc.proposal_id = ?
			 GROUP BY cr.comment_id, cr.emoji`,
			currentUserID, proposalID,
		)
		if err == nil {
			defer reactionRows.Close()
			for reactionRows.Next() {
				var commentID, emoji string
				var count, me int
				if err := reactionRows.Scan(&commentID, &emoji, &count, &me); err != nil {
					continue
				}
				if idx, ok := byID[commentID]; ok {
					all[idx].Reactions = append(all[idx].Reactions, reactionItem{
						Emoji: emoji,
						Count: count,
						Me:    me == 1,
					})
				}
			}
		}

		// Build threaded structure: top-level comments with nested replies.
		var topLevel []commentItem
		for i := range all {
			if all[i].parentID == nil {
				topLevel = append(topLevel, all[i].commentItem)
			} else {
				parentIdx, ok := byID[*all[i].parentID]
				if ok {
					all[parentIdx].Replies = append(all[parentIdx].Replies, all[i].commentItem)
				}
			}
		}

		// Copy updated replies back into topLevel items.
		// Since we modified all[parentIdx].Replies, rebuild topLevel from the flat list.
		topLevel = nil
		for i := range all {
			if all[i].parentID == nil {
				item := all[i].commentItem
				item.Replies = all[i].Replies
				topLevel = append(topLevel, item)
			}
		}

		if topLevel == nil {
			topLevel = []commentItem{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": topLevel,
		})
	}
}

// CreateComment handles POST /api/v1/proposals/{id}/comments.
func CreateComment(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		proposalID := r.PathValue("id")

		// Get proposal's node.
		var nodeID string
		err := db.QueryRow("SELECT node_id FROM proposals WHERE id = ?", proposalID).Scan(&nodeID)
		if err != nil {
			http.Error(w, `{"error":"proposal not found"}`, http.StatusNotFound)
			return
		}

		// Require membership in the proposal's node.
		if user.Role != "admin" && !userHasMembership(db, user.ID, nodeID) {
			http.Error(w, `{"error":"must be member of node"}`, http.StatusForbidden)
			return
		}

		var req struct {
			Body     string  `json:"body"`
			ParentID *string `json:"parent_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Body == "" {
			http.Error(w, `{"error":"body is required"}`, http.StatusBadRequest)
			return
		}

		// Validate parent_id if provided.
		if req.ParentID != nil && *req.ParentID != "" {
			var parentProposalID string
			err := db.QueryRow("SELECT proposal_id FROM proposal_comments WHERE id = ?", *req.ParentID).Scan(&parentProposalID)
			if err != nil || parentProposalID != proposalID {
				http.Error(w, `{"error":"parent comment not found or belongs to different proposal"}`, http.StatusBadRequest)
				return
			}
		}

		id := auth.NewUUIDv7()
		_, err = db.Exec(
			`INSERT INTO proposal_comments (id, proposal_id, parent_id, author_id, body) VALUES (?, ?, ?, ?, ?)`,
			id, proposalID, req.ParentID, user.ID, req.Body,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to create comment"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "comment.create", "comment", id, `{"proposal_id":"`+proposalID+`"}`, clientIP(r))

		// Notify proposal participants about the new comment.
		var nodeSlugN, nodeNameN, proposalTitle string
		db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlugN, &nodeNameN)
		db.QueryRow("SELECT title FROM proposals WHERE id = ?", proposalID).Scan(&proposalTitle)
		notify(notifications.Event{
			Type:     notifications.ProposalComment,
			NodeID:   nodeID,
			NodeSlug: nodeSlugN,
			NodeName: nodeNameN,
			ActorID:  user.ID,
			EntityID: proposalID,
			Title:    "New comment on: " + proposalTitle,
			Body:     req.Body,
			Link:     "/patches/" + nodeSlugN + "/governance/" + proposalID,
		})

		// Return the created comment.
		var c commentItem
		db.QueryRow(
			`SELECT c.id, c.body, COALESCE(u.display_name, u.username), c.author_id, c.created_at, c.updated_at, c.parent_id
			 FROM proposal_comments c LEFT JOIN users u ON u.id = c.author_id
			 WHERE c.id = ?`, id,
		).Scan(&c.ID, &c.Body, &c.AuthorName, &c.AuthorID, &c.CreatedAt, &c.UpdatedAt, &c.ParentID)
		c.Replies = []commentItem{}
		c.Reactions = []reactionItem{}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(c)
	}
}

// UpdateComment handles PATCH /api/v1/comments/{id}.
func UpdateComment(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		commentID := r.PathValue("id")

		var authorID string
		err := db.QueryRow("SELECT author_id FROM proposal_comments WHERE id = ?", commentID).Scan(&authorID)
		if err != nil {
			http.Error(w, `{"error":"comment not found"}`, http.StatusNotFound)
			return
		}

		if user.ID != authorID {
			http.Error(w, `{"error":"only the author can edit this comment"}`, http.StatusForbidden)
			return
		}

		var req struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Body == "" {
			http.Error(w, `{"error":"body is required"}`, http.StatusBadRequest)
			return
		}

		_, err = db.Exec(
			`UPDATE proposal_comments SET body = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
			req.Body, commentID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to update comment"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "comment.update", "comment", commentID, "{}", clientIP(r))

		var c commentItem
		db.QueryRow(
			`SELECT c.id, c.body, COALESCE(u.display_name, u.username), c.author_id, c.created_at, c.updated_at, c.parent_id
			 FROM proposal_comments c LEFT JOIN users u ON u.id = c.author_id
			 WHERE c.id = ?`, commentID,
		).Scan(&c.ID, &c.Body, &c.AuthorName, &c.AuthorID, &c.CreatedAt, &c.UpdatedAt, &c.ParentID)
		c.Replies = []commentItem{}
		c.Reactions = []reactionItem{}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(c)
	}
}

// DeleteComment handles DELETE /api/v1/comments/{id}.
func DeleteComment(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		commentID := r.PathValue("id")

		var authorID, proposalID string
		err := db.QueryRow("SELECT author_id, proposal_id FROM proposal_comments WHERE id = ?", commentID).Scan(&authorID, &proposalID)
		if err != nil {
			http.Error(w, `{"error":"comment not found"}`, http.StatusNotFound)
			return
		}

		// Get the node for admin check.
		var nodeID string
		db.QueryRow("SELECT node_id FROM proposals WHERE id = ?", proposalID).Scan(&nodeID)

		isAuthor := user.ID == authorID
		isAdmin := user.Role == "admin" || userHasNodeRole(db, user.ID, nodeID, "admin")

		if !isAuthor && !isAdmin {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		_, err = db.Exec("DELETE FROM proposal_comments WHERE id = ?", commentID)
		if err != nil {
			http.Error(w, `{"error":"failed to delete comment"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "comment.delete", "comment", commentID, `{"proposal_id":"`+proposalID+`"}`, clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	}
}

// AddReaction handles POST /api/v1/comments/{id}/reactions.
func AddReaction(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		commentID := r.PathValue("id")

		// Verify comment exists.
		var exists int
		if err := db.QueryRow("SELECT COUNT(*) FROM proposal_comments WHERE id = ?", commentID).Scan(&exists); err != nil || exists == 0 {
			http.Error(w, `{"error":"comment not found"}`, http.StatusNotFound)
			return
		}

		var req struct {
			Emoji string `json:"emoji"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if !allowedEmoji[req.Emoji] {
			http.Error(w, `{"error":"invalid emoji"}`, http.StatusBadRequest)
			return
		}

		id := auth.NewUUIDv7()
		_, err := db.Exec(
			`INSERT INTO comment_reactions (id, comment_id, user_id, emoji) VALUES (?, ?, ?, ?)
			 ON CONFLICT(comment_id, user_id, emoji) DO NOTHING`,
			id, commentID, user.ID, req.Emoji,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to add reaction"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// RemoveReaction handles DELETE /api/v1/comments/{id}/reactions/{emoji}.
func RemoveReaction(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		commentID := r.PathValue("id")
		emoji := r.PathValue("emoji")

		_, err := db.Exec(
			`DELETE FROM comment_reactions WHERE comment_id = ? AND user_id = ? AND emoji = ?`,
			commentID, user.ID, emoji,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to remove reaction"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
