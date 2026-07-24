package database

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	patchwork "github.com/patchwork-toolkit/patchwork"
)

// The database file holds session tokens, email addresses, and governance
// records. CLAUDE.md promises mode 600; SQLite's own default is 0666 & ~umask,
// which lands at 0644 in the production volume. Open must tighten it rather
// than trusting the deployment's umask.
func TestOpenRestrictsFilePerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes are not meaningful on Windows")
	}

	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "perms.db")
	db, err := Open(path, migrations)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Force a write so the WAL sidecar definitely exists.
	if _, err := db.Exec(`CREATE TABLE perms_probe (id TEXT)`); err != nil {
		t.Fatalf("probe write: %v", err)
	}
	restrictPerms(path)

	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(p)
		if err != nil {
			continue // sidecars are not guaranteed to exist
		}
		if mode := info.Mode().Perm(); mode != 0600 {
			t.Errorf("%s has mode %04o, want 0600", filepath.Base(p), mode)
		}
	}
}
