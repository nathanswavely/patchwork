package database_test

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// The event tier migration, run against a database that already holds the
// words it retires.
//
// A fresh migrate is the one database that cannot show this working: it has
// no 'private' or 'unlisted' event to map and no child row to lose. So this
// builds the database an instance actually has — every migration up to the
// rebuild, then rows written through the old CHECK — and only then runs it.
//
// The child rows are the half a rebuild of `events` gets wrong quietly.
// `events` is a parent four times over, the runner wraps each migration in a
// transaction, and PRAGMA foreign_keys cannot be turned off inside one, so a
// plain DROP TABLE takes every linked event, mention, hold and dismissal with
// it and nothing reports a thing.

const eventTiersMigration = "20260919T155435_event_visibility_tiers"

// migrationsBefore returns the embedded migrations with `version` and
// everything sorting after it left out — the schema as it stood the moment
// before that file ran.
func migrationsBefore(t *testing.T, version string) fstest.MapFS {
	t.Helper()
	all := migrationsFS(t)
	if _, ok := all[version+".sql"]; !ok {
		var names []string
		for n := range all {
			names = append(names, n)
		}
		sort.Strings(names)
		t.Fatalf("%s.sql is not in the migrations FS; have %v", version, names)
	}
	before := fstest.MapFS{}
	for name, f := range all {
		if strings.TrimSuffix(name, ".sql") < version {
			before[name] = f
		}
	}
	return before
}

func mustExec(t *testing.T, db *database.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %.60s...: %v", query, err)
	}
}

func TestEventVisibilityTiersMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "patchwork.db")

	old, err := database.Open(path, migrationsBefore(t, eventTiersMigration))
	if err != nil {
		t.Fatalf("open pre-migration database: %v", err)
	}

	// The cast: one person, two patches, one calendar feed, one aggregator
	// with a program credited to a patch.
	mustExec(t, old, `INSERT INTO users (id, username) VALUES ('u1', 'ada')`)
	mustExec(t, old, `INSERT INTO nodes (id, owner_id, name, slug) VALUES ('n1', 'u1', 'Host', 'host')`)
	mustExec(t, old, `INSERT INTO nodes (id, owner_id, name, slug) VALUES ('n2', 'u1', 'Linked', 'linked')`)
	mustExec(t, old, `INSERT INTO event_sources (id, node_id, url, added_by) VALUES ('s1', 'n1', 'https://example.test/f.ics', 'u1')`)
	mustExec(t, old, `INSERT INTO aggregators (id, name, url, added_by) VALUES ('g1', 'City', 'https://example.test/city.ics', 'u1')`)
	mustExec(t, old, `INSERT INTO aggregator_programs (id, aggregator_id, name_key, title_key, display_title, node_id, credited_by)
		VALUES ('p1', 'g1', 'binns', 'music', 'Music Friday', 'n1', 'u1')`)

	// Three events, one per word the old CHECK allowed. Written through the
	// old schema, which is the only place they can come from.
	for _, ev := range []struct{ id, vis string }{
		{"e-public", "public"},
		{"e-private", "private"},
		{"e-unlisted", "unlisted"},
	} {
		mustExec(t, old,
			`INSERT INTO events (id, node_id, created_by, title, starts_at, visibility, ap_id, event_url, timezone)
			 VALUES (?, 'n1', 'u1', ?, '2026-10-01T19:00:00Z', ?, ?, 'https://example.test/e', 'America/New_York')`,
			ev.id, ev.id+" night", ev.vis, "https://example.test/ap/"+ev.id)
	}
	// One imported row, so the partial unique index on the source identity
	// has something to be unique about after the rebuild.
	mustExec(t, old,
		`INSERT INTO events (id, node_id, created_by, title, starts_at, visibility, source_id, source_uid, source_occurrence)
		 VALUES ('e-feed', 'n1', 'u1', 'From the feed', '2026-10-02T19:00:00Z', 'public', 's1', 'uid-1', '')`)

	// A row in every table that hangs off events.
	mustExec(t, old, `INSERT INTO event_links (id, event_id, node_id, initiated_by, requested_by, absorb_event_id, status)
		VALUES ('l1', 'e-public', 'n2', 'owner', 'u1', 'e-feed', 'confirmed')`)
	mustExec(t, old, `INSERT INTO event_mentions (id, event_id, host, slug) VALUES ('m1', 'e-private', 'other.test', 'band')`)
	mustExec(t, old, `INSERT INTO aggregator_holds (id, source_id, node_id, uid, occurrence, rival_event_id, title, starts_at)
		VALUES ('h1', 's1', 'n1', 'uid-2', '', 'e-unlisted', 'Held', '2026-10-03T19:00:00Z')`)
	mustExec(t, old, `INSERT INTO aggregator_offer_dismissals (program_id, event_id, dismissed_by) VALUES ('p1', 'e-feed', 'u1')`)

	// The old CHECK is still the old CHECK.
	if _, err := old.Exec(`INSERT INTO events (id, node_id, created_by, title, starts_at, visibility)
		VALUES ('e-too-soon', 'n1', 'u1', 'Too soon', '2026-10-04T19:00:00Z', 'members')`); err == nil {
		t.Fatal("the pre-migration schema accepted 'members'; this test is not testing what it says it is")
	}
	old.Close()

	db, err := database.Open(path, migrationsFS(t))
	if err != nil {
		t.Fatalf("run the migration: %v", err)
	}
	defer db.Close()

	// 1. The mapping. private and unlisted both land on members — the
	//    non-exposing direction — and public is untouched.
	for id, want := range map[string]string{
		"e-public":   "public",
		"e-private":  "members",
		"e-unlisted": "members",
		"e-feed":     "public",
	} {
		var got string
		if err := db.QueryRow(`SELECT visibility FROM events WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if got != want {
			t.Errorf("%s: visibility = %q, want %q", id, got, want)
		}
	}

	// 2. Nothing else about the row moved.
	var title, apID, url, tz, status string
	if err := db.QueryRow(
		`SELECT title, ap_id, event_url, timezone, status FROM events WHERE id = 'e-private'`,
	).Scan(&title, &apID, &url, &tz, &status); err != nil {
		t.Fatal(err)
	}
	if title != "e-private night" || apID != "https://example.test/ap/e-private" ||
		url != "https://example.test/e" || tz != "America/New_York" || status != "active" {
		t.Errorf("columns did not survive the copy: %q %q %q %q %q", title, apID, url, tz, status)
	}

	// 3. The children are all still there. This is the assertion a fresh
	//    migrate cannot make.
	for _, c := range []struct {
		table string
		want  int
	}{
		{"event_links", 1},
		{"event_mentions", 1},
		{"aggregator_holds", 1},
		{"aggregator_offer_dismissals", 1},
	} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + c.table).Scan(&n); err != nil {
			t.Fatalf("%s: %v", c.table, err)
		}
		if n != c.want {
			t.Errorf("%s has %d rows after the rebuild, want %d — the DROP cascaded", c.table, n, c.want)
		}
	}
	var absorb string
	db.QueryRow(`SELECT COALESCE(absorb_event_id,'') FROM event_links WHERE id = 'l1'`).Scan(&absorb)
	if absorb != "e-feed" {
		t.Errorf("event_links.absorb_event_id = %q, want e-feed (ON DELETE SET NULL fired)", absorb)
	}
	var violations int
	db.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&violations)
	if violations != 0 {
		t.Errorf("foreign_key_check reports %d violations after the rebuild", violations)
	}

	// 4. Every index came back, including the two partial unique ones.
	for _, idx := range []string{
		"idx_events_node_id", "idx_events_starts_at", "idx_events_status",
		"idx_events_ap_id", "idx_events_source_identity",
	} {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, idx).Scan(&n)
		if n != 1 {
			t.Errorf("index %s did not survive the rebuild", idx)
		}
	}
	if _, err := db.Exec(
		`INSERT INTO events (id, node_id, created_by, title, starts_at, source_id, source_uid, source_occurrence)
		 VALUES ('e-dup', 'n1', 'u1', 'Same feed key', '2026-10-05T19:00:00Z', 's1', 'uid-1', '')`,
	); err == nil {
		t.Error("idx_events_source_identity no longer refuses a duplicate feed key")
	}

	// 5. The new CHECK, both directions.
	for _, v := range []string{"public", "followers", "members"} {
		if _, err := db.Exec(
			`INSERT INTO events (id, node_id, created_by, title, starts_at, visibility)
			 VALUES (?, 'n1', 'u1', 'ok', '2026-11-01T19:00:00Z', ?)`, "e-ok-"+v, v,
		); err != nil {
			t.Errorf("the rebuilt events table refuses %q: %v", v, err)
		}
	}
	for _, v := range []string{"private", "unlisted", "nonsense"} {
		if _, err := db.Exec(
			`INSERT INTO events (id, node_id, created_by, title, starts_at, visibility)
			 VALUES (?, 'n1', 'u1', 'no', '2026-11-02T19:00:00Z', ?)`, "e-no-"+v, v,
		); err == nil {
			t.Errorf("the rebuilt events table still accepts %q", v)
		}
	}

	// 6. nodes.visibility keeps its own three words. The two columns stopped
	//    sharing a vocabulary; they did not swap one.
	for _, v := range []string{"public", "private", "unlisted"} {
		if _, err := db.Exec(`UPDATE nodes SET visibility = ? WHERE id = 'n1'`, v); err != nil {
			t.Errorf("nodes.visibility no longer accepts %q: %v", v, err)
		}
	}
	if _, err := db.Exec(`UPDATE nodes SET visibility = 'followers' WHERE id = 'n1'`); err == nil {
		t.Error("nodes.visibility accepted 'followers'; the sweep reached the wrong column")
	}

	// 7. The event source default tier.
	var srcVis string
	if err := db.QueryRow(`SELECT visibility FROM event_sources WHERE id = 's1'`).Scan(&srcVis); err != nil {
		t.Fatalf("event_sources.visibility: %v", err)
	}
	if srcVis != "public" {
		t.Errorf("an existing feed's default tier is %q, want public", srcVis)
	}
	if _, err := db.Exec(`UPDATE event_sources SET visibility = 'members' WHERE id = 's1'`); err != nil {
		t.Errorf("event_sources.visibility refuses 'members': %v", err)
	}
	if _, err := db.Exec(`UPDATE event_sources SET visibility = 'private' WHERE id = 's1'`); err == nil {
		t.Error("event_sources.visibility accepts 'private'")
	}
}
