package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// The maintainer's verbs on an admin-decides patch (docs/adr/092).
//
// On a patch whose decision method is admin-decides, the maintainer decides
// every proposal and may consult the members before deciding. A member's
// proposal is born waiting on them (`state = 'awaiting_admin'`, no ballot, no
// clock); a vote an admin opens on it — or asks for on their own proposal —
// is advisory: it runs on the ordinary ballot and closes on the ordinary
// clock, and closing hands the proposal back to the admin with the tally
// attached instead of resolving on it. The only things that end such a
// proposal are the two decisions below, and the author withdrawing.
//
// Both are gated on the proposal's *frozen* terms, not the patch's live
// rules (docs/adr/047): a patch that switches to a voting method mid-way
// does not turn its maintainer's open questions into member majorities.

// maintainerDecidable loads a proposal and checks the caller may decide it:
// a patch admin (never an instance admin holding no role here), on a
// proposal whose terms are admin-decides, while it is open. Writes the
// refusal and returns ok=false otherwise.
func maintainerDecidable(db *database.DB, w http.ResponseWriter, user *model.User, proposalID string) (model.Proposal, model.GovernanceConfig, bool) {
	var p model.Proposal
	var votingEndsAt *string
	var seatsContested int
	err := db.QueryRow(
		`SELECT id, node_id, author_id, status, COALESCE(state,'voting'), proposal_type,
		 COALESCE(target_doc,''), COALESCE(proposed_branch,''), COALESCE(proposed_body,''), COALESCE(proposed_title,''),
		 voting_ends_at, seats_contested
		 FROM proposals WHERE id = ?`, proposalID,
	).Scan(&p.ID, &p.NodeID, &p.AuthorID, &p.Status, &p.State, &p.ProposalType, &p.TargetDoc, &p.ProposedBranch, &p.ProposedBody, &p.ProposedTitle, &votingEndsAt, &seatsContested)
	if err != nil {
		http.Error(w, `{"error":"proposal not found"}`, http.StatusNotFound)
		return p, model.GovernanceConfig{}, false
	}
	p.VotingEndsAt = votingEndsAt

	if !userHasNodeRole(db, user.ID, p.NodeID, "admin") {
		http.Error(w, `{"error":"only this patch's admins can decide its proposals"}`, http.StatusForbidden)
		return p, model.GovernanceConfig{}, false
	}
	gc := votingTerms(db, proposalID, p.NodeID)
	if gc.DecisionMethod != "admin" {
		http.Error(w, `{"error":"this patch decides proposals by vote, not by its admins"}`, http.StatusConflict)
		return p, gc, false
	}
	// An election is the one proposal an admin-decides patch never
	// decides by admin: docs/adr/051's holdover rule is that the
	// electorate closes it and nobody else does.
	if seatsContested > 0 {
		http.Error(w, `{"error":"an election is closed by the electorate, not by an admin (docs/adr/051)"}`, http.StatusConflict)
		return p, gc, false
	}
	if p.Status != "open" || (p.State != "voting" && p.State != "awaiting_admin") {
		http.Error(w, `{"error":"this proposal is not open for a decision"}`, http.StatusConflict)
		return p, gc, false
	}
	return p, gc, true
}

// DecideProposal handles POST /api/v1/proposals/{id}/decide — the
// maintainer's approve or decline, at any time while the proposal is open,
// an advisory vote in progress included (docs/adr/092).
func DecideProposal(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		proposalID := r.PathValue("id")

		var req struct {
			Decision string `json:"decision"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Decision != "approve" && req.Decision != "decline" {
			http.Error(w, `{"error":"decision must be approve or decline"}`, http.StatusBadRequest)
			return
		}

		p, _, ok := maintainerDecidable(db, w, user, proposalID)
		if !ok {
			return
		}

		approve, reject, abstain := tallyProposal(db, proposalID)
		detail := fmt.Sprintf(`{"decision":"%s","from_state":"%s","advice":{"approve":%d,"reject":%d,"abstain":%d}}`,
			req.Decision, p.State, approve, reject, abstain)

		var nodeSlug, nodeName string
		db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", p.NodeID).Scan(&nodeSlug, &nodeName)
		var title string
		db.QueryRow("SELECT title FROM proposals WHERE id = ?", proposalID).Scan(&title)

		if req.Decision == "approve" {
			if err := applyProposalChanges(db, p, user); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"failed to apply changes: %s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			auth.LogAuditEvent(db, user.ID, "proposal.decided", "proposal", proposalID, detail, clientIP(r))
			notifyProposalApplied(db, p.NodeID, proposalID, user.ID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "approved", "state": "in_effect"})
			return
		}

		// Declined. `declined_by` names the decider the way applied_by names
		// an applier (migration 067): without it a rejected row with no
		// tally reads as a vote that failed. The amendment branch is left
		// in place, like a vote that failed leaves it — the proposal page
		// still shows what was asked for.
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		if _, err := db.Exec(
			"UPDATE proposals SET status = 'rejected', state = 'rejected', declined_by = ?, updated_at = ? WHERE id = ?",
			user.ID, now, proposalID,
		); err != nil {
			http.Error(w, `{"error":"failed to decline proposal"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "proposal.decided", "proposal", proposalID, detail, clientIP(r))
		notify(notifications.Event{
			Type:     notifications.ProposalRejected,
			NodeID:   p.NodeID,
			NodeSlug: nodeSlug,
			NodeName: nodeName,
			ActorID:  user.ID,
			TargetID: p.AuthorID,
			EntityID: proposalID,
			Title:    "Proposal declined: " + title,
			Body:     user.DisplayName + " declined this proposal.",
			Link:     weblink.Proposal(nodeSlug, proposalID),
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "rejected", "state": "rejected"})
	}
}

// OpenAdvisoryVote handles POST /api/v1/proposals/{id}/open-vote — the
// maintainer putting a proposal that is waiting on them to the members
// before deciding (docs/adr/092). Only from `awaiting_admin` with no window
// ever set: a proposal whose advisory vote already closed has its advice,
// and reopening it would be asking again until the answer changed.
func OpenAdvisoryVote(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		proposalID := r.PathValue("id")

		var req struct {
			DurationHours int `json:"duration_hours"`
		}
		// An empty body is fine: the patch's default window applies.
		json.NewDecoder(r.Body).Decode(&req)

		p, gc, ok := maintainerDecidable(db, w, user, proposalID)
		if !ok {
			return
		}
		if p.State != "awaiting_admin" {
			http.Error(w, `{"error":"a vote is already open on this proposal"}`, http.StatusConflict)
			return
		}
		if p.VotingEndsAt != nil && *p.VotingEndsAt != "" {
			http.Error(w, `{"error":"the members have already been asked; the tally is on the proposal"}`, http.StatusConflict)
			return
		}

		if req.DurationHours <= 0 {
			req.DurationHours = gc.DefaultVoteDuration
		}
		if req.DurationHours <= 0 {
			req.DurationHours = 72
		}
		now := time.Now().UTC()
		endsAt := now.Add(time.Duration(req.DurationHours) * time.Hour).Format("2006-01-02T15:04:05.000Z")
		if _, err := db.Exec(
			"UPDATE proposals SET state = 'voting', voting_ends_at = ?, duration_hours = ?, updated_at = ? WHERE id = ?",
			endsAt, req.DurationHours, now.Format("2006-01-02T15:04:05.000Z"), proposalID,
		); err != nil {
			http.Error(w, `{"error":"failed to open vote"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "proposal.advisory_vote_opened", "proposal", proposalID,
			fmt.Sprintf(`{"duration_hours":%d}`, req.DurationHours), clientIP(r))

		var nodeSlug, nodeName, title string
		db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", p.NodeID).Scan(&nodeSlug, &nodeName)
		db.QueryRow("SELECT title FROM proposals WHERE id = ?", proposalID).Scan(&title)
		notify(notifications.Event{
			Type:     notifications.ProposalVoting,
			NodeID:   p.NodeID,
			NodeSlug: nodeSlug,
			NodeName: nodeName,
			ActorID:  user.ID,
			EntityID: proposalID,
			Title:    "The maintainer is asking: " + title,
			Body:     "An advisory vote is open. The maintainer decides after hearing from you.",
			Link:     weblink.Proposal(nodeSlug, proposalID),
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "state": "voting", "voting_ends_at": endsAt})
	}
}
