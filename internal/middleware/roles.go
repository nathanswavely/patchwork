package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// RequireNodeRole returns middleware that checks the authenticated user has one of the
// specified roles for the node identified by {slug} in the URL path.
// Site admins (user.Role == "admin") bypass node role checks.
func RequireNodeRole(db *database.DB, roles ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			if user == nil {
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
				return
			}

			// Site admin bypasses node role checks.
			if user.Role == "admin" {
				next.ServeHTTP(w, r)
				return
			}

			slug := r.PathValue("slug")
			if slug == "" {
				http.Error(w, `{"error":"slug required"}`, http.StatusBadRequest)
				return
			}

			// Resolve slug to node ID.
			var nodeID string
			err := db.QueryRow("SELECT id FROM nodes WHERE slug = ? AND status IN ('active','unclaimed') AND removed_at IS NULL", slug).Scan(&nodeID)
			if err != nil {
				http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
				return
			}

			// Check membership role.
			placeholders := make([]string, len(roles))
			args := make([]interface{}, 0, len(roles)+2)
			args = append(args, user.ID, nodeID)
			for i, role := range roles {
				placeholders[i] = "?"
				args = append(args, role)
			}

			var count int
			db.QueryRow(
				fmt.Sprintf("SELECT COUNT(*) FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active' AND role IN (%s)", strings.Join(placeholders, ",")),
				args...,
			).Scan(&count)

			if count == 0 {
				http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		}
	}
}
