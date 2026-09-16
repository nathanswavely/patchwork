package handler_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// A notification follows an obligation, not an event (docs/adr/093). These
// cover the pass that finds the people who may vote on an open proposal and
// have not been told, and the two chasing notices addressed to whoever still
// owes a ballot.

// outreachRig is a patch with an admin and one member, both long-standing,
// and one open proposal already announced to them.
type outreachRig struct {
	db         *database.DB
	nodeID     string
	proposalID string
	adminID    string
	memberID   string
	tokens     map[string]string
}

func newOutreachRig(t *testing.T, slug string, quorum, tenureDays, durationHours int) outreachRig {
	t.Helper()
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })

	rig := outreachRig{db: db, tokens: map[string]string{}}
	admin, adminTok := createTestUser(t, db, slug+"_admin", "member")
	rig.adminID = admin.ID
	rig.tokens[admin.ID] = adminTok
	rig.nodeID = createTestNode(t, db, admin.ID, "Outreach "+slug, slug, "open")
	createTestMembership(t, db, admin.ID, rig.nodeID, "admin", "active")

	member, memberTok := createTestUser(t, db, slug+"_m1", "member")
	rig.memberID = member.ID
	rig.tokens[member.ID] = memberTok
	createTestMembership(t, db, member.ID, rig.nodeID, "member", "active")

	// Everyone here joined long ago, so tenure is never what decides these
	// cases unless a test arranges it.
	db.Exec(`UPDATE memberships SET joined_at = '2020-01-01T00:00:00.000Z' WHERE node_id = ?`, rig.nodeID)
	db.Exec(`UPDATE nodes SET created_at = '2020-01-01T00:00:00.000Z', governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":`+itoaT(quorum)+`,"default_vote_duration_hours":`+itoaT(durationHours)+
			`,"amendment_threshold":"majority","min_voting_tenure_days":`+itoaT(tenureDays)+`}`,
		rig.nodeID)

	rig.proposalID = createTestProposal(t, db, rig.nodeID, admin.ID)
	rig.setRemaining(t, time.Duration(durationHours)*time.Hour)
	db.Exec(`UPDATE proposals SET state = 'voting', duration_hours = ? WHERE id = ?`, durationHours, rig.proposalID)
	// The announcement the create path would have made: everyone who could
	// vote at that moment is on record as told.
	handler.SeedVoteAudience(db, rig.proposalID, []string{admin.ID, member.ID})
	return rig
}

// setRemaining moves the deadline so that this much of the window is left.
func (r outreachRig) setRemaining(t *testing.T, left time.Duration) {
	t.Helper()
	ends := time.Now().UTC().Add(left).Format("2006-01-02T15:04:05.000Z")
	r.db.Exec(`UPDATE proposals SET voting_ends_at = ? WHERE id = ?`, ends, r.proposalID)
}

// joinLate adds somebody to the patch after the vote was announced.
func (r outreachRig) joinLate(t *testing.T, username, role string, joinedAt string) string {
	t.Helper()
	u, tok := createTestUser(t, r.db, username, "member")
	r.tokens[u.ID] = tok
	memID := createTestMembership(t, r.db, u.ID, r.nodeID, role, "active")
	if joinedAt != "" {
		r.db.Exec(`UPDATE memberships SET joined_at = ? WHERE id = ?`, joinedAt, memID)
	}
	return u.ID
}

func (r outreachRig) countOfType(t *testing.T, userID string, typ notifications.NotificationType) int {
	t.Helper()
	var n int
	r.db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND type = ?`, userID, string(typ)).Scan(&n)
	return n
}

func (r outreachRig) bodyOfType(t *testing.T, userID string, typ notifications.NotificationType) string {
	t.Helper()
	var body string
	r.db.QueryRow(`SELECT COALESCE(body,'') FROM notifications WHERE user_id = ? AND type = ? ORDER BY created_at DESC LIMIT 1`,
		userID, string(typ)).Scan(&body)
	return body
}

// F-065. Seven of a simulated co-op's eight members never received a
// proposal.new for the rules vote, because they joined after it was raised.
// They did not ignore the vote; they were never told it existed.
func TestVoteNotices_SomebodyWhoJoinsLateIsTold(t *testing.T) {
	rig := newOutreachRig(t, "vn-late", 50, 0, 336)
	latecomer := rig.joinLate(t, "vn-late_new", "member", "")

	handler.SweepVoteNotices(rig.db)

	if got := rig.countOfType(t, latecomer, notifications.ProposalOpenToYou); got != 1 {
		t.Fatalf("latecomer got %d open_to_you notices, want 1", got)
	}
	// And the people who were in the room when it was announced are not told
	// a second time.
	for _, id := range []string{rig.adminID, rig.memberID} {
		if got := rig.countOfType(t, id, notifications.ProposalOpenToYou); got != 0 {
			t.Errorf("an incumbent got %d open_to_you notices, want 0", got)
		}
	}
}

// Told once. The pass runs hourly; a voting window is days long.
func TestVoteNotices_TheLatecomerIsToldExactlyOnce(t *testing.T) {
	rig := newOutreachRig(t, "vn-once", 50, 0, 336)
	latecomer := rig.joinLate(t, "vn-once_new", "member", "")

	handler.SweepVoteNotices(rig.db)
	handler.SweepVoteNotices(rig.db)
	handler.SweepVoteNotices(rig.db)

	if got := rig.countOfType(t, latecomer, notifications.ProposalOpenToYou); got != 1 {
		t.Fatalf("after three passes the latecomer has %d open_to_you notices, want 1", got)
	}
}

// A follower has taken on no obligation, so there is nothing to tell them
// (docs/adr/044, docs/adr/093). They are not in the electorate, and the pass
// reads the electorate.
func TestVoteNotices_AFollowerIsNeverTold(t *testing.T) {
	rig := newOutreachRig(t, "vn-follow", 50, 0, 336)
	follower := rig.joinLate(t, "vn-follow_f", "follower", "")
	rig.setRemaining(t, 6*time.Hour)

	handler.SweepVoteNotices(rig.db)

	for _, typ := range []notifications.NotificationType{
		notifications.ProposalOpenToYou, notifications.ProposalTurnout, notifications.ProposalDeadline,
	} {
		if got := rig.countOfType(t, follower, typ); got != 0 {
			t.Errorf("a follower received %d %s notices, want 0", got, typ)
		}
	}
}

// Eligibility can begin after joining and with no membership event at all: a
// voting tenure comes due on a clock (docs/adr/098). The pass must find that
// person on the day they enter the electorate, not before.
func TestVoteNotices_TenureNotYetStartedWaits(t *testing.T) {
	// A patch older than its own thirty-day bar, so the bar is really in
	// force (docs/adr/098), and a member three days old.
	rig := newOutreachRig(t, "vn-tenure", 50, 30, 336)
	threeDaysAgo := time.Now().UTC().Add(-3 * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	newcomer := rig.joinLate(t, "vn-tenure_new", "member", threeDaysAgo)

	handler.SweepVoteNotices(rig.db)
	if got := rig.countOfType(t, newcomer, notifications.ProposalOpenToYou); got != 0 {
		t.Fatalf("a member inside the tenure window got %d open_to_you notices, want 0", got)
	}

	// Their tenure comes due. Nothing happened to their membership; the
	// calendar moved.
	longAgo := time.Now().UTC().Add(-40 * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	rig.db.Exec(`UPDATE memberships SET joined_at = ? WHERE user_id = ? AND node_id = ?`, longAgo, newcomer, rig.nodeID)

	handler.SweepVoteNotices(rig.db)
	if got := rig.countOfType(t, newcomer, notifications.ProposalOpenToYou); got != 1 {
		t.Fatalf("after tenure came due the member got %d open_to_you notices, want 1", got)
	}
}

// F-064, first half: tell people while it still matters, and say where the
// vote stands. The midpoint notice fires with days left, not hours.
func TestVoteNotices_MidWindowTurnoutSaysTheNumbers(t *testing.T) {
	// A fortnight's window, quorum 50 %, four in the electorate: three more
	// are needed and one has voted.
	rig := newOutreachRig(t, "vn-turnout", 50, 0, 336)
	rig.joinLate(t, "vn-turnout_a", "member", "2020-01-01T00:00:00.000Z")
	rig.joinLate(t, "vn-turnout_b", "member", "2020-01-01T00:00:00.000Z")

	if code := castVote(t, rig.db, rig.proposalID, rig.tokens[rig.memberID], "approve"); code != http.StatusOK {
		t.Fatalf("vote: %d", code)
	}
	// Six days left of a fourteen-day window: past the midpoint, and well
	// clear of the last 24 hours.
	rig.setRemaining(t, 6*24*time.Hour+time.Hour)

	handler.SweepVoteNotices(rig.db)

	body := rig.bodyOfType(t, rig.adminID, notifications.ProposalTurnout)
	if !strings.Contains(body, "Votes cast: 1 of 4.") {
		t.Errorf("turnout notice body %q does not name the turnout", body)
	}
	if !strings.Contains(body, "Quorum needs 2.") {
		t.Errorf("turnout notice body %q does not name what quorum needs", body)
	}
	if !strings.Contains(body, "6 days") {
		t.Errorf("turnout notice body %q does not say how long is left", body)
	}
	// The person who voted has discharged the obligation.
	if got := rig.countOfType(t, rig.memberID, notifications.ProposalTurnout); got != 0 {
		t.Errorf("somebody who already voted got %d turnout notices, want 0", got)
	}
}

// An abstention is a vote for this purpose: they turned up and said so.
func TestVoteNotices_AnAbstentionDischargesTheObligation(t *testing.T) {
	rig := newOutreachRig(t, "vn-abstain", 50, 0, 336)
	if code := castVote(t, rig.db, rig.proposalID, rig.tokens[rig.memberID], "abstain"); code != http.StatusOK {
		t.Fatalf("abstain: %d", code)
	}
	rig.setRemaining(t, 6*time.Hour)

	handler.SweepVoteNotices(rig.db)

	if got := rig.countOfType(t, rig.memberID, notifications.ProposalDeadline); got != 0 {
		t.Errorf("an abstainer got %d deadline notices, want 0", got)
	}
	// The author is not exempt: they can vote and have not.
	if got := rig.countOfType(t, rig.adminID, notifications.ProposalDeadline); got != 1 {
		t.Errorf("the author got %d deadline notices, want 1", got)
	}
}

// The dedupe holds across passes, and the two chasing notices have separate
// keys so the second is not swallowed by the first.
func TestVoteNotices_DedupeHoldsAcrossPasses(t *testing.T) {
	rig := newOutreachRig(t, "vn-dedupe", 50, 0, 336)
	rig.setRemaining(t, 6*24*time.Hour)

	handler.SweepVoteNotices(rig.db)
	handler.SweepVoteNotices(rig.db)
	if got := rig.countOfType(t, rig.adminID, notifications.ProposalTurnout); got != 1 {
		t.Fatalf("after two passes: %d turnout notices, want 1", got)
	}

	rig.setRemaining(t, 6*time.Hour)
	handler.SweepVoteNotices(rig.db)
	handler.SweepVoteNotices(rig.db)
	if got := rig.countOfType(t, rig.adminID, notifications.ProposalDeadline); got != 1 {
		t.Fatalf("after two more passes: %d deadline notices, want 1", got)
	}
	if got := rig.countOfType(t, rig.adminID, notifications.ProposalTurnout); got != 1 {
		t.Errorf("the deadline pass added %d turnout notices, want the original 1", got)
	}
}

// The midpoint notice exists because a vote is short of quorum. Where quorum
// is already met it has no news and is not sent.
func TestVoteNotices_NoMidWindowNudgeOnceQuorumIsMet(t *testing.T) {
	rig := newOutreachRig(t, "vn-quorum", 50, 0, 336)
	if code := castVote(t, rig.db, rig.proposalID, rig.tokens[rig.memberID], "approve"); code != http.StatusOK {
		t.Fatalf("vote: %d", code)
	}
	rig.setRemaining(t, 6*24*time.Hour)

	handler.SweepVoteNotices(rig.db)

	if got := rig.countOfType(t, rig.adminID, notifications.ProposalTurnout); got != 0 {
		t.Errorf("a vote already at quorum sent %d turnout notices, want 0", got)
	}
}

// The per-patch category switch gates every one of these, and nothing is
// written down as sent while it is off — or the switch coming back on would
// find everybody already reminded.
func TestVoteNotices_CategoryOffSendsNothing(t *testing.T) {
	rig := newOutreachRig(t, "vn-cat", 50, 0, 336)
	notifications.SetCategoryEnabled(rig.db, rig.nodeID, notifications.CategoryProposals, false)
	latecomer := rig.joinLate(t, "vn-cat_new", "member", "")
	rig.setRemaining(t, 6*time.Hour)

	handler.SweepVoteNotices(rig.db)

	var total int
	rig.db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE type IN ('proposal.open_to_you','proposal.turnout','proposal.deadline')`).Scan(&total)
	if total != 0 {
		t.Fatalf("a patch with proposals notifications off sent %d vote notices, want 0", total)
	}
	var marked int
	rig.db.QueryRow(`SELECT COUNT(*) FROM notification_reminders_sent WHERE entity_type = 'proposal_voter' AND entity_id LIKE ?`,
		rig.proposalID+":%").Scan(&marked)
	if marked != 2 {
		t.Errorf("reminders recorded while the category was off: %d rows, want only the 2 seeded at announcement", marked)
	}
	notifications.SetCategoryEnabled(rig.db, rig.nodeID, notifications.CategoryProposals, true)
	handler.SweepVoteNotices(rig.db)
	if got := rig.countOfType(t, latecomer, notifications.ProposalOpenToYou); got != 1 {
		t.Errorf("after the switch came back on the latecomer got %d open_to_you notices, want 1", got)
	}
}

// A vote that was already running when this shipped carries no record of who
// it was announced to. It must not announce itself again to a whole patch.
func TestVoteNotices_AnUnseededVoteIsNotReannounced(t *testing.T) {
	rig := newOutreachRig(t, "vn-seed", 50, 0, 336)
	rig.db.Exec(`DELETE FROM notification_reminders_sent WHERE entity_id = ? OR entity_id LIKE ?`,
		rig.proposalID, rig.proposalID+":%")

	handler.SweepVoteNotices(rig.db)
	handler.SweepVoteNotices(rig.db)

	for _, id := range []string{rig.adminID, rig.memberID} {
		if got := rig.countOfType(t, id, notifications.ProposalOpenToYou); got != 0 {
			t.Errorf("a pre-existing vote announced itself again: %d notices", got)
		}
	}
}

// The create path writes down who its announcement reached, so the first
// sweep after a proposal is raised has nobody new to tell.
func TestVoteNotices_CreationSeedsItsOwnAudience(t *testing.T) {
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })
	admin, adminTok := createTestUser(t, db, "vn-seedcreate_admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Seed On Create", "vn-seedcreate", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	db.Exec(`UPDATE memberships SET joined_at = '2020-01-01T00:00:00.000Z' WHERE node_id = ?`, nodeID)

	code, _ := createProposal(t, db, "vn-seedcreate", adminTok, map[string]interface{}{
		"title": "Buy the kiln", "body": "It is time.", "proposal_type": "action",
	})
	if code != http.StatusCreated {
		t.Fatalf("create proposal: %d", code)
	}

	var seeded int
	db.QueryRow(`SELECT COUNT(*) FROM notification_reminders_sent
	             WHERE entity_type = 'proposal_voter' AND reminder_type = 'announced'`).Scan(&seeded)
	if seeded != 1 {
		t.Fatalf("creation recorded %d announced rows, want 1 (the admin)", seeded)
	}

	handler.SweepVoteNotices(db)
	var told int
	db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE type = 'proposal.open_to_you'`).Scan(&told)
	if told != 0 {
		t.Errorf("the first sweep re-announced a fresh proposal to %d people, want 0", told)
	}
}
