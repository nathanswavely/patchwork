package handler

import (
	"encoding/json"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

type InstanceResponse struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Domain      string            `json:"domain"`
	IconURL     string            `json:"icon_url"`
	Geography   InstanceGeography `json:"geography"`
	Branding    InstanceBranding  `json:"branding"`
	Stats       InstanceStats     `json:"stats"`
	Tags        []string          `json:"tags"`
	Version     string            `json:"version"`
	MultiQuilt  bool              `json:"multi_quilt"`
	Federation  bool              `json:"federation"`
	Modules     map[string]bool   `json:"modules"`
	Submissions bool              `json:"submissions_enabled"`
	// Neighbor quilts are the instance's public statement of adjacency
	// (docs/adr/024) — visible to every visitor, anonymous included.
	NeighborQuilts []NeighborQuiltPublic `json:"neighbor_quilts"`
	// The Label's cross-quilt-safe summary: steward count, monthly total,
	// staleness — never handles (docs/adr/023).
	Label LabelSummary `json:"label"`
}

// NeighborQuiltPublic is the anonymous-visible shape of a neighbor quilt.
type NeighborQuiltPublic struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

type InstanceGeography struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Radius    float64 `json:"radius"`
}

type InstanceBranding struct {
	Color   string `json:"color"`
	LogoURL string `json:"logo_url"`
}

type InstanceStats struct {
	NodeCount   int `json:"node_count"`
	EventCount  int `json:"event_count"`
	MemberCount int `json:"member_count"`
}

// Instance returns a handler that serves full instance metadata.
func Instance(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Public counts describe what a visitor can actually discover: the
		// same set the quilt (tree handler) renders — live public patches
		// and their live public events.
		var stats InstanceStats
		db.QueryRow("SELECT COUNT(*) FROM nodes WHERE status IN ('active','unclaimed') AND removed_at IS NULL AND visibility = 'public'").Scan(&stats.NodeCount)
		db.QueryRow(
			`SELECT COUNT(*) FROM events e JOIN nodes n ON n.id = e.node_id
			 WHERE e.status = 'active' AND e.removed_at IS NULL AND e.visibility = 'public'
			   AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL AND n.visibility = 'public'`,
		).Scan(&stats.EventCount)
		db.QueryRow("SELECT COUNT(*) FROM users").Scan(&stats.MemberCount)

		// Aggregate unique tags across all nodes.
		var tags []string
		tagRows, err := db.Query(`SELECT DISTINCT t.name FROM tags t JOIN node_tags nt ON t.id = nt.tag_id ORDER BY t.name`)
		if err == nil {
			defer tagRows.Close()
			for tagRows.Next() {
				var tag string
				if tagRows.Scan(&tag) == nil {
					tags = append(tags, tag)
				}
			}
		}
		if tags == nil {
			tags = []string{}
		}

		neighbors := []NeighborQuiltPublic{}
		if list, err := loadNeighborQuilts(db); err == nil {
			for _, q := range list {
				neighbors = append(neighbors, NeighborQuiltPublic{URL: q.URL, Name: q.Name})
			}
		}

		resp := InstanceResponse{
			// Name and description honor the admin-UI overrides
			// (docs/adr/014); patchwork.yaml is the bootstrap default.
			Name:        settings.EffectiveName(db, cfg),
			Description: settings.EffectiveDescription(db, cfg),
			Domain:      cfg.Instance.Domain,
			IconURL:     "/api/v1/instance/icon",
			Geography: InstanceGeography{
				Latitude:  cfg.Geographic.Latitude,
				Longitude: cfg.Geographic.Longitude,
				Radius:    cfg.Geographic.Radius,
			},
			Branding: InstanceBranding{
				Color:   cfg.Branding.Color,
				LogoURL: cfg.Branding.LogoURL,
			},
			Stats:      stats,
			Tags:       tags,
			Version:    Version,
			MultiQuilt: cfg.MultiQuilt,
			Federation: cfg.Federation.Enabled,
			// Module toggles are hints for the SPA (which views to offer);
			// the underlying data endpoints stay available.
			Modules: map[string]bool{
				"map":        cfg.Modules.Map,
				"governance": cfg.Modules.Governance,
				"ledger":     cfg.Modules.Ledger,
			},
			Submissions:    cfg.Submissions.Enabled,
			NeighborQuilts: neighbors,
			Label:          labelSummary(db),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
