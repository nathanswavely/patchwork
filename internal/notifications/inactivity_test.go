package notifications

import (
	"io/fs"
	"os"
	"testing"
	"time"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Inactivity and succession (docs/adr/051). The shipped succession plan says a
// council member who stops taking part is contacted, and that the seat is
// declared vacant if they stay away — then "succession procedures begin".
// Neither half existed.

func sweepDB(t *testing.T) *database.DB {
	t.Helper()
	f, err := os.CreateTemp("", "patchwork-inactivity-*.db")
	if err != nil {
		t.Fatalf("temp db: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("migrations fs: %v", err)
	}
	db, err := database.Open(f.Name(), migrations)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func ago(days int) string {
	return time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02T15:04:05.000Z")
}

// seedPatch makes a node whose config carries an inactivity rule.
func seedPatch(t *testing.T, db *database.DB, slug, policy string, inactivityDays int) string {
	t.Helper()
	owner := seedPerson(t, db, slug+"-owner")
	id := auth.NewUUIDv7()
	gc := `{"decision_method":"majority","inactivity_days":` + itoa(inactivityDays) +
		`,"succession_policy":"` + policy + `"}`
	_, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug, description, visibility, membership_policy, status, governance_config)
		 VALUES (?, ?, ?, ?, '', 'public', 'open', 'active', ?)`,
		id, owner, slug, slug, gc)
	if err != nil {
		t.Fatalf("seed patch: %v", err)
	}
	return id
}

func seedPerson(t *testing.T, db *database.DB, username string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO users (id, email, username, display_name, role) VALUES (?, ?, ?, ?, 'member')`,
		id, username+"@localhost", username, username); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// seedSeat puts someone in a patch with a joined_at far enough back that their
// absence is measured from their activity rather than their arrival.
func seedSeat(t *testing.T, db *database.DB, userID, nodeID, role string, joinedDaysAgo int) string {
	t.Helper()
	id := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO memberships (id, user_id, node_id, role, status, joined_at) VALUES (?, ?, ?, ?, 'active', ?)`,
		id, userID, nodeID, role, ago(joinedDaysAgo)); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	return id
}

// seedProposalBy records governance participation at a given age.
func seedProposalBy(t *testing.T, db *database.DB, nodeID, authorID string, daysAgo int) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO proposals (id, node_id, author_id, title, body, status, proposal_type, duration_hours, created_at)
		 VALUES (?, ?, ?, 'x', '', 'open', 'action', 72, ?)`,
		auth.NewUUIDv7(), nodeID, authorID, ago(daysAgo)); err != nil {
		t.Fatalf("seed proposal: %v", err)
	}
}

func roleOfSeat(t *testing.T, db *database.DB, memID string) string {
	t.Helper()
	var role string
	db.QueryRow(`SELECT role FROM memberships WHERE id = ?`, memID).Scan(&role)
	return role
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	return out
}

// Quiet past the first threshold: contacted, seat intact.
func TestSweep_WarnsBeforeItVacates(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "quiet-one", "longest_tenure", 30)
	admin := seedPerson(t, db, "quietadmin")
	seat := seedSeat(t, db, admin, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, admin, 40) // quiet 40 days: past 30, short of 60
	// A second admin so nothing here depends on the last-admin case.
	other := seedPerson(t, db, "activeadmin")
	seedSeat(t, db, other, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, other, 1)

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, seat); got != "admin" {
		t.Errorf("a warning must not take the seat, got %q", got)
	}
	var warned int
	db.QueryRow(`SELECT COUNT(*) FROM notification_reminders_sent WHERE entity_id = ? AND reminder_type = 'inactivity_warning'`, seat).Scan(&warned)
	if warned != 1 {
		t.Errorf("expected one warning recorded, got %d", warned)
	}

	// Running again does not warn twice.
	SweepInactiveAdmins(n)
	db.QueryRow(`SELECT COUNT(*) FROM notification_reminders_sent WHERE entity_id = ? AND reminder_type = 'inactivity_warning'`, seat).Scan(&warned)
	if warned != 1 {
		t.Errorf("expected the warning not to repeat, got %d", warned)
	}
}

// Quiet past twice the threshold: the seat goes.
func TestSweep_VacatesAfterTwiceTheWindow(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "quiet-two", "longest_tenure", 30)
	gone := seedPerson(t, db, "goneadmin")
	goneSeat := seedSeat(t, db, gone, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, gone, 90)
	here := seedPerson(t, db, "hereadmin")
	hereSeat := seedSeat(t, db, here, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, here, 2)

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, goneSeat); got != "member" {
		t.Errorf("expected the vacated seat to drop to member, got %q", got)
	}
	if got := roleOfSeat(t, db, hereSeat); got != "admin" {
		t.Errorf("an active admin must be untouched, got %q", got)
	}
}

// Taking part again clears the warning, so a later absence gets its own.
func TestSweep_ActivityClearsTheWarning(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "quiet-three", "longest_tenure", 30)
	admin := seedPerson(t, db, "returningadmin")
	seat := seedSeat(t, db, admin, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, admin, 40)
	other := seedPerson(t, db, "steadyadmin")
	seedSeat(t, db, other, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, other, 1)

	SweepInactiveAdmins(n)
	var warned int
	db.QueryRow(`SELECT COUNT(*) FROM notification_reminders_sent WHERE entity_id = ?`, seat).Scan(&warned)
	if warned != 1 {
		t.Fatalf("expected a warning first, got %d", warned)
	}

	// They come back.
	seedProposalBy(t, db, nodeID, admin, 0)
	SweepInactiveAdmins(n)

	db.QueryRow(`SELECT COUNT(*) FROM notification_reminders_sent WHERE entity_id = ?`, seat).Scan(&warned)
	if warned != 0 {
		t.Errorf("expected the warning cleared once they took part again, got %d", warned)
	}
}

// Someone who just arrived has not been absent for a month, whatever their
// empty activity record suggests.
func TestSweep_NewAdminIsNotAbsent(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "quiet-four", "longest_tenure", 30)
	fresh := seedPerson(t, db, "freshadmin")
	seat := seedSeat(t, db, fresh, nodeID, "admin", 2) // joined two days ago

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, seat); got != "admin" {
		t.Errorf("a new admin must not be swept, got %q", got)
	}
	var warned int
	db.QueryRow(`SELECT COUNT(*) FROM notification_reminders_sent WHERE entity_id = ?`, seat).Scan(&warned)
	if warned != 0 {
		t.Errorf("a new admin must not be warned, got %d", warned)
	}
}

// The pair. Inactivity may empty a patch — unlike every voluntary path — and
// succession is what catches it in the same sweep. This is what makes
// succession_policy reachable at all.
func TestSweep_EmptiedPatchGetsLongestTenuredInterims(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "quiet-five", "longest_tenure", 30)
	gone := seedPerson(t, db, "vanishedadmin")
	goneSeat := seedSeat(t, db, gone, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, gone, 120)

	oldest := seedPerson(t, db, "oldestmember")
	oldestSeat := seedSeat(t, db, oldest, nodeID, "member", 300)
	middle := seedPerson(t, db, "middlemember")
	middleSeat := seedSeat(t, db, middle, nodeID, "member", 200)
	newest := seedPerson(t, db, "newestmember")
	newestSeat := seedSeat(t, db, newest, nodeID, "member", 10)
	fourth := seedPerson(t, db, "fourthmember")
	fourthSeat := seedSeat(t, db, fourth, nodeID, "member", 5)

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, goneSeat); got != "member" {
		t.Errorf("the absent sole admin's seat must go, got %q", got)
	}
	// The three longest-standing members become interim admins.
	for _, s := range []struct {
		name, id string
	}{{"oldest", oldestSeat}, {"middle", middleSeat}, {"newest", newestSeat}} {
		if got := roleOfSeat(t, db, s.id); got != "admin" {
			t.Errorf("%s should be an interim admin, got %q", s.name, got)
		}
	}
	if got := roleOfSeat(t, db, fourthSeat); got != "member" {
		t.Errorf("only three are promoted, got %q for the fourth", got)
	}
	// The patch is not left leaderless.
	var admins int
	db.QueryRow(`SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'`, nodeID).Scan(&admins)
	if admins != 3 {
		t.Errorf("expected three admins after succession, got %d", admins)
	}
}

// A patch that chose to stop rather than be reassigned is left stopped.
func TestSweep_FreezePolicyPromotesNobody(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "quiet-six", "freeze", 30)
	gone := seedPerson(t, db, "frozenadmin")
	seedSeat(t, db, gone, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, gone, 120)
	member := seedPerson(t, db, "frozenmember")
	memberSeat := seedSeat(t, db, member, nodeID, "member", 300)

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, memberSeat); got != "member" {
		t.Errorf("freeze must promote nobody, got %q", got)
	}
	var admins int
	db.QueryRow(`SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'`, nodeID).Scan(&admins)
	if admins != 0 {
		t.Errorf("expected the patch left with no admins, got %d", admins)
	}
}

// A patch with the rule switched off is never swept, which is every patch
// whose config carries no inactivity_days.
func TestSweep_SkipsPatchesWithNoRule(t *testing.T) {
	db := sweepDB(t)
	n := NewNotifier(db)
	nodeID := seedPatch(t, db, "quiet-seven", "longest_tenure", 0)
	gone := seedPerson(t, db, "untouchedadmin")
	seat := seedSeat(t, db, gone, nodeID, "admin", 400)
	seedProposalBy(t, db, nodeID, gone, 500)

	SweepInactiveAdmins(n)

	if got := roleOfSeat(t, db, seat); got != "admin" {
		t.Errorf("a patch with no inactivity rule must be left alone, got %q", got)
	}
}
