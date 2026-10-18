package handler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// A nomination takes effect where the decision lands
// (docs/adr/2026-10-18-a-nomination-takes-effect-where-the-decision-lands.md).
//
// On a voting patch resolution ratifies a nomination. On a patch whose
// decision method is admin-decides nothing resolves (docs/adr/092): the
// maintainer applies their own at creation, or approves through decide, or
// applies an open proposal. None of those three doors promoted anybody, so a
// nomination there settled `in_effect` and changed no role — and since the
// role dropdown is closed on meritocratic and elected patches, a patch that
// paired either with admin-decides had no way left to make an admin at all.

// adminDecidesNominationRig is a patch that decides by admin and makes admins
// by nomination: one admin, a nominee, and a second member so an advisory
// vote has an electorate once the nominee is recused.
type adminDecidesNominationRig struct {
	db        *database.DB
	nodeID    string
	slug      string
	adminID   string
	adminTok  string
	nomineeID string
	memberTok string
}

func newAdminDecidesNominationRig(t *testing.T, slug, leadership string) adminDecidesNominationRig {
	t.Helper()
	db := setupTestDB(t)
	admin, adminTok := createTestUser(t, db, slug+"_admin", "member")
	nominee, _ := createTestUser(t, db, slug+"_nominee", "member")
	_, memberTok := createTestUser(t, db, slug+"_member", "member")
	nodeID := createTestNode(t, db, admin.ID, "Rig "+slug, slug, "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")
	var memberID string
	db.QueryRow("SELECT id FROM users WHERE username = ?", slug+"_member").Scan(&memberID)
	createTestMembership(t, db, memberID, nodeID, "member", "active")
	db.Exec(`UPDATE memberships SET joined_at = '2020-01-01T00:00:00.000Z' WHERE node_id = ?`, nodeID)
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"admin","quorum_percent":0,"default_vote_duration_hours":72,"min_voting_tenure_days":0,`+
			`"leadership_model":"`+leadership+`","leadership_venue":"patchwork","admin_term_months":12}`,
		nodeID)
	return adminDecidesNominationRig{
		db: db, nodeID: nodeID, slug: slug,
		adminID: admin.ID, adminTok: adminTok,
		nomineeID: nominee.ID, memberTok: memberTok,
	}
}

func (rig adminDecidesNominationRig) nominate(t *testing.T, putToVote bool) map[string]interface{} {
	t.Helper()
	code, p := createProposal(t, rig.db, rig.slug, rig.adminTok, map[string]interface{}{
		"title": "Nominate for admin", "proposal_type": "membership",
		"target_user_id": rig.nomineeID, "put_to_vote": putToVote,
	})
	if code != http.StatusCreated {
		t.Fatalf("nominate: expected 201, got %d: %v", code, p)
	}
	return p
}

func proposalStateOf(t *testing.T, db *database.DB, id string) string {
	t.Helper()
	var state string
	db.QueryRow("SELECT COALESCE(state,'') FROM proposals WHERE id = ?", id).Scan(&state)
	return state
}

// ratifiedBy is the actor the membership.ratified record names, and whether
// there is one at all.
func ratifiedBy(t *testing.T, db *database.DB, proposalID string) (string, bool) {
	t.Helper()
	var actor *string
	err := db.QueryRow(`SELECT user_id FROM audit_log WHERE action = 'membership.ratified'
	                    AND metadata LIKE '%' || ? || '%'`, proposalID).Scan(&actor)
	if err != nil {
		return "", false
	}
	if actor == nil {
		return "", true
	}
	return *actor, true
}

// Door one: an admin's own nomination, applied at creation.
func TestAdminDecidesNomination_DirectChangePromotes(t *testing.T) {
	rig := newAdminDecidesNominationRig(t, "nomdirect", "meritocratic")
	p := rig.nominate(t, false)
	id := p["id"].(string)

	if got := proposalStateOf(t, rig.db, id); got != "in_effect" {
		t.Fatalf("expected the direct change in effect, got %q", got)
	}
	if got := roleOf(t, rig.db, rig.nomineeID, rig.nodeID); got != "admin" {
		t.Errorf("the maintainer's own nomination must promote, got %q", got)
	}
	if actor, ok := ratifiedBy(t, rig.db, id); !ok || actor != rig.adminID {
		t.Errorf("expected membership.ratified naming the maintainer, got %q (logged=%v)", actor, ok)
	}
}

// Door two: decide's approve, with an advisory vote running and after one
// has closed and handed the proposal back.
func TestAdminDecidesNomination_DecideApprovePromotes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		closeWindow bool
	}{
		{"mid-vote", false},
		{"after the advice is in", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newAdminDecidesNominationRig(t, "nomdecide", "meritocratic")
			id := rig.nominate(t, true)["id"].(string)
			if got := roleOf(t, rig.db, rig.nomineeID, rig.nodeID); got != "member" {
				t.Fatalf("asking the members must not promote, got %q", got)
			}
			if tc.closeWindow {
				voteOn(t, rig.db, id, rig.memberTok, "approve")
				closeWindow(t, rig.db, id)
				getProposal(t, rig.db, id, rig.adminTok)
				if got := proposalStateOf(t, rig.db, id); got != "awaiting_admin" {
					t.Fatalf("an advisory window closing hands it back, got %q", got)
				}
				if got := roleOf(t, rig.db, rig.nomineeID, rig.nodeID); got != "member" {
					t.Fatalf("advice is not a decision, got %q", got)
				}
			}

			if code, body := decideOn(t, rig.db, id, rig.adminTok, "approve"); code != http.StatusOK {
				t.Fatalf("decide approve: got %d: %s", code, body)
			}
			if got := roleOf(t, rig.db, rig.nomineeID, rig.nodeID); got != "admin" {
				t.Errorf("the maintainer approving must promote, got %q", got)
			}
			if actor, ok := ratifiedBy(t, rig.db, id); !ok || actor != rig.adminID {
				t.Errorf("expected membership.ratified naming the maintainer, got %q (logged=%v)", actor, ok)
			}
		})
	}
}

// Declining is a decision too, and promotes nobody.
func TestAdminDecidesNomination_DecideDeclineDoesNotPromote(t *testing.T) {
	rig := newAdminDecidesNominationRig(t, "nomdecline", "meritocratic")
	id := rig.nominate(t, true)["id"].(string)
	if code, body := decideOn(t, rig.db, id, rig.adminTok, "decline"); code != http.StatusOK {
		t.Fatalf("decide decline: got %d: %s", code, body)
	}
	if got := roleOf(t, rig.db, rig.nomineeID, rig.nodeID); got != "member" {
		t.Errorf("a declined nomination must not promote, got %q", got)
	}
	if _, ok := ratifiedBy(t, rig.db, id); ok {
		t.Error("a declined nomination must not be recorded as ratified")
	}
}

// Door three: apply on an open proposal, which on this patch is the
// maintainer's decision (docs/adr/092).
func TestAdminDecidesNomination_ApplyOpenPromotes(t *testing.T) {
	rig := newAdminDecidesNominationRig(t, "nomapply", "meritocratic")
	id := rig.nominate(t, true)["id"].(string)
	if code, body := applyOn(t, rig.db, id, rig.adminTok); code != http.StatusOK {
		t.Fatalf("apply: got %d: %s", code, body)
	}
	if got := roleOf(t, rig.db, rig.nomineeID, rig.nodeID); got != "admin" {
		t.Errorf("applying the nomination must promote, got %q", got)
	}
}

// A nominee who left before the maintainer decided is not dragged back, on
// these doors any more than on resolution's.
func TestAdminDecidesNomination_SkipsDepartedNominee(t *testing.T) {
	rig := newAdminDecidesNominationRig(t, "nomleft", "meritocratic")
	id := rig.nominate(t, true)["id"].(string)
	rig.db.Exec("UPDATE memberships SET status = 'left' WHERE user_id = ? AND node_id = ?", rig.nomineeID, rig.nodeID)

	if code, body := decideOn(t, rig.db, id, rig.adminTok, "approve"); code != http.StatusOK {
		t.Fatalf("decide approve: got %d: %s", code, body)
	}
	var role, status string
	rig.db.QueryRow("SELECT role, status FROM memberships WHERE user_id = ? AND node_id = ?", rig.nomineeID, rig.nodeID).Scan(&role, &status)
	if role == "admin" || status != "left" {
		t.Errorf("a departed nominee must stay departed, got role=%q status=%q", role, status)
	}
}

// On an elected patch the approval seats the nominee in the vacant chair
// with the chair's own remaining term (docs/adr/051, docs/adr/100), whoever
// decided it.
func TestAdminDecidesNomination_ElectedSeatsIntoTheVacancy(t *testing.T) {
	for _, door := range []string{"direct", "decide", "apply"} {
		t.Run(door, func(t *testing.T) {
			rig := newAdminDecidesNominationRig(t, "nomseat"+door, "elected")
			rig.db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, ?, ?)`,
				auth.NewUUIDv7(), rig.nodeID, rig.adminID, time.Now().UTC().AddDate(0, 12, 0).Format("2006-01-02"))
			remainder := time.Now().UTC().AddDate(0, 3, 0).Format("2006-01-02")
			seat := makeVacantSeat(t, rig.db, rig.nodeID, remainder)

			id := rig.nominate(t, door != "direct")["id"].(string)
			switch door {
			case "decide":
				if code, body := decideOn(t, rig.db, id, rig.adminTok, "approve"); code != http.StatusOK {
					t.Fatalf("decide approve: got %d: %s", code, body)
				}
			case "apply":
				if code, body := applyOn(t, rig.db, id, rig.adminTok); code != http.StatusOK {
					t.Fatalf("apply: got %d: %s", code, body)
				}
			}

			if got := roleOf(t, rig.db, rig.nomineeID, rig.nodeID); got != "admin" {
				t.Fatalf("expected the nominee promoted, got %q", got)
			}
			holder, termEnds := seatRow(t, rig.db, seat)
			if holder != rig.nomineeID {
				t.Errorf("expected the nominee seated in the vacant chair, got holder %q", holder)
			}
			if len(termEnds) < 10 || termEnds[:10] != remainder {
				t.Errorf("the appointee serves the seat's remainder, got term end %q want %q", termEnds, remainder)
			}
		})
	}
}

// On a voting patch the approved → in_effect apply step does not ratify a
// second time. Resolution already did, and an approval that found no chair
// is settled as such (docs/adr/100) rather than carried out later by
// whoever presses apply.
func TestVotingNomination_ResolutionStillRatifies(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "nomvoteadmin", "member")
	nodeID := meritocraticNode(t, db, admin.ID, "Nom Vote", "nom-vote")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	nominee, _ := createTestUser(t, db, "nomvotenominee", "member")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

	_, p := createProposal(t, db, "nom-vote", adminToken, map[string]interface{}{
		"title": "Nominate", "proposal_type": "membership", "target_user_id": nominee.ID,
	})
	id := p["id"].(string)
	voteOn(t, db, id, adminToken, "approve")
	expireProposal(t, db, id)
	getProposal(t, db, id, adminToken)

	if got := roleOf(t, db, nominee.ID, nodeID); got != "admin" {
		t.Fatalf("expected resolution to ratify, got %q", got)
	}
	if actor, ok := ratifiedBy(t, db, id); !ok || actor != "" {
		t.Errorf("a ratification by the electorate names nobody, got %q (logged=%v)", actor, ok)
	}
}
