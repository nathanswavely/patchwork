package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// WebFinger handles GET /.well-known/webfinger.
// Implements RFC 7033 for ActivityPub actor discovery.
// resource param: acct:{username}@{domain}
func WebFinger(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resource := r.URL.Query().Get("resource")
		if resource == "" {
			http.Error(w, `{"error":"resource parameter required"}`, http.StatusBadRequest)
			return
		}

		// Parse acct: URI.
		if !strings.HasPrefix(resource, "acct:") {
			http.Error(w, `{"error":"resource must be an acct: URI"}`, http.StatusBadRequest)
			return
		}

		acct := strings.TrimPrefix(resource, "acct:")
		parts := strings.SplitN(acct, "@", 2)
		if len(parts) != 2 {
			http.Error(w, `{"error":"invalid acct URI format"}`, http.StatusBadRequest)
			return
		}

		username := parts[0]
		reqDomain := parts[1]

		domain := ap.GetDomain()
		if reqDomain != domain {
			http.Error(w, `{"error":"unknown domain"}`, http.StatusNotFound)
			return
		}

		// Look up user by username first, then node by slug.
		// Both map to preferredUsername on their AP actors; users win on collision.
		var actorURL string
		var userID string
		err := db.QueryRow(
			"SELECT id FROM users WHERE username = ? AND suspended_at IS NULL", username,
		).Scan(&userID)
		if err == nil {
			actorURL = fmt.Sprintf("https://%s/ap/users/%s", domain, userID)
		} else {
			var nodeID string
			err = db.QueryRow(
				"SELECT id FROM nodes WHERE slug = ? AND status IN ('active','unclaimed') AND removed_at IS NULL", username,
			).Scan(&nodeID)
			if err != nil {
				http.Error(w, `{"error":"actor not found"}`, http.StatusNotFound)
				return
			}
			actorURL = fmt.Sprintf("https://%s/ap/nodes/%s", domain, nodeID)
		}

		jrd := map[string]interface{}{
			"subject": resource,
			"links": []map[string]string{
				{
					"rel":  "self",
					"type": "application/activity+json",
					"href": actorURL,
				},
			},
		}

		w.Header().Set("Content-Type", "application/jrd+json")
		json.NewEncoder(w).Encode(jrd)
	}
}
