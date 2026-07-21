package handler

import (
	"encoding/json"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// APPreview handles GET /api/v1/nodes/{slug}/ap-preview.
// Admin-only endpoint that shows what a node would look like as an AP actor.
func APPreview(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		if slug == "" {
			http.Error(w, `{"error":"slug required"}`, http.StatusBadRequest)
			return
		}

		var n model.Node
		err := db.QueryRow(
			`SELECT id, owner_id, name, slug, description, latitude, longitude, address, website, visibility, membership_policy, created_at, updated_at
			 FROM nodes WHERE slug = ? AND status IN ('active','unclaimed') AND removed_at IS NULL`, slug,
		).Scan(&n.ID, &n.OwnerID, &n.Name, &n.Slug, &n.Description, &n.Latitude, &n.Longitude, &n.Address, &n.Website, &n.Visibility, &n.MembershipPolicy, &n.CreatedAt, &n.UpdatedAt)
		if err != nil {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		domain := cfg.Instance.Domain
		if domain == "" {
			domain = "localhost"
		}

		actor := ap.NodeToActor(n, domain)

		w.Header().Set("Content-Type", "application/activity+json")
		json.NewEncoder(w).Encode(actor)
	}
}
