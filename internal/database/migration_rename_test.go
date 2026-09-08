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
// in the 50 minutes it was on main. Hence this test, which builds that
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

func TestRenamedMigrationDoesNotRerun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "patchwork.db")

	// An instance that upgraded while the collision was on main.
	pre, err := database.Open(path, asShipped(t, migrationsFS(t), "068_contact_items", "066_contact_items"))
	if err != nil {
		t.Fatalf("open with the shipped names: %v", err)
	}
	var version string
	if err := pre.QueryRow(
		"SELECT version FROM schema_migrations WHERE version LIKE '%_contact_items';").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "066_contact_items" {
		t.Fatalf("setup recorded %q, want 066_contact_items", version)
	}
	pre.Close()

	// The same database, upgraded to a build carrying the renumber.
	post, err := database.Open(path, migrationsFS(t))
	if err != nil {
		t.Fatalf("renamed migration re-ran on an existing database: %v", err)
	}
	defer post.Close()

	var rows int
	if err := post.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations WHERE version LIKE '%_contact_items';").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("schema_migrations has %d moved_to rows, want exactly 1", rows)
	}
	if err := post.QueryRow(
		"SELECT version FROM schema_migrations WHERE version LIKE '%_contact_items';").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "068_contact_items" {
		t.Errorf("recorded version is %q, want 068_contact_items", version)
	}
}

// A fresh database runs the renamed file normally: the guard is a no-op when
// there is nothing recorded under the old name.
func TestRenamedMigrationAppliesOnFreshDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "patchwork.db")
	db, err := database.Open(path, migrationsFS(t))
	if err != nil {
		t.Fatalf("open fresh: %v", err)
	}
	defer db.Close()

	var version string
	if err := db.QueryRow(
		"SELECT version FROM schema_migrations WHERE version LIKE '%_contact_items';").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "068_contact_items" {
		t.Errorf("fresh database recorded %q, want 068_contact_items", version)
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
