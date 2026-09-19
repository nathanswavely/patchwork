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

// One story about a seat (F-067, F-068).
//
// A member looking at his co-op's council page could see two vacant chairs
// and, beside them, three general sentences — "the community elects admins
// for fixed terms", "a vacant seat is filled by nomination", "next seat comes
// up Sep 1, 2027". All three true, all three about different chairs at
// different times, and none of them an answer to "what happens to these two".
// So the overview now decides per chair which route applies, and the founder
// who wanted her co-op's terms to end in November has a box to say so.

func overviewOf(t *testing.T, db *database.DB, slug string) map[string]interface{} {
	t.Helper()
	r := authedRequest("GET", "/api/v1/nodes/"+slug+"/governance/overview", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/governance/overview", handler.GovernanceOverview(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("overview: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	return decodeJSON(t, w)
}

func overviewSeats(t *testing.T, db *database.DB, slug string) []map[string]interface{} {
	t.Helper()
	raw, ok := overviewOf(t, db, slug)["seats"].([]interface{})
	if !ok {
		t.Fatal("overview: expected a seats array")
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, s := range raw {
		m, ok := s.(map[string]interface{})
		if !ok {
			t.Fatalf("overview: seat is %T, not an object", s)
		}
		out = append(out, m)
	}
	return out
}

func setTermVia(t *testing.T, db *database.DB, slug, seatID, date, token string) (int, string) {
	t.Helper()
	r := authedRequest("PATCH", "/api/v1/nodes/"+slug+"/seats/"+seatID,
		map[string]interface{}{"term_ends_at": date}, token)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/seats/{id}", handler.SetSeatTerm(db), r)
	return w.Code, w.Body.String()
}

// Each chair carries its own answer, and they differ on one page: a held seat
// is contested on the calendar, a vacant one is filled by nomination today.
func TestOverviewSeats_EachChairCarriesItsOwnAnswer(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "calfounder", "member")
	nodeID := electedNode(t, db, admin.ID, "Calendar Co-op", "calendar-coop", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	termEnd := time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02")
	db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, ?, ?)`,
		auth.NewUUIDv7(), nodeID, admin.ID, termEnd)
	makeVacantSeat(t, db, nodeID, termEnd)
	makeVacantSeat(t, db, nodeID, termEnd)

	seats := overviewSeats(t, db, "calendar-coop")
	if len(seats) != 3 {
		t.Fatalf("expected 3 seats, got %d", len(seats))
	}

	held := seats[0]
	if held["vacant"] != false {
		t.Errorf("the founder's chair should not read vacant: %v", held["vacant"])
	}
	if held["fill"] != "contest_scheduled" {
		t.Errorf("a held chair with a term is contested on the calendar, got %v", held["fill"])
	}
	opens, _ := held["contest_opens"].(string)
	if opens == "" || opens >= termEnd {
		t.Errorf("expected the contest to open before the term ends %s, got %q", termEnd, opens)
	}
	if held["contest_due"] == true {
		t.Errorf("a term ending in a year is not due now")
	}

	for i, seat := range seats[1:] {
		if seat["vacant"] != true {
			t.Errorf("seat %d: expected vacant", i+1)
		}
		if seat["fill"] != "nomination" {
			t.Errorf("seat %d: a vacant chair is filled by nomination today, got %v", i+1, seat["fill"])
		}
		if _, ok := seat["contest_opens"]; ok {
			t.Errorf("seat %d: a vacancy waits for no date", i+1)
		}
	}
}

// A council whose term has run out is overdue, not emptied: holdover carries
// the holder, and the chair says so rather than showing a date in the past
// with no explanation.
func TestOverviewSeats_AnOverdueChairSaysTheContestIsDue(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "overdueadmin", "member")
	nodeID := electedNode(t, db, admin.ID, "Overdue", "overdue-council", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, ?, ?)`,
		auth.NewUUIDv7(), nodeID, admin.ID, time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02"))

	seat := overviewSeats(t, db, "overdue-council")[0]
	if seat["fill"] != "contest_scheduled" {
		t.Fatalf("expected contest_scheduled, got %v", seat["fill"])
	}
	if seat["contest_due"] != true {
		t.Errorf("a term that ended three days ago is due now, got %v", seat["contest_due"])
	}
}

// While a contest is running, that is what is happening to every chair — one
// answer for the whole council, and the row can link to it.
func TestOverviewSeats_AnOpenContestOutranksEveryOtherRoute(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "contestseat", "member")
	nodeID := electedNode(t, db, admin.ID, "Contested", "contested-council", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, ?, ?)`,
		auth.NewUUIDv7(), nodeID, admin.ID, time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02"))
	makeVacantSeat(t, db, nodeID, time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02"))

	handler.ScheduleDueElections(db)
	var contestID string
	db.QueryRow(`SELECT id FROM proposals WHERE node_id = ? AND seats_contested > 0`, nodeID).Scan(&contestID)
	if contestID == "" {
		t.Fatal("expected the calendar to open a contest")
	}

	for i, seat := range overviewSeats(t, db, "contested-council") {
		if seat["fill"] != "contest_open" {
			t.Errorf("seat %d: expected contest_open, got %v", i, seat["fill"])
		}
		if seat["contest_id"] != contestID {
			t.Errorf("seat %d: expected the row to name the contest %s, got %v", i, contestID, seat["contest_id"])
		}
	}
}

// Priya's box: a vacant chair takes the date the members voted for.
func TestSetSeatTerm_AVacantChairTakesAnyFutureDate(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "termadmin", "member")
	nodeID := electedNode(t, db, admin.ID, "Term Co-op", "term-coop", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// A chair inheriting September when the members voted for November.
	september := time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02")
	seatID := makeVacantSeat(t, db, nodeID, september)
	november := time.Now().UTC().AddDate(1, 2, 0).Format("2006-01-02")

	code, body := setTermVia(t, db, "term-coop", seatID, november, adminToken)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", code, body)
	}
	if _, got := seatRow(t, db, seatID); got != november {
		t.Errorf("expected the seat's term to end %s, got %q", november, got)
	}

	// And the calendar follows it: the contest is scheduled from the seat's
	// own term end, never from a second stored date (docs/adr/051).
	seat := overviewSeats(t, db, "term-coop")[0]
	if opens, _ := seat["contest_opens"].(string); opens != "" && opens >= november {
		t.Errorf("expected the contest to open before %s, got %q", november, opens)
	}
}

// docs/adr/051's integrity argument, enforced: appointment can fill a gap but
// can never manufacture a mandate, and neither can the calendar box. A held
// chair's date comes forward or stays put.
func TestSetSeatTerm_AHeldChairComesForwardNeverBack(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "sittingadmin", "member")
	nodeID := electedNode(t, db, admin.ID, "Sitting", "sitting-council", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	current := time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02")
	seatID := auth.NewUUIDv7()
	db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, ?, ?)`,
		seatID, nodeID, admin.ID, current)

	// Out is refused, and the refusal says who holds the chair and why.
	later := time.Now().UTC().AddDate(2, 0, 0).Format("2006-01-02")
	code, body := setTermVia(t, db, "sitting-council", seatID, later, adminToken)
	if code != http.StatusConflict {
		t.Fatalf("extending a sitting term: expected 409, got %d: %s", code, body)
	}
	if !strings.Contains(body, "sittingadmin") {
		t.Errorf("expected the refusal to name the holder, got %s", body)
	}
	// And the date a person can read, not the column's ISO.
	if !strings.Contains(body, readableDayT(current)) {
		t.Errorf("expected the refusal to name %s readably, got %s", current, body)
	}
	if !strings.Contains(body, "nobody voted for") {
		t.Errorf("expected the refusal to say what it is protecting, got %s", body)
	}
	if _, got := seatRow(t, db, seatID); got != current {
		t.Errorf("expected the term untouched at %s, got %q", current, got)
	}

	// Forward is allowed. It calls the election sooner and removes nobody —
	// holdover carries the holder until a successor is elected.
	sooner := time.Now().UTC().AddDate(0, 2, 0).Format("2006-01-02")
	code, body = setTermVia(t, db, "sitting-council", seatID, sooner, adminToken)
	if code != http.StatusOK {
		t.Fatalf("bringing a term forward: expected 200, got %d: %s", code, body)
	}
	holder, got := seatRow(t, db, seatID)
	if got != sooner {
		t.Errorf("expected %s, got %q", sooner, got)
	}
	if holder != admin.ID {
		t.Errorf("shortening a term seats nobody and unseats nobody; holder is now %q", holder)
	}
}

// A held chair with no clock at all gains one. That is strictly more
// accountability — it cannot extend a mandate that had no end.
func TestSetSeatTerm_AnUndatedHeldChairCanBeGivenOne(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "undatedadmin", "member")
	nodeID := electedNode(t, db, admin.ID, "Undated", "undated-council", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	seatID := auth.NewUUIDv7()
	db.Exec(`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, ?, NULL)`,
		seatID, nodeID, admin.ID)

	date := time.Now().UTC().AddDate(0, 6, 0).Format("2006-01-02")
	if code, body := setTermVia(t, db, "undated-council", seatID, date, adminToken); code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", code, body)
	}
	if _, got := seatRow(t, db, seatID); got != date {
		t.Errorf("expected %s, got %q", date, got)
	}
}

// A date is a date, and it is not in the past: back-dating a term would say a
// contest was overdue when it never was.
func TestSetSeatTerm_RefusesJunkAndBackDating(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "junkadmin", "member")
	nodeID := electedNode(t, db, admin.ID, "Junk", "junk-council", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	seatID := makeVacantSeat(t, db, nodeID, time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02"))

	for _, bad := range []string{"", "next November", "2027-13-40"} {
		if code, body := setTermVia(t, db, "junk-council", seatID, bad, adminToken); code != http.StatusBadRequest {
			t.Errorf("%q: expected 400, got %d: %s", bad, code, body)
		}
	}
	past := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	if code, body := setTermVia(t, db, "junk-council", seatID, past, adminToken); code != http.StatusBadRequest {
		t.Errorf("a past date: expected 400, got %d: %s", code, body)
	}
}

// An election keeps the terms it opened with (docs/adr/047), and its own
// calendar is one of them.
func TestSetSeatTerm_RefusedWhileAContestRuns(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "midcontest", "member")
	nodeID := electedNode(t, db, admin.ID, "Mid Contest", "mid-contest", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	seatID := makeVacantSeat(t, db, nodeID, time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02"))

	handler.ScheduleDueElections(db)

	date := time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02")
	code, body := setTermVia(t, db, "mid-contest", seatID, date, adminToken)
	if code != http.StatusConflict {
		t.Fatalf("expected 409 while a contest runs, got %d: %s", code, body)
	}
	if !strings.Contains(body, "contest") {
		t.Errorf("expected the refusal to say a contest is running, got %s", body)
	}
}

// The calendar is the patch's own. A member, a follower and an instance admin
// holding no role here all get the same answer the other seat routes give.
func TestSetSeatTerm_PatchAdminOnly(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "calowner", "member")
	nodeID := electedNode(t, db, admin.ID, "Cal Owner", "cal-owner", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	seatID := makeVacantSeat(t, db, nodeID, time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02"))

	member, memberToken := createTestUser(t, db, "calmember", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	follower, followerToken := createTestUser(t, db, "calfollower", "member")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")
	_, instanceToken := createTestUser(t, db, "calsiteadmin", "admin")

	date := time.Now().UTC().AddDate(0, 6, 0).Format("2006-01-02")
	for name, token := range map[string]string{
		"member":         memberToken,
		"follower":       followerToken,
		"instance admin": instanceToken,
	} {
		if code, body := setTermVia(t, db, "cal-owner", seatID, date, token); code != http.StatusForbidden {
			t.Errorf("%s: expected 403, got %d: %s", name, code, body)
		}
	}
	if _, got := seatRow(t, db, seatID); got == date {
		t.Error("the term end moved for somebody who is not this patch's admin")
	}
}

// Seats belong to the elected model and to a patch that elects here
// (docs/adr/051, docs/adr/052), so the calendar box does too.
func TestSetSeatTerm_RefusedOffTheElectedModel(t *testing.T) {
	for _, c := range []struct{ name, gc, want string }{
		{"maintainer", `{"leadership_model":"maintainer"}`, "elected model"},
		{"elsewhere", `{"leadership_model":"elected","leadership_venue":"elsewhere"}`, "elsewhere"},
	} {
		db := setupTestDB(t)
		admin, adminToken := createTestUser(t, db, "caloff"+c.name, "member")
		nodeID := createTestNode(t, db, admin.ID, "Cal "+c.name, "cal-"+c.name, "open")
		createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
		db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`, c.gc, nodeID)
		seatID := makeVacantSeat(t, db, nodeID, "")

		code, body := setTermVia(t, db, "cal-"+c.name, seatID,
			time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02"), adminToken)
		if code != http.StatusConflict {
			t.Errorf("%s: expected 409, got %d: %s", c.name, code, body)
		} else if !strings.Contains(body, c.want) {
			t.Errorf("%s: expected the refusal to mention %q, got %s", c.name, c.want, body)
		}
	}
}

// The drawer marked Rejected opened onto three proposals that all said
// lapsed. The filter is by outcome now (docs/adr/097, amended): Rejected
// means the members answered no, and Not decided carries both absences.
func TestListProposals_FilterByOutcomeNotByStatusColumn(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "filteradmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Filter", "filter-patch", "open")
	openGovernanceRecord(t, db, nodeID)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	seed := func(id, title, status, state string) {
		db.Exec(`INSERT INTO proposals (id, node_id, author_id, title, body, status, state, proposal_type,
		         duration_hours, voting_ends_at, created_at, updated_at)
		         VALUES (?, ?, ?, ?, '', ?, ?, 'other', 72, NULL,
		                 strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
			id, nodeID, admin.ID, title, status, state)
	}
	seed(auth.NewUUIDv7(), "Turned down", "rejected", "rejected")
	seed(auth.NewUUIDv7(), "Ran out of time", "rejected", "lapsed")
	seed(auth.NewUUIDv7(), "Nobody stood", "rejected", "unsettled")
	seed(auth.NewUUIDv7(), "Carried", "approved", "in_effect")

	titles := func(status string) []string {
		r := authedRequest("GET", "/api/v1/nodes/filter-patch/proposals?status="+status, nil, "")
		w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/proposals", handler.ListProposals(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", status, w.Code, w.Body.String())
		}
		items, _ := decodeJSON(t, w)["items"].([]interface{})
		var out []string
		for _, it := range items {
			m, _ := it.(map[string]interface{})
			title, _ := m["title"].(string)
			out = append(out, title)
		}
		return out
	}

	rejected := titles("rejected")
	if len(rejected) != 1 || rejected[0] != "Turned down" {
		t.Errorf("Rejected should hold only what the members turned down, got %v", rejected)
	}

	notDecided := titles("not_decided")
	if len(notDecided) != 2 {
		t.Fatalf("Not decided should hold the lapse and the unsettled contest, got %v", notDecided)
	}
	for _, want := range []string{"Ran out of time", "Nobody stood"} {
		found := false
		for _, got := range notDecided {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %q under Not decided, got %v", want, notDecided)
		}
	}

	if got := len(titles("all")); got != 4 {
		t.Errorf("All should still hold every proposal, got %d", got)
	}
	if got := titles("approved"); len(got) != 1 || got[0] != "Carried" {
		t.Errorf("Approved is untouched, got %v", got)
	}
}

// readableDayT mirrors what the handler puts in front of a person: the stored
// column stays ISO, the sentence does not.
func readableDayT(iso string) string {
	d, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return d.Format("January 2, 2006")
}
