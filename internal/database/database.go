package database

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps a sql.DB with Patchwork-specific behavior.
type DB struct {
	*sql.DB
}

// Open creates or opens a SQLite database at path, runs startup checks,
// configures PRAGMAs, and applies pending migrations.
func Open(path string, migrationsFS fs.FS) (*DB, error) {
	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	// Startup checks run on a single connection before the pool opens.
	startup, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open startup conn: %w", err)
	}
	startup.SetMaxOpenConns(1)

	var integrity string
	if err := startup.QueryRow("PRAGMA integrity_check;").Scan(&integrity); err != nil {
		startup.Close()
		return nil, fmt.Errorf("integrity check: %w", err)
	}
	if integrity != "ok" {
		startup.Close()
		return nil, fmt.Errorf("integrity check failed: %s", integrity)
	}
	log.Println("database: integrity_check passed")

	if _, err := startup.Exec("PRAGMA wal_checkpoint(TRUNCATE);"); err != nil {
		startup.Close()
		return nil, fmt.Errorf("wal checkpoint: %w", err)
	}
	log.Println("database: wal_checkpoint(TRUNCATE) done")
	startup.Close()

	// Open the real pool with PRAGMAs applied per-connection via the DSN.
	dsn := path + "?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000&_foreign_keys=ON&_cache_size=-64000"
	pool, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}

	// Verify PRAGMAs are set on the pool connection.
	if _, err := pool.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA synchronous = NORMAL;
		PRAGMA busy_timeout = 5000;
		PRAGMA foreign_keys = ON;
		PRAGMA cache_size = -64000;
		PRAGMA wal_autocheckpoint = 1000;
	`); err != nil {
		pool.Close()
		return nil, fmt.Errorf("set pragmas: %w", err)
	}

	db := &DB{DB: pool}

	if migrationsFS != nil {
		if err := db.migrate(migrationsFS); err != nil {
			pool.Close()
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}

	// SQLite creates its files 0666 & ~umask — 0644 under the usual umask, which
	// contradicts the 600 this project promises. Enforce it from here rather than
	// relying on the deployment's umask. Runs after migrations so the -wal and
	// -shm sidecars, which hold the same data, exist and get the same treatment.
	restrictPerms(path)

	return db, nil
}

// OpenReadOnly opens path for reading only, and never writes to it.
//
// database.Open's startup sequence — a wal_checkpoint(TRUNCATE) followed by
// applying every embedded migration — is right for a server that owns the
// file. It is wrong for a tool that was only asked to read it: cmd/export,
// run from a source checkout newer than the image a running instance
// actually serves, silently upgraded a live instance's on-disk schema out
// from under it (GH issue #246; measured against a stopped 0.25.1 instance,
// the file's sha256 changed and its size dropped by 2/3 even though nothing
// but the export tool touched it). OpenReadOnly is the shape that cannot do
// that: it opens with SQLite's `mode=ro` URI, so the connection has no write
// access to the file at all, and it runs no migration.
//
// Only the PRAGMAs that make sense without write access are set —
// busy_timeout, foreign_keys, cache_size. journal_mode, synchronous and
// wal_autocheckpoint all require the ability to write the database file, and
// there is deliberately no startup wal_checkpoint here: that call writes the
// main database file (folding the WAL back into it), which is exactly the
// "export rewrote the file it was asked to read" behavior this exists to
// stop. A WAL database's committed-but-not-yet-checkpointed pages still live
// in the -wal file, so a read-only open needs the -wal and -shm sidecars
// readable alongside the main file — see docs/DEPLOYMENT.md.
//
// If migrationsFS is non-nil, OpenReadOnly also refuses to open a database
// whose schema_migrations table names a migration this binary's embedded set
// does not include: that database is from a *newer* build than the one
// asking to read it, so reading it without complaint would silently present
// a schema this binary cannot fully account for. The error names both the
// unrecognized version and the newest one this binary knows, so an operator
// can tell which side needs updating. Pass a nil migrationsFS to skip that
// check entirely — there is then nothing to compare the recorded versions
// against, and the caller is asserting it does not need the guarantee.
func OpenReadOnly(path string, migrationsFS fs.FS) (*DB, error) {
	// mode=ro does not create the file, and reports it missing with a less
	// direct error than this. Check first so the failure is legible.
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("open read-only: %w", err)
	}

	// The "file:" prefix is required for go-sqlite3 to forward the query
	// string to SQLite's own URI parser (see sqlite3.go's DSN handling) —
	// without it, everything after "?" is stripped before the C call and
	// mode=ro is silently ignored, opening read-write as usual.
	dsn := "file:" + path + "?mode=ro&_busy_timeout=5000&_foreign_keys=ON&_cache_size=-64000"
	pool, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open read-only pool: %w", err)
	}

	if _, err := pool.Exec(`
		PRAGMA busy_timeout = 5000;
		PRAGMA foreign_keys = ON;
		PRAGMA cache_size = -64000;
	`); err != nil {
		pool.Close()
		return nil, fmt.Errorf("set read-only pragmas: %w", err)
	}

	db := &DB{DB: pool}

	if migrationsFS != nil {
		if err := db.refuseIfSchemaAheadOfBinary(migrationsFS); err != nil {
			pool.Close()
			return nil, err
		}
	}

	return db, nil
}

// refuseIfSchemaAheadOfBinary compares the versions recorded in
// schema_migrations against migrationsFS and fails if the database has
// applied one this binary's embedded migrations do not include — the
// signal that the file was last written by a newer build.
//
// A database with no schema_migrations table at all has never been
// migrated, which is behind, not ahead; that is not this check's problem.
func (db *DB) refuseIfSchemaAheadOfBinary(migrationsFS fs.FS) error {
	entries, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	known := make(map[string]bool)
	var knownVersions []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		v := strings.TrimSuffix(e.Name(), ".sql")
		known[v] = true
		knownVersions = append(knownVersions, v)
	}
	// A migration recorded under its pre-rename name (renamedMigrations'
	// values) is a version this binary still recognizes, just under an old
	// filename — see database.go's rename table.
	for _, old := range renamedMigrations {
		known[old] = true
	}
	sort.Strings(knownVersions)
	newestKnown := "(none)"
	if len(knownVersions) > 0 {
		newestKnown = knownVersions[len(knownVersions)-1]
	}

	var tableExists int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations';",
	).Scan(&tableExists); err != nil {
		return fmt.Errorf("check for schema_migrations: %w", err)
	}
	if tableExists == 0 {
		return nil
	}

	rows, err := db.Query("SELECT version FROM schema_migrations;")
	if err != nil {
		return fmt.Errorf("query schema_migrations: %w", err)
	}
	defer rows.Close()

	var unknown []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("scan schema_migrations: %w", err)
		}
		if !known[v] {
			unknown = append(unknown, v)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	newestUnknown := unknown[len(unknown)-1]

	return fmt.Errorf(
		"database schema is ahead of this binary: it has applied migration %q, which this binary's "+
			"embedded migrations do not include (the newest one this binary knows is %q). This database "+
			"was written by a newer build. Run this tool from a checkout matching the running instance's "+
			"version, or upgrade this binary before reading its database",
		newestUnknown, newestKnown)
}

// restrictPerms tightens the SQLite database file and its WAL/SHM sidecars to
// 0600. Failures are logged, not fatal: chmod is a no-op on Windows dev boxes,
// and a running instance is more useful than one that refuses to start over
// file modes it may not own.
func restrictPerms(path string) {
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if _, err := os.Stat(p); err != nil {
			continue // sidecars may not exist yet
		}
		if err := os.Chmod(p, 0600); err != nil {
			log.Printf("database: could not chmod %s to 0600: %v", p, err)
		}
	}
}

// renamedMigrations maps a migration's current filename to a filename it has
// already shipped under, for the case where a merged migration is renumbered.
//
// The runner records the *whole filename* in schema_migrations, so renaming a
// file that already reached a real database makes it look unapplied. Migrations
// are not idempotent — 068_contact_items' CREATE TABLE fails with "table
// contact_items already exists" on a second pass — and Open's error is fatal in
// cmd/patchwork, so an unguarded rename is an instance that will not start.
// This table closes that: a database that recorded the old name has its row
// renamed in place, keeping applied_at, and the file is not run again.
//
// Entries are permanent. A name that once reached a real database never stops
// meaning something, and dropping an entry breaks the upgrade it exists for.
var renamedMigrations = map[string]string{
	// 066_contact_items.sql (docs/adr/083) and 066_moved_to.sql (docs/adr/090)
	// both reached main as 066 and both applied, because the versions differ
	// past the number. contact_items was renumbered to 068 afterwards, by which
	// point instances had already recorded it under the old name.
	"068_contact_items": "066_contact_items",
}

// applyRenames rewrites schema_migrations rows recorded under a superseded
// filename, so a database that already ran the file does not run it twice.
func (db *DB) applyRenames(applied map[string]bool) error {
	for current, old := range renamedMigrations {
		if !applied[old] || applied[current] {
			continue
		}
		if _, err := db.Exec(
			"UPDATE schema_migrations SET version = ? WHERE version = ?;",
			current, old,
		); err != nil {
			return fmt.Errorf("rename migration %s to %s: %w", old, current, err)
		}
		applied[current] = true
		delete(applied, old)
		log.Printf("database: migration %s was recorded as %s; renamed in place", current, old)
	}
	return nil
}

// migrate applies all .sql files from the migrations FS that haven't been run.
func (db *DB) migrate(migrationsFS fs.FS) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	);`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	applied := make(map[string]bool)
	rows, err := db.Query("SELECT version FROM schema_migrations;")
	if err != nil {
		return fmt.Errorf("query migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = true
	}
	// Closed before applyRenames writes: SQLite will not take the write while
	// this read is still open.
	rows.Close()

	if err := db.applyRenames(applied); err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		version := strings.TrimSuffix(name, ".sql")
		if applied[version] {
			continue
		}

		data, err := fs.ReadFile(migrationsFS, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", name, err)
		}

		if _, err := tx.Exec(string(data)); err != nil {
			tx.Rollback()
			return fmt.Errorf("exec %s: %w", name, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?);", version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record %s: %w", name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
		}

		log.Printf("database: applied migration %s", name)
	}

	return nil
}
