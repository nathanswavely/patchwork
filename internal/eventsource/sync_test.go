package eventsource

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/safehttp"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "patchwork-es-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("migrations fs: %v", err)
	}
	db, err := database.Open(tmpFile.Name(), migrations)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// feedServer serves mutable ICS content; swap Body/Status between syncs.
type feedServer struct {
	mu     sync.Mutex
	body   string
	status int
	srv    *httptest.Server
}

func newFeedServer(t *testing.T, body string) *feedServer {
	t.Helper()
	f := &feedServer{body: body, status: http.StatusOK}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.status != http.StatusOK {
			w.WriteHeader(f.status)
			return
		}
		w.Header().Set("Content-Type", "text/calendar")
		w.Write([]byte(strings.ReplaceAll(f.body, "\n", "\r\n")))
	}))
	t.Cleanup(f.srv.Close)

	prev := safehttp.SetAllowPrivateAddresses(true)
	t.Cleanup(func() { safehttp.SetAllowPrivateAddresses(prev) })
	return f
}

func (f *feedServer) set(body string, status int) {
	f.mu.Lock()
	f.body = body
	f.status = status
	f.mu.Unlock()
}

// seedSource creates a user, node, and attached source; returns source ID.
func seedSource(t *testing.T, db *database.DB, feedURL string) string {
	t.Helper()
	userID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO users (id, username, display_name, role) VALUES (?, 'steward', 'Steward', 'member')`,
		userID,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	nodeID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug, description, node_type, visibility, membership_policy, status)
		 VALUES (?, ?, 'The Selvage', 'the-selvage', '', 'leaf', 'public', 'open', 'active')`,
		nodeID, userID,
	); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	sourceID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO event_sources (id, node_id, type, url, added_by) VALUES (?, ?, 'ics', ?, ?)`,
		sourceID, nodeID, feedURL, userID,
	); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	return sourceID
}

func future(d time.Duration) string {
	return time.Now().Add(d).UTC().Format("20060102T150405Z")
}

func vevent(uid, summary, dtstart string) string {
	return "BEGIN:VEVENT\nUID:" + uid + "\nSUMMARY:" + summary + "\nDTSTART:" + dtstart + "\nEND:VEVENT\n"
}

func wrap(body string) string {
	return "BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\n" + body + "END:VCALENDAR\n"
}

func countEvents(t *testing.T, db *database.DB, sourceID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM events WHERE source_id = ?`, sourceID).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

func sourceState(t *testing.T, db *database.DB, sourceID string) (status string, lastError *string) {
	t.Helper()
	if err := db.QueryRow(`SELECT status, last_error FROM event_sources WHERE id = ?`, sourceID).Scan(&status, &lastError); err != nil {
		t.Fatalf("source state: %v", err)
	}
	return status, lastError
}

func TestSync_FirstSyncImportsQuietly(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(
		vevent("a@test", "Show A", future(48*time.Hour))+
			vevent("b@test", "Show B", future(72*time.Hour))))
	sourceID := seedSource(t, db, feed.srv.URL)

	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if n := countEvents(t, db, sourceID); n != 2 {
		t.Errorf("expected 2 imported events, got %d", n)
	}
	status, lastError := sourceState(t, db, sourceID)
	if status != "ok" || lastError != nil {
		t.Errorf("source state after success: %s / %v", status, lastError)
	}

	var eventStatus, visibility string
	if err := db.QueryRow(`SELECT status, visibility FROM events WHERE source_id = ? LIMIT 1`, sourceID).Scan(&eventStatus, &visibility); err != nil {
		t.Fatalf("inspect event: %v", err)
	}
	if eventStatus != "active" || visibility != "public" {
		t.Errorf("imported events publish directly as public: %s / %s", eventStatus, visibility)
	}
}

func TestSync_ReconcilesChanges(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(
		vevent("a@test", "Show A", future(48*time.Hour))+
			vevent("b@test", "Show B", future(72*time.Hour))))
	sourceID := seedSource(t, db, feed.srv.URL)

	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// A retitled, B withdrawn, C new.
	feed.set(wrap(
		vevent("a@test", "Show A (moved indoors)", future(48*time.Hour))+
			vevent("c@test", "Show C", future(96*time.Hour))), http.StatusOK)
	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	titles := map[string]bool{}
	rows, err := db.Query(`SELECT title FROM events WHERE source_id = ?`, sourceID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var title string
		rows.Scan(&title)
		titles[title] = true
	}
	if !titles["Show A (moved indoors)"] || !titles["Show C"] || titles["Show B"] || len(titles) != 2 {
		t.Errorf("reconciled titles: %v", titles)
	}
}

// The remote-follows rule applied to feeds: an unreachable feed never
// removes anything (docs/adr/031).
func TestSync_FailedFetchTouchesNothing(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(vevent("a@test", "Show A", future(48*time.Hour))))
	sourceID := seedSource(t, db, feed.srv.URL)

	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	feed.set("", http.StatusInternalServerError)
	if err := Sync(context.Background(), db, nil, sourceID); err == nil {
		t.Fatal("expected error from failing feed")
	}
	if n := countEvents(t, db, sourceID); n != 1 {
		t.Errorf("failed fetch must not remove events; have %d", n)
	}
	status, lastError := sourceState(t, db, sourceID)
	if status != "error" || lastError == nil {
		t.Errorf("source must record the failure: %s / %v", status, lastError)
	}

	// Recovery: the feed comes back, the error clears.
	feed.set(wrap(vevent("a@test", "Show A", future(48*time.Hour))), http.StatusOK)
	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("recovery sync: %v", err)
	}
	status, lastError = sourceState(t, db, sourceID)
	if status != "ok" || lastError != nil {
		t.Errorf("recovered source state: %s / %v", status, lastError)
	}
}

// Past events are history and belong to the patch; only future promises
// are withdrawn when the feed drops them (docs/adr/031).
func TestSync_PastEventsSurviveVanishing(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(vevent("future@test", "Upcoming", future(48*time.Hour))))
	sourceID := seedSource(t, db, feed.srv.URL)

	// Simulate an event imported on an earlier sync that has since passed.
	pastID := auth.NewUUIDv7()
	var nodeID string
	db.QueryRow(`SELECT node_id FROM event_sources WHERE id = ?`, sourceID).Scan(&nodeID)
	var addedBy string
	db.QueryRow(`SELECT added_by FROM event_sources WHERE id = ?`, sourceID).Scan(&addedBy)
	past := time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO events (id, node_id, created_by, title, description, location, starts_at,
		 recurrence, visibility, status, source_id, source_uid, source_occurrence)
		 VALUES (?, ?, ?, 'Last Month', '', '', ?, '', 'public', 'active', ?, 'past@test', '')`,
		pastID, nodeID, addedBy, past, sourceID,
	); err != nil {
		t.Fatalf("seed past event: %v", err)
	}
	// Mark the source as having synced before, so this isn't a first sync.
	if _, err := db.Exec(`UPDATE event_sources SET last_success_at = ? WHERE id = ?`, past, sourceID); err != nil {
		t.Fatalf("mark synced: %v", err)
	}

	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE id = ?`, pastID).Scan(&n)
	if n != 1 {
		t.Error("past event was deleted when it vanished from the feed")
	}
	if total := countEvents(t, db, sourceID); total != 2 {
		t.Errorf("expected past + upcoming, got %d", total)
	}
}

// Moderation outranks the feed: a soft-removed imported event still
// occupies the source-identity unique index, and a reconciler blind to
// it would re-INSERT the key, wedging the source in 'error' forever.
// The reconciler must match it, leave it alone, and stay healthy.
func TestSync_ModeratedEventNeverWedgesSource(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(vevent("mod@test", "Reported Show", future(48*time.Hour))))
	sourceID := seedSource(t, db, feed.srv.URL)

	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Moderation soft-removes the imported event (reports.go leaves
	// provenance intact and writes no skip row).
	var eventID string
	db.QueryRow(`SELECT id FROM events WHERE source_id = ?`, sourceID).Scan(&eventID)
	if _, err := db.Exec(`UPDATE events SET removed_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), eventID); err != nil {
		t.Fatalf("soft-remove: %v", err)
	}

	// Feed unchanged and still carrying the item; the retitle exercises
	// the update path too. Sync must succeed and not touch the row.
	feed.set(wrap(vevent("mod@test", "Reported Show (renamed)", future(48*time.Hour))), http.StatusOK)
	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("sync after moderation must not fail: %v", err)
	}

	status, lastError := sourceState(t, db, sourceID)
	if status != "ok" || lastError != nil {
		t.Errorf("source wedged after moderation: %s / %v", status, lastError)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE source_id = ?`, sourceID).Scan(&n)
	if n != 1 {
		t.Errorf("moderated event duplicated or deleted: %d rows", n)
	}
	var title string
	var removedAt *string
	db.QueryRow(`SELECT title, removed_at FROM events WHERE id = ?`, eventID).Scan(&title, &removedAt)
	if title != "Reported Show" || removedAt == nil {
		t.Errorf("sync must not revive or edit a moderated row: title=%q removed=%v", title, removedAt)
	}
}

// Remove holds the source lock and keeps the past / drops the future;
// moderated rows survive as detached history rather than being erased.
func TestRemove_ModeratedRowsSurviveDetached(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(
		vevent("keep@test", "Future Show", future(48*time.Hour))+
			vevent("mod@test", "Moderated Future Show", future(72*time.Hour))))
	sourceID := seedSource(t, db, feed.srv.URL)
	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var modID string
	db.QueryRow(`SELECT id FROM events WHERE source_uid = 'mod@test'`).Scan(&modID)
	if _, err := db.Exec(`UPDATE events SET removed_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), modID); err != nil {
		t.Fatalf("soft-remove: %v", err)
	}

	if err := Remove(db, sourceID); err != nil {
		t.Fatalf("remove: %v", err)
	}

	var sources, futureKept, modKept int
	db.QueryRow(`SELECT COUNT(*) FROM event_sources WHERE id = ?`, sourceID).Scan(&sources)
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE source_uid = 'keep@test'`).Scan(&futureKept)
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE id = ?`, modID).Scan(&modKept)
	if sources != 0 {
		t.Error("source row survived Remove")
	}
	if futureKept != 0 {
		t.Error("future imported event survived source removal")
	}
	if modKept != 1 {
		t.Error("moderated row was erased by source removal — moderation history lost")
	}
	var modSourceID *string
	db.QueryRow(`SELECT source_id FROM events WHERE id = ?`, modID).Scan(&modSourceID)
	if modSourceID != nil {
		t.Error("moderated row still points at the deleted source")
	}
}

// The hourly worker skips sources whose patch is archived or removed —
// the feed is never fetched while the patch is down. The source row
// survives, so restoring the patch to 'active' resumes syncing with no
// extra state.
func TestSyncAll_ArchivedPatchSourcesDormant(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(vevent("a@test", "Show A", future(48*time.Hour))))
	liveSource := seedSource(t, db, feed.srv.URL)

	var userID string
	if err := db.QueryRow(`SELECT added_by FROM event_sources WHERE id = ?`, liveSource).Scan(&userID); err != nil {
		t.Fatalf("look up user: %v", err)
	}
	seedExtra := func(slug, status string, removedAt *string) string {
		t.Helper()
		nodeID := auth.NewUUIDv7()
		if _, err := db.Exec(
			`INSERT INTO nodes (id, owner_id, name, slug, description, node_type, visibility, membership_policy, status, removed_at)
			 VALUES (?, ?, ?, ?, '', 'leaf', 'public', 'open', ?, ?)`,
			nodeID, userID, slug, slug, status, removedAt,
		); err != nil {
			t.Fatalf("seed node %s: %v", slug, err)
		}
		sourceID := auth.NewUUIDv7()
		if _, err := db.Exec(
			`INSERT INTO event_sources (id, node_id, type, url, added_by) VALUES (?, ?, 'ics', ?, ?)`,
			sourceID, nodeID, feed.srv.URL, userID,
		); err != nil {
			t.Fatalf("seed source for %s: %v", slug, err)
		}
		return sourceID
	}
	now := time.Now().UTC().Format(time.RFC3339)
	archivedSource := seedExtra("archived-patch", "archived", nil)
	removedSource := seedExtra("removed-patch", "active", &now)

	syncAll(context.Background(), db, nil)

	if n := countEvents(t, db, liveSource); n != 1 {
		t.Errorf("active patch's source should sync, got %d events", n)
	}
	for name, id := range map[string]string{"archived": archivedSource, "removed": removedSource} {
		if n := countEvents(t, db, id); n != 0 {
			t.Errorf("%s patch's source imported %d events; should lie dormant", name, n)
		}
		if status, _ := sourceState(t, db, id); status != "pending" {
			t.Errorf("%s patch's source was fetched (status %s); should never be touched", name, status)
		}
	}

	// Restore: flipping the patch back to active is all it takes.
	if _, err := db.Exec(`UPDATE nodes SET status = 'active' WHERE slug = 'archived-patch'`); err != nil {
		t.Fatalf("restore node: %v", err)
	}
	syncAll(context.Background(), db, nil)
	if n := countEvents(t, db, archivedSource); n != 1 {
		t.Errorf("restored patch's source should resume syncing, got %d events", n)
	}
}

// Detached / deleted items are on the skip list and never resurrected.
func TestSync_SkipListRespected(t *testing.T) {
	db := setupTestDB(t)
	feed := newFeedServer(t, wrap(
		vevent("keep@test", "Kept", future(48*time.Hour))+
			vevent("skip@test", "Skipped", future(72*time.Hour))))
	sourceID := seedSource(t, db, feed.srv.URL)

	if _, err := db.Exec(
		`INSERT INTO event_source_skips (source_id, uid, occurrence) VALUES (?, 'skip@test', '')`,
		sourceID,
	); err != nil {
		t.Fatalf("seed skip: %v", err)
	}

	if err := Sync(context.Background(), db, nil, sourceID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM events WHERE source_id = ? AND source_uid = 'skip@test'`, sourceID).Scan(&n)
	if n != 0 {
		t.Error("skip-listed item was imported")
	}
	if total := countEvents(t, db, sourceID); total != 1 {
		t.Errorf("expected only the kept event, got %d", total)
	}
}
