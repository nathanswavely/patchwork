package handler

import (
	"encoding/json"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// GetTemplate handles GET /api/v1/governance/templates/{id}.
// Returns template metadata + full document contents.
func GetTemplate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		templateID := r.PathValue("id")

		// Find the template metadata.
		var info *governance.TemplateInfo
		for _, t := range governance.TemplateList() {
			if t.ID == templateID {
				info = &t
				break
			}
		}
		if info == nil {
			http.Error(w, `{"error":"template not found"}`, http.StatusNotFound)
			return
		}

		// Build document contents from the template defaults.
		type docContent struct {
			Filename string `json:"filename"`
			Content  string `json:"content"`
		}

		// Get the lining (community-standards.md) + template files.
		allFiles := governance.ExportDefaultFiles()
		var docs []docContent

		// Always include the lining first.
		if content, ok := allFiles["community-standards.md"]; ok {
			docs = append(docs, docContent{Filename: "community-standards.md", Content: content})
		}

		// Add template-specific files.
		prefix := "templates/" + templateID + "/"
		for path, content := range allFiles {
			if len(path) > len(prefix) && path[:len(prefix)] == prefix {
				filename := path[len(prefix):]
				if filename != "governance-rules.json" {
					docs = append(docs, docContent{Filename: filename, Content: content})
				}
			}
		}

		// Parse the rules for this template.
		var rules json.RawMessage
		if rulesContent, ok := allFiles[prefix+"governance-rules.json"]; ok {
			rules = json.RawMessage(rulesContent)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"template":  info,
			"documents": docs,
			"rules":     rules,
		})
	}
}

// GovernanceOverview handles GET /api/v1/nodes/{slug}/governance/overview.
// Returns a comprehensive governance status for a patch.
func GovernanceOverview(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// Get governance config from DB cache.
		var gcJSON, membershipPolicy string
		db.QueryRow("SELECT COALESCE(governance_config,'{}'), membership_policy FROM nodes WHERE id = ?", nodeID).Scan(&gcJSON, &membershipPolicy)

		// Get admin list.
		type adminInfo struct {
			UserID      string `json:"user_id"`
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			AvatarURL   string `json:"avatar_url"`
			JoinedAt    string `json:"joined_at"`
		}

		var admins []adminInfo
		rows, err := db.Query(
			`SELECT u.id, u.username, u.display_name, u.avatar_url, m.joined_at
			 FROM memberships m JOIN users u ON m.user_id = u.id
			 WHERE m.node_id = ? AND m.role = 'admin' AND m.status = 'active'
			 ORDER BY m.joined_at ASC`, nodeID,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var a adminInfo
				rows.Scan(&a.UserID, &a.Username, &a.DisplayName, &a.AvatarURL, &a.JoinedAt)
				admins = append(admins, a)
			}
		}
		if admins == nil {
			admins = []adminInfo{}
		}

		// Count documents.
		var docCount int
		db.QueryRow("SELECT COUNT(*) FROM governance_docs WHERE node_id = ?", nodeID).Scan(&docCount)

		// Count proposals by status.
		var openProposals, passedProposals, rejectedProposals int
		db.QueryRow("SELECT COUNT(*) FROM proposals WHERE node_id = ? AND status = 'open'", nodeID).Scan(&openProposals)
		db.QueryRow("SELECT COUNT(*) FROM proposals WHERE node_id = ? AND status = 'approved'", nodeID).Scan(&passedProposals)
		db.QueryRow("SELECT COUNT(*) FROM proposals WHERE node_id = ? AND status = 'rejected'", nodeID).Scan(&rejectedProposals)

		// Count proposals needing current user's vote.
		var needsVote int
		if user := middleware.UserFromContext(r.Context()); user != nil {
			db.QueryRow(
				`SELECT COUNT(*) FROM proposals p
				 WHERE p.node_id = ? AND p.status = 'open'
				 AND NOT EXISTS (SELECT 1 FROM votes v WHERE v.proposal_id = p.id AND v.user_id = ?)`,
				nodeID, user.ID,
			).Scan(&needsVote)
		}

		// Member count.
		var memberCount int
		db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND status = 'active' AND role IN ('admin', 'member')", nodeID).Scan(&memberCount)

		resp := map[string]interface{}{
			"rules":              json.RawMessage(gcJSON),
			"membership_policy":  membershipPolicy,
			"admins":             admins,
			"member_count":       memberCount,
			"document_count":     docCount,
			"open_proposals":     openProposals,
			"passed_proposals":   passedProposals,
			"rejected_proposals": rejectedProposals,
			"needs_vote":         needsVote,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
