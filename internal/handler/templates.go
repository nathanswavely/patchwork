package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
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

// countProposalsAwaitingVote counts the open proposals a person may still cast
// a ballot on: ones they haven't voted on, in whose electorate they stand.
//
// Membership is read once — role and tenure are facts about the person, not
// about any one proposal — but the tenure *requirement* is read per proposal
// from its own voting terms (docs/adr/047), because a patch that raised the
// bar has open votes on both sides of the change. `nodeGCJSON` is the patch's
// live config, used for proposals that carry no terms of their own.
func countProposalsAwaitingVote(db *database.DB, nodeID, userID, nodeGCJSON string) int {
	var role, status, joinedAt string
	err := db.QueryRow(
		"SELECT role, status, joined_at FROM memberships WHERE node_id = ? AND user_id = ?",
		nodeID, userID,
	).Scan(&role, &status, &joinedAt)
	if err != nil || status != "active" || (role != "admin" && role != "member") {
		return 0
	}
	joined, parseErr := time.Parse("2006-01-02T15:04:05.000Z", joinedAt)
	if parseErr != nil {
		// An unreadable joined_at can't be shown to clear any requirement.
		// Only a patch with no tenure rule at all can still count.
		joined = time.Now().UTC()
	}

	rows, err := db.Query(
		`SELECT COALESCE(p.voting_terms,'') FROM proposals p
		 WHERE p.node_id = ? AND p.status = 'open'
		 AND NOT EXISTS (SELECT 1 FROM votes v WHERE v.proposal_id = p.id AND v.user_id = ?)`,
		nodeID, userID,
	)
	if err != nil {
		return 0
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var termsJSON string
		if rows.Scan(&termsJSON) != nil {
			continue
		}
		if termsJSON == "" || termsJSON == "{}" {
			termsJSON = nodeGCJSON
		}
		var gc model.GovernanceConfig
		json.Unmarshal([]byte(termsJSON), &gc)
		if gc.MinVotingTenureDays > 0 {
			if time.Since(joined) < time.Duration(gc.MinVotingTenureDays)*24*time.Hour {
				continue
			}
		}
		count++
	}
	return count
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

		// Count documents the viewer can actually open — a count that includes
		// charters they can't reach reads as a broken link, not a hint.
		docQuery := "SELECT COUNT(*) FROM governance_docs WHERE node_id = ?"
		if !canReadPatchDocs(db, r, nodeID) {
			docQuery += " AND visibility = 'public'"
		}
		var docCount int
		db.QueryRow(docQuery, nodeID).Scan(&docCount)

		// Count proposals by status.
		var openProposals, passedProposals, rejectedProposals int
		db.QueryRow("SELECT COUNT(*) FROM proposals WHERE node_id = ? AND status = 'open'", nodeID).Scan(&openProposals)
		db.QueryRow("SELECT COUNT(*) FROM proposals WHERE node_id = ? AND status = 'approved'", nodeID).Scan(&passedProposals)
		db.QueryRow("SELECT COUNT(*) FROM proposals WHERE node_id = ? AND status = 'rejected'", nodeID).Scan(&rejectedProposals)

		// Count proposals needing current user's vote — but only for someone
		// who may actually cast one. This counted every open proposal the
		// viewer hadn't voted on regardless of role or tenure, so a follower
		// was told "2 proposals need your vote" and VoteOnProposal answered
		// 403: the electorate has to be one set on the nudge too, not just at
		// the gate (docs/adr/044).
		//
		// Each open proposal is judged by the terms it opened with
		// (docs/adr/047), so this is no longer one question about the patch —
		// the viewer can be in one vote's electorate and outside another's,
		// when a tenure requirement changed between them. Ask per proposal.
		var needsVote int
		if user := middleware.UserFromContext(r.Context()); user != nil {
			needsVote = countProposalsAwaitingVote(db, nodeID, user.ID, gcJSON)
		}

		// Member count.
		var memberCount int
		db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND status = 'active' AND role IN ('admin', 'member')", nodeID).Scan(&memberCount)

		// The named successor, on maintainer patches only (docs/adr/051).
		// Read through designatedSuccessor so a person who has since left the
		// patch reads as nobody — the same answer the leave path gives, rather
		// than a name the overview shows and succession would refuse.
		successor := map[string]string{}
		if gcLeadership := leadershipModel(db, nodeID); gcLeadership == "maintainer" {
			if sid := designatedSuccessor(db, nodeID); sid != "" {
				var username, displayName string
				db.QueryRow("SELECT username, COALESCE(display_name,'') FROM users WHERE id = ?", sid).Scan(&username, &displayName)
				successor = map[string]string{"user_id": sid, "username": username, "display_name": displayName}
			}
		}

		resp := map[string]interface{}{
			"rules":              json.RawMessage(gcJSON),
			"membership_policy":  membershipPolicy,
			"admins":             admins,
			"successor":          successor,
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
