package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// Migration 044 drops the uploaded-icon table and the stale default-block
// key (docs/adr/042). setupTestDB has already run it, so there is nothing
// left for it to drop — this test rebuilds the pre-044 state an upgrading
// instance actually has and replays the migration's own SQL, the same
// shape as the 042 and 043 migration tests.
func TestMigration044DropsTheUploadedIcon(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()

	// An instance that had uploaded an icon and, earlier, picked a default.
	if _, err := db.Exec(`CREATE TABLE instance_icon (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		mime TEXT NOT NULL,
		data BLOB NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("recreate instance_icon: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO instance_icon (id, mime, data, updated_at)
		VALUES (1, 'image/png', X'89504E470D0A1A0A', '2026-07-01T00:00:00Z')`); err != nil {
		t.Fatalf("seed upload: %v", err)
	}
	if err := settings.Set(db, "icon_default", "pinwheel"); err != nil {
		t.Fatalf("seed icon_default: %v", err)
	}

	// The endpoint already ignores both — the upload stopped being served
	// the moment the new binary booted, not when the migration ran.
	r := httptest.NewRequest("GET", "/api/v1/instance/icon", nil)
	w := httptest.NewRecorder()
	handler.InstanceIcon(db, cfg)(w, r)
	if ct := w.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("with an upload still in the table, icon served as %q", ct)
	}

	replayMigration(t, db, "migrations/044_drop_instance_icon.sql")

	if tables := countRows(db, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='instance_icon'`); tables != 0 {
		t.Error("instance_icon survived the migration")
	}
	if keys := countRows(db, `SELECT COUNT(*) FROM instance_settings WHERE key='icon_default'`); keys != 0 {
		t.Error("the stale icon_default key survived the migration")
	}

	// Running it again is a no-op: an instance that never uploaded anything
	// upgrades just as cleanly.
	replayMigration(t, db, "migrations/044_drop_instance_icon.sql")

	w = httptest.NewRecorder()
	handler.InstanceIcon(db, cfg)(w, httptest.NewRequest("GET", "/api/v1/instance/icon", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("after the drop, icon: got %d", w.Code)
	}
}

func replayMigration(t *testing.T, db *database.DB, path string) {
	t.Helper()
	sql, err := patchwork.MigrationsFS.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if _, err := db.Exec(string(sql)); err != nil {
		t.Fatalf("run %s: %v", path, err)
	}
}
