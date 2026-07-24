package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// join is a helper to avoid importing strings in this file for a single use.
func join(elems []string, sep string) string {
	result := ""
	for i, e := range elems {
		if i > 0 {
			result += sep
		}
		result += e
	}
	return result
}

// DefaultLiningTitle/Body live in the governance package (docs/adr/011) so
// the canonical governance_docs row and the forked community-standards.md
// are two representations of one document — same title-derived filename,
// same body. These aliases keep existing callers and tests working.
const DefaultLiningTitle = governance.DefaultLiningTitle

// DefaultLiningBody is the body for the auto-created governance doc.
const DefaultLiningBody = governance.DefaultLiningBody

// CreateDefaultLining creates the default governance document for a node.
func CreateDefaultLining(db *database.DB, nodeID, userID string) {
	id := auth.NewUUIDv7()
	db.Exec(
		`INSERT INTO governance_docs (id, node_id, title, body, created_by) VALUES (?, ?, ?, ?, ?)`,
		id, nodeID, DefaultLiningTitle, DefaultLiningBody, userID,
	)
}

// ListProposals handles GET /api/v1/nodes/{slug}/proposals.
func ListProposals(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		after, limit := parsePaginationParams(r)
		status := r.URL.Query().Get("status")

		query := `SELECT p.id, p.node_id, p.author_id, p.title, p.body, p.status, p.proposal_type, p.duration_hours, p.voting_ends_at, p.created_at, p.updated_at,
			COALESCE(p.target_doc,''), COALESCE(p.proposed_branch,''), COALESCE(p.proposed_body,''), COALESCE(p.proposed_title,''), COALESCE(p.git_sha,''),
			COALESCE(u.display_name, u.username) as author_name,
			(SELECT COUNT(*) FROM votes WHERE proposal_id = p.id AND value = 'approve') as approve_count,
			(SELECT COUNT(*) FROM votes WHERE proposal_id = p.id AND value = 'reject') as reject_count,
			(SELECT COUNT(*) FROM votes WHERE proposal_id = p.id AND value = 'abstain') as abstain_count
			FROM proposals p
			LEFT JOIN users u ON u.id = p.author_id
			WHERE p.node_id = ?`
		args := []interface{}{nodeID}

		if status != "" && status != "all" {
			query += " AND p.status = ?"
			args = append(args, status)
		}
		if after != "" {
			query += " AND p.id < ?"
			args = append(args, after)
		}
		query += " ORDER BY p.id DESC LIMIT ?"
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list proposals"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type proposalItem struct {
			model.Proposal
			AuthorName   string `json:"author_name"`
			ApproveCount int    `json:"approve_count"`
			RejectCount  int    `json:"reject_count"`
			AbstainCount int    `json:"abstain_count"`
		}

		var proposals []proposalItem
		for rows.Next() {
			var p proposalItem
			if err := rows.Scan(&p.ID, &p.NodeID, &p.AuthorID, &p.Title, &p.Body, &p.Status, &p.ProposalType, &p.DurationHours, &p.VotingEndsAt, &p.CreatedAt, &p.UpdatedAt, &p.TargetDoc, &p.ProposedBranch, &p.ProposedBody, &p.ProposedTitle, &p.GitSHA, &p.AuthorName, &p.ApproveCount, &p.RejectCount, &p.AbstainCount); err != nil {
				continue
			}
			proposals = append(proposals, p)
		}

		var nextCursor string
		if len(proposals) > limit {
			nextCursor = proposals[limit-1].ID
			proposals = proposals[:limit]
		}
		if proposals == nil {
			proposals = []proposalItem{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":       proposals,
			"next_cursor": nextCursor,
		})
	}
}

// CreateProposal handles POST /api/v1/nodes/{slug}/proposals.
func CreateProposal(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// Require membership.
		if user.Role != "admin" && !userHasMembership(db, user.ID, nodeID) {
			http.Error(w, `{"error":"must be member of node"}`, http.StatusForbidden)
			return
		}

		var req struct {
			Title         string `json:"title"`
			Body          string `json:"body"`
			ProposalType  string `json:"proposal_type"`
			DurationHours int    `json:"duration_hours"`
			TargetDoc     string `json:"target_doc"`
			ProposedBody  string `json:"proposed_body"`
			ProposedTitle string `json:"proposed_title"`
			ChangeSummary string `json:"change_summary"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Title == "" {
			http.Error(w, `{"error":"title is required"}`, http.StatusBadRequest)
			return
		}

		// Defaults.
		if req.ProposalType == "" {
			req.ProposalType = "other"
		}
		validTypes := map[string]bool{"amendment": true, "membership": true, "action": true, "other": true}
		if !validTypes[req.ProposalType] {
			http.Error(w, `{"error":"invalid proposal_type"}`, http.StatusBadRequest)
			return
		}
		// Load governance config for default duration
		if req.DurationHours <= 0 {
			var gcJSON string
			db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", nodeID).Scan(&gcJSON)
			var gc model.GovernanceConfig
			json.Unmarshal([]byte(gcJSON), &gc)
			if gc.DefaultVoteDuration > 0 {
				req.DurationHours = gc.DefaultVoteDuration
			} else {
				req.DurationHours = 72
			}
		}

		id := auth.NewUUIDv7()
		now := time.Now().UTC()
		createdAt := now.Format("2006-01-02T15:04:05.000Z")
		votingEndsAt := now.Add(time.Duration(req.DurationHours) * time.Hour).Format("2006-01-02T15:04:05.000Z")

		// Amendment-specific: create git branch with proposed changes
		var branchName, gitSHA, baseSHA string
		if req.ProposalType == "amendment" && req.TargetDoc != "" {
			// Capture the base document SHA for conflict detection.
			history, _ := governance.GetHistory(governance.GetDataDir(), nodeID, req.TargetDoc)
			if len(history) > 0 {
				baseSHA = history[0].SHA
			}
			branchName = fmt.Sprintf("amendment-%s", id[:8])
			commitMsg := req.ChangeSummary
			if commitMsg == "" {
				commitMsg = fmt.Sprintf("Proposed amendment: %s", req.Title)
			}
			sha, branchErr := governance.CreateBranch(governance.GetDataDir(), nodeID, branchName, req.TargetDoc, req.ProposedBody, user.DisplayName, user.Email, commitMsg)
			if branchErr != nil {
				http.Error(w, fmt.Sprintf(`{"error":"failed to create amendment branch: %s"}`, branchErr.Error()), http.StatusInternalServerError)
				return
			}
			gitSHA = sha
		}

		// Determine initial state based on governance config + user role.
		isNodeAdmin := userHasNodeRole(db, user.ID, nodeID, "admin")
		var gcJSON string
		db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", nodeID).Scan(&gcJSON)
		var gc model.GovernanceConfig
		json.Unmarshal([]byte(gcJSON), &gc)

		// Template-driven ceremony:
		// - "admin" decision method (Minimal): admin proposals auto-apply immediately
		// - Casual (maintainer leadership, majority voting, 0 quorum): admin proposals auto-apply
		// - Collaborative/Formal: all proposals go through voting
		initialState := "voting"
		autoApplyNow := false

		if gc.DecisionMethod == "admin" && isNodeAdmin {
			// Minimal template: admin changes apply immediately.
			initialState = "in_effect"
			autoApplyNow = true
		} else if isNodeAdmin && gc.LeadershipModel == "maintainer" && gc.QuorumPercent == 0 {
			// Casual template with maintainer leadership and no quorum: admin fast-track.
			initialState = "in_effect"
			autoApplyNow = true
		}

		apID := ap.ProposalAPID(ap.GetDomain(), id)
		_, err := db.Exec(
			`INSERT INTO proposals (id, node_id, author_id, title, body, status, proposal_type, duration_hours, voting_ends_at, created_at, updated_at, ap_id, target_doc, proposed_branch, proposed_body, proposed_title, git_sha, base_sha, state) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, nodeID, user.ID, req.Title, req.Body, "open", req.ProposalType, req.DurationHours, votingEndsAt, createdAt, createdAt, apID, req.TargetDoc, branchName, req.ProposedBody, req.ProposedTitle, gitSHA, baseSHA, initialState,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to create proposal"}`, http.StatusInternalServerError)
			return
		}

		// Auto-apply for lightweight templates (admin fast-track).
		if autoApplyNow && req.ProposalType == "amendment" && branchName != "" {
			dataDir := governance.GetDataDir()
			sha, mergeErr := governance.MergeBranch(dataDir, nodeID, branchName, user.DisplayName, user.Email)
			if mergeErr != nil {
				log.Printf("proposal %s: fast-track merge failed: %v", id, mergeErr)
			} else {
				// 'approved' is the terminal success status everywhere else
				// (and the only one the schema CHECK allows — 'passed' was
				// silently rejected, leaving fast-tracked amendments 'open').
				if _, err := db.Exec("UPDATE proposals SET git_sha = ?, status = 'approved', applied_at = ?, applied_by = ? WHERE id = ?",
					sha, createdAt, user.ID, id); err != nil {
					log.Printf("proposal %s: fast-track status update failed: %v", id, err)
				}
				governance.DeleteBranch(dataDir, nodeID, branchName)
				// Same post-merge DB syncs as the other apply paths (docs/adr/011).
				if req.TargetDoc == "governance-rules.json" || req.TargetDoc == "Governance Rules" {
					governance.SyncRulesToDB(db, dataDir, nodeID)
				}
				syncLiningToDB(db, nodeID, req.TargetDoc, req.ProposedTitle, user.ID)
			}
		}

		auth.LogAuditEvent(db, user.ID, "proposal.create", "proposal", id, fmt.Sprintf(`{"state":"%s","auto_applied":%v}`, initialState, autoApplyNow), clientIP(r))

		var p model.Proposal
		db.QueryRow(
			`SELECT id, node_id, author_id, title, body, status, proposal_type, duration_hours, voting_ends_at, created_at, updated_at, COALESCE(target_doc,''), COALESCE(proposed_branch,''), COALESCE(proposed_body,''), COALESCE(proposed_title,''), COALESCE(git_sha,''), COALESCE(state,'voting') FROM proposals WHERE id = ?`, id,
		).Scan(&p.ID, &p.NodeID, &p.AuthorID, &p.Title, &p.Body, &p.Status, &p.ProposalType, &p.DurationHours, &p.VotingEndsAt, &p.CreatedAt, &p.UpdatedAt, &p.TargetDoc, &p.ProposedBranch, &p.ProposedBody, &p.ProposedTitle, &p.GitSHA, &p.State)

		// Broadcast to node followers
		go func() {
			proposalObj := ap.ProposalToObject(p, ap.GetDomain())
			activity := map[string]interface{}{
				"@context": ap.GovernanceContext(),
				"type":     "Create",
				"actor":    ap.NodeAPID(ap.GetDomain(), nodeID),
				"object":   proposalObj,
			}
			ap.BroadcastToFollowers(db, "node", nodeID, activity)
		}()

		// Notify members about the new proposal.
		var nodeName string
		db.QueryRow("SELECT name FROM nodes WHERE id = ?", nodeID).Scan(&nodeName)
		notify(notifications.Event{
			Type:     notifications.ProposalNew,
			NodeID:   nodeID,
			NodeSlug: slug,
			NodeName: nodeName,
			ActorID:  user.ID,
			EntityID: id,
			Title:    "New proposal: " + req.Title,
			Body:     req.Body,
			Link:     "/patches/" + slug + "/governance/" + id,
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(p)
	}
}

// GetProposal handles GET /api/v1/proposals/{id}.
func GetProposal(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proposalID := r.PathValue("id")

		var p model.Proposal
		var authorName string
		err := db.QueryRow(
			`SELECT p.id, p.node_id, p.author_id, p.title, p.body, p.status, p.proposal_type, p.duration_hours, p.voting_ends_at, p.created_at, p.updated_at,
			 COALESCE(p.target_doc,''), COALESCE(p.proposed_branch,''), COALESCE(p.proposed_body,''), COALESCE(p.proposed_title,''), COALESCE(p.git_sha,''),
			 COALESCE(u.display_name, u.username) as author_name
			 FROM proposals p LEFT JOIN users u ON u.id = p.author_id
			 WHERE p.id = ?`, proposalID,
		).Scan(&p.ID, &p.NodeID, &p.AuthorID, &p.Title, &p.Body, &p.Status, &p.ProposalType, &p.DurationHours, &p.VotingEndsAt, &p.CreatedAt, &p.UpdatedAt, &p.TargetDoc, &p.ProposedBranch, &p.ProposedBody, &p.ProposedTitle, &p.GitSHA, &authorName)
		if err != nil {
			http.Error(w, `{"error":"proposal not found"}`, http.StatusNotFound)
			return
		}

		// Vote resolution: if voting_ends_at has passed and status is open, resolve.
		if p.Status == "open" && p.VotingEndsAt != nil {
			endsAt, parseErr := time.Parse("2006-01-02T15:04:05.000Z", *p.VotingEndsAt)
			if parseErr != nil {
				endsAt, parseErr = time.Parse(time.RFC3339, *p.VotingEndsAt)
			}
			if parseErr == nil && time.Now().UTC().After(endsAt) {
				// Tally votes for resolution.
				var approveCount, rejectCount, abstainCount int
				db.QueryRow("SELECT COUNT(*) FROM votes WHERE proposal_id = ? AND value = 'approve'", proposalID).Scan(&approveCount)
				db.QueryRow("SELECT COUNT(*) FROM votes WHERE proposal_id = ? AND value = 'reject'", proposalID).Scan(&rejectCount)
				db.QueryRow("SELECT COUNT(*) FROM votes WHERE proposal_id = ? AND value = 'abstain'", proposalID).Scan(&abstainCount)

				// Load governance config for the node
				var gcJSON string
				db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", p.NodeID).Scan(&gcJSON)
				var gc model.GovernanceConfig
				json.Unmarshal([]byte(gcJSON), &gc)

				// Quorum check
				var activeMemberCount int
				db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND status = 'active' AND role IN ('admin','member')", p.NodeID).Scan(&activeMemberCount)
				totalVotes := approveCount + rejectCount + abstainCount
				quorumMet := gc.QuorumPercent == 0 || (activeMemberCount > 0 && (totalVotes*100/activeMemberCount) >= gc.QuorumPercent)

				if quorumMet {
					// Determine threshold
					threshold := gc.DecisionMethod
					if p.ProposalType == "amendment" && gc.AmendmentThreshold != "" {
						threshold = gc.AmendmentThreshold
					}

					passed := false
					switch threshold {
					case "supermajority":
						passed = approveCount > 0 && float64(approveCount)/float64(approveCount+rejectCount) >= 0.667
					case "consensus":
						passed = rejectCount == 0 && approveCount > 0
					default: // "majority"
						passed = approveCount > rejectCount
					}

					newStatus := "rejected"
					if passed {
						newStatus = "approved"
					}

					// Update status
					db.Exec("UPDATE proposals SET status = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?", newStatus, proposalID)
					p.Status = newStatus

					// Auto-apply amendment if approved and configured
					if newStatus == "approved" && p.ProposalType == "amendment" && p.TargetDoc != "" && gc.AmendmentAutoApply {
						var branch string
						db.QueryRow("SELECT COALESCE(proposed_branch,'') FROM proposals WHERE id = ?", proposalID).Scan(&branch)
						if branch != "" {
							sha, mergeErr := governance.MergeBranch(governance.GetDataDir(), p.NodeID, branch, "Patchwork System", "system@patchwork.local")
							if mergeErr == nil {
								db.Exec("UPDATE proposals SET git_sha = ? WHERE id = ?", sha, proposalID)
								// Same post-merge DB syncs as the manual
								// ApplyProposal path (docs/adr/011): rules
								// to governance config, markdown docs to
								// governance_docs.
								if p.TargetDoc == "governance-rules.json" || p.TargetDoc == "Governance Rules" {
									governance.SyncRulesToDB(db, governance.GetDataDir(), p.NodeID)
								}
								syncLiningToDB(db, p.NodeID, p.TargetDoc, p.ProposedTitle, p.AuthorID)
							}
						}
					}

					auth.LogAuditEvent(db, "", "proposal.resolved", "proposal", proposalID,
						fmt.Sprintf(`{"result":"%s","approve":%d,"reject":%d,"abstain":%d,"quorum_met":true}`, newStatus, approveCount, rejectCount, abstainCount), "")

					// Broadcast resolution
					go func() {
						resolveActivity := ap.ProposalResolvedActivity(
							ap.ProposalAPID(ap.GetDomain(), proposalID),
							ap.NodeAPID(ap.GetDomain(), p.NodeID),
							newStatus, approveCount, rejectCount, abstainCount,
						)
						ap.BroadcastToFollowers(db, "node", p.NodeID, resolveActivity)
					}()
				}
				// If quorum not met, don't resolve — leave as open
			}
		}

		// Tally.
		var approveCount, rejectCount, abstainCount int
		db.QueryRow("SELECT COUNT(*) FROM votes WHERE proposal_id = ? AND value = 'approve'", proposalID).Scan(&approveCount)
		db.QueryRow("SELECT COUNT(*) FROM votes WHERE proposal_id = ? AND value = 'reject'", proposalID).Scan(&rejectCount)
		db.QueryRow("SELECT COUNT(*) FROM votes WHERE proposal_id = ? AND value = 'abstain'", proposalID).Scan(&abstainCount)

		// Voter list.
		type voterInfo struct {
			UserID      string `json:"user_id"`
			DisplayName string `json:"display_name"`
			Username    string `json:"username"`
			Value       string `json:"value"`
		}
		var voters []voterInfo
		rows, err := db.Query(
			`SELECT v.user_id, COALESCE(u.display_name,'') as display_name, u.username, v.value
			 FROM votes v JOIN users u ON u.id = v.user_id
			 WHERE v.proposal_id = ? ORDER BY v.created_at ASC`, proposalID,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var vi voterInfo
				if err := rows.Scan(&vi.UserID, &vi.DisplayName, &vi.Username, &vi.Value); err == nil {
					voters = append(voters, vi)
				}
			}
		}
		if voters == nil {
			voters = []voterInfo{}
		}

		// Check current user's vote if logged in.
		var myVote string
		cookie, _ := r.Cookie(auth.CookieName)
		if cookie != nil {
			if u, _ := auth.ValidateSession(db, cookie.Value); u != nil {
				db.QueryRow("SELECT value FROM votes WHERE proposal_id = ? AND user_id = ?", proposalID, u.ID).Scan(&myVote)
			}
		}

		result := map[string]interface{}{
			"id":             p.ID,
			"node_id":        p.NodeID,
			"author_id":      p.AuthorID,
			"author_name":    authorName,
			"title":          p.Title,
			"body":           p.Body,
			"status":         p.Status,
			"proposal_type":  p.ProposalType,
			"duration_hours": p.DurationHours,
			"voting_ends_at": p.VotingEndsAt,
			"created_at":     p.CreatedAt,
			"updated_at":     p.UpdatedAt,
			"approve_count":  approveCount,
			"reject_count":   rejectCount,
			"abstain_count":  abstainCount,
			"voters":         voters,
			"my_vote":        myVote,
		}

		// Include amendment-specific fields if this is a governance amendment
		if p.TargetDoc != "" {
			result["target_doc"] = p.TargetDoc
			result["proposed_branch"] = p.ProposedBranch
			result["proposed_body"] = p.ProposedBody
			result["proposed_title"] = p.ProposedTitle
			result["git_sha"] = p.GitSHA
			currentContent, _ := governance.GetDocument(governance.GetDataDir(), p.NodeID, p.TargetDoc)
			result["current_doc_content"] = currentContent
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// VoteOnProposal handles POST /api/v1/proposals/{id}/vote.
func VoteOnProposal(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		proposalID := r.PathValue("id")

		// Get proposal's node, status, and voting_ends_at.
		var nodeID, status string
		var votingEndsAt *string
		err := db.QueryRow("SELECT node_id, status, voting_ends_at FROM proposals WHERE id = ?", proposalID).Scan(&nodeID, &status, &votingEndsAt)
		if err != nil {
			http.Error(w, `{"error":"proposal not found"}`, http.StatusNotFound)
			return
		}

		if status != "open" {
			http.Error(w, `{"error":"proposal is not open for voting"}`, http.StatusBadRequest)
			return
		}

		// Check if voting window has expired.
		if votingEndsAt != nil && *votingEndsAt != "" {
			endsAt, parseErr := time.Parse("2006-01-02T15:04:05.000Z", *votingEndsAt)
			if parseErr != nil {
				endsAt, parseErr = time.Parse(time.RFC3339, *votingEndsAt)
			}
			if parseErr == nil && time.Now().UTC().After(endsAt) {
				http.Error(w, `{"error":"voting period has ended"}`, http.StatusBadRequest)
				return
			}
		}

		// Require membership in the proposal's node.
		if user.Role != "admin" && !userHasMembership(db, user.ID, nodeID) {
			http.Error(w, `{"error":"must be member of node to vote"}`, http.StatusForbidden)
			return
		}

		// Tenure check from governance config
		var gcJSON string
		db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", nodeID).Scan(&gcJSON)
		var gc model.GovernanceConfig
		json.Unmarshal([]byte(gcJSON), &gc)

		if gc.MinVotingTenureDays > 0 {
			var joinedAt string
			db.QueryRow("SELECT joined_at FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active'", user.ID, nodeID).Scan(&joinedAt)
			if joinedAt != "" {
				joined, _ := time.Parse("2006-01-02T15:04:05.000Z", joinedAt)
				if time.Since(joined) < time.Duration(gc.MinVotingTenureDays)*24*time.Hour {
					http.Error(w, fmt.Sprintf(`{"error":"must be a member for at least %d days to vote"}`, gc.MinVotingTenureDays), http.StatusForbidden)
					return
				}
			}
		}

		var req struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Value != "approve" && req.Value != "reject" && req.Value != "abstain" {
			http.Error(w, `{"error":"value must be approve, reject, or abstain"}`, http.StatusBadRequest)
			return
		}

		// Upsert vote.
		var existingID string
		err = db.QueryRow("SELECT id FROM votes WHERE proposal_id = ? AND user_id = ?", proposalID, user.ID).Scan(&existingID)
		if err == nil {
			// Update existing vote.
			_, err = db.Exec("UPDATE votes SET value = ?, created_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?", req.Value, existingID)
		} else {
			// Create new vote.
			existingID = auth.NewUUIDv7()
			_, err = db.Exec(
				`INSERT INTO votes (id, proposal_id, user_id, value) VALUES (?, ?, ?, ?)`,
				existingID, proposalID, user.ID, req.Value,
			)
		}
		if err != nil {
			http.Error(w, `{"error":"failed to cast vote"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "proposal.vote", "proposal", proposalID, `{"value":"`+req.Value+`"}`, clientIP(r))

		// Broadcast vote (non-blocking)
		go func() {
			var pAPID string
			db.QueryRow("SELECT COALESCE(ap_id,'') FROM proposals WHERE id = ?", proposalID).Scan(&pAPID)
			if pAPID != "" {
				voteActivity := ap.VoteToActivity(
					model.Vote{Value: req.Value, CreatedAt: time.Now().Format("2006-01-02T15:04:05.000Z")},
					pAPID,
					ap.UserAPID(ap.GetDomain(), user.ID),
				)
				ap.BroadcastToFollowers(db, "node", nodeID, voteActivity)
			}
		}()

		// Notify proposal author about the vote.
		var authorID, proposalTitle, nodeSlug, nodeName string
		db.QueryRow("SELECT author_id, title FROM proposals WHERE id = ?", proposalID).Scan(&authorID, &proposalTitle)
		db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlug, &nodeName)
		notify(notifications.Event{
			Type:     notifications.ProposalVoteReceived,
			NodeID:   nodeID,
			NodeSlug: nodeSlug,
			NodeName: nodeName,
			ActorID:  user.ID,
			TargetID: authorID,
			EntityID: proposalID,
			Title:    "New vote on: " + proposalTitle,
			Body:     user.DisplayName + " voted " + req.Value,
			Link:     "/patches/" + nodeSlug + "/governance/" + proposalID,
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "vote_id": existingID})
	}
}

// WithdrawProposal handles DELETE /api/v1/proposals/{id}.
func WithdrawProposal(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		proposalID := r.PathValue("id")

		var authorID, nodeID, currentStatus string
		err := db.QueryRow("SELECT author_id, node_id, status FROM proposals WHERE id = ?", proposalID).Scan(&authorID, &nodeID, &currentStatus)
		if err != nil {
			http.Error(w, `{"error":"proposal not found"}`, http.StatusNotFound)
			return
		}

		if currentStatus != "open" {
			http.Error(w, `{"error":"can only withdraw open proposals"}`, http.StatusBadRequest)
			return
		}

		isAuthor := user.ID == authorID
		isAdmin := user.Role == "admin" || userHasNodeRole(db, user.ID, nodeID, "admin")

		if !isAuthor && !isAdmin {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		_, err = db.Exec(
			"UPDATE proposals SET status = 'withdrawn', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?",
			proposalID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to withdraw proposal"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "proposal.withdraw", "proposal", proposalID, "{}", clientIP(r))

		var nodeSlug, nodeName, proposalTitle string
		db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlug, &nodeName)
		db.QueryRow("SELECT title FROM proposals WHERE id = ?", proposalID).Scan(&proposalTitle)
		notify(notifications.Event{
			Type:     notifications.ProposalRejected,
			NodeID:   nodeID,
			NodeSlug: nodeSlug,
			NodeName: nodeName,
			ActorID:  user.ID,
			EntityID: proposalID,
			Title:    "Proposal withdrawn: " + proposalTitle,
			Body:     "This proposal was withdrawn by the author.",
			Link:     "/patches/" + nodeSlug + "/governance/" + proposalID,
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "withdrawn"})
	}
}

// UpdateProposal handles PATCH /api/v1/proposals/{id} — kept for backward compat.
func UpdateProposal(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		proposalID := r.PathValue("id")

		var authorID, nodeID, currentStatus string
		err := db.QueryRow("SELECT author_id, node_id, status FROM proposals WHERE id = ?", proposalID).Scan(&authorID, &nodeID, &currentStatus)
		if err != nil {
			http.Error(w, `{"error":"proposal not found"}`, http.StatusNotFound)
			return
		}

		isAuthor := user.ID == authorID
		isAdmin := user.Role == "admin" || userHasNodeRole(db, user.ID, nodeID, "admin")

		if !isAuthor && !isAdmin {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		var req struct {
			Title  *string `json:"title"`
			Body   *string `json:"body"`
			Status *string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Validate status transitions.
		if req.Status != nil {
			newStatus := *req.Status
			switch {
			case currentStatus == "open" && newStatus == "withdrawn" && (isAuthor || isAdmin):
				// OK
			case currentStatus == "open" && (newStatus == "approved" || newStatus == "rejected") && isAdmin:
				// OK
			default:
				http.Error(w, `{"error":"invalid status transition"}`, http.StatusBadRequest)
				return
			}
		}

		// Build update.
		var setClauses []string
		var args []interface{}
		if req.Title != nil {
			setClauses = append(setClauses, "title = ?")
			args = append(args, *req.Title)
		}
		if req.Body != nil {
			setClauses = append(setClauses, "body = ?")
			args = append(args, *req.Body)
		}
		if req.Status != nil {
			setClauses = append(setClauses, "status = ?")
			args = append(args, *req.Status)
		}

		if len(setClauses) == 0 {
			http.Error(w, `{"error":"no valid fields to update"}`, http.StatusBadRequest)
			return
		}

		setClauses = append(setClauses, "updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')")
		args = append(args, proposalID)

		_, err = db.Exec(
			"UPDATE proposals SET "+join(setClauses, ", ")+" WHERE id = ?",
			args...,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to update proposal"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "proposal.update", "proposal", proposalID, "{}", clientIP(r))

		var p model.Proposal
		db.QueryRow(
			`SELECT id, node_id, author_id, title, body, status, proposal_type, duration_hours, voting_ends_at, created_at, updated_at FROM proposals WHERE id = ?`, proposalID,
		).Scan(&p.ID, &p.NodeID, &p.AuthorID, &p.Title, &p.Body, &p.Status, &p.ProposalType, &p.DurationHours, &p.VotingEndsAt, &p.CreatedAt, &p.UpdatedAt)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	}
}

// ApplyProposal handles POST /api/v1/proposals/{id}/apply.
// Admin makes an approved proposal official (for manual-merge templates).
func ApplyProposal(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		proposalID := r.PathValue("id")

		var p model.Proposal
		// COALESCE like every other proposal query here: the amendment
		// columns are NULL on non-amendment proposals, and a bare Scan into
		// string fields turns that NULL into a bogus "proposal not found".
		err := db.QueryRow(
			`SELECT id, node_id, author_id, status, COALESCE(state,'voting'), proposal_type,
			 COALESCE(target_doc,''), COALESCE(proposed_branch,''), COALESCE(proposed_body,''), COALESCE(proposed_title,'')
			 FROM proposals WHERE id = ?`, proposalID,
		).Scan(&p.ID, &p.NodeID, &p.AuthorID, &p.Status, &p.State, &p.ProposalType, &p.TargetDoc, &p.ProposedBranch, &p.ProposedBody, &p.ProposedTitle)
		if err != nil {
			http.Error(w, `{"error":"proposal not found"}`, http.StatusNotFound)
			return
		}

		// Must be admin of the node.
		if !userHasNodeRole(db, user.ID, p.NodeID, "admin") && user.Role != "admin" {
			http.Error(w, `{"error":"only admins can make proposals official"}`, http.StatusForbidden)
			return
		}

		// Must be in a state that allows applying: approved (voted), or voting (admin fast-track approve).
		// 'passed' never existed in the DB (the schema CHECK rejects it), so
		// only the CHECK-legal statuses are considered here.
		validStates := p.State == "approved" || p.State == "voting" || p.Status == "approved" || p.Status == "open"
		if !validStates {
			http.Error(w, `{"error":"proposal cannot be applied in its current state"}`, http.StatusBadRequest)
			return
		}

		// For amendments, merge the git branch.
		if p.ProposalType == "amendment" && p.ProposedBranch != "" {
			dataDir := governance.GetDataDir()
			sha, err := governance.MergeBranch(dataDir, p.NodeID, p.ProposedBranch, user.DisplayName, user.Email)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"failed to apply changes: %s"}`, err.Error()), http.StatusInternalServerError)
				return
			}

			db.Exec("UPDATE proposals SET git_sha = ? WHERE id = ?", sha, proposalID)

			// Sync rules to DB if this was a rules change.
			if p.TargetDoc == "governance-rules.json" || p.TargetDoc == "Governance Rules" {
				governance.SyncRulesToDB(db, dataDir, p.NodeID)
			}

			// Mirror merged markdown docs into governance_docs — the DB is
			// canonical for linings (docs/adr/011); without this the applied
			// amendment never appears in the governance hub.
			syncLiningToDB(db, p.NodeID, p.TargetDoc, p.ProposedTitle, p.AuthorID)

			// Clean up the branch.
			governance.DeleteBranch(dataDir, p.NodeID, p.ProposedBranch)
		}

		// Update state to in_effect.
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		db.Exec(
			"UPDATE proposals SET state = 'in_effect', applied_at = ?, applied_by = ?, updated_at = ? WHERE id = ?",
			now, user.ID, now, proposalID,
		)

		auth.LogAuditEvent(db, user.ID, "proposal.applied", "proposal", proposalID, "{}", clientIP(r))

		// Notify members that the amendment was applied.
		var nodeSlug, nodeName, proposalTitle string
		db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", p.NodeID).Scan(&nodeSlug, &nodeName)
		db.QueryRow("SELECT title FROM proposals WHERE id = ?", proposalID).Scan(&proposalTitle)
		notify(notifications.Event{
			Type:     notifications.ProposalApplied,
			NodeID:   p.NodeID,
			NodeSlug: nodeSlug,
			NodeName: nodeName,
			ActorID:  user.ID,
			EntityID: proposalID,
			Title:    "Change applied: " + proposalTitle,
			Body:     "This proposal is now in effect.",
			Link:     "/patches/" + nodeSlug + "/governance/" + proposalID,
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "state": "in_effect"})
	}
}
