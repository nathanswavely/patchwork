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

		// A closed record answers 200 with an empty list and its own setting
		// beside it, never 404
		// (docs/adr/2026-09-18-the-default-should-match-the-assumption.md).
		// A client seeing only an empty array cannot tell "no proposals yet"
		// from "withheld", and those two want opposite copy — the same reason
		// ListMembers states public_member_list back.
		if !canReadGovernanceRecord(db, r, nodeID) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"items":                    []interface{}{},
				"next_cursor":              "",
				"public_governance_record": "nobody",
			})
			return
		}

		after, limit := parsePaginationParams(r)
		status := r.URL.Query().Get("status")

		query := `SELECT p.id, p.node_id, p.author_id, p.title, p.body, p.status, COALESCE(p.state,''), p.proposal_type, p.duration_hours, p.voting_ends_at, p.created_at, p.updated_at,
			COALESCE(p.target_doc,''), COALESCE(p.proposed_branch,''), COALESCE(p.proposed_body,''), COALESCE(p.proposed_title,''), COALESCE(p.git_sha,''),
			` + displayNameExpr("u") + ` as author_name,
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

		// The filter is by outcome, not by the status column (docs/adr/097,
		// amended). A lapse and an unsettled contest both carry `rejected` —
		// the schema's only terminal "no", and adding a fifth would be a
		// migration for a word — so filtering on the column put three
		// proposals nobody rejected in a drawer labelled Rejected. The two
		// state values are what the product means, and the query reads them.
		switch status {
		case "", "all":
			// Every proposal.
		case "not_decided":
			query += " AND COALESCE(p.state,'') IN ('lapsed', 'unsettled')"
		case "rejected":
			query += " AND p.status = 'rejected' AND COALESCE(p.state,'') NOT IN ('lapsed', 'unsettled')"
		default:
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
			if err := rows.Scan(&p.ID, &p.NodeID, &p.AuthorID, &p.Title, &p.Body, &p.Status, &p.State, &p.ProposalType, &p.DurationHours, &p.VotingEndsAt, &p.CreatedAt, &p.UpdatedAt, &p.TargetDoc, &p.ProposedBranch, &p.ProposedBody, &p.ProposedTitle, &p.GitSHA, &p.AuthorName, &p.ApproveCount, &p.RejectCount, &p.AbstainCount); err != nil {
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
			"items":                    proposals,
			"next_cursor":              nextCursor,
			"public_governance_record": "everyone",
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

		// Raising a proposal is a member act, and a member act is a member's
		// alone. Not userHasMembership — that counts any active membership row,
		// followers included, so it let a follower author a live proposal on a
		// patch they cannot vote in.
		//
		// No `user.Role == "admin"` bypass either. Instance admins keep one for
		// commenting, which is speech, and for stewardship — withdrawing,
		// applying, moderating. Proposing is neither: once it became a member
		// act, an instance admin holding no role here proposing in a patch's
		// governance was instance authority reaching into a per-patch choice,
		// which CONTEXT.md ("Instance admin") and ADR 026 both refuse. Wanting
		// a voice in a patch is what joining is for (docs/adr/044).
		if !mayPropose(db, user.ID, nodeID) {
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
			// The person this proposal is about, as opposed to the author who
			// raised it. Set on a meritocratic nomination (docs/adr/051).
			TargetUserID string `json:"target_user_id"`
			// On an admin-decides patch an admin's proposal is a direct change
			// unless they ask the members first (docs/adr/092). Ignored
			// everywhere else: on a voting patch every proposal is put to a
			// vote, and there is nothing for the flag to choose.
			PutToVote bool `json:"put_to_vote"`
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
		// A proposal about a person is a nomination, and carries conditions an
		// ordinary proposal does not (docs/adr/051). Validated before anything
		// is written, so a refused nomination leaves no record behind.
		if req.TargetUserID != "" {
			if req.ProposalType != "membership" {
				http.Error(w, `{"error":"a proposal about a person must be of type membership"}`, http.StatusBadRequest)
				return
			}
			if msg := validateNomination(db, nodeID, user.ID, req.TargetUserID); msg != "" {
				http.Error(w, `{"error":"`+msg+`"}`, http.StatusConflict)
				return
			}
		}
		// And a membership proposal has to be about somebody (docs/adr/100).
		// Without a target it is the shape a candidacy takes when the product
		// offers no other: it changes nothing whichever way it closes, and
		// somebody who wanted a seat found themselves the subject of a public
		// vote published under a Reject button. Elections are the exception
		// and never reach here — the calendar writes them directly, with
		// candidates rather than a target.
		if req.ProposalType == "membership" && req.TargetUserID == "" {
			http.Error(w, `{"error":"a membership proposal has to name the person it is about"}`, http.StatusBadRequest)
			return
		}

		// The rules in force, read once. Everything below decides from this
		// same photograph — the default duration, whether the ballot exists,
		// and the terms stored on the row — so nothing can be decided against
		// a config that moved mid-request.
		isNodeAdmin := userHasNodeRole(db, user.ID, nodeID, "admin")
		var gcJSON string
		db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", nodeID).Scan(&gcJSON)
		var gc model.GovernanceConfig
		json.Unmarshal([]byte(gcJSON), &gc)

		if req.DurationHours <= 0 {
			if gc.DefaultVoteDuration > 0 {
				req.DurationHours = gc.DefaultVoteDuration
			} else {
				req.DurationHours = 72
			}
		}

		// Where the patch decides its proposals (docs/adr/053). Elsewhere
		// removes the ballot and keeps the discussion: a proposal can still be
		// raised, argued over and revised, and the decision comes back as an
		// attestation. This is what makes the attestation gate mean anything —
		// a patch with both would let an admin who disliked where a tally was
		// heading record a meeting result instead.
		decidedElsewhere := gc.ProposalVenue == "elsewhere"
		isRulesDoc := req.TargetDoc == rulesFilename || req.TargetDoc == "Governance Rules"

		// The rules file is not a text a meeting adopts, so it never becomes an
		// attestation, so a rules proposal on such a patch has no way to ever
		// be decided. On this patch a rules change is a direct change an admin
		// applies (docs/adr/041's existing sense), and saying so is better than
		// accepting a proposal that would sit open forever.
		if decidedElsewhere && isRulesDoc && !isNodeAdmin {
			http.Error(w, `{"error":"this patch decides at meetings, and the governance rules are not something a meeting adopts. Ask an admin to apply the change directly (docs/adr/053)."}`, http.StatusConflict)
			return
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
			authorName, authorEmail := commitIdentity(user)
			sha, branchErr := governance.CreateBranch(governance.GetDataDir(), nodeID, branchName, req.TargetDoc, req.ProposedBody, authorName, authorEmail, commitMsg)
			if branchErr != nil {
				http.Error(w, fmt.Sprintf(`{"error":"failed to create amendment branch: %s"}`, branchErr.Error()), http.StatusInternalServerError)
				return
			}
			gitSHA = sha
		}

		// Ceremony follows the rules in force (docs/adr/041): only the
		// admin-decides decision method lets an admin apply directly — a
		// direct change, born applied. Every voting method votes, admins
		// included; the old maintainer+zero-quorum bypass let admins skip a
		// vote the patch's own charter promised, and was removed.
		initialState := "voting"
		autoApplyNow := false
		adminDecides := gc.DecisionMethod == "admin"

		switch {
		case adminDecides && isNodeAdmin && !req.PutToVote:
			initialState, autoApplyNow = "in_effect", true

		case decidedElsewhere && isRulesDoc:
			// Only an admin reaches here — the refusal above sent everyone
			// else away. docs/adr/053: on a patch that decides elsewhere a
			// rules change is a direct change, honest because it claims
			// nothing about a vote.
			initialState, autoApplyNow = "in_effect", true

		case decidedElsewhere:
			// No ballot, and therefore no clock: `voting_ends_at` stays NULL
			// so nothing resolves this, and `state` says why rather than
			// leaving a proposal that looks open for voting and refuses every
			// vote. Not docs/adr/048's retired `discussion` — what killed that
			// was a stage nothing governed, ahead of a vote that would come;
			// here the vote is not coming, and the venue is the rule that says
			// so.
			initialState = "elsewhere"

		case adminDecides && !isNodeAdmin:
			// The maintainer decides this patch's proposals (docs/adr/092).
			// A member's proposal is a request to them, not a question to
			// the members: it waits, with no ballot and no clock, until an
			// admin approves it, declines it, or puts it to an advisory
			// vote. Born voting it was decided by the members' majority,
			// which the patch's own rules say decides nothing here.

			initialState = "awaiting_admin"

		case adminDecides && isNodeAdmin && req.PutToVote:
			// An admin asking the members first. The vote is advisory: it
			// runs on the ordinary ballot and closes on the ordinary clock,
			// and when it closes the proposal comes back to the admin with
			// the tally attached instead of resolving on it.
			initialState = "voting"
		}

		// A proposal with no ballot has no window either. NULL rather than a
		// date nothing watches: resolveProposal only runs where there is an
		// end to have passed, so the absence is what keeps an undecidable
		// proposal from being decided by a clock. A direct change carries no
		// window for the same reason — it was never open.
		var votingEnds interface{} = votingEndsAt
		if initialState == "elsewhere" || initialState == "awaiting_admin" || initialState == "in_effect" {
			votingEnds = nil
		}

		apID := ap.ProposalAPID(ap.GetDomain(), id)
		// Photograph the rules this vote will be judged by (docs/adr/047).
		// `gcJSON` is the config read above, before anything in this request
		// could have changed it — the terms in force at the moment voting
		// opens. From here the node's rules may move; this vote's may not.
		_, err := db.Exec(
			`INSERT INTO proposals (id, node_id, author_id, title, body, status, proposal_type, duration_hours, voting_ends_at, created_at, updated_at, ap_id, target_doc, proposed_branch, proposed_body, proposed_title, git_sha, base_sha, state, voting_terms, target_user_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, nodeID, user.ID, req.Title, req.Body, "open", req.ProposalType, req.DurationHours, votingEnds, createdAt, createdAt, apID, req.TargetDoc, branchName, req.ProposedBody, req.ProposedTitle, gitSHA, baseSHA, initialState, gcJSON, nullIfEmpty(req.TargetUserID),
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
			mergedSHA := ""
			if req.ProposalType == "amendment" && branchName != "" {
				dataDir := governance.GetDataDir()
				mergeName, mergeEmail := commitIdentity(user)
				sha, mergeErr := governance.MergeBranch(dataDir, nodeID, branchName, mergeName, mergeEmail)
				if mergeErr != nil {
					log.Printf("proposal %s: direct-change merge failed: %v", id, mergeErr)
					applied = false
				} else {
					mergedSHA = sha
					governance.DeleteBranch(dataDir, nodeID, branchName)
					// Same post-merge DB syncs as the other apply paths (docs/adr/011).
					if req.TargetDoc == "governance-rules.json" || req.TargetDoc == "Governance Rules" {
						if err := syncRulesAndNotify(db, dataDir, nodeID, user.ID, id); err != nil {
							applyIncomplete(db, id, mergedSHA, "rules sync", err)
						}
					}
					if err := syncLiningToDB(db, nodeID, req.TargetDoc, req.ProposedTitle, user.ID); err != nil {
						applyIncomplete(db, id, mergedSHA, "charter mirror", err)
					}
				}
			}
			if applied {
				// 'approved' is the terminal success status everywhere else
				// (and the only one the schema CHECK allows — 'passed' was
				// silently rejected, leaving fast-tracked amendments 'open').
				// The sha travels with it: settleApplied writes both together.
				if err := settleApplied(db, id, mergedSHA, user.ID, createdAt); err != nil {
					applyIncomplete(db, id, mergedSHA, "settle", err)
				}
			} else {
				// The INSERT above stamped state 'in_effect'; roll it back so
				// the unapplied record reads as an open proposal, not an
				// applied change. It gets the window it would have had, so
				// the open proposal has a clock like every other. If even that
				// write fails the row claims a change nothing made, which is
				// the one thing this must not do quietly.
				if _, err := db.Exec("UPDATE proposals SET state = 'voting', voting_ends_at = ? WHERE id = ?", votingEndsAt, id); err != nil {
					applyIncomplete(db, id, "", "reopen after failed merge", err)
				}
			}
		}

		auth.LogAuditEvent(db, user.ID, "proposal.create", "proposal", id, fmt.Sprintf(`{"state":"%s","auto_applied":%v}`, initialState, autoApplyNow), clientIP(r))

		var p model.Proposal
		db.QueryRow(
			`SELECT id, node_id, author_id, title, body, status, proposal_type, duration_hours, voting_ends_at, created_at, updated_at, COALESCE(target_doc,''), COALESCE(proposed_branch,''), COALESCE(proposed_body,''), COALESCE(proposed_title,''), COALESCE(git_sha,''), COALESCE(state,'voting') FROM proposals WHERE id = ?`, id,
		).Scan(&p.ID, &p.NodeID, &p.AuthorID, &p.Title, &p.Body, &p.Status, &p.ProposalType, &p.DurationHours, &p.VotingEndsAt, &p.CreatedAt, &p.UpdatedAt, &p.TargetDoc, &p.ProposedBranch, &p.ProposedBody, &p.ProposedTitle, &p.GitSHA, &p.State)

		// Broadcast to node followers, behind two gates that answer different
		// questions. Whether this patch publishes its deliberation at all
		// (docs/adr/2026-09-18-the-default-should-match-the-assumption.md):
		// without it the record is closed to a browser and delivered in full
		// to every remote follower, in copies that never come back. And, if it
		// does publish, whether the author's own membership is switched out of
		// sight (docs/adr/006) — writing a proposal takes a membership, so
		// attributing one asserts that membership to every remote reader.
		//
		// Synchronous, for the reason broadcastDocUpdate gives: this only
		// writes rows to the outbox queue, and a goroutine racing the
		// assertion leaves a test unable to tell "withheld" from "hasn't run
		// yet".
		if governanceRecordIsPublic(db, nodeID) {
			ap.BroadcastToFollowers(db, "node", nodeID, map[string]interface{}{
				"@context": ap.GovernanceContext(),
				"type":     "Create",
				"actor":    ap.NodeAPID(ap.GetDomain(), nodeID),
				"object":   ap.ProposalToObject(p, ap.GetDomain(), !membershipHidden(db, nodeID, p.AuthorID)),
			})
		}

		// Write down who this announcement reaches with standing to vote, so
		// the hourly pass can tell a person who was never told from one who
		// was (docs/adr/093). Before the notify, because the notify is a
		// goroutine and this is the record it is measured against.
		if initialState == "voting" {
			seedVoteAudienceNow(db, id, nodeID)
		}

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

// effectiveTenureDays is the minimum voting tenure a patch may require right
// now: the configured number, capped at the patch's own age in whole days
// (docs/adr/098). Nobody can be asked to have been here longer than the
// patch has — on the Formal defaults a patch made this morning asked its
// founder for thirty days, counted an electorate of nobody, and lapsed its
// own first rules vote with no ballots.
//
// Age runs from `founded_at` where the patch states one (a date; an
// organisation moving rules it already lives by keeps its full bar from the
// first day) and otherwise from the row's creation instant. The instant, not
// its date: anchoring on midnight would put the founder outside the cutoff
// for part of every day of the ramp.
//
// Every reader of MinVotingTenureDays goes through here — the gate, the
// denominator, the denial message and the "needs your vote" count — so the
// electorate stays one set (docs/adr/044).
func effectiveTenureDays(db *database.DB, nodeID string, gc model.GovernanceConfig) int {
	if gc.MinVotingTenureDays <= 0 {
		return 0
	}
	age, known := patchAgeDays(db, nodeID)
	if !known {
		// A row with no readable age is treated as brand new rather than
		// ancient: the cap exists to let people vote, and an unreadable
		// timestamp should not be the thing that stops them.
		return 0
	}
	// Younger than its own bar, a patch has no bar (docs/adr/098). Not
	// min(bar, age): that only ever admits people who joined on the first
	// day, because everyone after has less tenure than the patch has age
	// and so waits the full configured number anyway — the co-op's board,
	// joining on day two, would have been shut out of the first month's
	// votes, which is the finding this exists to fix. The rule protects a
	// community from newcomers, and until the patch is as old as the rule
	// there is nobody who is not one.
	if age < gc.MinVotingTenureDays {
		return 0
	}
	return gc.MinVotingTenureDays
}

// patchAgeDays is how old a patch is, in whole days, and whether that is
// knowable at all.
//
// Age runs from `founded_at` where the patch states one (a date; an
// organisation moving rules it already lives by keeps its full bar from the
// first day) and otherwise from the row's creation instant. The instant, not
// its date: anchoring on midnight would put the founder outside the cutoff
// for part of every day of the ramp.
//
// Its own function because two surfaces ask: the tenure cap enforces it, and
// the rules editor states it back to the person setting the bar
// (docs/adr/104). One definition, or the editor promises a date the gate does
// not honour.
func patchAgeDays(db *database.DB, nodeID string) (int, bool) {
	var foundedAt, createdAt string
	db.QueryRow("SELECT COALESCE(founded_at,''), created_at FROM nodes WHERE id = ?", nodeID).Scan(&foundedAt, &createdAt)
	var since time.Time
	if d, err := time.Parse("2006-01-02", foundedAt); err == nil {
		since = d
	} else if t, err := parseStoredInstant(createdAt); err == nil {
		since = t
	} else {
		return 0, false
	}
	age := int(time.Since(since).Hours() / 24)
	if age < 0 {
		age = 0
	}
	return age, true
}

// parseStoredInstant reads a stored ISO 8601 timestamp in either of the
// shapes the schema writes: with milliseconds (the strftime default) or
// without.
func parseStoredInstant(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02T15:04:05.000Z", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

// electorateFilter is electorateMembership plus the minimum voting tenure in
// force — the whole condition for "may vote here, right now" — together with
// the args the tenure term binds. The tenure is effectiveTenureDays, never
// the raw configured number.
func electorateFilter(db *database.DB, nodeID, prefix string, gc model.GovernanceConfig) (string, []interface{}) {
	cond := electorateMembership(prefix)
	var args []interface{}
	if days := effectiveTenureDays(db, nodeID, gc); days > 0 {
		// Stored timestamps are ISO 8601 with a 'T'; format the cutoff the
		// same way so the string comparison stays chronological.
		cond += " AND " + prefix + "joined_at <= strftime('%Y-%m-%dT%H:%M:%fZ', 'now', ?)"
		args = append(args, fmt.Sprintf("-%d days", days))
	}
	return cond, args
}

// voteEligibleAt is the day a member still inside the tenure window may
// first cast a ballot here, as a date, or "" when tenure is not what is
// stopping them — they may already vote, they are not in the room at all,
// or this patch has no tenure bar in force.
//
// The page needs this because the frozen terms carry the *configured*
// number and the gate enforces the *effective* one (docs/adr/098), so a
// page reading `min_voting_tenure_days` recites a rule that may not be
// running. Five simulated members read "voting requires 30 days'
// membership" under a button that took their vote; one pressed it and
// could not tell whether it had counted.
func voteEligibleAt(db *database.DB, nodeID, userID string, gc model.GovernanceConfig) string {
	days := effectiveTenureDays(db, nodeID, gc)
	if days <= 0 || userID == "" {
		return ""
	}
	var eligible, today string
	err := db.QueryRow(
		`SELECT date(joined_at, ?), date('now') FROM memberships
		 WHERE node_id = ? AND user_id = ? AND `+electorateMembership(""),
		fmt.Sprintf("+%d days", days), nodeID, userID,
	).Scan(&eligible, &today)
	if err != nil || eligible <= today {
		return ""
	}
	return eligible
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
	return inElectorateExcept(db, userID, nodeID, gc, "")
}

// inElectorateExcept is inElectorate with the recused subject taken out, so
// the gate and the denominator agree (docs/adr/044).
func inElectorateExcept(db *database.DB, userID, nodeID string, gc model.GovernanceConfig, exceptUserID string) bool {
	if exceptUserID != "" && userID == exceptUserID {
		return false
	}
	cond, args := electorateFilter(db, nodeID, "", gc)
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
	return electorateDenialExcept(db, userID, nodeID, gc, "")
}

// electorateDenialExcept adds the third answer recusal makes possible: this
// proposal is about you. It is named separately from the other two because a
// recused subject is not "not one of us" — they are in the electorate for
// every other vote, and telling them they are not a member would be false.
func electorateDenialExcept(db *database.DB, userID, nodeID string, gc model.GovernanceConfig, exceptUserID string) string {
	if exceptUserID != "" && userID == exceptUserID {
		return "this patch does not let people vote on proposals about themselves"
	}
	if inElectorate(db, userID, nodeID, gc) {
		return ""
	}
	if days := effectiveTenureDays(db, nodeID, gc); days > 0 && inElectorate(db, userID, nodeID, model.GovernanceConfig{}) {
		return fmt.Sprintf("must be a member for at least %d days to vote", days)
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

// votingTerms returns the rules a proposal is judged by: the governance config
// photographed when its voting opened (docs/adr/047). Every surface that
// decides something about a running vote — who may cast a ballot, how many are
// eligible, whether quorum is met, which threshold carries it — asks here
// rather than reading the node's current config, so that a rules edit cannot
// redraw a contest people have already voted in.
//
// Falls back to the node's live config when the proposal carries no
// photograph: rows created before migration 045, and any resolved proposal,
// which the migration deliberately left NULL because its terms no longer
// decide anything.
//
// `amendment_auto_apply` is the exception and is NOT read from here — it says
// what happens after a vote, not who wins it, and a safety valve has to take
// effect when it is flipped. Callers that need it read the live config.
func votingTerms(db *database.DB, proposalID, nodeID string) model.GovernanceConfig {
	var termsJSON string
	db.QueryRow("SELECT COALESCE(voting_terms,'') FROM proposals WHERE id = ?", proposalID).Scan(&termsJSON)
	if termsJSON == "" || termsJSON == "{}" {
		db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", nodeID).Scan(&termsJSON)
	}
	var gc model.GovernanceConfig
	json.Unmarshal([]byte(termsJSON), &gc)
	return gc
}

// eligibleVoters returns how many people may currently vote on a node's
// proposals — active admins and members past the minimum voting tenure —
// and, when the electorate is exactly one person, that voter's user ID.
//
// The config passed in should be a proposal's votingTerms, not the node's
// live config, wherever the answer is about one proposal.
// recusedSubject returns the person a proposal's own terms bar from voting on
// it — its subject, when the patch recuses subjects (docs/adr/051).
//
// It returns "" when recusal would empty the electorate. Recusal exists to
// remove a conflicted voter, not to make a vote impossible: with nobody left
// eligible and a quorum above zero, quorumMet can never be true and the
// proposal sits open past its window forever — the exact silent-stall failure
// the comment in resolveProposal describes being fixed once already. A
// contrived self-vote is the lesser fault, and it is at least a vote that
// resolves.
func recusedSubject(db *database.DB, nodeID string, gc model.GovernanceConfig, targetUserID string) string {
	if !gc.SubjectRecusal || targetUserID == "" {
		return ""
	}
	if remaining, _ := eligibleVotersExcept(db, nodeID, gc, targetUserID); remaining == 0 {
		return ""
	}
	return targetUserID
}

func eligibleVoters(db *database.DB, nodeID string, gc model.GovernanceConfig) (int, string) {
	return eligibleVotersExcept(db, nodeID, gc, "")
}

// eligibleVotersExcept is eligibleVoters with one person left out — the
// recused subject of the proposal being counted. The exclusion belongs here
// rather than at the gate alone because ADR 044's rule is that the gate, the
// denominator, the tally, and the UI all name one set: barring someone from
// casting while still dividing quorum by them would make a nomination
// unpassable.
func eligibleVotersExcept(db *database.DB, nodeID string, gc model.GovernanceConfig, exceptUserID string) (int, string) {
	ids := electorateUserIDs(db, nodeID, gc, exceptUserID)
	sole := ""
	if len(ids) == 1 {
		sole = ids[0]
	}
	return len(ids), sole
}

// electorateUserIDs names the electorate rather than counting it: the same
// set eligibleVoters counts, the vote gate admits and the tally credits,
// listed out (docs/adr/044).
//
// Asking who is in the room is a different question from asking how many are,
// and the pass that tells people they still owe a vote needs the first one
// (docs/adr/093). It is one query, not a second definition: everything about
// who belongs comes from electorateFilter, and eligibleVoters is now a count
// of what this returns.
func electorateUserIDs(db *database.DB, nodeID string, gc model.GovernanceConfig, exceptUserID string) []string {
	cond, tenureArgs := electorateFilter(db, nodeID, "", gc)
	args := append([]interface{}{nodeID}, tenureArgs...)
	if exceptUserID != "" {
		cond += " AND user_id != ?"
		args = append(args, exceptUserID)
	}
	rows, err := db.Query(`SELECT user_id FROM memberships WHERE node_id = ? AND `+cond+` ORDER BY joined_at`, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// quorumReached is the quorum test, written once. resolveProposal decides a
// vote with it; the turnout notice tells people where the vote stands with it
// (docs/adr/093). A notice quoting arithmetic the resolver does not run would
// be worse than no notice.
func quorumReached(gc model.GovernanceConfig, cast, eligible int) bool {
	return gc.QuorumPercent == 0 || (eligible > 0 && (cast*100/eligible) >= gc.QuorumPercent)
}

// votesNeededForQuorum is the smallest number of ballots that satisfies
// quorumReached, or 0 where this patch asks for no quorum. The ceiling, not
// the percentage: "4 needed" is the sentence a person can act on, and
// "50% needed" is the one five simulated members read and did nothing about.
func votesNeededForQuorum(gc model.GovernanceConfig, eligible int) int {
	if gc.QuorumPercent <= 0 || eligible <= 0 {
		return 0
	}
	needed := (eligible*gc.QuorumPercent + 99) / 100
	if needed > eligible {
		needed = eligible
	}
	return needed
}

// resolveProposal tallies an open proposal and finalizes it: status update,
// amendment auto-apply, audit event, AP broadcast. Returns the new status
// ("approved" or "rejected"), or "" when the proposal stayed open (not open
// to begin with, or quorum not met). Callers decide when resolution is due —
// the voting window expiring, or the sole-voter early close (docs/adr/041).
func resolveProposal(db *database.DB, proposalID string) string {
	var p model.Proposal
	var votingEndsAt string
	var seatsContested int
	err := db.QueryRow(
		`SELECT id, node_id, author_id, title, status, proposal_type, COALESCE(target_doc,''), COALESCE(proposed_title,''), COALESCE(proposed_branch,''), COALESCE(proposed_body,''), COALESCE(target_user_id,''), COALESCE(voting_ends_at,''), COALESCE(seats_contested,0)
		 FROM proposals WHERE id = ?`, proposalID,
	).Scan(&p.ID, &p.NodeID, &p.AuthorID, &p.Title, &p.Status, &p.ProposalType, &p.TargetDoc, &p.ProposedTitle, &p.ProposedBranch, &p.ProposedBody, &p.TargetUserID, &votingEndsAt, &seatsContested)
	if err != nil || p.Status != "open" {
		return ""
	}

	// An election is resolveElection's, and only its (docs/adr/051): a
	// contest is settled by seating a council, not by a majority over a
	// question, and its ballots are not in `votes` at all. The sweep has
	// always split them; the read path never did, so a reader arriving
	// between an election's deadline and the next hourly pass could run it
	// through the ordinary tally. Nothing carried it there, and now that a
	// resolution tells the whole patch what it decided, nothing may.
	if seatsContested > 0 {
		return ""
	}

	// Tally votes for resolution.
	approveCount, rejectCount, abstainCount := tallyProposal(db, proposalID)

	// The rules this vote is judged by are the ones it opened with, not the
	// node's current ones (docs/adr/047).
	gc := votingTerms(db, proposalID, p.NodeID)

	// An advisory vote resolves nothing (docs/adr/092). Its window closing
	// hands the proposal back to the maintainer with the tally attached; the
	// only things that end it are an admin's approve or decline. Idempotent:
	// a second sweep finds the state already moved.
	if gc.DecisionMethod == "admin" {
		db.Exec(`UPDATE proposals SET state = 'awaiting_admin', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		         WHERE id = ? AND state = 'voting'`, proposalID)
		return ""
	}

	// ...except amendment_auto_apply, which is read live further down: it
	// decides what happens after a vote, not who wins it, and switching it
	// off has to stop in-flight amendments from applying themselves.
	var liveGC model.GovernanceConfig
	var liveGCJSON string
	db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", p.NodeID).Scan(&liveGCJSON)
	json.Unmarshal([]byte(liveGCJSON), &liveGC)

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
	// The subject of a nomination is out of its own electorate where the
	// patch recuses subjects (docs/adr/051) — out of the gate and out of this
	// denominator both, or the vote could not be carried.
	recused := recusedSubject(db, p.NodeID, gc, p.TargetUserID)
	eligibleCount, _ := eligibleVotersExcept(db, p.NodeID, gc, recused)
	totalVotes := approveCount + rejectCount + abstainCount
	quorumMet := quorumReached(gc, totalVotes, eligibleCount)
	if !quorumMet {
		// Under quorum while the window runs: leave open, votes may still
		// come. Under quorum once it has closed: the proposal lapses
		// (docs/adr/097). It used to stay open here forever — the list said
		// "voting", the vote endpoint said "voting period has ended", and
		// nothing told anybody. On the Formal defaults that was a new
		// co-op's first proposal, since tenure keeps most joiners out of
		// the electorate for a month.
		if windowClosed(votingEndsAt) {
			lapseProposal(db, p, totalVotes)
			return "lapsed"
		}
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
	//
	// `AND status = 'open'` makes this the one write that settles the vote,
	// the way lapseProposal's does: three paths call resolveProposal (the
	// sweep, a read after the window, a sole voter's ballot) and two of them
	// can arrive at once. Whoever loses the race stops here rather than
	// re-resolving a settled proposal and telling everybody a second time.
	res, updErr := db.Exec("UPDATE proposals SET status = ?, state = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ? AND status = 'open'", newStatus, newStatus, proposalID)
	if updErr != nil {
		log.Printf("proposal %s: resolve failed: %v", proposalID, updErr)
		return ""
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ""
	}

	// A ratified nomination takes effect on approval (docs/adr/051). There is
	// no admin "apply" step: the community ratifying is the whole decision,
	// and leaving it to an admin afterwards would be a veto over the
	// ratification the admins themselves asked for.
	if newStatus == "approved" && p.ProposalType == "membership" && p.TargetUserID != "" {
		ratifyNomination(db, proposalID, p.NodeID, p.TargetUserID)
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		// The promotion has already happened by here, so a failure to record
		// it leaves a new admin whose proposal still reads as merely
		// approved. Nothing can undo the promotion; the divergence gets said
		// out loud instead.
		if err := settleApplied(db, proposalID, "", "", now); err != nil {
			applyIncomplete(db, proposalID, "", "ratified nomination settle", err)
		}
	}

	// Auto-apply amendment if approved and configured
	// liveGC, not gc: the auto-apply switch is a safety valve and takes effect
	// the moment it is flipped, including for votes already running
	// (docs/adr/047).
	if newStatus == "approved" && p.ProposalType == "amendment" && p.TargetDoc != "" && liveGC.AmendmentAutoApply {
		branch := p.ProposedBranch
		if branch != "" {
			// The branch may be gone — a repo rebuilt from the database
			// carries `main` and nothing else (docs/adr/084). The proposed
			// text is not gone, so re-derive the branch from it rather than
			// fail an amendment the members carried.
			mergeErr := ensureAmendmentBranch(governance.GetDataDir(), p.NodeID, branch, p.TargetDoc, p.ProposedBody)
			var sha string
			if mergeErr == nil {
				sha, mergeErr = governance.MergeBranch(governance.GetDataDir(), p.NodeID, branch, "Patchwork System", "system@patchwork.local")
			}
			if mergeErr != nil {
				// The vote still carried; it is now an approved proposal
				// waiting on an admin, and the notice below says so. Loud in
				// the log, because an auto-apply patch is not expecting one.
				log.Printf("proposal %s: auto-apply merge failed, left approved for an admin: %v", proposalID, mergeErr)
			}
			if mergeErr == nil {
				// Same post-merge DB syncs as the manual ApplyProposal path
				// (docs/adr/011): rules to governance config, markdown docs
				// to governance_docs. Neither can be rolled back once the
				// merge is in, so a failure here is recorded rather than
				// returned — the charter moved and somebody has to be able
				// to find out that the mirror of it didn't.
				if p.TargetDoc == "governance-rules.json" || p.TargetDoc == "Governance Rules" {
					// No actor: resolution is the clock, not a person, so the
					// notice reaches everyone including the proposal's author.
					if err := syncRulesAndNotify(db, governance.GetDataDir(), p.NodeID, "", proposalID); err != nil {
						applyIncomplete(db, proposalID, sha, "rules sync", err)
					}
				}
				if err := syncLiningToDB(db, p.NodeID, p.TargetDoc, p.ProposedTitle, p.AuthorID); err != nil {
					applyIncomplete(db, proposalID, sha, "charter mirror", err)
				}
				// The merge already happened, so there is nothing left for an
				// admin to make official — skip 'approved' and land where the
				// manual apply path lands. applied_by stays NULL: no person
				// applied this one.
				now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
				if err := settleApplied(db, proposalID, sha, "", now); err != nil {
					applyIncomplete(db, proposalID, sha, "settle", err)
				}
			}
		}
	}

	auth.LogAuditEvent(db, "", "proposal.resolved", "proposal", proposalID,
		fmt.Sprintf(`{"result":"%s","approve":%d,"reject":%d,"abstain":%d,"quorum_met":true}`, newStatus, approveCount, rejectCount, abstainCount), "")

	notifyProposalResolved(db, p, newStatus)

	// Broadcast resolution
	go func() {
		if !governanceRecordIsPublic(db, p.NodeID) {
			return
		}
		resolveActivity := ap.ProposalResolvedActivity(
			ap.ProposalAPID(ap.GetDomain(), proposalID),
			ap.NodeAPID(ap.GetDomain(), p.NodeID),
			newStatus, approveCount, rejectCount, abstainCount,
		)
		ap.BroadcastToFollowers(db, "node", p.NodeID, resolveActivity)
	}()

	return newStatus
}

// notifyProposalResolved tells a patch's members what their own vote decided.
//
// A vote that carried told nobody at all. On the Formal template, which ships
// `amendment_auto_apply: false`, that is the whole failure: the proposal is
// stamped approved, the Record files it under what the patch has settled, and
// the rule does not change until an admin applies it — and the admin is
// exactly the person nobody informed. So the notice says which of the two
// happened, in its own words, rather than announcing an outcome and letting
// people assume the change landed with it.
//
// One notice per resolution, to the members (docs/adr/093: the obligation a
// member took on is that proposals are decided in their name, and admins hold
// the further duty inside the same patch). An admin-only second notice was
// the obvious alternative and would have reached every admin twice for one
// decision, which is how people learn to read neither.
//
// No actor: the clock and the electorate settled it, not whoever happened to
// be on the page, so the notice reaches everyone including the last voter.
func notifyProposalResolved(db *database.DB, p model.Proposal, outcome string) {
	var slug, name string
	db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", p.NodeID).Scan(&slug, &name)

	event := notifications.Event{
		NodeID:   p.NodeID,
		NodeSlug: slug,
		NodeName: name,
		EntityID: p.ID,
		Link:     weblink.Proposal(slug, p.ID),
	}

	if outcome != "approved" {
		event.Type = notifications.ProposalRejected
		event.Title = "Not carried: " + p.Title
		event.Body = "Voting closed and the proposal did not carry."
		notify(event)
		return
	}

	// Which of the two approved endings this is, read back from the row the
	// steps above just wrote rather than re-deriving it: a ratified
	// nomination and an auto-applied amendment both land in_effect, and
	// everything else is waiting on somebody.
	var state string
	db.QueryRow("SELECT COALESCE(state,'') FROM proposals WHERE id = ?", p.ID).Scan(&state)
	if state == "in_effect" {
		event.Type = notifications.ProposalApplied
		event.Title = "Carried and in effect: " + p.Title
		event.Body = "Voting closed and the proposal carried. The change is in effect."
		notify(event)
		return
	}

	// ProposalApproved has sat registered and unsent since the registry was
	// written. This is the case it was for, and it is not the same case as
	// ProposalApplied: saying "applied" here would tell a patch its rule had
	// changed while the rule sat exactly as it was.
	event.Type = notifications.ProposalApproved
	event.Title = "Carried: " + p.Title
	event.Body = "Voting closed and the proposal carried. It is not in effect yet — an admin of this patch has to apply it. Until then nothing has changed."
	notify(event)
}

// GetProposal handles GET /api/v1/proposals/{id}.
func GetProposal(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proposalID := r.PathValue("id")

		var p model.Proposal
		var authorName, appliedAt, targetUserName, nominationsCloseAt string
		var seatsContested int
		err := db.QueryRow(
			`SELECT p.id, p.node_id, p.author_id, p.title, p.body, p.status, COALESCE(p.state,''), COALESCE(p.applied_at,''), p.proposal_type, p.duration_hours, p.voting_ends_at, p.created_at, p.updated_at,
			 COALESCE(p.target_doc,''), COALESCE(p.target_user_id,''), COALESCE(p.proposed_branch,''), COALESCE(p.proposed_body,''), COALESCE(p.proposed_title,''), COALESCE(p.git_sha,''),
			 p.seats_contested, COALESCE(p.nominations_close_at,''),
			 `+displayNameExpr("u")+` as author_name,
			 `+displayNameExpr("tu")+` as target_user_name
			 FROM proposals p LEFT JOIN users u ON u.id = p.author_id
			 LEFT JOIN users tu ON tu.id = p.target_user_id
			 WHERE p.id = ?`, proposalID,
		).Scan(&p.ID, &p.NodeID, &p.AuthorID, &p.Title, &p.Body, &p.Status, &p.State, &appliedAt, &p.ProposalType, &p.DurationHours, &p.VotingEndsAt, &p.CreatedAt, &p.UpdatedAt, &p.TargetDoc, &p.TargetUserID, &p.ProposedBranch, &p.ProposedBody, &p.ProposedTitle, &p.GitSHA, &seatsContested, &nominationsCloseAt, &authorName, &targetUserName)
		if err != nil {
			http.Error(w, `{"error":"proposal not found"}`, http.StatusNotFound)
			return
		}

		// A single proposal on a closed record is a 404, not the list's
		// 200-with-the-setting
		// (docs/adr/2026-09-18-the-default-should-match-the-assumption.md).
		// Without the listing there is no legitimate way to be holding this
		// id, and it matches how a members-only charter answers.
		if !canReadGovernanceRecord(db, r, p.NodeID) {
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
				resolveProposal(db, proposalID)
				// Re-read rather than patching Status alone: resolution
				// moves state too, and an auto-applied amendment also
				// stamps applied_at. Unconditionally, because an advisory
				// window closing moves state without returning a status
				// (docs/adr/092).
				db.QueryRow(
					`SELECT status, COALESCE(state,''), COALESCE(applied_at,'') FROM proposals WHERE id = ?`, proposalID,
				).Scan(&p.Status, &p.State, &appliedAt)
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
		// A hidden membership is not named to anyone outside the room
		// (docs/adr/006). Only members vote, so a public voter list naming
		// somebody publishes the one fact their switch took down — and
		// across a patch's proposals it reassembles the member list the
		// switch removed them from. The room itself still sees every name,
		// which is what ADR 006 means by hidden-inside-the-workspace.
		//
		// The row stays. Substituting the name rather than dropping the
		// ballot is what keeps the paragraph above true: the list is the
		// complete record, and ProposalDetail.svelte reads an empty one as
		// "no vote ever happened" to recognise a direct change. Filter it
		// and a proposal that was voted on and passed would claim it never
		// was.
		//
		// The substitution is in Go rather than in SQL like
		// displayNameExpr's, because this one turns on who is asking. A
		// tombstone is a fact about the row; a hidden membership is a fact
		// about the row *and* the viewer, and a per-viewer CASE is a worse
		// place to read that than a named boolean here.
		inRoom := viewerIsInPatchRoom(db, r, p.NodeID)

		type voterInfo struct {
			UserID      string `json:"user_id"`
			DisplayName string `json:"display_name"`
			Username    string `json:"username"`
			Value       string `json:"value"`
			Counted     bool   `json:"counted"`
		}
		var voters []voterInfo
		rows, err := db.Query(
			`SELECT v.user_id, `+displayNameExpr("u")+` as display_name, `+usernameExpr("u")+` as username, v.value,
			        CASE WHEN `+countedBallot+` THEN 1 ELSE 0 END as counted,
			        COALESCE(m.visible, 1) as membership_visible
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
				var membershipVisible bool
				if err := rows.Scan(&vi.UserID, &vi.DisplayName, &vi.Username, &vi.Value, &vi.Counted, &membershipVisible); err == nil {
					// COALESCE(...,1) above means a voter with no membership
					// row reads as visible. That is deliberate: they left, and
					// a departed voter has no membership to hide. ADR 006
					// governs a switch on a row that exists.
					if !membershipVisible && !inRoom {
						// The id goes with the name. Left in place it would
						// link one anonymous ballot to another across every
						// proposal this patch has run, which is the member
						// list again, assembled from the other end.
						vi.UserID, vi.Username = "", ""
						vi.DisplayName = HiddenMemberName
					}
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
		// (docs/adr/041). Measured against this vote's own terms
		// (docs/adr/047), so the number matches who the gate will admit.
		gc := votingTerms(db, proposalID, p.NodeID)
		recused := recusedSubject(db, p.NodeID, gc, p.TargetUserID)
		eligibleCount, _ := eligibleVotersExcept(db, p.NodeID, gc, recused)

		// Whether this viewer is in the electorate, answered by the same
		// condition VoteOnProposal gates on. The page used to work this out in
		// JavaScript from membership_role alone, which knows nothing about
		// min_voting_tenure_days — so a member inside the tenure window was
		// shown vote buttons and got a 403 on click. Deciding who may vote is
		// the server's answer to give (docs/adr/044); the client still decides
		// whether the proposal is in a state that accepts one.
		//
		// A proposal with no ballot answers no: `can_vote` is the gate's own
		// answer, and the gate refuses this one (docs/adr/053). The page also
		// hides the vote surfaces on the state, but the two must agree — a
		// payload saying you may vote on something nothing will accept a vote
		// for is the contradiction docs/adr/044 was written to end.
		//
		// And only while there is a ballot: `status = 'open'`, and a state
		// with a vote in it. A direct change used to answer true here — it is
		// born approved, and nothing above looked at that — which is the
		// same contradiction one field over.
		canVote := viewerID != "" && p.Status == "open" &&
			p.State != "elsewhere" && p.State != "awaiting_admin" &&
			inElectorateExcept(db, viewerID, p.NodeID, gc, recused)

		// The maintainer's proposal, on a maintainer's patch (docs/adr/092).
		// `advisory` says any tally here is advice, and `can_decide` is the
		// decide gate's own answer for this viewer — a patch admin, on a
		// proposal whose frozen terms are admin-decides, while it is open.
		// Both from the terms rather than the patch's live rules, like
		// everything else that decides a proposal (docs/adr/047).
		advisory := gc.DecisionMethod == "admin"
		canDecide := advisory && p.Status == "open" && viewerID != "" &&
			userHasNodeRole(db, viewerID, p.NodeID, "admin")
		var declinedBy string
		db.QueryRow(`SELECT COALESCE(`+displayNameExpr("u")+`,'') FROM proposals p
		             JOIN users u ON u.id = p.declined_by WHERE p.id = ?`, proposalID).Scan(&declinedBy)

		result := map[string]interface{}{
			"id":          p.ID,
			"node_id":     p.NodeID,
			"author_id":   p.AuthorID,
			"author_name": authorName,
			// Who the proposal is *about*, when it is about anyone — a
			// meritocratic nomination (docs/adr/051). Empty on every proposal
			// that decides a thing rather than a person.
			"target_user_id":   p.TargetUserID,
			"target_user_name": targetUserName,
			// Election fields, empty on every proposal that is not one
			// (docs/adr/051). The page needs the phase to know whether to show
			// nominations or a ballot, and both dates to say what happens next.
			"seats_contested":      seatsContested,
			"nominations_close_at": nominationsCloseAt,
			"election_phase":       electionPhase(seatsContested, nominationsCloseAt, p.Status),
			"candidates":           electionCandidates(db, proposalID, viewerID),
			"title":                p.Title,
			"body":                 p.Body,
			"status":               p.Status,
			"proposal_type":        p.ProposalType,
			"duration_hours":       p.DurationHours,
			"voting_ends_at":       p.VotingEndsAt,
			"created_at":           p.CreatedAt,
			"updated_at":           p.UpdatedAt,
			"approve_count":        approveCount,
			"reject_count":         rejectCount,
			"abstain_count":        abstainCount,
			"voters":               voters,
			"my_vote":              myVote,
			"eligible_voters":      eligibleCount,
			"can_vote":             canVote,
			// The terms this vote is judged by, so the page can say so rather
			// than leaving a refused voter to guess (docs/adr/047). Fixed when
			// voting opened — which is created_at, already in this payload.
			"voting_terms": gc,
			// The tenure actually in force, which on a patch younger than its
			// own bar is none (docs/adr/098). The page must say this number
			// and never `voting_terms.min_voting_tenure_days`, or it recites a
			// rule the gate is not running.
			"tenure_days": effectiveTenureDays(db, p.NodeID, gc),
			// And, for a member the tenure is still holding back, the day it
			// stops: "you can vote from the 15th" is the whole answer to the
			// question they are actually asking.
			"vote_eligible_at": voteEligibleAt(db, p.NodeID, viewerID, gc),
			"state":            p.State,
			"applied_at":       appliedAt,
			"advisory":         advisory,
			"can_decide":       canDecide,
			"declined_by":      declinedBy,
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
			// Whether this document was adopted elsewhere since the draft was
			// written, which makes the diff below a comparison against
			// something that has moved (docs/adr/053).
			if moved, decidedAt := amendmentGroundMoved(db, p.NodeID, p.TargetDoc, p.CreatedAt, p.Status); moved {
				result["ground_moved"] = true
				result["ground_moved_at"] = decidedAt
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

		// Get proposal's node, status, state, and voting_ends_at.
		var nodeID, status, state string
		var votingEndsAt *string
		err := db.QueryRow("SELECT node_id, status, COALESCE(state,''), voting_ends_at FROM proposals WHERE id = ?", proposalID).Scan(&nodeID, &status, &state, &votingEndsAt)
		if err != nil {
			http.Error(w, `{"error":"proposal not found"}`, http.StatusNotFound)
			return
		}

		if status != "open" {
			http.Error(w, `{"error":"proposal is not open for voting"}`, http.StatusBadRequest)
			return
		}

		// A proposal on a patch that decides elsewhere is open and has no
		// ballot (docs/adr/053). It is still `status = 'open'` — it can be
		// discussed and withdrawn — so the state is what refuses here. Read
		// from the row rather than from the patch's current venue: a patch
		// that flips the venue does not reach back into proposals already
		// raised, the same rule docs/adr/047 applies to terms.
		if state == "elsewhere" {
			http.Error(w, `{"error":"this patch decides elsewhere; the decision is recorded here after it is made"}`, http.StatusConflict)
			return
		}

		// Waiting on the maintainer (docs/adr/092): open, discussable, and
		// carrying no ballot until an admin opens one.
		if state == "awaiting_admin" {
			http.Error(w, `{"error":"this proposal is waiting on the maintainer; there is no vote open on it"}`, http.StatusConflict)
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

		// This vote's terms, not the patch's current rules (docs/adr/047):
		// someone who was eligible when voting opened stays eligible for it,
		// and someone who became eligible afterward votes on the next one.
		gc := votingTerms(db, proposalID, nodeID)

		// Who this proposal is about, if anyone, and whether its terms recuse
		// them (docs/adr/051). Read from the frozen terms like everything else
		// that decides the vote: a patch switching recusal on mid-vote does not
		// retroactively void a ballot already cast (docs/adr/047).
		var voteTargetUser string
		db.QueryRow("SELECT COALESCE(target_user_id,'') FROM proposals WHERE id = ?", proposalID).Scan(&voteTargetUser)
		voteRecused := recusedSubject(db, nodeID, gc, voteTargetUser)

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
		if denial := electorateDenialExcept(db, user.ID, nodeID, gc, voteRecused); denial != "" {
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
		//
		// Never on an advisory vote (docs/adr/092): the sole voter there is
		// not who decides, so their ballot settles nothing early.
		if req.Value != "abstain" && gc.DecisionMethod != "admin" {
			if _, sole := eligibleVotersExcept(db, nodeID, gc, voteRecused); sole == user.ID {
				resolveProposal(db, proposalID)
			}
		}

		// Broadcast vote.
		//
		// A hidden membership must not travel as a fact *or as an inference*
		// (docs/adr/006). Only a member of this patch may vote on its
		// proposals, so a gv:Vote carrying an actor and a node is a
		// membership assertion in all but name — the roster three hundred
		// lines up substitutes HiddenMemberName for precisely that reason,
		// and the wire used to announce the name it had just withheld.
		//
		// The patch-level gate is the same statement drawn wider: where the
		// record is not published, no ballot leaves at all, whoever cast it
		// (docs/adr/2026-09-18-the-default-should-match-the-assumption.md,
		// which corrects docs/adr/095 decision 7 on the same reasoning).
		//
		// Suppressed rather than anonymized. An activity with the actor
		// stripped still says somebody in this patch voted approve at 14:03,
		// which against the patch's own followers collection is often enough
		// to re-identify on a small patch; and the outcome federates anyway,
		// as counts, when the proposal resolves. See VoteToActivity.
		//
		// Synchronous for the same reason as the create broadcast above: a
		// suppression that cannot be asserted on is a suppression that can be
		// deleted without a test noticing.
		if governanceRecordIsPublic(db, nodeID) && !membershipHidden(db, nodeID, user.ID) {
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
		}

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
		var seatsContested int
		err := db.QueryRow("SELECT author_id, node_id, status, seats_contested FROM proposals WHERE id = ?", proposalID).Scan(&authorID, &nodeID, &currentStatus, &seatsContested)
		if err != nil {
			http.Error(w, `{"error":"proposal not found"}`, http.StatusNotFound)
			return
		}

		if currentStatus != "open" {
			http.Error(w, `{"error":"can only withdraw open proposals"}`, http.StatusBadRequest)
			return
		}

		// Nobody calls an election and nobody closes one (docs/adr/051). The
		// calendar opens it, and `systemAuthorFor` names the longest-standing
		// admin as its author only because a record needs one — that is a
		// stand-in for the calendar, not somebody who raised a proposal, and it
		// must not carry an author's power to take it back.
		//
		// Left open this was a live bypass, not a cosmetic button. Any admin
		// could withdraw the contest; `scheduleFor` opens one whenever none is
		// open and the council is due, so the next sweep reopened it with a
		// fresh nomination window — and `status = 'withdrawn'` is not
		// 'rejected', so the anti-retry breather never applied either. A
		// council facing an election it might lose could withdraw it on every
		// pass and hold its seats forever, with holdover doing the rest.
		if seatsContested > 0 {
			http.Error(w, `{"error":"an election cannot be withdrawn: the calendar opens it and the electorate closes it (docs/adr/051)"}`, http.StatusConflict)
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

		// A patch admin's act. No `user.Role == "admin"` bypass: an instance
		// admin "curates instance-wide options; does not override per-patch
		// choices" (CONTEXT.md), and making a patch's decision official is
		// the most per-patch choice there is.
		if !userHasNodeRole(db, user.ID, p.NodeID, "admin") {
			http.Error(w, `{"error":"only this patch's admins can make proposals official"}`, http.StatusForbidden)
			return
		}

		// Two things an admin may apply: a proposal the electorate approved,
		// on any patch — the approved → in_effect step — and an open one on
		// an admin-decides patch, where applying is the decision itself
		// (docs/adr/092). Nothing else. This once accepted any open proposal
		// from any admin, which on a majority patch was an apply-before-the-
		// vote-ends bypass the rules never granted; docs/adr/041 says every
		// voting method votes, admins included, and the endpoint now agrees.
		terms := votingTerms(db, proposalID, p.NodeID)
		electorateApproved := p.State == "approved" || p.Status == "approved"
		maintainersToDecide := terms.DecisionMethod == "admin" && p.Status == "open" &&
			(p.State == "voting" || p.State == "awaiting_admin")
		if !electorateApproved && !maintainersToDecide {
			if p.Status == "open" {
				http.Error(w, `{"error":"this patch decides proposals by vote; wait for the window to close"}`, http.StatusConflict)
				return
			}
			http.Error(w, `{"error":"proposal cannot be applied in its current state"}`, http.StatusBadRequest)
			return
		}

		if err := applyProposalChanges(db, p, user); err != nil {
			log.Printf("proposal %s: apply failed: %v", proposalID, err)
			http.Error(w, `{"error":"`+applyFailureMessage+`"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "proposal.applied", "proposal", proposalID, "{}", clientIP(r))
		notifyProposalApplied(db, p.NodeID, proposalID, user.ID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "state": "in_effect"})
	}
}

// applyProposalChanges makes a proposal official: merges an amendment's
// branch and runs the post-merge syncs (docs/adr/011), then moves the row to
// approved / in_effect stamped with who applied it. Shared by the
// approved → in_effect step and the maintainer's approve (docs/adr/092) so
// there is one apply path, not two that drift.
func applyProposalChanges(db *database.DB, p model.Proposal, actor *model.User) error {
	sha := ""
	if p.ProposalType == "amendment" && p.ProposedBranch != "" {
		dataDir := governance.GetDataDir()
		if err := ensureAmendmentBranch(dataDir, p.NodeID, p.ProposedBranch, p.TargetDoc, p.ProposedBody); err != nil {
			return err
		}
		actorName, actorEmail := commitIdentity(actor)
		merged, err := governance.MergeBranch(dataDir, p.NodeID, p.ProposedBranch, actorName, actorEmail)
		if err != nil {
			return err
		}
		// From here the change is real and no database error can take it
		// back. The sha is carried to settleApplied below rather than written
		// on its own, so the row never holds a commit without the state that
		// commit put the patch in.
		sha = merged

		if p.TargetDoc == "governance-rules.json" || p.TargetDoc == "Governance Rules" {
			if err := syncRulesAndNotify(db, dataDir, p.NodeID, actor.ID, p.ID); err != nil {
				applyIncomplete(db, p.ID, sha, "rules sync", err)
			}
		}
		// Mirror merged markdown docs into governance_docs — the DB is
		// canonical for linings (docs/adr/011); without this the applied
		// amendment never appears in the governance hub.
		if err := syncLiningToDB(db, p.NodeID, p.TargetDoc, p.ProposedTitle, p.AuthorID); err != nil {
			applyIncomplete(db, p.ID, sha, "charter mirror", err)
		}
		governance.DeleteBranch(dataDir, p.NodeID, p.ProposedBranch)
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if err := settleApplied(db, p.ID, sha, actor.ID, now); err != nil {
		// The caller turns this into the admin's error message, which says
		// nothing has changed yet. Where a merge did land that is no longer
		// true, so the divergence is recorded before the error goes back.
		if sha != "" {
			applyIncomplete(db, p.ID, sha, "settle", err)
		}
		return err
	}
	return nil
}

// settleApplied stamps a proposal the moment its change is real: the commit
// the merge produced, and the row state that commit put the patch in, written
// together in one transaction. Written separately they can disagree — a sha
// with no state, a state with no sha — and every page renders from the row,
// so a half-written settle shows an open vote over a charter that has already
// been amended. Callers that merged nothing pass an empty sha and only the
// row moves.
//
// `at` is the caller's timestamp rather than one taken here, because a direct
// change stamps the row with the moment it was created.
//
// COALESCE leaves applied_by alone where no person applied it: an
// auto-applied amendment and a ratified nomination were settled by the
// electorate and the clock, and naming somebody would misstate who decided.
func settleApplied(db *database.DB, proposalID, sha, appliedBy, at string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if sha != "" {
		if _, err := tx.Exec("UPDATE proposals SET git_sha = ? WHERE id = ?", sha, proposalID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(
		"UPDATE proposals SET status = 'approved', state = 'in_effect', applied_at = ?, applied_by = COALESCE(?, applied_by), updated_at = ? WHERE id = ?",
		at, nullIfEmpty(appliedBy), at, proposalID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// applyIncomplete records a write that failed after the change it was meant
// to describe already landed — a merge that is in git, a member already
// promoted. SQLite cannot roll git back and this does not try to. What it
// must not do is go quiet: the divergence that follows is invisible from both
// sides, with `proposals` still saying approved or open while the charter
// reads amended, and an admin's next apply merging a branch that is already
// in.
//
// So it is loud in the log and leaves an audit entry an operator can search
// for, carrying the proposal and the commit the row does not.
func applyIncomplete(db *database.DB, proposalID, sha, step string, cause error) {
	merge := ""
	if sha != "" {
		merge = " (merge " + sha + ")"
	}
	log.Printf("proposal %s: %s failed after the change landed%s; the governance record and the proposal row now disagree: %v",
		proposalID, step, merge, cause)
	detail, err := json.Marshal(map[string]string{"step": step, "git_sha": sha, "error": cause.Error()})
	if err != nil {
		detail = []byte("{}")
	}
	auth.LogAuditEvent(db, "", "proposal.apply_incomplete", "proposal", proposalID, string(detail), "")
}

// ensureAmendmentBranch makes sure the branch an amendment merges from is
// there before anything tries to merge it.
//
// A rebuilt repo has `main` and nothing else (docs/adr/084): `Repair` writes
// what the `governance_docs` rows attest to, and a pending amendment's
// proposed text has no row — it lives in `proposals.proposed_body`, which is
// canonical all the same (docs/adr/011). So a patch restored from its
// database alone held decisions its members had taken and could never enact,
// and the button that was supposed to enact them answered with a git error
// about a ref. The text was never lost; only the mirror of it was, and the
// mirror is the derived half.
//
// A branch that is still there is used as it is. This creates an absence and
// overwrites nothing.
func ensureAmendmentBranch(dataDir, nodeID, branch, targetDoc, proposedBody string) error {
	if branch == "" || targetDoc == "" {
		return nil
	}
	created, err := governance.EnsureBranch(dataDir, nodeID, branch, targetDoc, proposedBody)
	if err != nil {
		return err
	}
	if created {
		log.Printf("governance: reconstructed branch %s for node %s from the proposal's canonical text", branch, nodeID)
	}
	return nil
}

// applyFailureMessage is what a person sees when making a decision official
// does not work. The Go error goes to the log, where somebody can act on it;
// what reaches the admin says what state their patch is in and what to do
// next, because "branch amendment-01a0a26e not found: reference not found"
// is the app talking to itself in front of them.
const applyFailureMessage = "This change could not be written to the patch's governance record, so nothing has changed yet. The decision still stands and you can try again. If it keeps failing, your instance admin can run the governance repair (patchwork -repair-governance) with the server stopped."

// notifyProposalApplied tells the patch a proposal is now in effect.
func notifyProposalApplied(db *database.DB, nodeID, proposalID, actorID string) {
	var nodeSlug, nodeName, proposalTitle string
	db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlug, &nodeName)
	db.QueryRow("SELECT title FROM proposals WHERE id = ?", proposalID).Scan(&proposalTitle)
	notify(notifications.Event{
		Type:     notifications.ProposalApplied,
		NodeID:   nodeID,
		NodeSlug: nodeSlug,
		NodeName: nodeName,
		ActorID:  actorID,
		EntityID: proposalID,
		Title:    "Change applied: " + proposalTitle,
		Body:     "This proposal is now in effect.",
		Link:     weblink.Proposal(nodeSlug, proposalID),
	})
}
