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
