package handler_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// A seat is a chair you can count (docs/adr/100).
//
// Two founders of elected patches made colleagues admins from the dropdown on
// Settings → Members. The site allowed it: three role changes, zero new
// seats, no election, and a 2027 contest for one seat while three people held
// power. These tests hold the two halves of the fix apart — the furniture is
// administrative, sitting in it is not.

// seatRow reads a seat straight from the table, since the whole question is
// what the row says.
func seatRow(t *testing.T, db *database.DB, seatID string) (holder, termEnds string) {
	t.Helper()
	db.QueryRow(`SELECT COALESCE(holder_id,''), COALESCE(term_ends_at,'') FROM seats WHERE id = ?`, seatID).
		Scan(&holder, &termEnds)
	return
}

func seatIDs(t *testing.T, db *database.DB, nodeID string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT id FROM seats WHERE node_id = ? ORDER BY created_at ASC`, nodeID)
	if err != nil {
		t.Fatalf("seats: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

func makeVacantSeat(t *testing.T, db *database.DB, nodeID, termEnds string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	if _, err := db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, NULL, ?)`,
		id, nodeID, termEnds); err != nil {
		t.Fatalf("insert seat: %v", err)
	}
	return id
}

func promoteVia(t *testing.T, db *database.DB, slug, userID, token string) (int, string) {
	t.Helper()
	r := authedRequest("PATCH", "/api/v1/nodes/"+slug+"/members/"+userID,
		map[string]interface{}{"role": "admin"}, token)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)
	return w.Code, w.Body.String()
}

func TestAddSeat_PatchAdminOnly(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "seatfounder", "member")
	nodeID := electedNode(t, db, admin.ID, "Seat Hall", "seat-hall", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	member, memberToken := createTestUser(t, db, "seatmember", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	follower, followerToken := createTestUser(t, db, "seatfollower", "member")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")
	_, instanceToken := createTestUser(t, db, "seatsiteadmin", "admin")

	// The patch's own admin may.
	r := authedRequest("POST", "/api/v1/nodes/seat-hall/seats", nil, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/seats", handler.AddSeat(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("patch admin adding a seat: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if got := len(seatIDs(t, db, nodeID)); got != 1 {
		t.Fatalf("expected 1 seat, got %d", got)
	}

	// Nobody else does. A 403, not the noticeboard's 404: a council's seats
	// are public, so the refusal hides the button rather than the patch
	// (docs/adr/100).
	for _, c := range []struct {
		who, token string
	}{
		{"member", memberToken},
		{"follower", followerToken},
		{"instance admin with no role here", instanceToken},
	} {
		r := authedRequest("POST", "/api/v1/nodes/seat-hall/seats", nil, c.token)
		w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/seats", handler.AddSeat(db), r)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s adding a seat: expected 403, got %d: %s", c.who, w.Code, w.Body.String())
		}
	}
	if got := len(seatIDs(t, db, nodeID)); got != 1 {
		t.Errorf("expected the council still at 1 seat, got %d", got)
	}
}

// A new chair joins the council's calendar rather than starting one of its
// own: aligned seats share a date (docs/adr/051).
func TestAddSeat_InheritsTheCouncilsTermEnd(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "seatcal", "member")
	nodeID := electedNode(t, db, admin.ID, "Seat Calendar", "seat-calendar", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	founding := time.Now().UTC().AddDate(0, 8, 0).Format("2006-01-02")
	db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, ?, ?)`,
		auth.NewUUIDv7(), nodeID, admin.ID, founding)

	r := authedRequest("POST", "/api/v1/nodes/seat-calendar/seats", nil, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/seats", handler.AddSeat(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	ids := seatIDs(t, db, nodeID)
	if len(ids) != 2 {
		t.Fatalf("expected 2 seats, got %d", len(ids))
	}
	holder, termEnds := seatRow(t, db, ids[1])
	if holder != "" {
		t.Errorf("a new seat is vacant; got holder %q", holder)
	}
	if termEnds != founding {
		t.Errorf("expected the new seat to share the council's term end %q, got %q", founding, termEnds)
	}
}

func TestRemoveSeat_VacantYesHeldNo(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "seatremover", "member")
	nodeID := electedNode(t, db, admin.ID, "Seat Removal", "seat-removal", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	held := auth.NewUUIDv7()
	db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, ?, ?)`,
		held, nodeID, admin.ID, time.Now().UTC().AddDate(0, 12, 0).Format("2006-01-02"))
	vacant := makeVacantSeat(t, db, nodeID, time.Now().UTC().AddDate(0, 12, 0).Format("2006-01-02"))

	r := authedRequest("DELETE", "/api/v1/nodes/seat-removal/seats/"+vacant, nil, adminToken)
	w := serveMux(t, db, "DELETE", "/api/v1/nodes/{slug}/seats/{id}", handler.RemoveSeat(db), r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("removing a vacant seat: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// A held seat cannot be dissolved to remove its holder (docs/adr/051), and
	// the refusal names who is in it.
	r = authedRequest("DELETE", "/api/v1/nodes/seat-removal/seats/"+held, nil, adminToken)
	w = serveMux(t, db, "DELETE", "/api/v1/nodes/{slug}/seats/{id}", handler.RemoveSeat(db), r)
	if w.Code != http.StatusConflict {
		t.Fatalf("removing a held seat: expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "seatremover") {
		t.Errorf("expected the refusal to name the holder, got %s", w.Body.String())
	}
	if got := len(seatIDs(t, db, nodeID)); got != 1 {
		t.Errorf("expected the held seat to survive, got %d seats", got)
	}
}

// Seats belong to the elected model and to a patch that elects here. A
// maintainer designates, a meritocratic patch ratifies, and an elsewhere
// council comes from its attestations (docs/adr/052).
func TestSeats_RefusedOffTheElectedModelAndOffPatchwork(t *testing.T) {
	cases := []struct {
		name, gc, want string
	}{
		{"maintainer", `{"leadership_model":"maintainer"}`, "elected model"},
		{"meritocratic", `{"leadership_model":"meritocratic"}`, "elected model"},
		{"elsewhere", `{"leadership_model":"elected","leadership_venue":"elsewhere"}`, "elsewhere"},
	}
	for _, c := range cases {
		db := setupTestDB(t)
		admin, adminToken := createTestUser(t, db, "seatoff"+c.name, "member")
		nodeID := createTestNode(t, db, admin.ID, "Seat "+c.name, "seat-"+c.name, "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`, c.gc, nodeID)
		existing := makeVacantSeat(t, db, nodeID, "")

		r := authedRequest("POST", "/api/v1/nodes/seat-"+c.name+"/seats", nil, adminToken)
		w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/seats", handler.AddSeat(db), r)
		if w.Code != http.StatusConflict {
			t.Errorf("%s: adding a seat expected 409, got %d: %s", c.name, w.Code, w.Body.String())
		} else if !strings.Contains(w.Body.String(), c.want) {
			t.Errorf("%s: expected the refusal to mention %q, got %s", c.name, c.want, w.Body.String())
		}

		r = authedRequest("DELETE", "/api/v1/nodes/seat-"+c.name+"/seats/"+existing, nil, adminToken)
		w = serveMux(t, db, "DELETE", "/api/v1/nodes/{slug}/seats/{id}", handler.RemoveSeat(db), r)
		if w.Code != http.StatusConflict {
			t.Errorf("%s: removing a seat expected 409, got %d: %s", c.name, w.Code, w.Body.String())
		}
	}
}

// Nell's finding, answered: "either the election means something or the
// dropdown does" (docs/adr/100).
func TestPromoteOnElected_RefusedWithTheWayIn(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "dropadmin", "member")
	nodeID := electedNode(t, db, admin.ID, "Dropdown", "dropdown", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	member, _ := createTestUser(t, db, "dropmember", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	// A vacant seat: the answer is the nomination that fills it.
	seat := makeVacantSeat(t, db, nodeID, time.Now().UTC().AddDate(0, 12, 0).Format("2006-01-02"))
	code, body := promoteVia(t, db, "dropdown", member.ID, adminToken)
	if code != http.StatusConflict {
		t.Fatalf("promotion with a vacancy: expected 409, got %d: %s", code, body)
	}
	if !strings.Contains(body, "nominate") {
		t.Errorf("expected the refusal to point at the nomination, got %s", body)
	}

	// Council full: the answer is the date of the next contest, derived from
	// the seats rather than stored anywhere.
	db.Exec(`UPDATE seats SET holder_id = ? WHERE id = ?`, admin.ID, seat)
	code, body = promoteVia(t, db, "dropdown", member.ID, adminToken)
	if code != http.StatusConflict {
		t.Fatalf("promotion with a full council: expected 409, got %d: %s", code, body)
	}
	if !strings.Contains(body, "next contest") {
		t.Errorf("expected the refusal to name the next contest, got %s", body)
	}

	if got := roleOf(t, db, member.ID, nodeID); got != "member" {
		t.Errorf("expected no promotion to have landed, got %q", got)
	}
}

// Demotion is untouched, as it already was for meritocratic: nothing about
// electing a council says the community must vote to end somebody's role, and
// the last-admin floor is the guard that matters there.
func TestDemoteOnElected_StillWorks(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "demoteadmin", "member")
	nodeID := electedNode(t, db, admin.ID, "Demote", "demote", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	other, _ := createTestUser(t, db, "demoteother", "member")
	createTestMembership(t, db, other.ID, nodeID, "admin", "active")

	r := authedRequest("PATCH", "/api/v1/nodes/demote/members/"+other.ID,
		map[string]interface{}{"role": "member"}, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("demotion: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := roleOf(t, db, other.ID, nodeID); got != "member" {
		t.Errorf("expected the demotion to land, got %q", got)
	}
}

// Sam's proposal, refused at the door (docs/adr/100): no target, no effect,
// and a public up-or-down vote on a person's name.
func TestCreateMembershipProposal_MustNameSomebody(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "namesomebody", "member")
	nodeID := electedNode(t, db, admin.ID, "Name Somebody", "name-somebody", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	body := map[string]interface{}{"title": "I would like a seat", "proposal_type": "membership"}
	r := authedRequest("POST", "/api/v1/nodes/name-somebody/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM proposals WHERE node_id = ?", nodeID).Scan(&count)
	if count != 0 {
		t.Errorf("expected no proposal written, got %d", count)
	}
}

// A nomination on an elected patch is a mid-term appointment into a chair, so
// with no chair there is nothing to nominate into and the answer is the date.
func TestCreateNominationOnElected_NeedsAVacancy(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "electnomadmin", "member")
	nodeID := electedNode(t, db, admin.ID, "Elect Nom", "elect-nom", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	nominee, _ := createTestUser(t, db, "electnominee", "member")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

	db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, ?, ?)`,
		auth.NewUUIDv7(), nodeID, admin.ID, time.Now().UTC().AddDate(0, 12, 0).Format("2006-01-02"))

	body := map[string]interface{}{"title": "Nominate", "proposal_type": "membership", "target_user_id": nominee.ID}
	r := authedRequest("POST", "/api/v1/nodes/elect-nom/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusConflict {
		t.Fatalf("nomination with no vacancy: expected 409, got %d: %s", w.Code, w.Body.String())
	}

	// Open a chair and the same nomination is ordinary.
	makeVacantSeat(t, db, nodeID, time.Now().UTC().AddDate(0, 12, 0).Format("2006-01-02"))
	r = authedRequest("POST", "/api/v1/nodes/elect-nom/proposals", body, adminToken)
	w = serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("nomination into a vacancy: expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// The clock belongs to the seat (docs/adr/051): an appointee serves out the
// remainder, never a fresh term.
func TestRatifiedNominationOnElected_SeatsWithTheSeatsOwnTerm(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "seatratadmin", "member")
	nodeID := electedNode(t, db, admin.ID, "Seat Ratify", "seat-ratify", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	nominee, _ := createTestUser(t, db, "seatratnominee", "member")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

	// A chair with three months left on it, not twelve.
	remainder := time.Now().UTC().AddDate(0, 3, 0).Format("2006-01-02")
	seat := makeVacantSeat(t, db, nodeID, remainder)

	body := map[string]interface{}{"title": "Nominate", "proposal_type": "membership", "target_user_id": nominee.ID}
	r := authedRequest("POST", "/api/v1/nodes/seat-ratify/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create nomination: got %d: %s", w.Code, w.Body.String())
	}
	proposalID := decodeJSON(t, w)["id"].(string)

	if code := voteVia(t, db, proposalID, adminToken, "approve"); code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("vote: got %d", code)
	}
	expireProposal(t, db, proposalID)
	gr := authedRequest("GET", "/api/v1/proposals/"+proposalID, nil, adminToken)
	serveMux(t, db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), gr)

	if got := roleOf(t, db, nominee.ID, nodeID); got != "admin" {
		t.Fatalf("expected the ratified nominee promoted, got %q", got)
	}
	holder, termEnds := seatRow(t, db, seat)
	if holder != nominee.ID {
		t.Errorf("expected the nominee seated in the vacant chair, got holder %q", holder)
	}
	if termEnds != remainder {
		t.Errorf("expected the seat's own term end %q to carry over, got %q", remainder, termEnds)
	}
}

// The narrow race: the chair is gone by the time the vote closes. Nobody is
// promoted, and the patch is told rather than left with a silently dead
// approval.
func TestRatifiedNominationOnElected_NoVacancyPromotesNobody(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "noseatadmin", "member")
	nodeID := electedNode(t, db, admin.ID, "No Seat", "no-seat", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	nominee, _ := createTestUser(t, db, "noseatnominee", "member")
	createTestMembership(t, db, nominee.ID, nodeID, "member", "active")

	seat := makeVacantSeat(t, db, nodeID, time.Now().UTC().AddDate(0, 12, 0).Format("2006-01-02"))

	body := map[string]interface{}{"title": "Nominate", "proposal_type": "membership", "target_user_id": nominee.ID}
	r := authedRequest("POST", "/api/v1/nodes/no-seat/proposals", body, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create nomination: got %d: %s", w.Code, w.Body.String())
	}
	proposalID := decodeJSON(t, w)["id"].(string)

	if code := voteVia(t, db, proposalID, adminToken, "approve"); code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("vote: got %d", code)
	}
	// Somebody else took the chair while the vote ran.
	db.Exec(`UPDATE seats SET holder_id = ? WHERE id = ?`, admin.ID, seat)

	expireProposal(t, db, proposalID)
	gr := authedRequest("GET", "/api/v1/proposals/"+proposalID, nil, adminToken)
	serveMux(t, db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), gr)

	if got := roleOf(t, db, nominee.ID, nodeID); got != "member" {
		t.Errorf("expected nobody promoted with no chair to seat them in, got %q", got)
	}
	var said int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'membership.ratification_unseated'`).Scan(&said)
	if said != 1 {
		t.Errorf("expected the unseatable ratification on the record, got %d entries", said)
	}
}

// The contest contests the seats that exist, not however many admins happen
// to hold the role — which is how a three-person council was heading for a
// one-seat election (docs/adr/100).
func TestScheduledElection_ContestsEverySeat(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "contestadmin", "member")
	nodeID := electedNode(t, db, admin.ID, "Contest", "contest", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// Three chairs, all due: one held, two vacant.
	due := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, ?, ?)`,
		auth.NewUUIDv7(), nodeID, admin.ID, due)
	makeVacantSeat(t, db, nodeID, due)
	makeVacantSeat(t, db, nodeID, due)

	handler.ScheduleDueElections(db)

	var seats int
	db.QueryRow(`SELECT COALESCE(MAX(seats_contested),0) FROM proposals WHERE node_id = ?`, nodeID).Scan(&seats)
	if seats != 3 {
		t.Errorf("expected the contest to fill all 3 seats, got %d", seats)
	}
}
