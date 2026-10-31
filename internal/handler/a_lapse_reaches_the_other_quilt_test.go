package handler_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// A lapse returned before the ResolveProposal broadcast, so a remote follower
// sent the Create for a proposal was never told it closed, and it stayed open
// for ever on the other quilt (F-081). A lapse now goes out like any other
// close, as "lapsed" rather than "rejected", because nobody rejected it
// (docs/adr/097).
func TestLapse_ReachesRemoteFollowers(t *testing.T) {
	rig := newSweepRig(t, "swp-fed", 50)
	db := rig.db
	if _, err := db.Exec(`UPDATE nodes SET visibility = 'public' WHERE id = ?`, rig.nodeID); err != nil {
		t.Fatalf("public: %v", err)
	}
	openGovernanceRecord(t, db, rig.nodeID)
	if _, err := db.Exec(`INSERT INTO ap_followers (id, local_actor_type, local_actor_id, remote_actor_id, remote_inbox, accepted)
	         VALUES (?, 'node', ?, 'https://remote.example/ap/instance', 'https://remote.example/ap/inbox', 1)`,
		auth.NewUUIDv7(), rig.nodeID); err != nil {
		t.Fatalf("follower: %v", err)
	}

	// One of four eligible: 25 %, under the 50 % quorum.
	if code := castVote(t, db, rig.proposalID, rig.tokens[1], "approve"); code != http.StatusOK {
		t.Fatalf("vote: %d", code)
	}
	rig.closeWindow(t)

	handler.SweepProposals(db)
	if _, state := rig.statusAndState(t); state != "lapsed" {
		t.Fatalf("expected the proposal to lapse, got state %q", state)
	}

	resolutions := func() []string {
		rows, err := db.Query(`SELECT activity_json FROM ap_outbox_queue WHERE activity_json LIKE '%gv:ResolveProposal%'`)
		if err != nil {
			t.Fatalf("outbox: %v", err)
		}
		defer func() { _ = rows.Close() }()
		var out []string
		for rows.Next() {
			var a string
			if err := rows.Scan(&a); err != nil {
				t.Fatalf("scan: %v", err)
			}
			out = append(out, a)
		}
		return out
	}

	// The broadcast is on a goroutine.
	var got []string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got = resolutions(); len(got) > 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if len(got) != 1 {
		t.Fatalf("expected one ResolveProposal queued for the remote follower, got %d", len(got))
	}
	if !strings.Contains(got[0], `"gv:result":"lapsed"`) {
		t.Errorf("a lapse should federate as lapsed, not rejected: %s", got[0])
	}

	// Idempotent: a second pass lapses nothing new and sends nothing new.
	handler.SweepProposals(db)
	time.Sleep(200 * time.Millisecond)
	if n := len(resolutions()); n != 1 {
		t.Errorf("a second sweep must not federate the lapse again, got %d", n)
	}
}
