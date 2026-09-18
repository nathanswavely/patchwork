package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/clock"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// Tag statuses (docs/adr/114). A tag is approved vocabulary, a suggested tag
// waiting on an instance admin, or a word an admin declined. Only approved
// rows are the vocabulary: they filter, derive motifs, attract patches on the
// quilt and appear in the public tag list. The other two are the instance's
// review record.
const (
	tagApproved = "approved"
	tagPending  = "pending"
	tagRejected = "rejected"
)

// maxTagNameLen caps a suggested name. A tag is a chip and a filter value,
// not a sentence.
const maxTagNameLen = 32

type tagResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Motif is the optional motif slug this tag contributes to patches that
	// chose no explicit motif (docs/adr/021). Slugs name entries in the
	// frontend motif registry; unknown slugs fall through there (ADR 004),
	// so the server treats this as an opaque string.
	Motif     string `json:"motif,omitempty"`
	CreatedAt string `json:"created_at"`
	NodeCount int    `json:"node_count"`
}

// tagSuggestionResponse is one row of the admin review queue (docs/adr/114).
// It carries the patches wearing the word, because "who wants this and for
// what" is the whole of the decision.
type tagSuggestionResponse struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	CreatedAt   string                `json:"created_at"`
	SuggestedBy *suggestorResponse    `json:"suggested_by,omitempty"`
	Patches     []tagSuggestionPatch  `json:"patches"`
}

type suggestorResponse struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type tagSuggestionPatch struct {
	ID   string `json:"-"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// normalizeTagName folds a name to its canonical form (docs/adr/114): trim,
// NFC, internal whitespace to hyphens, lowercase, no leading or trailing
// hyphens, capped length. Deliberately not ASCII-only — a Spanish or Japanese
// quilt needs its own alphabet, and ADR 021's white-label reasoning forbids
// baking the reference instance's in. Returns "" when nothing usable remains.
func normalizeTagName(raw string) string {
	s := norm.NFC.String(strings.TrimSpace(raw))
	var b strings.Builder
	lastHyphen := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r) || r == '_' || r == '-':
			// Runs of separators collapse to one hyphen.
			if b.Len() > 0 && !lastHyphen {
				b.WriteRune('-')
				lastHyphen = true
			}
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(unicode.ToLower(r))
			lastHyphen = false
		default:
			// Punctuation, symbols and control characters are dropped: a tag
			// is rendered as a chip and used as a filter value.
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len([]rune(out)) > maxTagNameLen {
		out = strings.TrimRight(string([]rune(out)[:maxTagNameLen]), "-")
	}
	return out
}

// findTagByName looks a normalized name up case-insensitively, matching the
// idx_tags_name_nocase index rather than trusting every caller to have
// normalized first. Returns sql.ErrNoRows when absent.
func findTagByName(db *database.DB, name string) (id, status string, err error) {
	err = db.QueryRow(
		"SELECT id, status FROM tags WHERE name = ? COLLATE NOCASE", name,
	).Scan(&id, &status)
	return id, status, err
}

// ListTags handles GET /api/v1/tags.
func ListTags(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Approved only. This endpoint is unauthenticated, so the status
		// filter is a disclosure boundary and not presentation: a pending
		// word is one patch admin's request, and a rejected one is a word an
		// admin declined. Neither is the quilt's vocabulary (docs/adr/114).
		//
		// node_count likewise only counts patches the public tree would show.
		rows, err := db.Query(`
			SELECT t.id, t.name, COALESCE(t.motif,''), t.created_at,
				(SELECT COUNT(*) FROM node_tags nt
				 JOIN nodes n ON n.id = nt.node_id
				 WHERE nt.tag_id = t.id
				   AND n.status IN ('active','unclaimed')
				   AND n.removed_at IS NULL
				   AND n.visibility = 'public') AS node_count
			FROM tags t
			WHERE t.status = 'approved'
			ORDER BY t.name ASC`)
		if err != nil {
			http.Error(w, `{"error":"failed to list tags"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var tags []tagResponse
		for rows.Next() {
			var t tagResponse
			if err := rows.Scan(&t.ID, &t.Name, &t.Motif, &t.CreatedAt, &t.NodeCount); err != nil {
				continue
			}
			tags = append(tags, t)
		}
		if tags == nil {
			tags = []tagResponse{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tags)
	}
}

// CreateTag handles POST /api/v1/admin/tags.
//
// It resurrects: a name sitting pending or rejected becomes approved rather
// than colliding. Without that, an admin who declined a word in January and
// wants it in April gets "tag already exists" for a tag absent from their own
// vocabulary page — the UI and the UNIQUE constraint disagreeing, with only
// the constraint right (docs/adr/114).
func CreateTag(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			Name  string `json:"name"`
			Motif string `json:"motif"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
			return
		}
		name := normalizeTagName(req.Name)
		if name == "" {
			http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
			return
		}

		var motif interface{}
		if req.Motif != "" {
			motif = req.Motif
		}

		now := clock.Now()
		if id, status, err := findTagByName(db, name); err == nil {
			if status == tagApproved {
				http.Error(w, `{"error":"tag already exists"}`, http.StatusConflict)
				return
			}
			// Approving by the other door. Whatever patches already wear it
			// pending simply keep wearing it, now for real.
			if _, err := db.Exec(
				`UPDATE tags SET status = 'approved', motif = COALESCE(?, motif),
				        decided_by = ?, decided_at = ? WHERE id = ?`,
				motif, user.ID, now, id,
			); err != nil {
				http.Error(w, `{"error":"failed to create tag"}`, http.StatusInternalServerError)
				return
			}
			auth.LogAuditEvent(db, user.ID, "tag.suggestion_approved", "tag", id, r.RemoteAddr, fmt.Sprintf(`{"name":%q,"via":"create"}`, name))
			notifyTagDecision(tagWearers(db, id), name, name, true)
			writeTagByID(w, db, id)
			return
		}

		id := auth.NewUUIDv7()
		_, err := db.Exec(
			"INSERT INTO tags (id, name, motif, status) VALUES (?, ?, ?, 'approved')",
			id, name, motif,
		)
		if err != nil {
			http.Error(w, `{"error":"tag already exists or failed to create"}`, http.StatusConflict)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(tagResponse{ID: id, Name: name, Motif: req.Motif})
	}
}

func writeTagByID(w http.ResponseWriter, db *database.DB, id string) {
	var t tagResponse
	db.QueryRow("SELECT id, name, COALESCE(motif,''), created_at FROM tags WHERE id = ?", id).
		Scan(&t.ID, &t.Name, &t.Motif, &t.CreatedAt)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(t)
}

// UpdateTag handles PATCH /api/v1/admin/tags/{id}.
// Only the motif is mutable: renaming a tag would silently change what every
// patch wearing it says about itself, so a rename is delete + create.
func UpdateTag(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tagID := r.PathValue("id")
		if tagID == "" {
			http.Error(w, `{"error":"tag id required"}`, http.StatusBadRequest)
			return
		}

		var req struct {
			Motif *string `json:"motif"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Motif == nil {
			http.Error(w, `{"error":"motif is required (empty string clears it)"}`, http.StatusBadRequest)
			return
		}

		var motif interface{}
		if *req.Motif != "" {
			motif = *req.Motif
		}
		result, err := db.Exec("UPDATE tags SET motif = ? WHERE id = ?", motif, tagID)
		if err != nil {
			http.Error(w, `{"error":"failed to update tag"}`, http.StatusInternalServerError)
			return
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			http.Error(w, `{"error":"tag not found"}`, http.StatusNotFound)
			return
		}

		writeTagByID(w, db, tagID)
	}
}

// DeleteTag handles DELETE /api/v1/admin/tags/{id}.
//
// Deleting is not rejecting (docs/adr/114): it strips the word from every
// patch wearing it and frees the name to be suggested again. The review queue
// never uses this route.
func DeleteTag(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tagID := r.PathValue("id")
		if tagID == "" {
			http.Error(w, `{"error":"tag id required"}`, http.StatusBadRequest)
			return
		}

		// Delete associations first, then tag.
		db.Exec("DELETE FROM node_tags WHERE tag_id = ?", tagID)
		result, err := db.Exec("DELETE FROM tags WHERE id = ?", tagID)
		if err != nil {
			http.Error(w, `{"error":"failed to delete tag"}`, http.StatusInternalServerError)
			return
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			http.Error(w, `{"error":"tag not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// resolveTagIDs maps tag names to IDs, preserving order. Unknown names are
// rejected — patch admins pick from the curated vocabulary (docs/adr/021).
// Proposing a new word is a separate field on the request, so that a typo
// stays an immediate error instead of quietly coining vocabulary
// (docs/adr/114).
//
// Pending and rejected rows are not the vocabulary and do not resolve here.
func resolveTagIDs(db *database.DB, names []string) ([]string, string) {
	ids := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		normalized := normalizeTagName(name)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		var id string
		if err := db.QueryRow(
			"SELECT id FROM tags WHERE name = ? COLLATE NOCASE AND status = 'approved'", normalized,
		).Scan(&id); err != nil {
			return nil, name
		}
		ids = append(ids, id)
	}
	return ids, ""
}

// setNodeTags replaces a node's approved tags with the given tag IDs, storing
// the array order as position — the patch admin's priority order.
//
// The delete is scoped to approved tags on purpose (docs/adr/114). A patch's
// settings form loads node.tags and spreads it back into a wholesale PATCH,
// so an unscoped delete would silently destroy that patch's own pending
// suggestion every time its admin edited tags for an unrelated reason. A
// suggestion can only be removed by asking.
func setNodeTags(db *database.DB, nodeID string, tagIDs []string) error {
	if _, err := db.Exec(
		`DELETE FROM node_tags WHERE node_id = ? AND tag_id IN (
		     SELECT id FROM tags WHERE status = 'approved')`, nodeID,
	); err != nil {
		return err
	}
	for i, tagID := range tagIDs {
		if _, err := db.Exec(
			"INSERT OR IGNORE INTO node_tags (node_id, tag_id, position) VALUES (?, ?, ?)",
			nodeID, tagID, i,
		); err != nil {
			return err
		}
	}
	return nil
}

// nodeTagNames returns a node's approved tag names in stored (priority)
// order. This is the public "tags" array: it means the same thing to every
// reader (docs/adr/114).
func nodeTagNames(db *database.DB, nodeID string) []string {
	return nodeTagNamesByStatus(db, nodeID, tagApproved)
}

// nodePendingTagNames returns the suggested tags a patch is wearing
// provisionally. Sent only to that patch's admins.
func nodePendingTagNames(db *database.DB, nodeID string) []string {
	return nodeTagNamesByStatus(db, nodeID, tagPending)
}

func nodeTagNamesByStatus(db *database.DB, nodeID, status string) []string {
	tags := []string{}
	rows, err := db.Query(
		`SELECT t.name FROM node_tags nt JOIN tags t ON nt.tag_id = t.id
		 WHERE nt.node_id = ? AND t.status = ?
		 ORDER BY COALESCE(nt.position, 1000000), t.name`, nodeID, status,
	)
	if err != nil {
		return tags
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			tags = append(tags, name)
		}
	}
	return tags
}

// suggestTagsForNode attaches proposed words to a patch (docs/adr/114).
//
// A name that normalizes onto an existing approved tag is not a suggestion at
// all: it is an ordinary pick, attached immediately with no queue entry. A
// name already pending attaches to that row rather than coining a second one,
// which is what makes approval one decision about one word. A name an admin
// already declined is refused by name, so the person hears why instead of
// watching a chip fail to appear.
//
// Returns the number of new suggestions coined, and an error message fit to
// show the person.
func suggestTagsForNode(db *database.DB, nodeID, userID string, names []string) (int, string) {
	coined := 0
	for _, raw := range names {
		name := normalizeTagName(raw)
		if name == "" {
			return coined, fmt.Sprintf("%q is not a usable tag name", raw)
		}

		id, status, err := findTagByName(db, name)
		switch {
		case err == sql.ErrNoRows:
			id = auth.NewUUIDv7()
			if _, err := db.Exec(
				"INSERT INTO tags (id, name, status, suggested_by) VALUES (?, ?, 'pending', ?)",
				id, name, userID,
			); err != nil {
				return coined, "failed to suggest tag: " + name
			}
			coined++
		case err != nil:
			return coined, "failed to suggest tag: " + name
		case status == tagRejected:
			return coined, "an admin has already declined the tag " + name
		}

		// Approved or pending, the patch wears it. Position puts a suggestion
		// after the picked tags: it is not vocabulary yet, so it should not
		// win motif derivation the moment it is approved.
		if _, err := db.Exec(
			`INSERT OR IGNORE INTO node_tags (node_id, tag_id, position)
			 VALUES (?, ?, (SELECT COALESCE(MAX(position), -1) + 1 FROM node_tags WHERE node_id = ?))`,
			nodeID, id, nodeID,
		); err != nil {
			return coined, "failed to attach tag: " + name
		}
	}
	return coined, ""
}

// WithdrawSuggestedTag handles DELETE /api/v1/nodes/{slug}/suggested-tags/{name}.
// Taking a suggestion back is an explicit act, never a side effect of saving
// a form (docs/adr/114).
func WithdrawSuggestedTag(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")
		name := normalizeTagName(r.PathValue("name"))

		var nodeID string
		if err := db.QueryRow("SELECT id FROM nodes WHERE slug = ?", slug).Scan(&nodeID); err != nil {
			http.Error(w, `{"error":"patch not found"}`, http.StatusNotFound)
			return
		}
		if !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"only patch admins can withdraw a suggested tag"}`, http.StatusForbidden)
			return
		}

		res, err := db.Exec(
			`DELETE FROM node_tags WHERE node_id = ? AND tag_id IN (
			     SELECT id FROM tags WHERE name = ? COLLATE NOCASE AND status = 'pending')`,
			nodeID, name,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to withdraw suggested tag"}`, http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			http.Error(w, `{"error":"no such suggested tag on this patch"}`, http.StatusNotFound)
			return
		}

		// A word nobody is asking for any more leaves the queue. The row is
		// only removed when it was never decided: a rejected name keeps its
		// row so it stays spent.
		db.Exec(
			`DELETE FROM tags WHERE name = ? COLLATE NOCASE AND status = 'pending'
			   AND id NOT IN (SELECT tag_id FROM node_tags)`, name,
		)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// ListTagSuggestions handles GET /api/v1/admin/tag-suggestions.
func ListTagSuggestions(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// displayNameExpr/usernameExpr substitute for a deleted account
		// (docs/adr/086): writing the COALESCE by hand here would leak a
		// retired handle into the queue and nothing would fail.
		rows, err := db.Query(`
			SELECT t.id, t.name, t.created_at,
			       ` + usernameExpr("u") + `, ` + displayNameExpr("u") + `
			FROM tags t
			LEFT JOIN users u ON u.id = t.suggested_by
			WHERE t.status = 'pending'
			ORDER BY t.created_at ASC`)
		if err != nil {
			http.Error(w, `{"error":"failed to list tag suggestions"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		out := []tagSuggestionResponse{}
		for rows.Next() {
			var s tagSuggestionResponse
			var username, displayName sql.NullString
			if err := rows.Scan(&s.ID, &s.Name, &s.CreatedAt, &username, &displayName); err != nil {
				continue
			}
			if username.Valid {
				s.SuggestedBy = &suggestorResponse{Username: username.String, DisplayName: displayName.String}
			}
			s.Patches = []tagSuggestionPatch{}
			out = append(out, s)
		}

		// The patches wearing each word: the decision is "who wants this, and
		// for what", so the queue names them.
		for i := range out {
			if w := tagWearers(db, out[i].ID); w != nil {
				out[i].Patches = w
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
}

// DecideTagSuggestion handles PATCH /api/v1/admin/tag-suggestions/{id}.
//
// Approving may rename, and renaming may merge: onto an approved tag, a
// rejected one, or another pending one (docs/adr/114). Merge re-points the
// attachments rather than inserting, because the patch may already wear the
// target.
func DecideTagSuggestion(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		id := r.PathValue("id")

		var req struct {
			Action string `json:"action"`
			Name   string `json:"name"`
			Motif  string `json:"motif"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		var originalName, status string
		if err := db.QueryRow("SELECT name, status FROM tags WHERE id = ?", id).Scan(&originalName, &status); err != nil {
			http.Error(w, `{"error":"suggestion not found"}`, http.StatusNotFound)
			return
		}
		if status != tagPending {
			http.Error(w, `{"error":"that tag is not awaiting review"}`, http.StatusConflict)
			return
		}

		now := clock.Now()

		switch req.Action {
		case "reject":
			// The row stays so the name is spent: one rejection settles a
			// word. The attachments go, because the word does not exist for
			// patches — so the audience is read first.
			wearers := tagWearers(db, id)
			db.Exec("DELETE FROM node_tags WHERE tag_id = ?", id)
			if _, err := db.Exec(
				"UPDATE tags SET status = 'rejected', decided_by = ?, decided_at = ? WHERE id = ?",
				user.ID, now, id,
			); err != nil {
				http.Error(w, `{"error":"failed to reject suggestion"}`, http.StatusInternalServerError)
				return
			}
			auth.LogAuditEvent(db, user.ID, "tag.suggestion_rejected", "tag", id, r.RemoteAddr, fmt.Sprintf(`{"name":%q}`, originalName))
			notifyTagDecision(wearers, originalName, originalName, false)

		case "approve":
			finalName := originalName
			if req.Name != "" {
				finalName = normalizeTagName(req.Name)
				if finalName == "" {
					http.Error(w, `{"error":"that is not a usable tag name"}`, http.StatusBadRequest)
					return
				}
			}

			targetID := id
			if !strings.EqualFold(finalName, originalName) {
				existingID, existingStatus, err := findTagByName(db, finalName)
				if err == nil && existingID != id {
					// Merge: re-point every attachment, skipping patches that
					// already wear the target so the composite primary key
					// holds and the existing position survives.
					db.Exec(
						`INSERT OR IGNORE INTO node_tags (node_id, tag_id, position)
						 SELECT node_id, ?, position FROM node_tags WHERE tag_id = ?`,
						existingID, id,
					)
					db.Exec("DELETE FROM node_tags WHERE tag_id = ?", id)
					db.Exec("DELETE FROM tags WHERE id = ?", id)
					targetID = existingID
					if existingStatus == tagApproved && req.Motif == "" {
						// Already vocabulary; nothing to approve.
						auth.LogAuditEvent(db, user.ID, "tag.suggestion_approved", "tag", targetID, r.RemoteAddr,
							fmt.Sprintf(`{"name":%q,"suggested_as":%q,"merged":true}`, finalName, originalName))
						notifyTagDecision(tagWearers(db, targetID), originalName, finalName, true)
						writeTagByID(w, db, targetID)
						return
					}
				} else if err == sql.ErrNoRows {
					if _, uerr := db.Exec("UPDATE tags SET name = ? WHERE id = ?", finalName, id); uerr != nil {
						http.Error(w, `{"error":"failed to rename suggestion"}`, http.StatusInternalServerError)
						return
					}
				}
			}

			var motif interface{}
			if req.Motif != "" {
				motif = req.Motif
			}
			if _, err := db.Exec(
				`UPDATE tags SET status = 'approved', motif = COALESCE(?, motif),
				        decided_by = ?, decided_at = ? WHERE id = ?`,
				motif, user.ID, now, targetID,
			); err != nil {
				http.Error(w, `{"error":"failed to approve suggestion"}`, http.StatusInternalServerError)
				return
			}
			auth.LogAuditEvent(db, user.ID, "tag.suggestion_approved", "tag", targetID, r.RemoteAddr,
				fmt.Sprintf(`{"name":%q,"suggested_as":%q}`, finalName, originalName))
			notifyTagDecision(tagWearers(db, targetID), originalName, finalName, true)
			writeTagByID(w, db, targetID)
			return

		default:
			http.Error(w, `{"error":"action must be approve or reject"}`, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// tagWearers is every patch wearing a tag, collected before anything is
// deleted.
//
// Order matters and is the whole reason this is a separate call: notify()
// dispatches on a goroutine and the audience is resolved from NodeID at
// delivery time, so rejecting (which deletes the attachments) has to read the
// list first or the notice reaches nobody.
func tagWearers(db *database.DB, tagID string) []tagSuggestionPatch {
	rows, err := db.Query(
		`SELECT n.id, n.slug, n.name FROM node_tags nt JOIN nodes n ON n.id = nt.node_id
		 WHERE nt.tag_id = ?`, tagID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []tagSuggestionPatch
	for rows.Next() {
		var p tagSuggestionPatch
		if rows.Scan(&p.ID, &p.Slug, &p.Name) == nil {
			out = append(out, p)
		}
	}
	return out
}

// notifyTagDecision tells the admins of every patch wearing the word how it
// went (docs/adr/114). Not the coiner alone: the thing that changes is a chip
// on their patch, and an admin watching one appear or vanish with no
// explanation is the failure this prevents.
func notifyTagDecision(wearers []tagSuggestionPatch, suggestedAs, finalName string, approved bool) {
	for _, p := range wearers {
		ev := notifications.Event{
			NodeID:   p.ID,
			NodeSlug: p.Slug,
			NodeName: p.Name,
			Link:     "/patches/" + p.Slug + "/settings/info",
		}
		if approved {
			ev.Type = notifications.TagSuggestionApproved
			ev.Title = "Tag approved: " + finalName
			if !strings.EqualFold(suggestedAs, finalName) {
				// Telling somebody a word they never typed was approved is
				// worse than silence.
				ev.Body = "Suggested as " + suggestedAs + ", now live as " + finalName + " and public on " + p.Name + "."
			} else {
				ev.Body = finalName + " is now one of the quilt's tags, and public on " + p.Name + "."
			}
		} else {
			ev.Type = notifications.TagSuggestionRejected
			ev.Title = "Tag not added: " + suggestedAs
			ev.Body = "An admin declined " + suggestedAs + ", so it is no longer on " + p.Name + "."
		}
		notify(ev)
	}
}

// notifyAdminsOfTagSuggestion tells the instance admins a word is waiting.
func notifyAdminsOfTagSuggestion(name, byUserID string) {
	notify(notifications.Event{
		Type:    notifications.AdminTagSuggestion,
		ActorID: byUserID,
		Title:   "Tag suggested: " + name,
		Body:    "Someone asked for a tag that is not in the vocabulary yet.",
		Link:    "/admin/tags",
	})
}
