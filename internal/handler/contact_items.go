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
)

// Per-kind limits. Phone, email and note keep migration 062's lengths so a
// converted card can never be too long for the field it lands in.
const (
	maxContactHandle = 100
	maxContactLabel  = 40
	// A card is a way to be reached, not a page. The cap exists so the
	// Members room stays readable and a person card stays a card.
	maxContactItems = 12
)

// validateContactValue checks a value against its kind and returns the
// message to send back, or "" when it is fine.
func validateContactValue(kind, value string) string {
	if value == "" {
		return "an item needs a value — delete it instead of emptying it"
	}
	switch kind {
	case model.ContactKindPhone:
		if len(value) > maxContactPhone {
			return "phone must be 60 characters or fewer"
		}
	case model.ContactKindEmail:
		if len(value) > maxContactEmail {
			return "email must be 254 characters or fewer"
		}
		if !strings.Contains(value, "@") || strings.ContainsAny(value, " \t\n") {
			return "that doesn't look like an email address"
		}
	case model.ContactKindHandle:
		if len(value) > maxContactHandle {
			return "handle must be 100 characters or fewer"
		}
	case model.ContactKindNote:
		if len(value) > maxContactNote {
			return "note must be 200 characters or fewer"
		}
	}
	return ""
}

// loadMyContactItems reads a person's own items, with the count of patches
// each is shared into. The count is for the owner alone: a viewer reading
// somebody's items inside a room learns nothing about the other rooms
// (docs/adr/083 decision 4).
func loadMyContactItems(db *database.DB, userID string) ([]model.ContactItem, error) {
	rows, err := db.Query(`SELECT ci.id, ci.kind, ci.value, ci.label, ci.position,
			(SELECT COUNT(*) FROM contact_item_shares s WHERE s.item_id = ci.id)
		FROM contact_items ci WHERE ci.user_id = ?
		ORDER BY ci.position ASC, ci.id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.ContactItem{}
	for rows.Next() {
		var it model.ContactItem
		if err := rows.Scan(&it.ID, &it.Kind, &it.Value, &it.Label, &it.Position, &it.SharedWith); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// ListMyContactItems handles GET /api/v1/users/me/contact-items.
func ListMyContactItems(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		items, err := loadMyContactItems(db, user.ID)
		if err != nil {
			http.Error(w, `{"error":"failed to load contact items"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"items": items})
	}
}

// CreateMyContactItem handles POST /api/v1/users/me/contact-items. A new item
// is born shared with nothing: sharing is a separate, patch-first act
// (docs/adr/083 decision 5), so there is no field here that could grant one.
func CreateMyContactItem(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		var req struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
			Label string `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		req.Value = strings.TrimSpace(req.Value)
		req.Label = strings.TrimSpace(req.Label)
		if !model.ValidContactKind(req.Kind) {
			http.Error(w, `{"error":"kind must be phone, email, handle or note"}`, http.StatusBadRequest)
			return
		}
		if msg := validateContactValue(req.Kind, req.Value); msg != "" {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, msg), http.StatusBadRequest)
			return
		}
		if len(req.Label) > maxContactLabel {
			http.Error(w, `{"error":"label must be 40 characters or fewer"}`, http.StatusBadRequest)
			return
		}

		var count, nextPos int
		db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(position) + 1, 0) FROM contact_items WHERE user_id = ?`,
			user.ID).Scan(&count, &nextPos)
		if count >= maxContactItems {
			http.Error(w, `{"error":"a contact card holds 12 items at most"}`, http.StatusBadRequest)
			return
		}

		id := auth.NewUUIDv7()
		if _, err := db.Exec(
			`INSERT INTO contact_items (id, user_id, kind, value, label, position) VALUES (?, ?, ?, ?, ?, ?)`,
			id, user.ID, req.Kind, req.Value, req.Label, nextPos,
		); err != nil {
			http.Error(w, `{"error":"failed to add contact item"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(model.ContactItem{ID: id, Kind: req.Kind, Value: req.Value, Label: req.Label, Position: nextPos})
	}
}

// UpdateMyContactItem handles PATCH /api/v1/users/me/contact-items/{id}.
// Editing a value in place keeps every share it already has — sharing is a
// pairing, not a copy, so a new phone number reaches the same rooms without
// anyone re-deciding anything.
func UpdateMyContactItem(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		itemID := r.PathValue("id")

		var cur model.ContactItem
		if err := db.QueryRow(
			`SELECT id, kind, value, label, position FROM contact_items WHERE id = ? AND user_id = ?`,
			itemID, user.ID,
		).Scan(&cur.ID, &cur.Kind, &cur.Value, &cur.Label, &cur.Position); err != nil {
			http.Error(w, `{"error":"contact item not found"}`, http.StatusNotFound)
			return
		}

		var req struct {
			Kind     *string `json:"kind"`
			Value    *string `json:"value"`
			Label    *string `json:"label"`
			Position *int    `json:"position"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Kind != nil {
			if !model.ValidContactKind(*req.Kind) {
				http.Error(w, `{"error":"kind must be phone, email, handle or note"}`, http.StatusBadRequest)
				return
			}
			cur.Kind = *req.Kind
		}
		if req.Value != nil {
			cur.Value = strings.TrimSpace(*req.Value)
		}
		if req.Label != nil {
			cur.Label = strings.TrimSpace(*req.Label)
			if len(cur.Label) > maxContactLabel {
				http.Error(w, `{"error":"label must be 40 characters or fewer"}`, http.StatusBadRequest)
				return
			}
		}
		if req.Position != nil {
			cur.Position = *req.Position
		}
		// Re-checked against the kind in force after the patch, not the one
		// it arrived with: changing an item from note to email has to meet
		// the email rule.
		if msg := validateContactValue(cur.Kind, cur.Value); msg != "" {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, msg), http.StatusBadRequest)
			return
		}

		if _, err := db.Exec(
			`UPDATE contact_items SET kind = ?, value = ?, label = ?, position = ?, updated_at = ? WHERE id = ?`,
			cur.Kind, cur.Value, cur.Label, cur.Position, time.Now().UTC().Format(time.RFC3339), itemID,
		); err != nil {
			http.Error(w, `{"error":"failed to update contact item"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cur)
	}
}

// DeleteMyContactItem handles DELETE /api/v1/users/me/contact-items/{id}.
// Shares cascade with it.
func DeleteMyContactItem(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		res, err := db.Exec(`DELETE FROM contact_items WHERE id = ? AND user_id = ?`,
			r.PathValue("id"), user.ID)
		if err != nil {
			http.Error(w, `{"error":"failed to delete contact item"}`, http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			http.Error(w, `{"error":"contact item not found"}`, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// UnshareMyContactItemEverywhere handles
// DELETE /api/v1/users/me/contact-items/{id}/shares.
//
// The one item-first write there is, and it exists because item-first writes
// may only ever reduce exposure (docs/adr/083 decision 5). Granting stays
// patch-first; this is the panic button for a value that got somewhere it
// should not have. It stops the surface, not the knowledge — no copy here
// says "revoke".
func UnshareMyContactItemEverywhere(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		itemID := r.PathValue("id")

		var owned string
		if err := db.QueryRow(`SELECT id FROM contact_items WHERE id = ? AND user_id = ?`,
			itemID, user.ID).Scan(&owned); err != nil {
			http.Error(w, `{"error":"contact item not found"}`, http.StatusNotFound)
			return
		}
		res, err := db.Exec(`DELETE FROM contact_item_shares WHERE item_id = ?`, itemID)
		if err != nil {
			http.Error(w, `{"error":"failed to stop sharing"}`, http.StatusInternalServerError)
			return
		}
		n, _ := res.RowsAffected()
		auth.LogAuditEvent(db, user.ID, "contact.unshare_all", "contact_item", itemID,
			fmt.Sprintf(`{"patches":%d}`, n), clientIP(r))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"unshared_from": n})
	}
}

// contactRoomRole returns the caller's active role in a patch, or "" when
// they have none. Sharing is refused outside member/admin: a follower has no
// room to share into, so their card would be shown to people they never
// chose (docs/adr/080 decision 4, carried into docs/adr/083).
func contactRoomRole(db *database.DB, userID, nodeID string) string {
	var role string
	db.QueryRow(`SELECT role FROM memberships
		WHERE user_id = ? AND node_id = ? AND status = 'active' AND role IN ('member','admin')`,
		userID, nodeID).Scan(&role)
	return role
}

// GetMyContactSharesForNode handles GET /api/v1/nodes/{slug}/contact-shares:
// the caller's own items, each flagged with whether this patch sees it. The
// patch-first view of the matrix, and what both entry points render.
func GetMyContactSharesForNode(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID := NodeIDFromSlug(db, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"patch not found"}`, http.StatusNotFound)
			return
		}
		role := contactRoomRole(db, user.ID, nodeID)

		rows, err := db.Query(`SELECT ci.id, ci.kind, ci.value, ci.label, ci.position,
				EXISTS(SELECT 1 FROM contact_item_shares s WHERE s.item_id = ci.id AND s.node_id = ?)
			FROM contact_items ci WHERE ci.user_id = ?
			ORDER BY ci.position ASC, ci.id ASC`, nodeID, user.ID)
		if err != nil {
			http.Error(w, `{"error":"failed to load contact shares"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type sharedItem struct {
			model.ContactItem
			Shared bool `json:"shared"`
		}
		items := []sharedItem{}
		for rows.Next() {
			var it sharedItem
			if err := rows.Scan(&it.ID, &it.Kind, &it.Value, &it.Label, &it.Position, &it.Shared); err != nil {
				continue
			}
			items = append(items, it)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": items,
			// False for a follower and for someone with no standing at all:
			// the control renders read-only rather than disappearing, so the
			// reason is legible instead of the option merely being absent.
			"can_share": role != "",
		})
	}
}

// PutMyContactSharesForNode handles PUT /api/v1/nodes/{slug}/contact-shares.
//
// The whole set, replaced wholesale — the shape docs/adr/051's ballot uses,
// for the same reason: a partial update of a disclosure set is a request to
// get it half-applied. The body names the caller's items this patch may read;
// anything absent stops being shared with it.
func PutMyContactSharesForNode(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID := NodeIDFromSlug(db, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"patch not found"}`, http.StatusNotFound)
			return
		}
		if contactRoomRole(db, user.ID, nodeID) == "" {
			http.Error(w, `{"error":"only this patch's members and admins can share contact details with it"}`,
				http.StatusForbidden)
			return
		}

		var req struct {
			ItemIDs []string `json:"item_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"body must include item_ids"}`, http.StatusBadRequest)
			return
		}

		// Every id must be the caller's own. Checked before anything is
		// written, so a request naming somebody else's item changes nothing
		// rather than applying the rest of itself.
		for _, id := range req.ItemIDs {
			var owned string
			if err := db.QueryRow(`SELECT id FROM contact_items WHERE id = ? AND user_id = ?`,
				id, user.ID).Scan(&owned); err != nil {
				http.Error(w, `{"error":"that contact item is not yours"}`, http.StatusForbidden)
				return
			}
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, `{"error":"failed to update contact shares"}`, http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// Scoped to this caller's items: another member's shares with the
		// same patch are none of this request's business.
		if _, err := tx.Exec(`DELETE FROM contact_item_shares
			WHERE node_id = ? AND item_id IN (SELECT id FROM contact_items WHERE user_id = ?)`,
			nodeID, user.ID); err != nil {
			http.Error(w, `{"error":"failed to update contact shares"}`, http.StatusInternalServerError)
			return
		}
		for _, id := range req.ItemIDs {
			if _, err := tx.Exec(
				`INSERT OR IGNORE INTO contact_item_shares (item_id, node_id) VALUES (?, ?)`,
				id, nodeID); err != nil {
				http.Error(w, `{"error":"failed to update contact shares"}`, http.StatusInternalServerError)
				return
			}
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, `{"error":"failed to update contact shares"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "contact.shares_set", "node", nodeID,
			fmt.Sprintf(`{"items":%d}`, len(req.ItemIDs)), clientIP(r))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"node_id":  nodeID,
			"item_ids": req.ItemIDs,
		})
	}
}

// DropContactSharesFor removes every contact item this person shared into
// this patch. Called wherever a membership stops being a member/admin one:
// leaving, a ban, and demotion to follower (docs/adr/083 decision 6).
//
// Membership rows persist as status = 'left', so nothing here can be inferred
// from the row's absence — the shares have to be deleted outright. Keeping
// them dormant would make *rejoining* an act that discloses a phone number
// without a decision, which is the standing-rule fault the whole design
// removes. Best-effort by design: the caller has already changed the
// membership, and the surfaces all require an active member/admin row, so a
// failure here leaves rows that grant nothing until the next attempt.
func DropContactSharesFor(db *database.DB, userID, nodeID string) {
	db.Exec(`DELETE FROM contact_item_shares
		WHERE node_id = ? AND item_id IN (SELECT id FROM contact_items WHERE user_id = ?)`,
		nodeID, userID)
}

// sharedContactItemsFor returns the items `ownerID` shares into any patch
// where `viewerID` is also an active member or admin — the one predicate both
// surfaces run (docs/adr/083 decision 3).
//
// Both memberships are checked, not just the viewer's. Shares are dropped
// when a membership ends, so the owner's row should always be active here;
// requiring it anyway means a share row that somehow outlives its membership
// grants nothing. On a disclosure path the redundant check is worth its cost.
//
// DISTINCT because two shared patches yield the same item twice, and the
// profile must not hint at how many rooms the viewer and the owner have in
// common.
func sharedContactItemsFor(db *database.DB, ownerID, viewerID string) []model.ContactItem {
	items := []model.ContactItem{}
	if ownerID == "" || viewerID == "" {
		return items
	}
	rows, err := db.Query(`SELECT DISTINCT ci.id, ci.kind, ci.value, ci.label, ci.position
		FROM contact_items ci
		JOIN contact_item_shares s ON s.item_id = ci.id
		JOIN memberships vm ON vm.node_id = s.node_id AND vm.user_id = ?
			AND vm.status = 'active' AND vm.role IN ('member','admin')
		JOIN memberships om ON om.node_id = s.node_id AND om.user_id = ci.user_id
			AND om.status = 'active' AND om.role IN ('member','admin')
		WHERE ci.user_id = ?
		ORDER BY ci.position ASC, ci.id ASC`, viewerID, ownerID)
	if err != nil {
		return items
	}
	defer rows.Close()
	for rows.Next() {
		var it model.ContactItem
		if err := rows.Scan(&it.ID, &it.Kind, &it.Value, &it.Label, &it.Position); err != nil {
			continue
		}
		items = append(items, it)
	}
	return items
}

// sharedContactItemsForNode returns, for each of the given people, the items
// they share into this one patch — the Members room's half of the predicate
// docs/adr/083 decision 3 states once.
//
// Batched over the loaded page rather than asked per row: a Members room is a
// list, and a query per member would make the room's cost grow with the
// people in it.
//
// The owner's own membership is re-checked here for the same reason
// sharedContactItemsFor checks it — a share row that somehow outlived its
// membership must grant nothing. The *viewer's* standing is not checked here:
// the caller has already established it (`inRoom`), and duplicating that gate
// in two places is how the two drift apart.
func sharedContactItemsForNode(db *database.DB, nodeID string, userIDs []string) map[string][]model.ContactItem {
	out := map[string][]model.ContactItem{}
	if nodeID == "" || len(userIDs) == 0 {
		return out
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(userIDs)), ",")
	args := []interface{}{nodeID, nodeID}
	for _, id := range userIDs {
		args = append(args, id)
	}
	rows, err := db.Query(`SELECT ci.user_id, ci.id, ci.kind, ci.value, ci.label, ci.position
		FROM contact_items ci
		JOIN contact_item_shares s ON s.item_id = ci.id AND s.node_id = ?
		JOIN memberships om ON om.user_id = ci.user_id AND om.node_id = ?
			AND om.status = 'active' AND om.role IN ('member','admin')
		WHERE ci.user_id IN (`+placeholders+`)
		ORDER BY ci.position ASC, ci.id ASC`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var owner string
		var it model.ContactItem
		if err := rows.Scan(&owner, &it.ID, &it.Kind, &it.Value, &it.Label, &it.Position); err != nil {
			continue
		}
		out[owner] = append(out[owner], it)
	}
	return out
}
