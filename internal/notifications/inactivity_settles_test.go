package notifications

import (
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// A quiet year, and the three faults it found (docs/adr/051).
//
// A simulated world advanced 320 days with nobody acting produced 1,348
// `membership.succession` rows across five patches: the role walked through
// every membership for ever, because promotion did not reset the inactivity
// clock, because vacating an admin left their chair recorded as held, and
// because succession promoted by tenure on patches whose own page promises an
// election. These are those three, plus the one that matters most — a patch
// left alone settles instead of churning.

// seedPatchGC is seedPatch with the whole governance config written out, for
// the tests that turn on leadership_model and leadership_venue.
func seedPatchGC(t *testing.T, db *database.DB, slug, gc string) string {
	t.Helper()
	owner := seedPerson(t, db, slug+"-owner")
	id := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug, description, visibility, membership_policy, status, governance_config)
		 VALUES (?, ?, ?, ?, '', 'public', 'open', 'active', ?)`,
		id, owner, slug, slug, gc); err != nil {
		t.Fatalf("seed patch: %v", err)
	}
	return id
}

// seedChair puts a chair on the council, held or vacant (docs/adr/100).
func seedChair(t *testing.T, db *database.DB, nodeID, holderID, termEndsAt string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	var holder, term interface{}
	if holderID != "" {
		holder = holderID
	}
	if termEndsAt != "" {
		term = termEndsAt
	}
	if _, err := db.Exec(
		`INSERT INTO seats (id, node_id, holder_id, term_ends_at) VALUES (?, ?, ?, ?)`,
		id, nodeID, holder, term); err != nil {
		t.Fatalf("seed seat: %v", err)
	}
	return id
}

func seedInstanceAdmin(t *testing.T, db *database.DB, username string) string {
	t.Helper()
	id := seedPerson(t, db, username)
	if _, err := db.Exec(`UPDATE users SET role = 'admin' WHERE id = ?`, id); err != nil {
		t.Fatalf("seed instance admin: %v", err)
	}
	return id
}

// advanceDays moves the world, not the clock (docs/adr/096): every stored
// timestamp the sweep reads is shifted back a day, which is what "a day
// passed" means to code that measures from time.Now.
func advanceDays(t *testing.T, db *database.DB, days int) {
	t.Helper()
	shift := "-" + itoa(days) + " days"
	for _, col := range []struct{ table, name string }{
		{"memberships", "joined_at"},
		{"memberships", "role_since"},
		{"proposals", "created_at"},
		{"votes", "created_at"},
		{"proposal_comments", "created_at"},
		{"events", "created_at"},
		{"notices", "created_at"},
		{"notice_replies", "created_at"},
	} {
		if _, err := db.Exec(
			`UPDATE `+col.table+` SET `+col.name+` = strftime('%Y-%m-%dT%H:%M:%fZ', `+col.name+`, ?)
			 WHERE `+col.name+` IS NOT NULL`, shift); err != nil {
			t.Fatalf("advance %s.%s: %v", col.table, col.name, err)
		}
	}
	if _, err := db.Exec(
		`UPDATE seats SET term_ends_at = date(term_ends_at, ?) WHERE term_ends_at IS NOT NULL`, shift); err != nil {
		t.Fatalf("advance seats.term_ends_at: %v", err)
	}
}

func countRows(t *testing.T, db *database.DB, query string, args ...interface{}) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v (%s)", err, query)
	}
	return n
}

func adminsOf(t *testing.T, db *database.DB, nodeID string) int {
	t.Helper()
	return countRows(t, db, `SELECT COUNT(*) FROM memberships
	                         WHERE node_id = ? AND role = 'admin' AND status = 'active'`, nodeID)
}

// worstDuplicateNotice is the largest number of times any one person was told
// any one thing. One is fine; two is the bug (docs/adr/093 — and the sweep
// that sent 1,347 of them).
func worstDuplicateNotice(t *testing.T, db *database.DB) (int, string) {
	t.Helper()
	var worst int
	var title string
	db.QueryRow(`SELECT COUNT(*) AS c, title FROM notifications
	             GROUP BY user_id, title ORDER BY c DESC LIMIT 1`).Scan(&worst, &title)
	return worst, title
}

// The regression that matters. A patch left alone for a year settles: the
// absent admin loses the seat once, whoever is still around picks it up once,
// and when they lapse too the patch stops rather than handing the role round
// the membership for ever.
func TestSweep_AQuietYearSettlesInsteadOfChurning(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "quiet-year", "longest_tenure", 30)

	// The founder, last seen in governance ten weeks ago.
	founder := seedPerson(t, db, "yearfounder")
	founderSeat := seedSeat(t, db, founder, nodeID, "admin", 500)
	seedProposalBy(t, db, nodeID, founder, 70)

	// Five members. Two were around last week; three have not been seen
	// since long before the vacate threshold.
	var memberSeats []string
	for _, spec := range []struct {
		name       string
		joinedDays int
		activeDays int // negative: no activity at all
	}{
		{"yearpresentold", 400, 5},
		{"yearpresentnew", 120, 5},
		{"yeargoneone", 380, -1},
		{"yeargonetwo", 260, -1},
		{"yeargonethree", 90, -1},
	} {
		uid := seedPerson(t, db, spec.name)
		memberSeats = append(memberSeats, seedSeat(t, db, uid, nodeID, "member", spec.joinedDays))
		if spec.activeDays >= 0 {
			seedProposalBy(t, db, nodeID, uid, spec.activeDays)
		}
	}

	// 320 days, one sweep a day, nobody acting.
	for day := 0; day < 320; day++ {
		SweepInactiveAdmins(n)
		advanceDays(t, db, 1)
	}

	successions := countRows(t, db, `SELECT COUNT(*) FROM audit_log WHERE action = 'membership.succession'`)
	vacated := countRows(t, db, `SELECT COUNT(*) FROM audit_log WHERE action = 'membership.seat_vacated'`)

	// Two people were present on day one, so the role moves exactly twice.
	// What matters is that the number is bounded by the patch rather than by
	// the length of the simulation: the old sweep produced hundreds.
	if successions != 2 {
		t.Errorf("expected the role to move twice in a year, got %d successions", successions)
	}
	// The founder, then the two interims. Nobody else, ever.
	if vacated != 3 {
		t.Errorf("expected three seats vacated in a year, got %d", vacated)
	}

	// The end state is stable and honest: nobody is left holding a seat they
	// never asked for, and the record says the patch has no admins.
	if got := adminsOf(t, db, nodeID); got != 0 {
		t.Errorf("expected the patch to settle with no admins, got %d", got)
	}
	if got := roleOfSeat(t, db, founderSeat); got != "member" {
		t.Errorf("the founder should end as a member, got %q", got)
	}
	for _, seat := range memberSeats {
		if got := roleOfSeat(t, db, seat); got != "member" {
			t.Errorf("every member should end as a member, got %q", got)
		}
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM audit_log WHERE action = 'node.council_empty'`); got != 1 {
		t.Errorf("expected the emptiness recorded once, got %d", got)
	}

	// And nobody was told the same thing twice.
	if worst, title := worstDuplicateNotice(t, db); worst > 1 {
		t.Errorf("somebody was told %q %d times", title, worst)
	}
}

// F-082. A newly promoted admin starts their own clock: the floor is the
// later of their activity, their joining, and when they became an admin.
func TestSweep_PromotedAdminIsNotVacatedOnSight(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "fresh-clock", "longest_tenure", 30)

	gone := seedPerson(t, db, "clockgone")
	seedSeat(t, db, gone, nodeID, "admin", 500)
	seedProposalBy(t, db, nodeID, gone, 200)

	// A member of well over a year's standing who was around last week. Under
	// the old floor — the later of activity and *joining* — promoting them
	// installed somebody the very next pass judged absent.
	heir := seedPerson(t, db, "clockheir")
	heirSeat := seedSeat(t, db, heir, nodeID, "member", 500)
	seedProposalBy(t, db, nodeID, heir, 3)

	SweepInactiveAdmins(n)
	if got := roleOfSeat(t, db, heirSeat); got != "admin" {
		t.Fatalf("expected the present member promoted, got %q", got)
	}

	// Their last governance act is now older than the vacate threshold, and
	// their joining older still. Only role_since keeps the seat.
	advanceDays(t, db, 50)
	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, heirSeat); got != "admin" {
		t.Errorf("a promotion must reset the clock, got %q after 50 days", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM audit_log WHERE action = 'membership.succession'`); got != 1 {
		t.Errorf("expected exactly one succession, got %d", got)
	}
}

// F-083. Emptying a chair empties the chair — and leaves the chair.
func TestSweep_VacatingAnAdminEmptiesTheirChair(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatchGC(t, db, "empty-chair",
		`{"inactivity_days":30,"leadership_model":"elected","admin_term_months":12}`)

	chair := seedPerson(t, db, "chairholder")
	memID := seedSeat(t, db, chair, nodeID, "admin", 500)
	seedProposalBy(t, db, nodeID, chair, 200)
	seatID := seedChair(t, db, nodeID, chair, time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02"))

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, memID); got != "member" {
		t.Errorf("expected the admin vacated, got %q", got)
	}
	var holder string
	db.QueryRow(`SELECT COALESCE(holder_id,'') FROM seats WHERE id = ?`, seatID).Scan(&holder)
	if holder != "" {
		t.Errorf("the council still records the chair as held by %q", holder)
	}
	// The chair itself stays: a seat outlives its holder (docs/adr/051).
	if got := countRows(t, db, `SELECT COUNT(*) FROM seats WHERE node_id = ?`, nodeID); got != 1 {
		t.Errorf("expected the chair to survive its holder, got %d seats", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM audit_log WHERE action = 'seat.vacated'`); got != 1 {
		t.Errorf("expected the empty chair recorded once, got %d", got)
	}
}

// F-084. An elected patch does not get admins by tenure — that is the act
// docs/adr/100 refuses the role dropdown. The chair empties and the contest
// that fills it comes due now.
func TestSweep_ElectedPatchElectsRatherThanPromoting(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatchGC(t, db, "elects-back",
		`{"inactivity_days":30,"succession_policy":"longest_tenure","leadership_model":"elected","admin_term_months":12}`)

	gone := seedPerson(t, db, "electedgone")
	seedSeat(t, db, gone, nodeID, "admin", 500)
	seedProposalBy(t, db, nodeID, gone, 200)
	seatID := seedChair(t, db, nodeID, gone, time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02"))

	// A member who is plainly around, and would have been promoted by tenure.
	eager := seedPerson(t, db, "electedeager")
	eagerSeat := seedSeat(t, db, eager, nodeID, "member", 400)
	seedProposalBy(t, db, nodeID, eager, 2)

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, eagerSeat); got != "member" {
		t.Errorf("an elected council must not be filled by tenure, got %q", got)
	}
	if got := adminsOf(t, db, nodeID); got != 0 {
		t.Errorf("expected no admins until the contest settles, got %d", got)
	}
	// The contest is due now rather than next year: holdover has nobody left
	// to hold over.
	var term string
	db.QueryRow(`SELECT COALESCE(term_ends_at,'') FROM seats WHERE id = ?`, seatID).Scan(&term)
	if term != time.Now().UTC().Format("2006-01-02") {
		t.Errorf("expected the contest brought forward to today, got %q", term)
	}
	// And the members are told, in so many words.
	if got := countRows(t, db,
		`SELECT COUNT(*) FROM notifications WHERE type = 'governance.council_empty'`); got == 0 {
		t.Error("the members were never told the patch has no admins")
	}
}

// Where the council is decided elsewhere, Patchwork conducts nothing
// (docs/adr/052): vacate, record, appoint nobody.
func TestSweep_ElsewhereAppointsNobody(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatchGC(t, db, "decides-elsewhere",
		`{"inactivity_days":30,"succession_policy":"longest_tenure","leadership_model":"elected","leadership_venue":"elsewhere","admin_term_months":12}`)

	gone := seedPerson(t, db, "elsewheregone")
	goneSeat := seedSeat(t, db, gone, nodeID, "admin", 500)
	seedProposalBy(t, db, nodeID, gone, 200)
	present := seedPerson(t, db, "elsewherepresent")
	presentSeat := seedSeat(t, db, present, nodeID, "member", 400)
	seedProposalBy(t, db, nodeID, present, 2)

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, goneSeat); got != "member" {
		t.Errorf("expected the absent admin vacated, got %q", got)
	}
	if got := roleOfSeat(t, db, presentSeat); got != "member" {
		t.Errorf("Patchwork must appoint nobody where leadership is decided elsewhere, got %q", got)
	}
	if got := adminsOf(t, db, nodeID); got != 0 {
		t.Errorf("expected no admins, got %d", got)
	}
}

// Meritocratic fills a seat by nomination and ratification, and a nomination
// is raised by an admin — so an emptied one genuinely needs a hand.
func TestSweep_MeritocraticAsksAnInstanceAdmin(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	seedInstanceAdmin(t, db, "siteadmin")
	nodeID := seedPatchGC(t, db, "merit-empty",
		`{"inactivity_days":30,"succession_policy":"longest_tenure","leadership_model":"meritocratic"}`)

	gone := seedPerson(t, db, "meritgone")
	seedSeat(t, db, gone, nodeID, "admin", 500)
	seedProposalBy(t, db, nodeID, gone, 200)
	present := seedPerson(t, db, "meritpresent")
	presentSeat := seedSeat(t, db, present, nodeID, "member", 400)
	seedProposalBy(t, db, nodeID, present, 2)

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, presentSeat); got != "member" {
		t.Errorf("a meritocratic patch ratifies admins; tenure must not make one, got %q", got)
	}
	if got := countRows(t, db,
		`SELECT COUNT(*) FROM notifications WHERE type = 'governance.succession_needed'`); got != 1 {
		t.Errorf("expected the instance admins told once, got %d", got)
	}
}

// A maintainer patch passes to the person the maintainer named — the mechanic
// that model's own description promises (docs/adr/051).
func TestSweep_MaintainerPassesToTheNamedSuccessor(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatchGC(t, db, "maintainer-gone",
		`{"inactivity_days":30,"succession_policy":"longest_tenure","leadership_model":"maintainer"}`)

	gone := seedPerson(t, db, "maintgone")
	seedSeat(t, db, gone, nodeID, "admin", 500)
	seedProposalBy(t, db, nodeID, gone, 200)

	// The named successor is not the longest-standing member, so this also
	// says the designation outranks the tenure rule.
	oldest := seedPerson(t, db, "maintoldest")
	oldestSeat := seedSeat(t, db, oldest, nodeID, "member", 450)
	seedProposalBy(t, db, nodeID, oldest, 2)
	heir := seedPerson(t, db, "maintheir")
	heirSeat := seedSeat(t, db, heir, nodeID, "member", 100)
	db.Exec(`UPDATE nodes SET designated_successor_id = ? WHERE id = ?`, heir, nodeID)

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, heirSeat); got != "admin" {
		t.Errorf("expected the named successor to take the patch, got %q", got)
	}
	if got := roleOfSeat(t, db, oldestSeat); got != "member" {
		t.Errorf("the designation outranks tenure, got %q for the longest-standing member", got)
	}
	var successor string
	db.QueryRow(`SELECT COALESCE(designated_successor_id,'') FROM nodes WHERE id = ?`, nodeID).Scan(&successor)
	if successor != "" {
		t.Errorf("the designation is spent on use, still holds %q", successor)
	}
}

// "The three longest-tenured *active* members" means members who are still
// here. Handing the patch to people who have not been seen in a year is how
// the role walked round the membership for ever.
func TestSweep_AbsentMembersAreNotMadeInterimAdmins(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	seedInstanceAdmin(t, db, "absentsiteadmin")
	nodeID := seedPatch(t, db, "all-gone", "longest_tenure", 30)

	gone := seedPerson(t, db, "allgoneadmin")
	seedSeat(t, db, gone, nodeID, "admin", 500)
	seedProposalBy(t, db, nodeID, gone, 200)
	for _, name := range []string{"absentone", "absenttwo", "absentthree"} {
		uid := seedPerson(t, db, name)
		seedSeat(t, db, uid, nodeID, "member", 400)
	}

	SweepInactiveAdmins(n)

	if got := adminsOf(t, db, nodeID); got != 0 {
		t.Errorf("expected nobody promoted out of an empty room, got %d admins", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM audit_log WHERE action = 'membership.succession'`); got != 0 {
		t.Errorf("expected no successions, got %d", got)
	}
	if got := countRows(t, db,
		`SELECT COUNT(*) FROM notifications WHERE type = 'governance.succession_needed'`); got != 1 {
		t.Errorf("expected the instance admins told once, got %d", got)
	}
}

// Presence is wider than the participation an admin is measured by: a seat is
// lost by not governing, and offered to somebody who is around. A member who
// posted an event last week qualifies.
func TestSweep_PostingAnEventCountsAsBeingHere(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "event-poster", "longest_tenure", 30)

	gone := seedPerson(t, db, "eventgone")
	seedSeat(t, db, gone, nodeID, "admin", 500)
	seedProposalBy(t, db, nodeID, gone, 200)

	poster := seedPerson(t, db, "eventposter")
	posterSeat := seedSeat(t, db, poster, nodeID, "member", 400)
	if _, err := db.Exec(
		`INSERT INTO events (id, node_id, created_by, title, starts_at, created_at)
		 VALUES (?, ?, ?, 'a show', ?, ?)`,
		auth.NewUUIDv7(), nodeID, poster, ago(-7), ago(6)); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, posterSeat); got != "admin" {
		t.Errorf("a member who posted an event last week is here, got %q", got)
	}
}

// Nobody is told the same thing twice, however often the sweep runs.
func TestSweep_NobodyIsToldTwice(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	seedInstanceAdmin(t, db, "twicesiteadmin")
	nodeID := seedPatch(t, db, "told-once", "longest_tenure", 30)

	warned := seedPerson(t, db, "twicewarned")
	seedSeat(t, db, warned, nodeID, "admin", 500)
	seedProposalBy(t, db, nodeID, warned, 40)
	gone := seedPerson(t, db, "twicegone")
	seedSeat(t, db, gone, nodeID, "admin", 500)
	seedProposalBy(t, db, nodeID, gone, 200)

	for i := 0; i < 6; i++ {
		SweepInactiveAdmins(n)
	}

	if worst, title := worstDuplicateNotice(t, db); worst > 1 {
		t.Errorf("somebody was told %q %d times", title, worst)
	}
}
