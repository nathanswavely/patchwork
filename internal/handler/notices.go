package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// The noticeboard (docs/adr/081): a patch's members-only room for notices.
//
// Every route here goes through inRoom, and inRoom names exactly one
// population: the patch's active admins and members. Not followers — a
// follower who could read would be back in a click after a ban, since
// following is frictionless by design. Not an instance admin holding no
// role in the patch — they curate the quilt, they are not in this room, and
// the same line was drawn for the contact card (docs/adr/080). The check is
// here, in the handler, and never a hidden tab: follower permissions hide
// tabs over public reads (docs/adr/050); this is a genuinely withheld read.

const (
	maxNoticeTitle = 140
	maxNoticeBody  = 20000
	maxReplyBody   = 5000
)

// inRoom reports whether user may read this patch's noticeboard.
func inRoom(db *database.DB, user *model.User, nodeID string) bool {
	if user == nil {
		return false
	}
	return userHasNodeRole(db, user.ID, nodeID, "member", "admin")
}

// mayPostNotice applies the patch's notice_posting setting: its admins, or
// its members too. Callers have already established the user is in the room.
func mayPostNotice(db *database.DB, user *model.User, nodeID string) bool {
	var policy string
	db.QueryRow("SELECT notice_posting FROM nodes WHERE id = ?", nodeID).Scan(&policy)
	if policy == "admins" {
		return userHasNodeRole(db, user.ID, nodeID, "admin")
	}
	return true
}

// noticeItem is the JSON shape of a notice on the list and detail routes.
type noticeItem struct {
	model.Notice
	AuthorUsername    string `json:"author_username"`
	AuthorDisplayName string `json:"author_display_name"`
	ReplyCount        int    `json:"reply_count"`
}

const noticeSelect = `SELECT n.id, n.node_id, n.author_id, n.title, n.body, n.image_url, n.image_alt,
	n.replies_open, n.members_told, n.created_at, n.updated_at,
	u.username, u.display_name,
	(SELECT COUNT(*) FROM notice_replies r WHERE r.notice_id = n.id)
	FROM notices n JOIN users u ON u.id = n.author_id`

func scanNotice(row interface{ Scan(...interface{}) error }) (noticeItem, error) {
	var n noticeItem
	err := row.Scan(&n.ID, &n.NodeID, &n.AuthorID, &n.Title, &n.Body, &n.ImageURL, &n.ImageAlt,
		&n.RepliesOpen, &n.MembersTold, &n.CreatedAt, &n.UpdatedAt,
		&n.AuthorUsername, &n.AuthorDisplayName, &n.ReplyCount)
	return n, err
}

// noticeRoom resolves a notice to its patch and checks the caller is in the
// room. Returns the notice and true, or writes the error and returns false.
// A notice the caller may not read is reported as not found, not forbidden:
// the room's existence is not the caller's to learn.
func noticeRoom(db *database.DB, w http.ResponseWriter, user *model.User, noticeID string) (noticeItem, bool) {
	n, err := scanNotice(db.QueryRow(noticeSelect+" WHERE n.id = ?", noticeID))
	if err != nil || !inRoom(db, user, n.NodeID) {
		http.Error(w, `{"error":"notice not found"}`, http.StatusNotFound)
		return noticeItem{}, false
	}
	return n, true
}

// ListNotices handles GET /api/v1/nodes/{slug}/notices — newest first.
func ListNotices(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID := NodeIDFromSlug(db, r.PathValue("slug"))
		if nodeID == "" || !inRoom(db, user, nodeID) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		after, limit := parsePaginationParams(r)

		query := noticeSelect + " WHERE n.node_id = ?"
		args := []interface{}{nodeID}
		if after != "" {
			query += " AND n.id < ?"
			args = append(args, after)
		}
		query += " ORDER BY n.id DESC LIMIT ?"
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list notices"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		items := []noticeItem{}
		for rows.Next() {
			n, err := scanNotice(rows)
			if err != nil {
				continue
			}
			items = append(items, n)
		}
		var next string
		if len(items) > limit {
			next = items[limit-1].ID
			items = items[:limit]
		}

		var posting string
		var repliesDefault bool
		db.QueryRow("SELECT notice_posting, notice_replies_default FROM nodes WHERE id = ?", nodeID).Scan(&posting, &repliesDefault)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":       items,
			"next_cursor": next,
			// What this caller may do, so the page never has to guess and
			// then be told no.
			"may_post":        mayPostNotice(db, user, nodeID),
			"replies_default": repliesDefault,
			"notice_posting":  posting,
		})
	}
}

// noticeRequest is the body of create and update: the notice, whole.
type noticeRequest struct {
	Title       string `json:"title"`
	Body        string `json:"body"`
	ImageURL    string `json:"image_url"`
	ImageAlt    string `json:"image_alt"`
	RepliesOpen *bool  `json:"replies_open"`
	// TellMembers is the one way a notice reaches the bell (docs/adr/081).
	// Off unless the author reaches for it; honoured on create only.
	TellMembers bool `json:"tell_members"`
}

func (req *noticeRequest) validate() string {
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	req.ImageURL = strings.TrimSpace(req.ImageURL)
	req.ImageAlt = strings.TrimSpace(req.ImageAlt)
	switch {
	case req.Title == "":
		return "a notice needs a title"
	case len(req.Title) > maxNoticeTitle:
		return "keep the title under 140 characters"
	case len(req.Body) > maxNoticeBody:
		return "that notice is too long"
	}
	return validateImageRef(req.ImageURL, req.ImageAlt)
}

// CreateNotice handles POST /api/v1/nodes/{slug}/notices.
func CreateNotice(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")
		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" || !inRoom(db, user, nodeID) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		if !mayPostNotice(db, user, nodeID) {
			http.Error(w, `{"error":"this patch's admins put up its notices"}`, http.StatusForbidden)
			return
		}

		var req noticeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if msg := req.validate(); msg != "" {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, msg), http.StatusBadRequest)
			return
		}

		repliesOpen := true
		db.QueryRow("SELECT notice_replies_default FROM nodes WHERE id = ?", nodeID).Scan(&repliesOpen)
		if req.RepliesOpen != nil {
			repliesOpen = *req.RepliesOpen
		}

		id := auth.NewUUIDv7()
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := db.Exec(`INSERT INTO notices (id, node_id, author_id, title, body, image_url, image_alt, replies_open, members_told, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, nodeID, user.ID, req.Title, req.Body, req.ImageURL, req.ImageAlt, repliesOpen, req.TellMembers, now, now)
		if err != nil {
			http.Error(w, `{"error":"failed to create notice"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "notice.create", "notice", id,
			fmt.Sprintf(`{"node_id":"%s","members_told":%t}`, nodeID, req.TellMembers), clientIP(r))

		if req.TellMembers {
			var nodeName string
			db.QueryRow("SELECT name FROM nodes WHERE id = ?", nodeID).Scan(&nodeName)
			notify(notifications.Event{
				Type:     notifications.NoticePosted,
				NodeID:   nodeID,
				NodeSlug: slug,
				NodeName: nodeName,
				ActorID:  user.ID,
				EntityID: id,
				Title:    req.Title,
				Body:     excerpt(req.Body, 200),
				Link:     weblink.Notice(slug, id),
			})
		}

		n, _ := scanNotice(db.QueryRow(noticeSelect+" WHERE n.id = ?", id))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(n)
	}
}

// GetNotice handles GET /api/v1/notices/{id}.
func GetNotice(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		n, ok := noticeRoom(db, w, user, r.PathValue("id"))
		if !ok {
			return
		}
		var slug string
		db.QueryRow("SELECT slug FROM nodes WHERE id = ?", n.NodeID).Scan(&slug)
		isAdmin := userHasNodeRole(db, user.ID, n.NodeID, "admin")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"notice":    n,
			"node_slug": slug,
			// The two things the page decides from: editing is the author's;
			// the reply switch and removal are the author's or an admin's.
			"may_edit":   n.AuthorID == user.ID,
			"may_manage": n.AuthorID == user.ID || isAdmin,
		})
	}
}

// UpdateNotice handles PATCH /api/v1/notices/{id}. The author edits the
// notice; the author or a patch admin flips replies_open — at any time, and
// switching it off keeps the replies already made (docs/adr/081, tool 1).
func UpdateNotice(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		n, ok := noticeRoom(db, w, user, r.PathValue("id"))
		if !ok {
			return
		}
		isAuthor := n.AuthorID == user.ID
		isAdmin := userHasNodeRole(db, user.ID, n.NodeID, "admin")

		var req struct {
			Title       *string `json:"title"`
			Body        *string `json:"body"`
			ImageURL    *string `json:"image_url"`
			ImageAlt    *string `json:"image_alt"`
			RepliesOpen *bool   `json:"replies_open"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		editing := req.Title != nil || req.Body != nil || req.ImageURL != nil || req.ImageAlt != nil
		if editing {
			if !isAuthor {
				http.Error(w, `{"error":"only the author can edit a notice"}`, http.StatusForbidden)
				return
			}
			full := noticeRequest{Title: n.Title, Body: n.Body, ImageURL: n.ImageURL, ImageAlt: n.ImageAlt}
			if req.Title != nil {
				full.Title = *req.Title
			}
			if req.Body != nil {
				full.Body = *req.Body
			}
			if req.ImageURL != nil {
				full.ImageURL = *req.ImageURL
			}
			if req.ImageAlt != nil {
				full.ImageAlt = *req.ImageAlt
			}
			if msg := full.validate(); msg != "" {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, msg), http.StatusBadRequest)
				return
			}
			if _, err := db.Exec(`UPDATE notices SET title = ?, body = ?, image_url = ?, image_alt = ?, updated_at = ? WHERE id = ?`,
				full.Title, full.Body, full.ImageURL, full.ImageAlt, time.Now().UTC().Format(time.RFC3339), n.ID); err != nil {
				http.Error(w, `{"error":"failed to update notice"}`, http.StatusInternalServerError)
				return
			}
			auth.LogAuditEvent(db, user.ID, "notice.update", "notice", n.ID, "{}", clientIP(r))
		}

		if req.RepliesOpen != nil {
			if !isAuthor && !isAdmin {
				http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
				return
			}
			if _, err := db.Exec("UPDATE notices SET replies_open = ? WHERE id = ?", *req.RepliesOpen, n.ID); err != nil {
				http.Error(w, `{"error":"failed to update notice"}`, http.StatusInternalServerError)
				return
			}
			auth.LogAuditEvent(db, user.ID, "notice.replies", "notice", n.ID,
				fmt.Sprintf(`{"replies_open":%t}`, *req.RepliesOpen), clientIP(r))
		}

		out, _ := scanNotice(db.QueryRow(noticeSelect+" WHERE n.id = ?", n.ID))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
}

// DeleteNotice handles DELETE /api/v1/notices/{id} — the author or a patch
// admin takes it down. Hard delete, replies with it, audited.
func DeleteNotice(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		n, ok := noticeRoom(db, w, user, r.PathValue("id"))
		if !ok {
			return
		}
		if n.AuthorID != user.ID && !userHasNodeRole(db, user.ID, n.NodeID, "admin") {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}
		if _, err := db.Exec("DELETE FROM notices WHERE id = ?", n.ID); err != nil {
			http.Error(w, `{"error":"failed to delete notice"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "notice.delete", "notice", n.ID,
			fmt.Sprintf(`{"node_id":"%s","author_id":"%s"}`, n.NodeID, n.AuthorID), clientIP(r))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	}
}

// replyItem is the JSON shape of a reply.
type replyItem struct {
	model.NoticeReply
	AuthorUsername    string `json:"author_username"`
	AuthorDisplayName string `json:"author_display_name"`
}

const replySelect = `SELECT r.id, r.notice_id, r.author_id, r.body, r.created_at, r.updated_at, u.username, u.display_name
	FROM notice_replies r JOIN users u ON u.id = r.author_id`

func scanReply(row interface{ Scan(...interface{}) error }) (replyItem, error) {
	var x replyItem
	err := row.Scan(&x.ID, &x.NoticeID, &x.AuthorID, &x.Body, &x.CreatedAt, &x.UpdatedAt, &x.AuthorUsername, &x.AuthorDisplayName)
	return x, err
}

// ListReplies handles GET /api/v1/notices/{id}/replies — oldest first, a
// flat list (docs/adr/081, decision 2).
func ListReplies(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		n, ok := noticeRoom(db, w, user, r.PathValue("id"))
		if !ok {
			return
		}
		after, limit := parsePaginationParams(r)
		query := replySelect + " WHERE r.notice_id = ?"
		args := []interface{}{n.ID}
		if after != "" {
			query += " AND r.id > ?"
			args = append(args, after)
		}
		query += " ORDER BY r.id ASC LIMIT ?"
		args = append(args, limit+1)
		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list replies"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		items := []replyItem{}
		for rows.Next() {
			x, err := scanReply(rows)
			if err != nil {
				continue
			}
			items = append(items, x)
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

// CreateReply handles POST /api/v1/notices/{id}/replies — anyone in the
// room, while the notice takes replies.
func CreateReply(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		n, ok := noticeRoom(db, w, user, r.PathValue("id"))
		if !ok {
			return
		}
		if !n.RepliesOpen {
			http.Error(w, `{"error":"replies are off on this notice"}`, http.StatusForbidden)
			return
		}
		var req struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		req.Body = strings.TrimSpace(req.Body)
		if req.Body == "" || len(req.Body) > maxReplyBody {
			http.Error(w, `{"error":"a reply needs a body under 5000 characters"}`, http.StatusBadRequest)
			return
		}

		id := auth.NewUUIDv7()
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := db.Exec(`INSERT INTO notice_replies (id, notice_id, author_id, body, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			id, n.ID, user.ID, req.Body, now, now); err != nil {
			http.Error(w, `{"error":"failed to create reply"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "notice.reply", "notice_reply", id, fmt.Sprintf(`{"notice_id":"%s"}`, n.ID), clientIP(r))

		// Participants only: the author and those who already replied —
		// which, now, includes this replier, and the actor filter removes
		// them (docs/adr/081, decision 4).
		var slug, nodeName string
		db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", n.NodeID).Scan(&slug, &nodeName)
		notify(notifications.Event{
			Type:     notifications.NoticeReply,
			NodeID:   n.NodeID,
			NodeSlug: slug,
			NodeName: nodeName,
			ActorID:  user.ID,
			EntityID: n.ID,
			Title:    "Reply on: " + n.Title,
			Body:     excerpt(req.Body, 200),
			Link:     weblink.Notice(slug, n.ID),
		})

		x, _ := scanReply(db.QueryRow(replySelect+" WHERE r.id = ?", id))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(x)
	}
}

// replyRoom resolves a reply to its notice's room.
func replyRoom(db *database.DB, w http.ResponseWriter, user *model.User, replyID string) (replyItem, noticeItem, bool) {
	x, err := scanReply(db.QueryRow(replySelect+" WHERE r.id = ?", replyID))
	if err != nil {
		http.Error(w, `{"error":"reply not found"}`, http.StatusNotFound)
		return replyItem{}, noticeItem{}, false
	}
	n, ok := noticeRoom(db, w, user, x.NoticeID)
	return x, n, ok
}

// UpdateReply handles PATCH /api/v1/replies/{id} — the author edits.
func UpdateReply(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		x, _, ok := replyRoom(db, w, user, r.PathValue("id"))
		if !ok {
			return
		}
		if x.AuthorID != user.ID {
			http.Error(w, `{"error":"only the author can edit a reply"}`, http.StatusForbidden)
			return
		}
		var req struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		req.Body = strings.TrimSpace(req.Body)
		if req.Body == "" || len(req.Body) > maxReplyBody {
			http.Error(w, `{"error":"a reply needs a body under 5000 characters"}`, http.StatusBadRequest)
			return
		}
		if _, err := db.Exec("UPDATE notice_replies SET body = ?, updated_at = ? WHERE id = ?",
			req.Body, time.Now().UTC().Format(time.RFC3339), x.ID); err != nil {
			http.Error(w, `{"error":"failed to update reply"}`, http.StatusInternalServerError)
			return
		}
		out, _ := scanReply(db.QueryRow(replySelect+" WHERE r.id = ?", x.ID))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
}

// DeleteReply handles DELETE /api/v1/replies/{id} — the author or a patch
// admin removes it. Hard delete, audited; flat replies orphan nothing
// (docs/adr/081, tool 2).
func DeleteReply(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		x, n, ok := replyRoom(db, w, user, r.PathValue("id"))
		if !ok {
			return
		}
		if x.AuthorID != user.ID && !userHasNodeRole(db, user.ID, n.NodeID, "admin") {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}
		if _, err := db.Exec("DELETE FROM notice_replies WHERE id = ?", x.ID); err != nil {
			http.Error(w, `{"error":"failed to delete reply"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "notice.reply_delete", "notice_reply", x.ID,
			fmt.Sprintf(`{"notice_id":"%s","author_id":"%s"}`, n.ID, x.AuthorID), clientIP(r))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	}
}

// excerpt trims a body for a notification line.
func excerpt(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
