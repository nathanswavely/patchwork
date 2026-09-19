package database_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// OpenReadOnly must be unable to write, and its own open must not be a write
// either — GH issue #246 was exactly this: an exporter opened through Open,
// which checkpoints the WAL and migrates the schema before handing back a
// connection, so a tool that only meant to read the database rewrote it.

func TestOpenReadOnlyRejectsWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "patchwork.db")

	rw, err := database.Open(path, migrationsFS(t))
	if err != nil {
		t.Fatalf("open read-write: %v", err)
	}
	defer rw.Close()

	ro, err := database.OpenReadOnly(path, migrationsFS(t))
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer ro.Close()

	if _, err := ro.Exec(
		"INSERT INTO schema_migrations (version) VALUES ('readonly_probe');",
	); err == nil {
		t.Fatal("write through a read-only connection succeeded, want an error")
	}
}

// The open itself must not touch the file: no checkpoint, no migration.
// database.Open's startup wal_checkpoint(TRUNCATE) would shrink or remove
// the -wal file, which is precisely the "export rewrote what it read"
// behavior from GH issue #246.
func TestOpenReadOnlyDoesNotCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "patchwork.db")

	rw, err := database.Open(path, migrationsFS(t))
	if err != nil {
		t.Fatalf("open read-write: %v", err)
	}
	defer rw.Close()

	// Pad the WAL past whatever the migrations alone left behind, so a
	// checkpoint that truncated it would be unmistakable. Ordinary data,
	// not schema_migrations — a fake version row would trip the
	// ahead-of-binary check this test isn't exercising.
	for i := 0; i < 50; i++ {
		if _, err := rw.Exec(
			"INSERT INTO tags (id, name) VALUES (?, ?);",
			"wal-padding-"+strings.Repeat("0", i), "wal padding tag "+strings.Repeat("x", i),
		); err != nil {
			t.Fatalf("pad wal: %v", err)
		}
	}

	walPath := path + "-wal"
	before, err := os.Stat(walPath)
	if err != nil {
		t.Fatalf("stat -wal before: %v", err)
	}

	ro, err := database.OpenReadOnly(path, migrationsFS(t))
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer ro.Close()

	// Touch the connection — a lazy driver that never opens the file until
	// first use would otherwise let this test pass for the wrong reason.
	var n int
	if err := ro.QueryRow("SELECT COUNT(*) FROM schema_migrations;").Scan(&n); err != nil {
		t.Fatalf("query through read-only connection: %v", err)
	}

	after, err := os.Stat(walPath)
	if err != nil {
		t.Fatalf("stat -wal after: %v", err)
	}

	if after.Size() != before.Size() {
		t.Errorf("-wal size changed from %d to %d bytes opening read-only — the open wrote to the file",
			before.Size(), after.Size())
	}
}

// A database carrying a schema_migrations row this binary's embedded
// migrations don't include is from a newer build. OpenReadOnly must refuse
// it rather than read a schema it cannot fully account for.
func TestOpenReadOnlyRefusesSchemaAheadOfBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "patchwork.db")

	rw, err := database.Open(path, migrationsFS(t))
	if err != nil {
		t.Fatalf("open read-write: %v", err)
	}
	if _, err := rw.Exec(
		"INSERT INTO schema_migrations (version) VALUES ('99999999T000000_from_the_future');",
	); err != nil {
		rw.Close()
		t.Fatalf("insert future migration row: %v", err)
	}
	rw.Close()

	_, err = database.OpenReadOnly(path, migrationsFS(t))
	if err == nil {
		t.Fatal("open read-only on a schema ahead of the binary succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "99999999T000000_from_the_future") {
		t.Errorf("error %q does not name the unrecognized version", err)
	}
}

// A nil migrationsFS means the caller has nothing to compare the recorded
// versions against, so OpenReadOnly skips the ahead-of-binary check rather
// than refusing everything.
func TestOpenReadOnlyWithNilMigrationsFSSkipsCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "patchwork.db")

	rw, err := database.Open(path, migrationsFS(t))
	if err != nil {
		t.Fatalf("open read-write: %v", err)
	}
	if _, err := rw.Exec(
		"INSERT INTO schema_migrations (version) VALUES ('99999999T000000_from_the_future');",
	); err != nil {
		rw.Close()
		t.Fatalf("insert future migration row: %v", err)
	}
	rw.Close()

	ro, err := database.OpenReadOnly(path, nil)
	if err != nil {
		t.Fatalf("open read-only with nil migrationsFS: %v", err)
	}
	ro.Close()
}

func TestOpenReadOnlyMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.db")
	if _, err := database.OpenReadOnly(path, migrationsFS(t)); err == nil {
		t.Fatal("open read-only on a missing file succeeded, want an error")
	}
}
