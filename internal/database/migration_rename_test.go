package database_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// A migration that was renamed after it merged does not run twice.
//
// The runner keys schema_migrations on the whole filename, so renaming a file
// makes an already-migrated database see it as pending. Migrations are not
// idempotent — 068_contact_items' CREATE TABLE fails with "table contact_items
// already exists" on the second pass — and Open's error is fatal in
// cmd/patchwork, so an unguarded rename turns every upgrade into a server that
// will not start.
//
// database.renamedMigrations is what makes the rename safe, and it is
// load-bearing only on databases CI never has: one that ran 066_contact_items
// in the 50 minutes it was on main, or 075_follower_charters_off_by_default
// before it was stamped. Hence this test, which builds that
// database on purpose.

// migrationsFS copies the embedded migrations into a mutable in-memory FS.
func migrationsFS(t *testing.T) fstest.MapFS {
	t.Helper()
	sub, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	m := fstest.MapFS{}
	entries, err := fs.ReadDir(sub, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := fs.ReadFile(sub, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		m[e.Name()] = &fstest.MapFile{Data: b}
	}
	return m
}

// asShipped re-keys a renamed migration back to the name it merged under, so
// the test can build the database an existing instance actually has.
func asShipped(t *testing.T, m fstest.MapFS, current, old string) fstest.MapFS {
	t.Helper()
	f, ok := m[current+".sql"]
	if !ok {
		t.Fatalf("%s.sql is not in the migrations FS — was it renamed again?", current)
	}
	delete(m, current+".sql")
	m[old+".sql"] = f
	return m
}

// renames is every entry in database.renamedMigrations, restated here because
// the map is unexported and because a rename that lands without a test is the
// exact gap this file exists to close. Add a pair here for every entry there.
var renames = []struct{ current, old string }{
	{"068_contact_items", "066_contact_items"},
	// 075 merged after the migration number space closed at 074 and ran on
	// the reference instance under that name before the naming guard caught
	// it (docs/adr/2026-09-16-a-name-nobody-has-to-ask-for.md).
	{"20260917T022312_follower_charters_off_by_default", "075_follower_charters_off_by_default"},
}

// suffix is the part of a version both names share, for a LIKE lookup that
// finds the row whichever name it is recorded under.
func suffix(version string) string {
	_, rest, _ := strings.Cut(version, "_")
	return "%_" + rest
}

func recordedVersions(t *testing.T, db *database.DB, like string) []string {
	t.Helper()
	rows, err := db.Query("SELECT version FROM schema_migrations WHERE version LIKE ? ORDER BY version;", like)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

// Each rename is proven from a database that recorded the OLD name — never
// from a fresh migrate, which is the one database that cannot show the
// problem. The last case carries every old name at once, which is the
// database an instance that upgraded through both windows has.
func TestRenamedMigrationDoesNotRerun(t *testing.T) {
	cases := map[string][]struct{ current, old string }{}
	for _, r := range renames {
		cases[r.old] = []struct{ current, old string }{r}
	}
	cases["every old name at once"] = renames

	for name, pairs := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "patchwork.db")

			// An instance that upgraded while the old name was on main.
			shipped := migrationsFS(t)
			for _, p := range pairs {
				shipped = asShipped(t, shipped, p.current, p.old)
			}
			pre, err := database.Open(path, shipped)
			if err != nil {
				t.Fatalf("open with the shipped names: %v", err)
			}
			for _, p := range pairs {
				if got := recordedVersions(t, pre, suffix(p.old)); len(got) != 1 || got[0] != p.old {
					t.Fatalf("setup recorded %q, want [%s]", got, p.old)
				}
			}
			pre.Close()

			// The same database, upgraded to a build carrying the rename.
			post, err := database.Open(path, migrationsFS(t))
			if err != nil {
				t.Fatalf("renamed migration re-ran on an existing database: %v", err)
			}
			defer post.Close()

			for _, p := range pairs {
				got := recordedVersions(t, post, suffix(p.current))
				if len(got) != 1 || got[0] != p.current {
					t.Errorf("schema_migrations rows for %s: %q, want exactly [%s]", p.current, got, p.current)
				}
			}
		})
	}
}

// A fresh database runs the renamed files normally: the guard is a no-op when
// there is nothing recorded under the old name.
func TestRenamedMigrationAppliesOnFreshDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "patchwork.db")
	db, err := database.Open(path, migrationsFS(t))
	if err != nil {
		t.Fatalf("open fresh: %v", err)
	}
	defer db.Close()

	for _, r := range renames {
		got := recordedVersions(t, db, suffix(r.current))
		if len(got) != 1 || got[0] != r.current {
			t.Errorf("fresh database recorded %q, want [%s]", got, r.current)
		}
	}
	for _, table := range []string{"contact_items", "contact_item_shares"} {
		if _, err := db.Exec("SELECT 1 FROM " + table + " LIMIT 1;"); err != nil {
			t.Errorf("%s missing after a fresh migrate: %v", table, err)
		}
	}
}

// No two migrations share a number. The runner sorts by filename and records
// the whole filename, so a collision applies cleanly and silently — nothing
// but this test notices, and every "migration 0NN" citation goes ambiguous.
//
// The number space closed at 074 and new migrations are timestamped, so this
// now guards a fixed range rather than a growing one: it can only fail if a
// legacy file is renamed. The check that a *new* migration is not numbered
// lives in the root package's TestNewRecordsAreNotNumbered. See
// docs/adr/2026-09-16-a-name-nobody-has-to-ask-for.md.
func TestMigrationNumbersAreUnique(t *testing.T) {
	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		number, _, ok := strings.Cut(name, "_")
		if !ok {
			t.Errorf("migration %s has no NNN_ prefix", name)
			continue
		}
		if prior, dup := seen[number]; dup {
			t.Errorf("migrations %s and %s share the number %s", prior, name, number)
			continue
		}
		seen[number] = name
	}
}
