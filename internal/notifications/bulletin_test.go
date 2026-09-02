package notifications

import (
	"io/fs"
	"os"
	"testing"
	"time"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

func bulletinDB(t *testing.T) *database.DB {
	t.Helper()
	f, err := os.CreateTemp("", "patchwork-bulletin-*.db")
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

func addUser(t *testing.T, db *database.DB, id, username string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO users (id, username, display_name, role) VALUES (?, ?, ?, 'member')`,
		id, username, username,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

// addPatch inserts a public active patch that joined at `joined` ("" = never).
func addPatch(t *testing.T, db *database.DB, id, name, joined string) {
	t.Helper()
	var at any
	if joined != "" {
		at = joined
	}
	// nodes.owner_id is a real foreign key; every patch needs someone to own it.
	db.Exec(`INSERT OR IGNORE INTO users (id, username, display_name, role) VALUES ('owner', 'owner', 'Owner', 'member')`)
	if _, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug, description, visibility, membership_policy, status, activated_at)
		 VALUES (?, ?, ?, ?, '', 'public', 'open', 'active', ?)`,
		id, "owner", name, id, at,
	); err != nil {
		t.Fatalf("insert node: %v", err)
	}
}

func subscribe(t *testing.T, db *database.DB, userID string, enabled bool) {
	t.Helper()
	v := 0
	if enabled {
		v = 1
	}
	if _, err := db.Exec(
		`INSERT INTO notification_preferences (id, user_id, notification_type, channel, enabled)
		 VALUES (?, ?, ?, 'in_app', ?)`,
		userID+"-pref", userID, string(QuiltBulletin), v,
	); err != nil {
		t.Fatalf("insert preference: %v", err)
	}
}

func bulletins(t *testing.T, db *database.DB, userID string) []string {
	t.Helper()
	rows, err := db.Query(
		`SELECT title FROM notifications WHERE user_id = ? AND type = ? ORDER BY created_at`,
		userID, string(QuiltBulletin),
	)
	if err != nil {
		t.Fatalf("read notifications: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		out = append(out, s)
	}
	return out
}

func bodyOf(t *testing.T, db *database.DB, userID string) string {
	t.Helper()
	var body string
	db.QueryRow(`SELECT body FROM notifications WHERE user_id = ? AND type = ?`,
		userID, string(QuiltBulletin)).Scan(&body)
	return body
}

// backdate moves the cursor so the next pass is due.
func backdate(t *testing.T, db *database.DB, d time.Duration) {
	t.Helper()
	if err := settings.Set(db, bulletinLastSentKey, bulletinStamp(time.Now().UTC().Add(-d))); err != nil {
		t.Fatalf("set cursor: %v", err)
	}
}

// The bulletin ships off (docs/adr/076): nobody receives it without asking.
func TestBulletinReachesOnlyThoseWhoAsked(t *testing.T) {
	if DefaultEnabled(QuiltBulletin, "in_app") || DefaultEnabled(QuiltBulletin, "email") {
		t.Fatal("the bulletin defaults on; opt-in is the whole of what keeps the promise")
	}

	db := bulletinDB(t)
	addUser(t, db, "asked", "asked")
	addUser(t, db, "declined", "declined")
	addUser(t, db, "silent", "silent")
	subscribe(t, db, "asked", true)
	subscribe(t, db, "declined", false)

	backdate(t, db, 40*24*time.Hour)
	addPatch(t, db, "p1", "The Selvage", bulletinStamp(time.Now().UTC().Add(-2*24*time.Hour)))

	sendBulletin(NewNotifier(db))

	if got := bulletins(t, db, "asked"); len(got) != 1 {
		t.Errorf("subscriber got %d bulletins, want 1", len(got))
	}
	if got := bulletins(t, db, "declined"); len(got) != 0 {
		t.Errorf("someone who declined got %d bulletins", len(got))
	}
	if got := bulletins(t, db, "silent"); len(got) != 0 {
		t.Errorf("someone who never answered got %d bulletins", len(got))
	}
}

// Complete and unranked: every arrival in the window, in arrival order.
func TestBulletinIsCompleteAndInArrivalOrder(t *testing.T) {
	db := bulletinDB(t)
	addUser(t, db, "sub", "sub")
	subscribe(t, db, "sub", true)
	backdate(t, db, 40*24*time.Hour)

	now := time.Now().UTC()
	addPatch(t, db, "third", "Third", bulletinStamp(now.Add(-1*24*time.Hour)))
	addPatch(t, db, "first", "First", bulletinStamp(now.Add(-20*24*time.Hour)))
	addPatch(t, db, "second", "Second", bulletinStamp(now.Add(-10*24*time.Hour)))

	sendBulletin(NewNotifier(db))

	if body := bodyOf(t, db, "sub"); body != "First, Second, Third" {
		t.Errorf("bulletin body %q — want every arrival, in arrival order", body)
	}
}

// It names arrivals, not rows: the directory backfill that must not fire.
func TestBulletinIgnoresListingsAndPrivatePatches(t *testing.T) {
	db := bulletinDB(t)
	addUser(t, db, "sub", "sub")
	subscribe(t, db, "sub", true)
	backdate(t, db, 40*24*time.Hour)

	joined := bulletinStamp(time.Now().UTC().Add(-2 * 24 * time.Hour))
	addPatch(t, db, "real", "A Real Arrival", joined)
	// An unclaimed listing: a row someone typed in, not a community arriving.
	addPatch(t, db, "listing", "Directory Listing", "")
	db.Exec(`UPDATE nodes SET status = 'unclaimed' WHERE id = 'listing'`)
	// A private patch is off the quilt entirely; announcing it discloses it.
	addPatch(t, db, "secret", "Private Patch", joined)
	db.Exec(`UPDATE nodes SET visibility = 'private' WHERE id = 'secret'`)

	sendBulletin(NewNotifier(db))

	if body := bodyOf(t, db, "sub"); body != "A Real Arrival" {
		t.Errorf("bulletin body %q — a listing or a private patch leaked in", body)
	}
}

// The first pass on an instance starts the clock rather than announcing
// everything that ever joined.
func TestFirstPassAnnouncesNothing(t *testing.T) {
	db := bulletinDB(t)
	addUser(t, db, "sub", "sub")
	subscribe(t, db, "sub", true)
	addPatch(t, db, "old", "Joined Long Ago", bulletinStamp(time.Now().UTC().Add(-200*24*time.Hour)))

	sendBulletin(NewNotifier(db))

	if got := bulletins(t, db, "sub"); len(got) != 0 {
		t.Errorf("the first pass announced %d bulletins; a quilt's whole history is not news", len(got))
	}
	if _, ok := settings.Get(db, bulletinLastSentKey); !ok {
		t.Error("the first pass did not start the clock, so it will announce everything next time")
	}
}

// Monthly means monthly: the hourly worker runs this on nearly every pass.
func TestBulletinWaitsAMonthAndSkipsQuietOnes(t *testing.T) {
	db := bulletinDB(t)
	addUser(t, db, "sub", "sub")
	subscribe(t, db, "sub", true)
	n := NewNotifier(db)

	backdate(t, db, 3*24*time.Hour)
	addPatch(t, db, "p", "Too Soon", bulletinStamp(time.Now().UTC().Add(-1*24*time.Hour)))
	sendBulletin(n)
	if got := bulletins(t, db, "sub"); len(got) != 0 {
		t.Fatalf("sent after 3 days: %d", len(got))
	}

	// Due, but nothing joined in the window: silence, and the cursor still
	// advances so a quiet quarter never delivers a quarter's worth at once.
	db.Exec(`DELETE FROM nodes`)
	backdate(t, db, 40*24*time.Hour)
	sendBulletin(n)
	if got := bulletins(t, db, "sub"); len(got) != 0 {
		t.Fatalf("sent a bulletin with nothing in it: %d", len(got))
	}
	before, _ := settings.Get(db, bulletinLastSentKey)
	if time.Since(mustParse(t, before)) > time.Hour {
		t.Error("a quiet month left the cursor behind; the next window would be two months long")
	}
}

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.Parse("2006-01-02T15:04:05.000Z", s)
	if err != nil {
		t.Fatalf("parse cursor %q: %v", s, err)
	}
	return at
}

// A patch admin has no switch over the broadcast, because no patch emits it.
func TestNoPatchOwnsTheBroadcast(t *testing.T) {
	for _, ci := range PatchCategories() {
		if ci.ID == CategoryQuilt {
			t.Fatal("the bulletin appears in a patch's own category toggles")
		}
	}
	var found bool
	for _, ci := range AllCategories() {
		if ci.ID == CategoryQuilt {
			found = true
		}
	}
	if !found {
		t.Fatal("the bulletin has no category of its own, so nobody can turn it on")
	}
	if got := len(TypesForCategory(CategoryQuilt)); got != 1 {
		t.Errorf("the quilt category holds %d types; the exception is meant to be exactly one", got)
	}
}

func TestBulletinTitleCountsPlainly(t *testing.T) {
	for n, want := range map[int]string{
		1: "1 patch joined this month",
		3: "3 patches joined this month",
	} {
		if got := bulletinTitle(n); got != want {
			t.Errorf("bulletinTitle(%d) = %q, want %q", n, got, want)
		}
	}
}
