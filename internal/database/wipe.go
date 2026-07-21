package database

import (
	"context"
	"fmt"
)

// Wipe deletes every row from every data table, returning the database to
// first-run state while keeping the schema and migration history
// (docs/adr/014: wiping the quilt's data is not tearing down the
// deployment).
//
// All deletes happen in one transaction on a dedicated connection with
// foreign keys disabled — the pragma is per-connection and must be set
// outside a transaction, so the pool's other connections keep enforcing
// FKs throughout.
func (db *DB) Wipe(ctx context.Context) error {
	rows, err := db.Query(`SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name != 'schema_migrations'`)
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}
	// The connection goes back to the pool afterwards, so always restore.
	restore := func() {
		conn.ExecContext(ctx, "PRAGMA foreign_keys = ON") //nolint:errcheck
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		restore()
		return fmt.Errorf("begin wipe tx: %w", err)
	}
	for _, t := range tables {
		// Table names come from sqlite_master (our own schema), quoted
		// defensively anyway.
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %q`, t)); err != nil {
			tx.Rollback()
			restore()
			return fmt.Errorf("wipe %s: %w", t, err)
		}
	}
	if err := tx.Commit(); err != nil {
		restore()
		return fmt.Errorf("commit wipe: %w", err)
	}
	restore()

	// Reclaim the space; best effort.
	conn.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)") //nolint:errcheck
	return nil
}
