package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// Interim admin offers (docs/adr/2026-10-19-an-interim-admin-is-asked-first.md).
//
// When inactivity empties a longest-tenure patch, the sweep asks its
// longest-standing present members whether they will step in, and only their
// answer makes an admin. These are the two halves of that answer, given by
// the person asked and nobody else: there is no admin path through either,
// because the answer is theirs, the way an invitation's is (docs/adr/098).

// AcceptInterimOffer handles POST /api/v1/nodes/{slug}/interim-offer/accept.
func AcceptInterimOffer(db *database.DB) http.HandlerFunc {
	return answerInterimOffer(db, true)
}

// DeclineInterimOffer handles POST /api/v1/nodes/{slug}/interim-offer/decline.
func DeclineInterimOffer(db *database.DB) http.HandlerFunc {
	return answerInterimOffer(db, false)
}

func answerInterimOffer(db *database.DB, accept bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID := NodeIDFromSlug(db, r.PathValue("slug"))
		// One 404 for an unknown patch and for a caller nobody asked: the
		// route must not say who was asked to step in.
		if nodeID == "" {
			http.Error(w, `{"error":"no offer to answer"}`, http.StatusNotFound)
			return
		}
		n := pkgNotifier
		if n == nil {
			n = notifications.NewNotifier(db)
		}
		err := notifications.AnswerInterimOffer(n, nodeID, user.ID, accept, clientIP(r))
		switch {
		case errors.Is(err, notifications.ErrNoInterimOffer):
			http.Error(w, `{"error":"no offer to answer"}`, http.StatusNotFound)
			return
		case errors.Is(err, notifications.ErrNoLongerHere):
			http.Error(w, `{"error":"you are no longer a member of this patch, so the offer has been passed on"}`, http.StatusConflict)
			return
		case err != nil:
			http.Error(w, `{"error":"failed to answer the offer"}`, http.StatusInternalServerError)
			return
		}
		status := "declined"
		if accept {
			status = "accepted"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": status})
	}
}

// ListMyInterimOffers handles GET /api/v1/users/me/interim-offers: the open
// questions addressed to the caller, so the patch's Governance page can put
// Accept and Decline where the notification sends them. Only offers they can
// still answer: the patch is active and they are still a member there.
func ListMyInterimOffers(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		type offer struct {
			NodeSlug  string `json:"node_slug"`
			NodeName  string `json:"node_name"`
			OfferedAt string `json:"offered_at"`
		}
		items := []offer{}
		rows, err := db.Query(`
			SELECT n.slug, n.name, o.offered_at
			FROM interim_admin_offers o
			JOIN nodes n ON n.id = o.node_id AND n.status = 'active' AND n.removed_at IS NULL
			JOIN memberships m ON m.node_id = o.node_id AND m.user_id = o.user_id
			                  AND m.status = 'active' AND m.role = 'member'
			WHERE o.user_id = ? AND o.answered_at IS NULL
			ORDER BY o.offered_at`, user.ID)
		if err != nil {
			http.Error(w, `{"error":"failed to list offers"}`, http.StatusInternalServerError)
			return
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var o offer
			if rows.Scan(&o.NodeSlug, &o.NodeName, &o.OfferedAt) == nil {
				items = append(items, o)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": items})
	}
}

// interimOffersOpen reports whether any interim admin question on this patch
// is still waiting on somebody who is still a member here to answer it.
func interimOffersOpen(db *database.DB, nodeID string) bool {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM interim_admin_offers o
	             JOIN memberships m ON m.node_id = o.node_id AND m.user_id = o.user_id
	                              AND m.status = 'active' AND m.role = 'member'
	             WHERE o.node_id = ? AND o.answered_at IS NULL`, nodeID).Scan(&n); err != nil {
		return false
	}
	return n > 0
}
