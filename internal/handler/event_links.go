package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// Event links (docs/adr/032): one owner, two consents. Admins on either
// side propose a link; admins on the other side confirm it. Pending
// links are invisible everywhere. The sync never writes links — every
// link is human-initiated.

// userSpeaksForNode reports whether a user may act for a patch in the
// link handshake. Speaking for a patch is admin territory: patch admins
// on active patches, the instance admin everywhere (who holds unclaimed
// patches' calendars in trust — docs/adr/031).
func userSpeaksForNode(db *database.DB, user *model.User, nodeID string) bool {
	if user == nil {
		return false
	}
	if user.Role == "admin" {
		return true
	}
	return userHasNodeRole(db, user.ID, nodeID, "admin")
}

// linkTargetNode resolves the request's target — a slug or a pasted
// patch URL — to a local node ID, or to a remote (host, slug) pair when
// the URL points at another quilt. A local URL routes into the real
// handshake, never a consent-free mention (docs/adr/032).
func linkTargetNode(db *database.DB, cfg *config.Config, target string) (nodeID, remoteHost, remoteSlug string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", ""
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		u, err := url.Parse(target)
		if err != nil {
			return "", "", ""
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) != 2 || parts[0] != "patches" || parts[1] == "" {
			return "", "", ""
		}
		if !strings.EqualFold(u.Hostname(), cfg.Instance.Domain) {
			return "", u.Hostname(), parts[1]
		}
		target = parts[1]
	}
	return NodeIDFromSlug(db, target), "", ""
}

// loadLinkableEvent loads what the handshake needs and enforces that
// only active events are linkable (docs/adr/032).
func loadLinkableEvent(db *database.DB, eventID string) (nodeID, title, status string, ok bool) {
	err := db.QueryRow(
		`SELECT e.node_id, e.title, e.status FROM events e
		 JOIN nodes n ON e.node_id = n.id
		 WHERE e.id = ? AND e.removed_at IS NULL
		   AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL`,
		eventID,
	).Scan(&nodeID, &title, &status)
	return nodeID, title, status, err == nil
}

// notifyLinkRequest routes the pending link to whoever confirms it:
// patch admins for active patches, site admins for unclaimed ones (the
// same door as event submissions — docs/adr/026).
func notifyLinkRequest(db *database.DB, confirmingNodeID, actorID, eventID, eventTitle, ownerSlug string) {
	var slug, name, status string
	db.QueryRow("SELECT slug, name, status FROM nodes WHERE id = ?", confirmingNodeID).Scan(&slug, &name, &status)
	link := "/patches/" + ownerSlug + "/events/" + eventID
	if status == "unclaimed" {
		notify(notifications.Event{
			Type:     notifications.AdminEventLinkRequest,
			NodeID:   confirmingNodeID,
			NodeSlug: slug,
			NodeName: name,
			ActorID:  actorID,
			EntityID: eventID,
			Title:    "Event link request: " + eventTitle,
			Link:     link,
		})
		return
	}
	notify(notifications.Event{
		Type:     notifications.EventLinkRequested,
		NodeID:   confirmingNodeID,
		NodeSlug: slug,
		NodeName: name,
		ActorID:  actorID,
		EntityID: eventID,
		Title:    "Event link request: " + eventTitle,
		Link:     link,
	})
}

// CreateEventLink handles POST /api/v1/events/{id}/links.
//
// Body: {"target": "<slug or patch URL>", "absorb_event_id": "..."}.
// The caller's admin standing decides which side is proposing; adminning
// both sides confirms instantly. A remote patch URL becomes a
// cross-quilt mention instead — display-only, owner-side action.
func CreateEventLink(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		eventID := r.PathValue("id")

		var req struct {
			Target        string `json:"target"`
			AbsorbEventID string `json:"absorb_event_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Target) == "" {
			http.Error(w, `{"error":"target is required"}`, http.StatusBadRequest)
			return
		}

		ownerNodeID, eventTitle, eventStatus, ok := loadLinkableEvent(db, eventID)
		if !ok {
			http.Error(w, `{"error":"event not found"}`, http.StatusNotFound)
			return
		}
		if eventStatus != "active" {
			http.Error(w, `{"error":"only active events can be linked"}`, http.StatusConflict)
			return
		}

		ownerSide := userSpeaksForNode(db, user, ownerNodeID)

		targetNodeID, remoteHost, remoteSlug := linkTargetNode(db, cfg, req.Target)

		// A remote patch URL is a cross-quilt mention: a doorway on the
		// event page, owned by the event's own patch (docs/adr/032).
		if remoteHost != "" {
			if !ownerSide {
				http.Error(w, `{"error":"only this patch's admins can add a mention"}`, http.StatusForbidden)
				return
			}
			id := auth.NewUUIDv7()
			if _, err := db.Exec(
				`INSERT INTO event_mentions (id, event_id, host, slug, name) VALUES (?, ?, ?, ?, ?)`,
				id, eventID, remoteHost, remoteSlug, remoteSlug,
			); err != nil {
				http.Error(w, `{"error":"already mentioned"}`, http.StatusConflict)
				return
			}
			auth.LogAuditEvent(db, user.ID, "event.mention_add", "event", eventID, "{}", clientIP(r))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(model.EventMention{ID: id, EventID: eventID, Host: remoteHost, Slug: remoteSlug, Name: remoteSlug})
			return
		}

		if targetNodeID == "" {
			http.Error(w, `{"error":"patch not found"}`, http.StatusNotFound)
			return
		}
		if targetNodeID == ownerNodeID {
			http.Error(w, `{"error":"an event is already on its own patch"}`, http.StatusBadRequest)
			return
		}

		linkedSide := userSpeaksForNode(db, user, targetNodeID)
		if !ownerSide && !linkedSide {
			http.Error(w, `{"error":"only admins of either patch can link"}`, http.StatusForbidden)
			return
		}

		initiatedBy := "owner"
		if !ownerSide {
			initiatedBy = "linked"
		}
		status := "pending"
		if ownerSide && linkedSide {
			// One person admins both sides — the handshake completes itself.
			status = "confirmed"
		}

		// Absorption is the linked side's call: the duplicate must be that
		// patch's own event, chosen by one of its admins (docs/adr/032).
		var absorb *string
		if req.AbsorbEventID != "" {
			if !linkedSide {
				http.Error(w, `{"error":"only the linked patch's admins choose an event to absorb"}`, http.StatusForbidden)
				return
			}
			if !validAbsorbTarget(db, req.AbsorbEventID, targetNodeID, eventID) {
				http.Error(w, `{"error":"absorb_event_id must be an event on the linked patch"}`, http.StatusBadRequest)
				return
			}
			absorb = &req.AbsorbEventID
		}

		id := auth.NewUUIDv7()
		var err error
		if status == "confirmed" {
			_, err = db.Exec(
				`INSERT INTO event_links (id, event_id, node_id, status, initiated_by, requested_by, absorb_event_id, confirmed_at)
				 VALUES (?, ?, ?, 'confirmed', ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`,
				id, eventID, targetNodeID, initiatedBy, user.ID, absorb,
			)
		} else {
			_, err = db.Exec(
				`INSERT INTO event_links (id, event_id, node_id, status, initiated_by, requested_by, absorb_event_id)
				 VALUES (?, ?, ?, 'pending', ?, ?, ?)`,
				id, eventID, targetNodeID, initiatedBy, user.ID, absorb,
			)
		}
		if err != nil {
			http.Error(w, `{"error":"link already exists"}`, http.StatusConflict)
			return
		}
		auth.LogAuditEvent(db, user.ID, "event.link_request", "event", eventID, `{"node_id":"`+targetNodeID+`"}`, clientIP(r))

		var ownerSlug string
		db.QueryRow("SELECT slug FROM nodes WHERE id = ?", ownerNodeID).Scan(&ownerSlug)

		if status == "confirmed" {
			finalizeConfirmedLink(db, user.ID, eventID, eventTitle, ownerNodeID, ownerSlug, targetNodeID, absorb)
		} else {
			confirming := targetNodeID
			if initiatedBy == "linked" {
				confirming = ownerNodeID
			}
			notifyLinkRequest(db, confirming, user.ID, eventID, eventTitle, ownerSlug)
		}

		link := loadEventLink(db, eventID, targetNodeID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(link)
	}
}

// validAbsorbTarget checks a chosen duplicate: an event on the linked
// patch that isn't the event being linked to.
func validAbsorbTarget(db *database.DB, absorbID, linkedNodeID, eventID string) bool {
	if absorbID == eventID {
		return false
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE id = ? AND node_id = ? AND removed_at IS NULL`, absorbID, linkedNodeID).Scan(&n)
	return n > 0
}

// absorbDuplicate deletes the linked side's duplicate event, skip-listing
// its feed item first if it was imported, so the next sync doesn't
// resurrect it (docs/adr/031, docs/adr/032).
func absorbDuplicate(db *database.DB, absorbID string) {
	var sourceID, sourceUID *string
	var sourceOccurrence string
	if err := db.QueryRow(
		`SELECT source_id, source_uid, source_occurrence FROM events WHERE id = ?`, absorbID,
	).Scan(&sourceID, &sourceUID, &sourceOccurrence); err != nil {
		return
	}
	if sourceID != nil && sourceUID != nil {
		db.Exec(`INSERT OR IGNORE INTO event_source_skips (source_id, uid, occurrence) VALUES (?, ?, ?)`,
			*sourceID, *sourceUID, sourceOccurrence)
	}
	db.Exec(`DELETE FROM events WHERE id = ?`, absorbID)
}

// finalizeConfirmedLink runs everything a link confirmation owes: the
// absorb, the notification back to the requesting side, and the AP
// Announce from the linked patch's actor (docs/adr/032 — never Create;
// the object stays attributed to the owner).
func finalizeConfirmedLink(db *database.DB, actorID, eventID, eventTitle, ownerNodeID, ownerSlug, linkedNodeID string, absorb *string) {
	if absorb != nil {
		absorbDuplicate(db, *absorb)
	}

	var linkedSlug, linkedName string
	db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", linkedNodeID).Scan(&linkedSlug, &linkedName)
	notify(notifications.Event{
		Type:     notifications.EventLinkConfirmed,
		NodeID:   linkedNodeID,
		NodeSlug: linkedSlug,
		NodeName: linkedName,
		ActorID:  actorID,
		EntityID: eventID,
		Title:    "Event linked: " + eventTitle,
		Link:     "/patches/" + ownerSlug + "/events/" + eventID,
	})

	// Only public events federate (matching broadcastEventCreate).
	var visibility string
	db.QueryRow("SELECT visibility FROM events WHERE id = ?", eventID).Scan(&visibility)
	if visibility == "public" {
		go func() {
			domain := ap.GetDomain()
			eventAPID := ap.EventAPID(domain, eventID)
			activity := map[string]interface{}{
				"@context": ap.Context,
				"type":     "Announce",
				"id":       eventAPID + "/announce/" + linkedNodeID,
				"actor":    ap.NodeAPID(domain, linkedNodeID),
				"object":   eventAPID,
			}
			ap.BroadcastToFollowers(db, "node", linkedNodeID, activity)
		}()
	}
}

// ConfirmEventLink handles POST /api/v1/events/{id}/links/{nodeId}/confirm.
// Whichever side didn't initiate confirms. When the owner initiated, the
// confirmer is the linked patch's admin and may pick a duplicate of their
// own to absorb.
func ConfirmEventLink(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		eventID := r.PathValue("id")
		linkedNodeID := r.PathValue("nodeId")

		var status, initiatedBy string
		var absorb *string
		err := db.QueryRow(
			`SELECT status, initiated_by, absorb_event_id FROM event_links WHERE event_id = ? AND node_id = ?`,
			eventID, linkedNodeID,
		).Scan(&status, &initiatedBy, &absorb)
		if err != nil {
			http.Error(w, `{"error":"link not found"}`, http.StatusNotFound)
			return
		}
		if status != "pending" {
			http.Error(w, `{"error":"link is already confirmed"}`, http.StatusConflict)
			return
		}

		ownerNodeID, eventTitle, _, ok := loadLinkableEvent(db, eventID)
		if !ok {
			http.Error(w, `{"error":"event not found"}`, http.StatusNotFound)
			return
		}

		// The non-initiating side confirms (docs/adr/032).
		confirmingNodeID := linkedNodeID
		if initiatedBy == "linked" {
			confirmingNodeID = ownerNodeID
		}
		if !userSpeaksForNode(db, user, confirmingNodeID) {
			http.Error(w, `{"error":"only the other patch's admins can confirm"}`, http.StatusForbidden)
			return
		}

		// The linked side's confirmer may choose a duplicate to absorb.
		var req struct {
			AbsorbEventID string `json:"absorb_event_id"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.AbsorbEventID != "" {
			if initiatedBy != "owner" {
				http.Error(w, `{"error":"only the linked patch's admins choose an event to absorb"}`, http.StatusForbidden)
				return
			}
			if !validAbsorbTarget(db, req.AbsorbEventID, linkedNodeID, eventID) {
				http.Error(w, `{"error":"absorb_event_id must be an event on the linked patch"}`, http.StatusBadRequest)
				return
			}
			absorb = &req.AbsorbEventID
		}

		if _, err := db.Exec(
			`UPDATE event_links SET status = 'confirmed', confirmed_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			 WHERE event_id = ? AND node_id = ?`,
			eventID, linkedNodeID,
		); err != nil {
			http.Error(w, `{"error":"failed to confirm link"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "event.link_confirm", "event", eventID, `{"node_id":"`+linkedNodeID+`"}`, clientIP(r))

		var ownerSlug string
		db.QueryRow("SELECT slug FROM nodes WHERE id = ?", ownerNodeID).Scan(&ownerSlug)
		finalizeConfirmedLink(db, user.ID, eventID, eventTitle, ownerNodeID, ownerSlug, linkedNodeID, absorb)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(loadEventLink(db, eventID, linkedNodeID))
	}
}

// RemoveEventLink handles DELETE /api/v1/events/{id}/links/{nodeId}.
// Either side severs unilaterally, one tap — consent is continuous
// (docs/adr/032). Removing a pending link is declining it.
func RemoveEventLink(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		eventID := r.PathValue("id")
		linkedNodeID := r.PathValue("nodeId")

		var ownerNodeID string
		err := db.QueryRow(
			`SELECT e.node_id FROM event_links l JOIN events e ON e.id = l.event_id
			 WHERE l.event_id = ? AND l.node_id = ?`,
			eventID, linkedNodeID,
		).Scan(&ownerNodeID)
		if err != nil {
			http.Error(w, `{"error":"link not found"}`, http.StatusNotFound)
			return
		}

		if !userSpeaksForNode(db, user, ownerNodeID) && !userSpeaksForNode(db, user, linkedNodeID) {
			http.Error(w, `{"error":"only admins of either patch can remove a link"}`, http.StatusForbidden)
			return
		}

		if _, err := db.Exec(`DELETE FROM event_links WHERE event_id = ? AND node_id = ?`, eventID, linkedNodeID); err != nil {
			http.Error(w, `{"error":"failed to remove link"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "event.link_remove", "event", eventID, `{"node_id":"`+linkedNodeID+`"}`, clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// RemoveEventMention handles DELETE /api/v1/events/{id}/mentions/{mentionId}.
// A mention is the owner's display content — owner-side admins remove it.
func RemoveEventMention(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		eventID := r.PathValue("id")
		mentionID := r.PathValue("mentionId")

		var ownerNodeID string
		err := db.QueryRow(
			`SELECT e.node_id FROM event_mentions m JOIN events e ON e.id = m.event_id
			 WHERE m.id = ? AND m.event_id = ?`,
			mentionID, eventID,
		).Scan(&ownerNodeID)
		if err != nil {
			http.Error(w, `{"error":"mention not found"}`, http.StatusNotFound)
			return
		}
		if !userSpeaksForNode(db, user, ownerNodeID) {
			http.Error(w, `{"error":"only this patch's admins can remove a mention"}`, http.StatusForbidden)
			return
		}

		db.Exec(`DELETE FROM event_mentions WHERE id = ?`, mentionID)
		auth.LogAuditEvent(db, user.ID, "event.mention_remove", "event", eventID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// loadEventLink loads one link with its display fields.
func loadEventLink(db *database.DB, eventID, nodeID string) model.EventLink {
	var l model.EventLink
	db.QueryRow(
		`SELECT l.id, l.event_id, l.node_id, l.status, l.initiated_by, l.requested_by, l.created_at, n.name, n.slug
		 FROM event_links l JOIN nodes n ON l.node_id = n.id
		 WHERE l.event_id = ? AND l.node_id = ?`,
		eventID, nodeID,
	).Scan(&l.ID, &l.EventID, &l.NodeID, &l.Status, &l.InitiatedBy, &l.RequestedBy, &l.CreatedAt, &l.NodeName, &l.NodeSlug)
	return l
}

// eventLinksForViewer loads an event's links: confirmed for everyone,
// pending only for viewers who could act on them (admins of either
// side). Pending links are invisible to the public (docs/adr/032).
func eventLinksForViewer(db *database.DB, user *model.User, eventID, ownerNodeID string) []model.EventLink {
	rows, err := db.Query(
		`SELECT l.id, l.event_id, l.node_id, l.status, l.initiated_by, l.requested_by, l.created_at, n.name, n.slug
		 FROM event_links l JOIN nodes n ON l.node_id = n.id
		 WHERE l.event_id = ? AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL
		 ORDER BY l.created_at`,
		eventID,
	)
	if err != nil {
		return []model.EventLink{}
	}
	defer rows.Close()

	links := []model.EventLink{}
	ownerSide := userSpeaksForNode(db, user, ownerNodeID)
	for rows.Next() {
		var l model.EventLink
		if err := rows.Scan(&l.ID, &l.EventID, &l.NodeID, &l.Status, &l.InitiatedBy, &l.RequestedBy, &l.CreatedAt, &l.NodeName, &l.NodeSlug); err != nil {
			continue
		}
		if l.Status != "confirmed" && !ownerSide && !userSpeaksForNode(db, user, l.NodeID) {
			continue
		}
		if l.Status == "confirmed" {
			// requested_by is handshake bookkeeping, not public data.
			l.RequestedBy = ""
		}
		links = append(links, l)
	}
	return links
}

// eventMentions loads an event's cross-quilt mentions (public display).
func eventMentions(db *database.DB, eventID string) []model.EventMention {
	rows, err := db.Query(
		`SELECT id, event_id, host, slug, name FROM event_mentions WHERE event_id = ? ORDER BY created_at`,
		eventID,
	)
	if err != nil {
		return []model.EventMention{}
	}
	defer rows.Close()
	mentions := []model.EventMention{}
	for rows.Next() {
		var m model.EventMention
		if err := rows.Scan(&m.ID, &m.EventID, &m.Host, &m.Slug, &m.Name); err != nil {
			continue
		}
		mentions = append(mentions, m)
	}
	return mentions
}
