package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// The trust request
// (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar.md,
// decision 7: "a trust request is answered, never merely seen").
//
// The ask is made from the one place the review cost is being paid — the
// event form, when the event is about to queue on an unclaimed patch — and it
// is answered from Admin → Users, where the quilt-wide toggle already lives.
// What the person asked for and what the admin gave are kept as two facts, on
// the row and in the audit line, because an admin who grants one patch out of
// four has made a judgement, and a record that only shows the outcome cannot
// tell that from an admin who gave exactly what was asked.
//
// There is deliberately no entry point in navigation or account settings:
// nobody should go looking for a rank.

// trustRequestCooldown is how long a declined asker waits before asking
// again. A decline is not spent forever the way a rejected tag name is — a
// person is not a word — but neither is it an invitation to ask weekly. An
// admin may grant at any time regardless, so this delays only the asking.
const trustRequestCooldown = 30 * 24 * time.Hour

// trustNodeRef is a patch named by a request or a grant, in the shape every
// surface here renders it.
type trustNodeRef struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

func trustNow() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// requestNodes returns the patches a request named, in the order the asker
// named them. ORDER BY rowid rather than by id: the ids are UUIDv7, so
// sorting by them would return the patches in the order somebody else created
// them, which is not an order the asker would recognise.
func requestNodes(db *database.DB, requestID string) []trustNodeRef {
	out := []trustNodeRef{}
	rows, err := db.Query(
		`SELECT n.id, n.slug, n.name
		 FROM trust_request_nodes trn JOIN nodes n ON n.id = trn.node_id
		 WHERE trn.request_id = ? ORDER BY trn.rowid`, requestID,
	)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var n trustNodeRef
		if err := rows.Scan(&n.ID, &n.Slug, &n.Name); err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

// nodeRefs looks up patches by id, keeping the order of the ids given. Used
// for the granted scope, whose order is the admin's.
func nodeRefs(db *database.DB, ids []string) []trustNodeRef {
	out := []trustNodeRef{}
	for _, id := range ids {
		var n trustNodeRef
		if err := db.QueryRow(`SELECT id, slug, name FROM nodes WHERE id = ?`, id).Scan(&n.ID, &n.Slug, &n.Name); err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

// grantedScopeValue renders the stored granted_scope for the API: the string
// "all", an array of patches, or null. The column holds JSON so that the two
// scopes are one fact rather than a string column and a join table that can
// disagree.
func grantedScopeValue(db *database.DB, stored sql.NullString) interface{} {
	if !stored.Valid || stored.String == "" {
		return nil
	}
	var asString string
	if err := json.Unmarshal([]byte(stored.String), &asString); err == nil {
		return asString
	}
	var ids []string
	if err := json.Unmarshal([]byte(stored.String), &ids); err == nil {
		return nodeRefs(db, ids)
	}
	return nil
}

// unclaimedNodeIDs validates that every id names a listing — an unclaimed,
// un-removed patch — and returns them deduped in the order given. An active
// patch is never askable and never grantable: the grant is standing on a
// calendar nobody is keeping, and a patch with admins has somebody keeping it.
func unclaimedNodeIDs(db *database.DB, ids []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		var status string
		if err := db.QueryRow(
			`SELECT status FROM nodes WHERE id = ? AND removed_at IS NULL`, id,
		).Scan(&status); err != nil || status != "unclaimed" {
			return nil, fmt.Errorf("%s is not an unclaimed patch", id)
		}
		out = append(out, id)
	}
	return out, nil
}

// sweepMootTrustRequests resolves the requests that answered themselves: a
// patches-scoped ask whose every named patch has since been claimed, so the
// claim gave the calendar an owner and there is nothing left for an admin to
// grant. Moot rather than declined, because nobody judged anybody.
//
// Lazy, at the top of the two reads, the way expireAllPastDueApprovedClaims
// is: a request that has gone moot must never be listed as waiting on
// somebody, and there is no worker to notice it turning.
func sweepMootTrustRequests(db *database.DB) {
	rows, err := db.Query(
		`SELECT id, user_id FROM trust_requests
		 WHERE status = 'pending' AND scope = 'patches'
		   AND NOT EXISTS (
		     SELECT 1 FROM trust_request_nodes trn JOIN nodes n ON n.id = trn.node_id
		     WHERE trn.request_id = trust_requests.id
		       AND n.status = 'unclaimed' AND n.removed_at IS NULL
		   )`,
	)
	if err != nil {
		return
	}
	type mooted struct{ id, userID string }
	var list []mooted
	for rows.Next() {
		var m mooted
		if err := rows.Scan(&m.id, &m.userID); err != nil {
			continue
		}
		list = append(list, m)
	}
	rows.Close()

	now := trustNow()
	for _, m := range list {
		if _, err := db.Exec(
			`UPDATE trust_requests SET status = 'moot', decided_at = ? WHERE id = ? AND status = 'pending'`,
			now, m.id,
		); err != nil {
			continue
		}
		notify(notifications.Event{
			Type:     notifications.TrustRequestMoot,
			TargetID: m.userID,
			EntityID: m.id,
			Title:    "Your trust request no longer applies",
			Body:     "The patches you asked about have been claimed, so there is nothing left to grant.",
			Link:     "/",
		})
	}
}

// trustRequestPayload builds the {request, can_ask_again_at} body both the
// read and the write answer with. A nil request is a person who has never
// asked, which is a different thing from one whose ask was answered — so the
// key is always present and null rather than absent.
func trustRequestPayload(db *database.DB, userID string) map[string]interface{} {
	payload := map[string]interface{}{"request": nil, "can_ask_again_at": nil}

	var id, scope, message, status, note, createdAt string
	var grantedScope, decidedAt sql.NullString
	err := db.QueryRow(
		`SELECT id, scope, message, status, note, granted_scope, decided_at, created_at
		 FROM trust_requests WHERE user_id = ? ORDER BY created_at DESC LIMIT 1`, userID,
	).Scan(&id, &scope, &message, &status, &note, &grantedScope, &decidedAt, &createdAt)
	if err != nil {
		return payload
	}

	req := map[string]interface{}{
		"id":            id,
		"scope":         scope,
		"nodes":         requestNodes(db, id),
		"message":       message,
		"status":        status,
		"note":          note,
		"granted_scope": grantedScopeValue(db, grantedScope),
		"created_at":    createdAt,
		"decided_at":    nil,
	}
	if decidedAt.Valid && decidedAt.String != "" {
		req["decided_at"] = decidedAt.String
	}
	payload["request"] = req

	if again := canAskAgainAt(status, decidedAt); again != "" {
		payload["can_ask_again_at"] = again
	}
	return payload
}

// canAskAgainAt returns the instant a declined asker may ask again, or "" if
// nothing is holding them — either the last answer was not a decline, or the
// cooldown has already run out.
func canAskAgainAt(status string, decidedAt sql.NullString) string {
	if status != "declined" || !decidedAt.Valid || decidedAt.String == "" {
		return ""
	}
	decided, err := time.Parse("2006-01-02T15:04:05.000Z", decidedAt.String)
	if err != nil {
		decided, err = time.Parse(time.RFC3339, decidedAt.String)
		if err != nil {
			return ""
		}
	}
	until := decided.Add(trustRequestCooldown)
	if !until.After(time.Now().UTC()) {
		return ""
	}
	return until.UTC().Format("2006-01-02T15:04:05.000Z")
}

// GetMyTrustRequest handles GET /api/v1/users/me/trust-request.
func GetMyTrustRequest(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		sweepMootTrustRequests(db)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(trustRequestPayload(db, user.ID))
	}
}

// CreateTrustRequest handles POST /api/v1/users/me/trust-request.
func CreateTrustRequest(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			Scope   string   `json:"scope"`
			NodeIDs []string `json:"node_ids"`
			Message string   `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Somebody who already holds the quilt-wide grant is asking for what
		// they have. Said plainly rather than filed: the form they asked from
		// should not have been offered.
		if user.TrustedContributor {
			http.Error(w, `{"error":"you are already a trusted contributor"}`, http.StatusForbidden)
			return
		}
		if req.Scope != "all" && req.Scope != "patches" {
			http.Error(w, `{"error":"scope must be all or patches"}`, http.StatusBadRequest)
			return
		}
		if len(req.Message) > 500 {
			http.Error(w, `{"error":"message must be 500 characters or fewer"}`, http.StatusBadRequest)
			return
		}

		// A request whose patches have all been claimed is moot, not open;
		// settle it first so it never blocks the next ask.
		sweepMootTrustRequests(db)

		// One open ask per person (decision 7). Enforced here rather than by a
		// partial unique index, because 'declined' is not spent forever.
		var pendingID string
		db.QueryRow(
			`SELECT id FROM trust_requests WHERE user_id = ? AND status = 'pending' LIMIT 1`, user.ID,
		).Scan(&pendingID)
		if pendingID != "" {
			http.Error(w, `{"error":"you already have a request waiting"}`, http.StatusConflict)
			return
		}

		var lastStatus string
		var lastDecidedAt sql.NullString
		db.QueryRow(
			`SELECT status, decided_at FROM trust_requests WHERE user_id = ? ORDER BY created_at DESC LIMIT 1`,
			user.ID,
		).Scan(&lastStatus, &lastDecidedAt)
		if again := canAskAgainAt(lastStatus, lastDecidedAt); again != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":            "you can ask again on " + again[:10],
				"can_ask_again_at": again,
			})
			return
		}

		var nodeIDs []string
		if req.Scope == "patches" {
			if len(req.NodeIDs) == 0 {
				http.Error(w, `{"error":"name at least one patch"}`, http.StatusBadRequest)
				return
			}
			var err error
			nodeIDs, err = unclaimedNodeIDs(db, req.NodeIDs)
			if err != nil || len(nodeIDs) == 0 {
				http.Error(w, `{"error":"a trust request may only name unclaimed patches"}`, http.StatusBadRequest)
				return
			}
		}

		id := auth.NewUUIDv7()
		now := trustNow()
		if _, err := db.Exec(
			`INSERT INTO trust_requests (id, user_id, scope, message, status, created_at)
			 VALUES (?, ?, ?, ?, 'pending', ?)`,
			id, user.ID, req.Scope, req.Message, now,
		); err != nil {
			http.Error(w, `{"error":"failed to record the request"}`, http.StatusInternalServerError)
			return
		}
		for _, nodeID := range nodeIDs {
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO trust_request_nodes (request_id, node_id) VALUES (?, ?)`, id, nodeID,
			); err != nil {
				http.Error(w, `{"error":"failed to record the request"}`, http.StatusInternalServerError)
				return
			}
		}

		// Always an array, even for an empty/nil request, so the field has
		// one type in the audit log.
		auditNodeIDs := nodeIDs
		if auditNodeIDs == nil {
			auditNodeIDs = []string{}
		}
		auth.LogAuditEventJSON(db, user.ID, "trust.requested", "trust_request", id,
			map[string]any{"scope": req.Scope, "node_ids": auditNodeIDs}, clientIP(r))

		notify(notifications.Event{
			Type:     notifications.AdminTrustRequest,
			ActorID:  user.ID,
			EntityID: id,
			Title:    trustAskerName(user.DisplayName, user.Username) + " asked to be a trusted contributor",
			Link:     "/admin/users",
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(trustRequestPayload(db, user.ID))
	}
}

// trustAskerName is the name a notification title uses for the asker.
func trustAskerName(displayName, username string) string {
	if strings.TrimSpace(displayName) != "" {
		return displayName
	}
	return username
}

// ListTrustRequests handles GET /api/v1/admin/trust-requests — the queue,
// pending only, oldest first, because the longest wait is the one that has
// cost somebody the most.
func ListTrustRequests(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sweepMootTrustRequests(db)

		rows, err := db.Query(
			`SELECT tr.id, tr.scope, tr.message, tr.created_at,
			        u.id, ` + usernameExpr("u") + `, ` + displayNameExpr("u") + `, COALESCE(u.avatar_url, '')
			 FROM trust_requests tr JOIN users u ON u.id = tr.user_id
			 WHERE tr.status = 'pending'
			 ORDER BY tr.created_at ASC`,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to list trust requests"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		items := []map[string]interface{}{}
		for rows.Next() {
			var id, scope, message, createdAt string
			var userID, username, displayName, avatarURL string
			if err := rows.Scan(&id, &scope, &message, &createdAt, &userID, &username, &displayName, &avatarURL); err != nil {
				continue
			}
			items = append(items, map[string]interface{}{
				"id": id,
				"user": map[string]string{
					"id":           userID,
					"username":     username,
					"display_name": displayName,
					"avatar_url":   avatarURL,
				},
				"scope":      scope,
				"nodes":      requestNodes(db, id),
				"message":    message,
				"created_at": createdAt,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"items": items})
	}
}

// DecideTrustRequest handles PATCH /api/v1/admin/trust-requests/{id}.
//
// The admin answers at whatever scope they judge right, wider or narrower
// than the ask. That is why the granted node ids come off the body rather
// than off the request: narrowing four patches to one is an answer, and a
// handler that could only say yes or no to the whole ask would force an admin
// with a partial yes to decline and ask the person to try again.
func DecideTrustRequest(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := middleware.UserFromContext(r.Context())
		requestID := r.PathValue("id")

		var req struct {
			Action  string   `json:"action"`
			Scope   string   `json:"scope"`
			NodeIDs []string `json:"node_ids"`
			Note    string   `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Action != "approve" && req.Action != "decline" {
			http.Error(w, `{"error":"action must be approve or decline"}`, http.StatusBadRequest)
			return
		}

		var askerID, askedScope string
		if err := db.QueryRow(
			`SELECT user_id, scope FROM trust_requests WHERE id = ? AND status = 'pending'`, requestID,
		).Scan(&askerID, &askedScope); err != nil {
			http.Error(w, `{"error":"trust request not found"}`, http.StatusNotFound)
			return
		}

		// What was asked for, kept beside what was granted in the audit line.
		requested := interface{}("all")
		if askedScope == "patches" {
			asked := requestNodes(db, requestID)
			ids := make([]string, 0, len(asked))
			for _, n := range asked {
				ids = append(ids, n.ID)
			}
			requested = ids
		}
		now := trustNow()

		if req.Action == "decline" {
			if _, err := db.Exec(
				`UPDATE trust_requests SET status = 'declined', note = ?, decided_by = ?, decided_at = ?
				 WHERE id = ? AND status = 'pending'`,
				req.Note, admin.ID, now, requestID,
			); err != nil {
				http.Error(w, `{"error":"failed to record the decision"}`, http.StatusInternalServerError)
				return
			}
			auth.LogAuditEvent(db, admin.ID, "trust.declined", "user", askerID,
				auditJSON(map[string]interface{}{
					"requested":  requested,
					"request_id": requestID,
					"note":       req.Note,
				}), clientIP(r))

			notify(notifications.Event{
				Type:     notifications.TrustRequestDeclined,
				ActorID:  admin.ID,
				TargetID: askerID,
				EntityID: requestID,
				Title:    "Your trust request was declined",
				Body:     req.Note,
				Link:     "/",
			})

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}

		if req.Scope != "all" && req.Scope != "patches" {
			http.Error(w, `{"error":"scope must be all or patches"}`, http.StatusBadRequest)
			return
		}

		var granted interface{}
		var grantedNodes []trustNodeRef
		if req.Scope == "all" {
			if _, err := db.Exec(
				`UPDATE users SET trusted_contributor = 1, updated_at = ? WHERE id = ?`, now, askerID,
			); err != nil {
				http.Error(w, `{"error":"failed to grant"}`, http.StatusInternalServerError)
				return
			}
			granted = "all"
		} else {
			if len(req.NodeIDs) == 0 {
				http.Error(w, `{"error":"name at least one patch"}`, http.StatusBadRequest)
				return
			}
			ids, err := unclaimedNodeIDs(db, req.NodeIDs)
			if err != nil || len(ids) == 0 {
				http.Error(w, `{"error":"a grant may only name unclaimed patches"}`, http.StatusBadRequest)
				return
			}
			for _, nodeID := range ids {
				if _, err := db.Exec(
					`INSERT OR IGNORE INTO node_trusted_contributors (user_id, node_id, granted_by, granted_at, source)
					 VALUES (?, ?, ?, ?, 'request')`,
					askerID, nodeID, admin.ID, now,
				); err != nil {
					http.Error(w, `{"error":"failed to grant"}`, http.StatusInternalServerError)
					return
				}
			}
			granted = ids
			grantedNodes = nodeRefs(db, ids)
		}

		grantedScopeJSON, _ := json.Marshal(granted)
		if _, err := db.Exec(
			`UPDATE trust_requests SET status = 'approved', note = ?, granted_scope = ?, decided_by = ?, decided_at = ?
			 WHERE id = ? AND status = 'pending'`,
			req.Note, string(grantedScopeJSON), admin.ID, now, requestID,
		); err != nil {
			http.Error(w, `{"error":"failed to record the decision"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, admin.ID, "trust.granted", "user", askerID,
			auditJSON(map[string]interface{}{
				"scope":      req.Scope,
				"requested":  requested,
				"granted":    granted,
				"request_id": requestID,
			}), clientIP(r))

		title := "You're a trusted contributor"
		link := "/"
		if req.Scope == "patches" {
			if len(grantedNodes) == 1 {
				title = "You're a trusted contributor on 1 patch"
			} else {
				title = fmt.Sprintf("You're a trusted contributor on %d patches", len(grantedNodes))
			}
			if len(grantedNodes) > 0 {
				link = weblink.Patch(grantedNodes[0].Slug)
			}
		}
		notify(notifications.Event{
			Type:     notifications.TrustRequestApproved,
			ActorID:  admin.ID,
			TargetID: askerID,
			EntityID: requestID,
			Title:    title,
			Body:     req.Note,
			Link:     link,
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// auditJSON renders an audit metadata object, falling back to an empty object
// rather than writing a half-escaped line.
func auditJSON(m map[string]interface{}) string {
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// GrantTrustedPatch handles POST /api/v1/admin/users/{id}/trusted-patches —
// the per-patch grant given outright, with no request behind it.
//
// No notification, matching the quilt-wide toggle in UpdateUser, which has
// never sent one either. The two are the same act at two scopes, and one of
// them announcing itself while the other stays quiet would be the kind of
// inconsistency nobody can explain later.
func GrantTrustedPatch(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := middleware.UserFromContext(r.Context())
		targetID := r.PathValue("id")

		var req struct {
			NodeID string `json:"node_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if !userExistsUndeleted(db, targetID) {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}
		ids, err := unclaimedNodeIDs(db, []string{req.NodeID})
		if err != nil || len(ids) != 1 {
			http.Error(w, `{"error":"a grant may only name an unclaimed patch"}`, http.StatusBadRequest)
			return
		}

		if _, err := db.Exec(
			`INSERT OR IGNORE INTO node_trusted_contributors (user_id, node_id, granted_by, granted_at, source)
			 VALUES (?, ?, ?, ?, 'admin')`,
			targetID, ids[0], admin.ID, trustNow(),
		); err != nil {
			http.Error(w, `{"error":"failed to grant"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, admin.ID, "trust.granted", "user", targetID,
			auditJSON(map[string]interface{}{"scope": "patches", "granted": ids, "source": "admin"}), clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// RevokeTrustedPatch handles DELETE /api/v1/admin/users/{id}/trusted-patches/{nodeId}.
func RevokeTrustedPatch(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := middleware.UserFromContext(r.Context())
		targetID := r.PathValue("id")
		nodeID := r.PathValue("nodeId")
		if !userExistsUndeleted(db, targetID) {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		if _, err := db.Exec(
			`DELETE FROM node_trusted_contributors WHERE user_id = ? AND node_id = ?`, targetID, nodeID,
		); err != nil {
			http.Error(w, `{"error":"failed to revoke"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, admin.ID, "trust.revoked", "user", targetID,
			auditJSON(map[string]interface{}{"scope": "patches", "revoked": []string{nodeID}}), clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// trustedNodesByUser returns the per-patch grants held by a page of users,
// keyed by user id. Listings only: a grant on a patch that has since gone
// active was deleted by the claim (claims.go), and this filters by status
// anyway, because a control that offers to revoke something the claim already
// took is a control that lies about the state of the world.
func trustedNodesByUser(db *database.DB, userIDs []string) map[string][]trustNodeRef {
	out := map[string][]trustNodeRef{}
	if len(userIDs) == 0 {
		return out
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(userIDs)), ",")
	args := make([]interface{}, 0, len(userIDs))
	for _, id := range userIDs {
		args = append(args, id)
	}
	rows, err := db.Query(
		`SELECT ntc.user_id, n.id, n.slug, n.name
		 FROM node_trusted_contributors ntc JOIN nodes n ON n.id = ntc.node_id
		 WHERE ntc.user_id IN (`+placeholders+`)
		   AND n.status = 'unclaimed' AND n.removed_at IS NULL
		 ORDER BY ntc.granted_at ASC`, args...,
	)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var userID string
		var n trustNodeRef
		if err := rows.Scan(&userID, &n.ID, &n.Slug, &n.Name); err != nil {
			continue
		}
		out[userID] = append(out[userID], n)
	}
	return out
}

// userExistsUndeleted answers whether an account id names a living account.
// A tombstone (docs/adr/086) is not one: nothing may be granted to it.
func userExistsUndeleted(db *database.DB, userID string) bool {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ? AND deleted_at IS NULL`, userID).Scan(&n)
	return n == 1
}
