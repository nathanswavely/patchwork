package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
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

// DefaultLiningBody is the body for the auto-created governance doc — the
// head of the shipped lineage (docs/adr/037), so it is a var, not a const.
var DefaultLiningBody = governance.DefaultLiningBody

// CreateDefaultLining creates the lining for a node: kind='lining' is its
// durable identity, and it is born public — the one doc the members-only
// default never applies to (docs/adr/037). Best-effort mirrors to the node's
// governance repo exactly like AutoUpdateLinings' heal path does — until
// this, the create path (including patch setup, docs/adr/039) was the one
// write that never reached git, a live-verified bug.
func CreateDefaultLining(db *database.DB, nodeID, userID string) {
	id := auth.NewUUIDv7()
	db.Exec(
		`INSERT INTO governance_docs (id, node_id, title, body, kind, visibility, created_by) VALUES (?, ?, ?, ?, 'lining', 'public', ?)`,
		id, nodeID, DefaultLiningTitle, DefaultLiningBody, userID,
	)

	if dataDir := governance.GetDataDir(); dataDir != "" {
		var slug string
		if err := db.QueryRow("SELECT slug FROM nodes WHERE id = ?", nodeID).Scan(&slug); err == nil {
			commitMsg := "The lining, v" + strconv.Itoa(governance.CurrentLiningVersion()) + " (shipped with Patchwork)"
			if _, gitErr := governance.DirectEdit(dataDir, nodeID,
				governanceFilename(DefaultLiningTitle), governance.CurrentLiningBody(),
				"Patchwork", "patchwork@"+slug+".local", commitMsg); gitErr != nil {
				log.Printf("lining: git mirror for node %s: %v", nodeID, gitErr)
			}
		}
	}
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
			(SELECT COUNT(*) FROM votes v JOIN memberships m ON m.user_id = v.user_id AND m.node_id = p.node_id
				WHERE v.proposal_id = p.id AND v.value = 'approve' AND ` + countedBallot + `) as approve_count,
			(SELECT COUNT(*) FROM votes v JOIN memberships m ON m.user_id = v.user_id AND m.node_id = p.node_id
				WHERE v.proposal_id = p.id AND v.value = 'reject' AND ` + countedBallot + `) as reject_count,
			(SELECT COUNT(*) FROM votes v JOIN memberships m ON m.user_id = v.user_id AND m.node_id = p.node_id
				WHERE v.proposal_id = p.id AND v.value = 'abstain' AND ` + countedBallot + `) as abstain_count
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

		docHidden := hiddenDocRedactor(db, r, nodeID)

		var proposals []proposalItem
		for rows.Next() {
			var p proposalItem
			if err := rows.Scan(&p.ID, &p.NodeID, &p.AuthorID, &p.Title, &p.Body, &p.Status, &p.ProposalType, &p.DurationHours, &p.VotingEndsAt, &p.CreatedAt, &p.UpdatedAt, &p.TargetDoc, &p.ProposedBranch, &p.ProposedBody, &p.ProposedTitle, &p.GitSHA, &p.AuthorName, &p.ApproveCount, &p.RejectCount, &p.AbstainCount); err != nil {
				continue
			}
			if docHidden(p.TargetDoc) {
				p.ProposedBody, p.ProposedTitle = "", ""
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

		// Raising a proposal is a member act. Not userHasMembership — that
		// counts any active membership row, followers included, so it let a
		// follower author a live proposal on a patch they cannot vote in.
		if user.Role != "admin" && !mayPropose(db, user.ID, nodeID) {
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

		// Ceremony follows the rules in force (docs/adr/041): only the
		// admin-decides decision method lets an admin apply directly — a
		// direct change, born applied. Every voting method votes, admins
		// included; the old maintainer+zero-quorum bypass let admins skip a
		// vote the patch's own charter promised, and was removed.
		initialState := "voting"
		autoApplyNow := false

		if gc.DecisionMethod == "admin" && isNodeAdmin {
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

		// Apply a direct change now (docs/adr/041). Amendments merge their
		// branch first; a merge failure leaves the record open rather than
		// claiming an application that didn't happen.
		if autoApplyNow {
			applied := true
			if req.ProposalType == "amendment" && branchName != "" {
				dataDir := governance.GetDataDir()
				sha, mergeErr := governance.MergeBranch(dataDir, nodeID, branchName, user.DisplayName, user.Email)
				if mergeErr != nil {
					log.Printf("proposal %s: direct-change merge failed: %v", id, mergeErr)
					applied = false
				} else {
					if _, err := db.Exec("UPDATE proposals SET git_sha = ? WHERE id = ?", sha, id); err != nil {
						log.Printf("proposal %s: direct-change sha update failed: %v", id, err)
					}
					governance.DeleteBranch(dataDir, nodeID, branchName)
					// Same post-merge DB syncs as the other apply paths (docs/adr/011).
					if req.TargetDoc == "governance-rules.json" || req.TargetDoc == "Governance Rules" {
						governance.SyncRulesToDB(db, dataDir, nodeID)
					}
					syncLiningToDB(db, nodeID, req.TargetDoc, req.ProposedTitle, user.ID)
				}
			}
			if applied {
				// 'approved' is the terminal success status everywhere else
				// (and the only one the schema CHECK allows — 'passed' was
				// silently rejected, leaving fast-tracked amendments 'open').
				if _, err := db.Exec("UPDATE proposals SET status = 'approved', applied_at = ?, applied_by = ? WHERE id = ?",
					createdAt, user.ID, id); err != nil {
					log.Printf("proposal %s: direct-change status update failed: %v", id, err)
				}
			} else {
				// The INSERT above stamped state 'in_effect'; roll it back so
				// the unapplied record reads as an open proposal, not an
				// applied change.
				db.Exec("UPDATE proposals SET state = 'voting' WHERE id = ?", id)
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
			Link:     weblink.Proposal(slug, id),
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(p)
	}
}

// electorateMembership is the condition a `memberships` row must meet for its
// person to belong to a patch's electorate: an active admin or member.
// CONTEXT.md, "Member count" — following carries no voting rights. `prefix`
// qualifies the columns with a table alias ("m."); pass "" when the query names
// only one table.
//
// The electorate is one set, expressed once (docs/adr/044). It went wrong three
// times because the gate and the denominator each said who could vote in their
// own words; the fix is that they no longer have their own words.
func electorateMembership(prefix string) string {
	return prefix + "status = 'active' AND " + prefix + "role IN ('admin','member')"
}

// electorateFilter is electorateMembership plus the governance config's minimum
// voting tenure — the whole condition for "may vote here, right now" — together
// with the args the tenure term binds.
func electorateFilter(prefix string, gc model.GovernanceConfig) (string, []interface{}) {
	cond := electorateMembership(prefix)
	var args []interface{}
	if gc.MinVotingTenureDays > 0 {
		// Stored timestamps are ISO 8601 with a 'T'; format the cutoff the
		// same way so the string comparison stays chronological.
		cond += " AND " + prefix + "joined_at <= strftime('%Y-%m-%dT%H:%M:%fZ', 'now', ?)"
		args = append(args, fmt.Sprintf("-%d days", gc.MinVotingTenureDays))
	}
	return cond, args
}

// mayPropose reports whether one person may author a proposal on a node.
//
// It is electorateMembership without the tenure clause, and deliberately so:
// the minimum *voting* tenure gates casting a ballot, not raising the question.
// Everything else about the two is the same set, so it is read off the same
// condition rather than spelled out again (docs/adr/044) — the frontend gates
// say `isAdmin || membershipRole === 'member' || membershipRole === 'admin'`,
// and this is that sentence in SQL.
func mayPropose(db *database.DB, userID, nodeID string) bool {
	var one int
	return db.QueryRow(
		`SELECT 1 FROM memberships WHERE node_id = ? AND user_id = ? AND `+electorateMembership(""),
		nodeID, userID,
	).Scan(&one) == nil
}

// inElectorate reports whether one person may currently vote on a node's
// proposals. Every surface that asks "may this person vote?" — the vote gate,
// and the governance hub's "needs your vote" count, which must not nudge
// someone the gate will refuse — asks it here.
func inElectorate(db *database.DB, userID, nodeID string, gc model.GovernanceConfig) bool {
	cond, args := electorateFilter("", gc)
	all := append([]interface{}{nodeID, userID}, args...)
	var one int
	return db.QueryRow(
		`SELECT 1 FROM memberships WHERE node_id = ? AND user_id = ? AND `+cond, all...,
	).Scan(&one) == nil
}

// electorateDenial explains why a person may not vote, or returns "" when they
// may. The two answers worth telling apart — not one of us, not here long
// enough — are both read off the one electorate condition rather than from a
// second implementation of it.
func electorateDenial(db *database.DB, userID, nodeID string, gc model.GovernanceConfig) string {
	if inElectorate(db, userID, nodeID, gc) {
		return ""
	}
	if gc.MinVotingTenureDays > 0 && inElectorate(db, userID, nodeID, model.GovernanceConfig{}) {
		return fmt.Sprintf("must be a member for at least %d days to vote", gc.MinVotingTenureDays)
	}
	return "must be member of node to vote"
}

// countedBallot separates a vote that counts from a row that merely exists.
// It expects the ballot's membership joined as `m`.
//
// The electorate is active admins and members (see electorateMembership), but
// membership moves underneath a vote: a member can vote and then be demoted to
// follower, leave — LeaveNode sets status 'left' rather than deleting the row —
// or be banned. Nothing purges their votes, and deliberately so: the record of
// who voted is worth keeping. So the tally asks who counts *now* instead of
// trusting that every membership path remembered to clean up after itself.
// Every surface that counts ballots shares this predicate, so the resolution
// math and the tally people read can't diverge.
var countedBallot = electorateMembership("m.")

// tallyProposal counts a proposal's approve/reject/abstain ballots, excluding
// any cast by people the electorate no longer counts (see countedBallot).
func tallyProposal(db *database.DB, proposalID string) (approve, reject, abstain int) {
	rows, err := db.Query(
		`SELECT v.value, COUNT(*)
		 FROM votes v
		 JOIN proposals p ON p.id = v.proposal_id
		 JOIN memberships m ON m.user_id = v.user_id AND m.node_id = p.node_id
		 WHERE v.proposal_id = ? AND `+countedBallot+`
		 GROUP BY v.value`, proposalID)
	if err != nil {
		return 0, 0, 0
	}
	defer rows.Close()
	for rows.Next() {
		var value string
		var n int
		if rows.Scan(&value, &n) != nil {
			continue
		}
		switch value {
		case "approve":
			approve = n
		case "reject":
			reject = n
		case "abstain":
			abstain = n
		}
	}
	return approve, reject, abstain
}

// eligibleVoters returns how many people may currently vote on a node's
// proposals — active admins and members past the minimum voting tenure —
// and, when the electorate is exactly one person, that voter's user ID.
func eligibleVoters(db *database.DB, nodeID string, gc model.GovernanceConfig) (int, string) {
	cond, tenureArgs := electorateFilter("", gc)
	args := append([]interface{}{nodeID}, tenureArgs...)
	rows, err := db.Query(`SELECT user_id FROM memberships WHERE node_id = ? AND `+cond, args...)
	if err != nil {
		return 0, ""
	}
	defer rows.Close()
	count := 0
	sole := ""
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			count++
			sole = id
		}
	}
	if count != 1 {
		sole = ""
	}
	return count, sole
}

// resolveProposal tallies an open proposal and finalizes it: status update,
// amendment auto-apply, audit event, AP broadcast. Returns the new status
// ("approved" or "rejected"), or "" when the proposal stayed open (not open
// to begin with, or quorum not met). Callers decide when resolution is due —
// the voting window expiring, or the sole-voter early close (docs/adr/041).
func resolveProposal(db *database.DB, proposalID string) string {
	var p model.Proposal
	err := db.QueryRow(
		`SELECT id, node_id, author_id, status, proposal_type, COALESCE(target_doc,''), COALESCE(proposed_title,'')
		 FROM proposals WHERE id = ?`, proposalID,
	).Scan(&p.ID, &p.NodeID, &p.AuthorID, &p.Status, &p.ProposalType, &p.TargetDoc, &p.ProposedTitle)
	if err != nil || p.Status != "open" {
		return ""
	}

	// Tally votes for resolution.
	approveCount, rejectCount, abstainCount := tallyProposal(db, proposalID)

	// Load governance config for the node
	var gcJSON string
	db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", p.NodeID).Scan(&gcJSON)
	var gc model.GovernanceConfig
	json.Unmarshal([]byte(gcJSON), &gc)

	// Quorum check. The denominator is the electorate — the same set the vote
	// gate admits and the proposal page displays (docs/adr/044).
	//
	// This once counted active admins and members inline, ignoring
	// min_voting_tenure_days, so the arithmetic divided by people who could
	// not cast a ballot. On the Formal defaults (quorum 50%, tenure 30 days)
	// a patch where more than half the members joined inside the tenure
	// window could not reach quorum at all: every proposal sat open past its
	// window and never resolved, silently. ADR 044 called eligibleVoters "the
	// denominator every quorum calculation divides by"; this is the quorum
	// calculation, and it wasn't.
	eligibleCount, _ := eligibleVoters(db, p.NodeID, gc)
	totalVotes := approveCount + rejectCount + abstainCount
	quorumMet := gc.QuorumPercent == 0 || (eligibleCount > 0 && (totalVotes*100/eligibleCount) >= gc.QuorumPercent)
	if !quorumMet {
		// Quorum not met — leave open.
		return ""
	}

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

	// Update status and state together. `state` is what the proposal page
	// renders from (migration 016) — moving only `status` left a resolved
	// proposal still showing an open vote. 'approved' is a resting state:
	// the community decided, an admin still makes it official, which is the
	// approved → in_effect step the state machine describes.
	db.Exec("UPDATE proposals SET status = ?, state = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?", newStatus, newStatus, proposalID)

	// Auto-apply amendment if approved and configured
	if newStatus == "approved" && p.ProposalType == "amendment" && p.TargetDoc != "" && gc.AmendmentAutoApply {
		var branch string
		db.QueryRow("SELECT COALESCE(proposed_branch,'') FROM proposals WHERE id = ?", proposalID).Scan(&branch)
		if branch != "" {
			sha, mergeErr := governance.MergeBranch(governance.GetDataDir(), p.NodeID, branch, "Patchwork System", "system@patchwork.local")
			if mergeErr == nil {
				db.Exec("UPDATE proposals SET git_sha = ? WHERE id = ?", sha, proposalID)
				// Same post-merge DB syncs as the manual ApplyProposal path
				// (docs/adr/011): rules to governance config, markdown docs
				// to governance_docs.
				if p.TargetDoc == "governance-rules.json" || p.TargetDoc == "Governance Rules" {
					governance.SyncRulesToDB(db, governance.GetDataDir(), p.NodeID)
				}
				syncLiningToDB(db, p.NodeID, p.TargetDoc, p.ProposedTitle, p.AuthorID)
				// The merge already happened, so there is nothing left for an
				// admin to make official — skip 'approved' and land where the
				// manual apply path lands. applied_by stays NULL: no person
				// applied this one.
				now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
				db.Exec("UPDATE proposals SET state = 'in_effect', applied_at = ?, updated_at = ? WHERE id = ?", now, now, proposalID)
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

	return newStatus
}

// GetProposal handles GET /api/v1/proposals/{id}.
func GetProposal(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proposalID := r.PathValue("id")

		var p model.Proposal
		var authorName, appliedAt string
		err := db.QueryRow(
			`SELECT p.id, p.node_id, p.author_id, p.title, p.body, p.status, COALESCE(p.state,''), COALESCE(p.applied_at,''), p.proposal_type, p.duration_hours, p.voting_ends_at, p.created_at, p.updated_at,
			 COALESCE(p.target_doc,''), COALESCE(p.proposed_branch,''), COALESCE(p.proposed_body,''), COALESCE(p.proposed_title,''), COALESCE(p.git_sha,''),
			 COALESCE(u.display_name, u.username) as author_name
			 FROM proposals p LEFT JOIN users u ON u.id = p.author_id
			 WHERE p.id = ?`, proposalID,
		).Scan(&p.ID, &p.NodeID, &p.AuthorID, &p.Title, &p.Body, &p.Status, &p.State, &appliedAt, &p.ProposalType, &p.DurationHours, &p.VotingEndsAt, &p.CreatedAt, &p.UpdatedAt, &p.TargetDoc, &p.ProposedBranch, &p.ProposedBody, &p.ProposedTitle, &p.GitSHA, &authorName)
		if err != nil {
			http.Error(w, `{"error":"proposal not found"}`, http.StatusNotFound)
			return
		}

		// The mirrored charter text follows that charter's visibility, even
		// when the proposal quoting it is public (docs/adr/036).
		docTextHidden := hiddenDocRedactor(db, r, p.NodeID)(p.TargetDoc)
		if docTextHidden {
			p.ProposedBody, p.ProposedTitle = "", ""
		}

		// Vote resolution: if voting_ends_at has passed and status is open, resolve.
		if p.Status == "open" && p.VotingEndsAt != nil {
			endsAt, parseErr := time.Parse("2006-01-02T15:04:05.000Z", *p.VotingEndsAt)
			if parseErr != nil {
				endsAt, parseErr = time.Parse(time.RFC3339, *p.VotingEndsAt)
			}
			if parseErr == nil && time.Now().UTC().After(endsAt) {
				if newStatus := resolveProposal(db, proposalID); newStatus != "" {
					// Re-read rather than patching Status alone: resolution
					// moves state too, and an auto-applied amendment also
					// stamps applied_at.
					db.QueryRow(
						`SELECT status, COALESCE(state,''), COALESCE(applied_at,'') FROM proposals WHERE id = ?`, proposalID,
					).Scan(&p.Status, &p.State, &appliedAt)
				}
			}
		}

		// Tally.
		approveCount, rejectCount, abstainCount := tallyProposal(db, proposalID)

		// Voter list — the whole record, with each ballot saying whether it
		// still counts. Deliberately not filtered like the tally.
		//
		// A vote is a fact; whether it counts is a separate question asked
		// fresh at read time (docs/adr/044). Dropping the uncounted ones from
		// this list answered the second question by erasing the first, and it
		// broke a third thing: the UI reads an empty voter list as "no vote
		// ever happened" to recognise a direct change (docs/adr/041). That
		// inference is only sound while the list is complete. Filtered, a
		// proposal that was genuinely voted on and passed would — once its
		// voters had left or been demoted — render as one that was applied
		// without a vote. A governance record must not describe a vote that
		// happened as a vote that did not.
		//
		// LEFT JOIN, so a voter with no membership row at all (an instance
		// admin from before the vote gate closed) still appears, uncounted:
		// countedBallot is NULL for them, and the CASE falls through to 0.
		type voterInfo struct {
			UserID      string `json:"user_id"`
			DisplayName string `json:"display_name"`
			Username    string `json:"username"`
			Value       string `json:"value"`
			Counted     bool   `json:"counted"`
		}
		var voters []voterInfo
		rows, err := db.Query(
			`SELECT v.user_id, COALESCE(u.display_name,'') as display_name, u.username, v.value,
			        CASE WHEN `+countedBallot+` THEN 1 ELSE 0 END as counted
			 FROM votes v
			 JOIN users u ON u.id = v.user_id
			 JOIN proposals p ON p.id = v.proposal_id
			 LEFT JOIN memberships m ON m.user_id = v.user_id AND m.node_id = p.node_id
			 WHERE v.proposal_id = ?
			 ORDER BY v.created_at ASC`, proposalID,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var vi voterInfo
				if err := rows.Scan(&vi.UserID, &vi.DisplayName, &vi.Username, &vi.Value, &vi.Counted); err == nil {
					voters = append(voters, vi)
				}
			}
		}
		if voters == nil {
			voters = []voterInfo{}
		}

		// Check current user's vote if logged in.
		var myVote, viewerID string
		cookie, _ := r.Cookie(auth.CookieName)
		if cookie != nil {
			if u, _ := auth.ValidateSession(db, cookie.Value); u != nil {
				viewerID = u.ID
				db.QueryRow("SELECT value FROM votes WHERE proposal_id = ? AND user_id = ?", proposalID, u.ID).Scan(&myVote)
			}
		}

		// Electorate size — drives the sole-voter notice in the UI
		// (docs/adr/041).
		var gcJSON string
		db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", p.NodeID).Scan(&gcJSON)
		var gc model.GovernanceConfig
		json.Unmarshal([]byte(gcJSON), &gc)
		eligibleCount, _ := eligibleVoters(db, p.NodeID, gc)

		// Whether this viewer is in the electorate, answered by the same
		// condition VoteOnProposal gates on. The page used to work this out in
		// JavaScript from membership_role alone, which knows nothing about
		// min_voting_tenure_days — so a member inside the tenure window was
		// shown vote buttons and got a 403 on click. Deciding who may vote is
		// the server's answer to give (docs/adr/044); the client still decides
		// whether the proposal is in a state that accepts one.
		canVote := viewerID != "" && inElectorate(db, viewerID, p.NodeID, gc)

		result := map[string]interface{}{
			"id":              p.ID,
			"node_id":         p.NodeID,
			"author_id":       p.AuthorID,
			"author_name":     authorName,
			"title":           p.Title,
			"body":            p.Body,
			"status":          p.Status,
			"proposal_type":   p.ProposalType,
			"duration_hours":  p.DurationHours,
			"voting_ends_at":  p.VotingEndsAt,
			"created_at":      p.CreatedAt,
			"updated_at":      p.UpdatedAt,
			"approve_count":   approveCount,
			"reject_count":    rejectCount,
			"abstain_count":   abstainCount,
			"voters":          voters,
			"my_vote":         myVote,
			"eligible_voters": eligibleCount,
			"can_vote":        canVote,
			"state":           p.State,
			"applied_at":      appliedAt,
		}

		// Include amendment-specific fields if this is a governance amendment.
		// The two text fields — the proposed body and the current text it would
		// replace — are the charter itself, so they carry its visibility; the
		// rest of the amendment stays public.
		if p.TargetDoc != "" {
			result["target_doc"] = p.TargetDoc
			result["proposed_branch"] = p.ProposedBranch
			result["proposed_body"] = p.ProposedBody
			result["proposed_title"] = p.ProposedTitle
			result["git_sha"] = p.GitSHA
			result["doc_text_hidden"] = docTextHidden
			if !docTextHidden {
				currentContent, _ := governance.GetDocument(governance.GetDataDir(), p.NodeID, p.TargetDoc)
				result["current_doc_content"] = currentContent
			}
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

		var gcJSON string
		db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", nodeID).Scan(&gcJSON)
		var gc model.GovernanceConfig
		json.Unmarshal([]byte(gcJSON), &gc)

		// Require the vote to come from someone the electorate counts — role
		// and tenure both, evaluated by the one condition eligibleVoters
		// counts by, so that who may vote and who is counted can never be two
		// different sets.
		//
		// This once called userHasMembership, which counts any active
		// membership, so a follower could vote; following carries no voting
		// rights (CONTEXT.md, "Member count"). It also carried the usual
		// `user.Role == "admin"` bypass, which let an instance admin vote in a
		// patch they hold no role in — but an instance admin "curates
		// instance-wide options; does not override per-patch choices"
		// (CONTEXT.md, "Instance admin"), and ADR 026 refuses instance
		// authority reaching into an active patch for the far smaller matter
		// of an event queue. A patch's vote is its own. An instance admin who
		// is also a member votes as that member, like anyone else.
		if denial := electorateDenial(db, user.ID, nodeID, gc); denial != "" {
			http.Error(w, `{"error":"`+denial+`"}`, http.StatusForbidden)
			return
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

		// Sole-voter early close (docs/adr/041): when exactly one person is
		// eligible to vote and that person has cast a decisive vote, the
		// outcome is settled — a voting window for an electorate of one
		// holds space for nobody. An abstain never closes early: it reads
		// as "not deciding yet" and stays changeable until the window ends.
		if req.Value != "abstain" {
			if _, sole := eligibleVoters(db, nodeID, gc); sole == user.ID {
				resolveProposal(db, proposalID)
			}
		}

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
			Link:     weblink.Proposal(nodeSlug, proposalID),
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

		// Both columns: `state` is what the SPA renders from (migration 016),
		// so leaving it at 'voting' left the page looking untouched.
		_, err = db.Exec(
			"UPDATE proposals SET status = 'withdrawn', state = 'withdrawn', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?",
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
			Link:     weblink.Proposal(nodeSlug, proposalID),
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
			// Decoded only to refuse it. A proposal opens for voting when it
			// is created (docs/adr/048) — there is no pre-voting state to
			// leave, so nothing promotes one. Dropping the field silently
			// answered 400 "no valid fields to update", which reads like a
			// bug in the caller; the SPA carried a "Submit for voting" button
			// against this endpoint for exactly that reason. Say why instead.
			State *string `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		if req.State != nil {
			http.Error(w, `{"error":"state is not settable: proposals open for voting when they are created (docs/adr/048)"}`, http.StatusBadRequest)
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
			Link:     weblink.Proposal(nodeSlug, proposalID),
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "state": "in_effect"})
	}
}
