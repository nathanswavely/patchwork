package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// acceptsActivityPub returns true if the Accept header indicates an AP client.
func acceptsActivityPub(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/activity+json") ||
		strings.Contains(accept, "application/ld+json")
}

// writeAP encodes v as JSON with the AP content type.
func writeAP(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/activity+json")
	json.NewEncoder(w).Encode(v)
}

// publicKeyObject builds the publicKey sub-object for an AP actor.
func publicKeyObject(actorID, publicKeyPEM string) map[string]string {
	return map[string]string{
		"id":           actorID + "#main-key",
		"owner":        actorID,
		"publicKeyPem": publicKeyPEM,
	}
}

// APUser serves GET /ap/users/{id}.
// Returns a Person actor with publicKey, inbox, outbox, followers, following.
func APUser(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.PathValue("id")
		if userID == "" {
			http.Error(w, `{"error":"user id required"}`, http.StatusBadRequest)
			return
		}

		if !acceptsActivityPub(r) {
			domain := ap.GetDomain()
			// Redirect to web UI.
			var username string
			err := db.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username)
			if err != nil {
				http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
				return
			}
			http.Redirect(w, r, fmt.Sprintf("https://%s/users/%s", domain, username), http.StatusSeeOther)
			return
		}

		var u model.User
		var publicKey sql.NullString
		err := db.QueryRow(
			`SELECT id, username, display_name, bio, avatar_url, role, created_at, updated_at, public_key
			 FROM users WHERE id = ? AND suspended_at IS NULL`, userID,
		).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Bio, &u.AvatarURL, &u.Role, &u.CreatedAt, &u.UpdatedAt, &publicKey)
		if err != nil {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		domain := ap.GetDomain()
		actor := ap.UserToActor(u, domain)

		// Build response map to include publicKey.
		resp := map[string]interface{}{
			"@context":          actor.Context,
			"type":              actor.Type,
			"id":                actor.ID,
			"name":              actor.Name,
			"preferredUsername": actor.PreferredUsername,
			"summary":           actor.Summary,
			"url":               actor.URL,
			"inbox":             actor.Inbox,
			"outbox":            actor.Outbox,
			"followers":         actor.Followers,
			"following":         actor.Following,
		}
		if publicKey.Valid && publicKey.String != "" {
			resp["publicKey"] = publicKeyObject(actor.ID, publicKey.String)
		}

		writeAP(w, resp)
	}
}

// APNode serves GET /ap/nodes/{id}.
// Returns an Organization actor with publicKey, inbox, outbox, followers.
func APNode(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.PathValue("id")
		if nodeID == "" {
			http.Error(w, `{"error":"node id required"}`, http.StatusBadRequest)
			return
		}

		if !acceptsActivityPub(r) {
			domain := ap.GetDomain()
			var slug string
			err := db.QueryRow("SELECT slug FROM nodes WHERE id = ? AND status IN ('active','unclaimed') AND removed_at IS NULL", nodeID).Scan(&slug)
			if err != nil {
				http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
				return
			}
			http.Redirect(w, r, fmt.Sprintf("https://%s/patches/%s", domain, slug), http.StatusSeeOther)
			return
		}

		var n model.Node
		var publicKey sql.NullString
		err := db.QueryRow(
			`SELECT id, owner_id, name, slug, description, latitude, longitude, address, website, visibility, membership_policy, created_at, updated_at, public_key
			 FROM nodes WHERE id = ? AND status IN ('active','unclaimed') AND removed_at IS NULL`, nodeID,
		).Scan(&n.ID, &n.OwnerID, &n.Name, &n.Slug, &n.Description, &n.Latitude, &n.Longitude, &n.Address, &n.Website, &n.Visibility, &n.MembershipPolicy, &n.CreatedAt, &n.UpdatedAt, &publicKey)
		if err != nil {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		domain := ap.GetDomain()
		actor := ap.NodeToActor(n, domain)

		resp := map[string]interface{}{
			"@context":          actor.Context,
			"type":              actor.Type,
			"id":                actor.ID,
			"name":              actor.Name,
			"preferredUsername": actor.PreferredUsername,
			"summary":           actor.Summary,
			"url":               actor.URL,
			"inbox":             actor.Inbox,
			"outbox":            actor.Outbox,
			"followers":         actor.Followers,
			"following":         actor.Following,
		}
		if publicKey.Valid && publicKey.String != "" {
			resp["publicKey"] = publicKeyObject(actor.ID, publicKey.String)
		}

		writeAP(w, resp)
	}
}

// APEvent serves GET /ap/events/{id}.
// Returns an Event object.
func APEvent(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		eventID := r.PathValue("id")
		if eventID == "" {
			http.Error(w, `{"error":"event id required"}`, http.StatusBadRequest)
			return
		}

		if !acceptsActivityPub(r) {
			domain := ap.GetDomain()
			http.Redirect(w, r, fmt.Sprintf("https://%s/events/%s", domain, eventID), http.StatusSeeOther)
			return
		}

		var e model.Event
		err := db.QueryRow(
			`SELECT id, node_id, created_by, title, description, location, latitude, longitude, starts_at, ends_at, recurrence, visibility, created_at, updated_at
			 FROM events WHERE id = ? AND removed_at IS NULL AND status = 'active'`, eventID,
		).Scan(&e.ID, &e.NodeID, &e.CreatedBy, &e.Title, &e.Description, &e.Location, &e.Latitude, &e.Longitude, &e.StartsAt, &e.EndsAt, &e.Recurrence, &e.Visibility, &e.CreatedAt, &e.UpdatedAt)
		if err != nil {
			http.Error(w, `{"error":"event not found"}`, http.StatusNotFound)
			return
		}

		domain := ap.GetDomain()
		obj := ap.EventToObject(e, domain)

		writeAP(w, obj)
	}
}

// APProposal serves GET /ap/proposals/{id}.
// Returns a gv:Proposal object.
func APProposal(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proposalID := r.PathValue("id")
		if proposalID == "" {
			http.Error(w, `{"error":"proposal id required"}`, http.StatusBadRequest)
			return
		}

		if !acceptsActivityPub(r) {
			domain := ap.GetDomain()
			http.Redirect(w, r, fmt.Sprintf("https://%s/proposals/%s", domain, proposalID), http.StatusSeeOther)
			return
		}

		var p model.Proposal
		var apID sql.NullString
		err := db.QueryRow(
			`SELECT id, node_id, author_id, title, body, status, proposal_type, duration_hours, voting_ends_at, created_at, updated_at, ap_id
			 FROM proposals WHERE id = ?`, proposalID,
		).Scan(&p.ID, &p.NodeID, &p.AuthorID, &p.Title, &p.Body, &p.Status, &p.ProposalType, &p.DurationHours, &p.VotingEndsAt, &p.CreatedAt, &p.UpdatedAt, &apID)
		if err != nil {
			http.Error(w, `{"error":"proposal not found"}`, http.StatusNotFound)
			return
		}

		domain := ap.GetDomain()
		proposalAPID := ap.ProposalAPID(domain, p.ID)
		if apID.Valid && apID.String != "" {
			proposalAPID = apID.String
		}

		resp := map[string]interface{}{
			"@context": []interface{}{
				"https://www.w3.org/ns/activitystreams",
				map[string]string{"gv": "https://" + domain + "/ns/governance#"},
			},
			"type":          "gv:Proposal",
			"id":            proposalAPID,
			"name":          p.Title,
			"content":       p.Body,
			"attributedTo":  ap.UserAPID(domain, p.AuthorID),
			"context":       ap.NodeAPID(domain, p.NodeID),
			"status":        p.Status,
			"proposalType":  p.ProposalType,
			"durationHours": p.DurationHours,
			"published":     p.CreatedAt,
			"updated":       p.UpdatedAt,
		}
		if p.VotingEndsAt != nil {
			resp["votingEndsAt"] = *p.VotingEndsAt
		}

		writeAP(w, resp)
	}
}

// APGovernanceDoc serves GET /ap/governance/{id}.
// Returns a gv:GovernanceDocument object.
func APGovernanceDoc(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID := r.PathValue("id")
		if docID == "" {
			http.Error(w, `{"error":"governance doc id required"}`, http.StatusBadRequest)
			return
		}

		if !acceptsActivityPub(r) {
			domain := ap.GetDomain()
			http.Redirect(w, r, fmt.Sprintf("https://%s/governance/%s", domain, docID), http.StatusSeeOther)
			return
		}

		var doc model.GovernanceDoc
		err := db.QueryRow(
			`SELECT id, node_id, title, body, version, created_by, created_at, updated_at
			 FROM governance_docs WHERE id = ?`, docID,
		).Scan(&doc.ID, &doc.NodeID, &doc.Title, &doc.Body, &doc.Version, &doc.CreatedBy, &doc.CreatedAt, &doc.UpdatedAt)
		if err != nil {
			http.Error(w, `{"error":"governance doc not found"}`, http.StatusNotFound)
			return
		}

		domain := ap.GetDomain()

		resp := map[string]interface{}{
			"@context": []interface{}{
				"https://www.w3.org/ns/activitystreams",
				map[string]string{"gv": "https://" + domain + "/ns/governance#"},
			},
			"type":         "gv:GovernanceDocument",
			"id":           fmt.Sprintf("https://%s/ap/governance/%s", domain, doc.ID),
			"name":         doc.Title,
			"content":      doc.Body,
			"version":      doc.Version,
			"attributedTo": ap.UserAPID(domain, doc.CreatedBy),
			"context":      ap.NodeAPID(domain, doc.NodeID),
			"published":    doc.CreatedAt,
			"updated":      doc.UpdatedAt,
		}

		writeAP(w, resp)
	}
}

// APNodeOutbox serves GET /ap/nodes/{id}/outbox.
// Returns an OrderedCollection of recent activities for this node.
func APNodeOutbox(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.PathValue("id")
		if nodeID == "" {
			http.Error(w, `{"error":"node id required"}`, http.StatusBadRequest)
			return
		}

		// Verify node exists.
		var exists int
		if err := db.QueryRow("SELECT 1 FROM nodes WHERE id = ? AND status IN ('active','unclaimed') AND removed_at IS NULL", nodeID).Scan(&exists); err != nil {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		domain := ap.GetDomain()
		baseURL := fmt.Sprintf("https://%s", domain)
		outboxID := fmt.Sprintf("%s/ap/nodes/%s/outbox", baseURL, nodeID)

		// Collect recent events as Create activities.
		rows, err := db.Query(
			`SELECT id, title, description, location, latitude, longitude, starts_at, ends_at, recurrence, visibility, created_at, updated_at
			 FROM events WHERE node_id = ? AND removed_at IS NULL AND status = 'active' ORDER BY created_at DESC LIMIT 50`, nodeID,
		)
		if err != nil {
			http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var items []interface{}
		for rows.Next() {
			var e model.Event
			if err := rows.Scan(&e.ID, &e.Title, &e.Description, &e.Location, &e.Latitude, &e.Longitude, &e.StartsAt, &e.EndsAt, &e.Recurrence, &e.Visibility, &e.CreatedAt, &e.UpdatedAt); err != nil {
				continue
			}
			e.NodeID = nodeID
			obj := ap.EventToObject(e, domain)
			activity := ap.Activity{
				Context: ap.Context,
				Type:    "Create",
				ID:      fmt.Sprintf("%s/ap/nodes/%s/outbox/create-event-%s", baseURL, nodeID, e.ID),
				Actor:   fmt.Sprintf("%s/ap/nodes/%s", baseURL, nodeID),
				Object:  obj,
			}
			items = append(items, activity)
		}

		collection := ap.OrderedCollection{
			Context:      ap.Context,
			Type:         "OrderedCollection",
			ID:           outboxID,
			TotalItems:   len(items),
			OrderedItems: items,
		}

		writeAP(w, collection)
	}
}

// APNodeFollowers serves GET /ap/nodes/{id}/followers.
// Returns a Collection of followers.
func APNodeFollowers(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.PathValue("id")
		if nodeID == "" {
			http.Error(w, `{"error":"node id required"}`, http.StatusBadRequest)
			return
		}

		// Verify node exists.
		var exists int
		if err := db.QueryRow("SELECT 1 FROM nodes WHERE id = ? AND status IN ('active','unclaimed') AND removed_at IS NULL", nodeID).Scan(&exists); err != nil {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		domain := ap.GetDomain()
		baseURL := fmt.Sprintf("https://%s", domain)
		followersID := fmt.Sprintf("%s/ap/nodes/%s/followers", baseURL, nodeID)

		// Count local followers (members with role = 'follower').
		var localCount int
		db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'follower' AND status = 'active'", nodeID).Scan(&localCount)

		// Count remote AP followers.
		var remoteCount int
		db.QueryRow("SELECT COUNT(*) FROM ap_followers WHERE local_actor_type = 'node' AND local_actor_id = ? AND accepted = 1", nodeID).Scan(&remoteCount)

		// Collect remote follower actor IDs.
		var items []interface{}
		rows, err := db.Query("SELECT remote_actor_id FROM ap_followers WHERE local_actor_type = 'node' AND local_actor_id = ? AND accepted = 1", nodeID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var actorID string
				if rows.Scan(&actorID) == nil {
					items = append(items, actorID)
				}
			}
		}

		// Also include local follower AP IDs.
		localRows, err := db.Query(
			`SELECT u.ap_id FROM memberships m JOIN users u ON m.user_id = u.id
			 WHERE m.node_id = ? AND m.role = 'follower' AND m.status = 'active' AND u.ap_id IS NOT NULL AND u.ap_id != ''`, nodeID,
		)
		if err == nil {
			defer localRows.Close()
			for localRows.Next() {
				var apID string
				if localRows.Scan(&apID) == nil {
					items = append(items, apID)
				}
			}
		}

		collection := ap.Collection{
			Context:    ap.Context,
			Type:       "Collection",
			ID:         followersID,
			TotalItems: localCount + remoteCount,
			Items:      items,
		}

		writeAP(w, collection)
	}
}

// APUserOutbox serves GET /ap/users/{id}/outbox.
// Returns an OrderedCollection (currently empty, placeholder for future federation).
func APUserOutbox(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.PathValue("id")
		if userID == "" {
			http.Error(w, `{"error":"user id required"}`, http.StatusBadRequest)
			return
		}

		// Verify user exists.
		var exists int
		if err := db.QueryRow("SELECT 1 FROM users WHERE id = ? AND suspended_at IS NULL", userID).Scan(&exists); err != nil {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		domain := ap.GetDomain()
		outboxID := fmt.Sprintf("https://%s/ap/users/%s/outbox", domain, userID)

		collection := ap.OrderedCollection{
			Context:      ap.Context,
			Type:         "OrderedCollection",
			ID:           outboxID,
			TotalItems:   0,
			OrderedItems: []interface{}{},
		}

		writeAP(w, collection)
	}
}

// APUserFollowers serves GET /ap/users/{id}/followers.
// Returns a Collection of followers (currently minimal).
func APUserFollowers(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.PathValue("id")
		if userID == "" {
			http.Error(w, `{"error":"user id required"}`, http.StatusBadRequest)
			return
		}

		// Verify user exists.
		var exists int
		if err := db.QueryRow("SELECT 1 FROM users WHERE id = ? AND suspended_at IS NULL", userID).Scan(&exists); err != nil {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		domain := ap.GetDomain()
		followersID := fmt.Sprintf("https://%s/ap/users/%s/followers", domain, userID)

		// Count remote AP followers.
		var remoteCount int
		db.QueryRow("SELECT COUNT(*) FROM ap_followers WHERE local_actor_type = 'user' AND local_actor_id = ? AND accepted = 1", userID).Scan(&remoteCount)

		var items []interface{}
		rows, err := db.Query("SELECT remote_actor_id FROM ap_followers WHERE local_actor_type = 'user' AND local_actor_id = ? AND accepted = 1", userID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var actorID string
				if rows.Scan(&actorID) == nil {
					items = append(items, actorID)
				}
			}
		}

		collection := ap.Collection{
			Context:    ap.Context,
			Type:       "Collection",
			ID:         followersID,
			TotalItems: remoteCount,
			Items:      items,
		}

		writeAP(w, collection)
	}
}
