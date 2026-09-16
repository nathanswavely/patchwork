package main

import (
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

func TestShiftWhole_KeepsEachLayout(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct{ in, want string }{
		{"2026-09-14T10:00:00.000Z", "2026-09-13T10:00:00.000Z"},
		{"2026-09-14T10:00:00Z", "2026-09-13T10:00:00Z"},
		{"2026-09-14T10:00:00.123456Z", "2026-09-13T10:00:00.123456Z"},
		{"2026-09-14T10:00:00-04:00", "2026-09-13T10:00:00-04:00"},
		{"2026-09-14T10:00:00+00:00", "2026-09-13T10:00:00+00:00"},
		{"2026-09-14T10:00:00", "2026-09-13T10:00:00"},
		{"2026-09-14 10:00:00", "2026-09-13 10:00:00"},
		{"2026-09-14", "2026-09-13"},
		{"2026-03-01", "2026-02-28"},
	}
	for _, c := range cases {
		got, ok := shiftWhole(c.in, day)
		if !ok || got != c.want {
			t.Errorf("shiftWhole(%q) = %q, %v; want %q", c.in, got, ok, c.want)
		}
	}
	for _, notATime := range []string{"", "hello", "2026", "20260914T100000Z", "the 2026-09-14 meeting", "12:00:00", "2026-09-14-extra"} {
		if _, ok := shiftWhole(notATime, day); ok {
			t.Errorf("shiftWhole(%q) moved a value that is not a timestamp", notATime)
		}
	}
}

func TestShiftText_MovesInstantsInsideJSON(t *testing.T) {
	in := `{"opened":"2026-09-14T10:00:00.000Z","closes":"2026-09-21T10:00:00Z","note":"meet on 2026-09-30 at the hall"}`
	got, n := shiftText(in, 7*24*time.Hour)
	want := `{"opened":"2026-09-07T10:00:00.000Z","closes":"2026-09-14T10:00:00Z","note":"meet on 2026-09-30 at the hall"}`
	if got != want {
		t.Errorf("shiftText =\n %s\nwant\n %s", got, want)
	}
	if n != 2 {
		t.Errorf("moved %d instants, want 2 (the bare date in prose stays)", n)
	}
}

func openTestDB(t *testing.T) *database.DB {
	t.Helper()
	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(filepath.Join(t.TempDir(), "sim.db"), migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// The exclusion list names tables. A rename would leave a stale name that
// excludes nothing and quietly starts moving sign-in expiries; this is the
// check that a listed name still means something.
func TestExcludedTablesExist(t *testing.T) {
	db := openTestDB(t)
	tables, err := listTables(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	present := map[string]bool{}
	for _, name := range tables {
		present[name] = true
	}
	for name := range excludedTables {
		if !present[name] {
			t.Errorf("excludedTables names %q, which is not a table in the schema", name)
		}
	}
}

func TestShiftWorld_MovesTheCommunityAndKeepsSignIn(t *testing.T) {
	db := openTestDB(t)
	uid := auth.NewUUIDv7()
	if _, err := db.Exec(`INSERT INTO users (id, email, username, display_name, bio, role, created_at, updated_at, ap_id)
		VALUES (?, 'p@sim.localhost', 'p', 'P', '', 'member', '2030-01-31T12:00:00.000Z', '2030-01-31T12:00:00.000Z', 'https://localhost/ap/users/p')`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sessions (id, user_id, token, expires_at, ip_address) VALUES (?, ?, 'h', '2031-01-01T00:00:00Z', '127.0.0.1')`,
		auth.NewUUIDv7(), uid); err != nil {
		t.Fatal(err)
	}
	auth.LogAuditEvent(db, uid, "sim.test", "user", uid, `{"until":"2030-02-10T09:30:00.000Z"}`, "127.0.0.1")

	report, err := shiftWorld(db.DB, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if report.Instants == 0 {
		t.Fatal("nothing moved")
	}
	if _, kept := report.Skipped["sessions"]; !kept {
		t.Error("sessions should be reported as kept")
	}

	var created, expires, details string
	db.QueryRow(`SELECT created_at FROM users WHERE id = ?`, uid).Scan(&created)
	db.QueryRow(`SELECT expires_at FROM sessions WHERE user_id = ?`, uid).Scan(&expires)
	db.QueryRow(`SELECT metadata FROM audit_log WHERE user_id = ? AND action = 'sim.test'`, uid).Scan(&details)
	if created != "2030-01-01T12:00:00.000Z" {
		t.Errorf("users.created_at = %q, want moved back 30 days in its own layout", created)
	}
	if expires != "2031-01-01T00:00:00Z" {
		t.Errorf("sessions.expires_at = %q, want untouched", expires)
	}
	if details != `{"until":"2030-01-11T09:30:00.000Z"}` {
		t.Errorf("audit_log.metadata = %q, want the embedded instant moved", details)
	}
}

func TestParseAdvance(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"30d": 30 * 24 * time.Hour, "2w": 14 * 24 * time.Hour, "36h": 36 * time.Hour, "1.5d": 36 * time.Hour,
	} {
		got, err := parseAdvance(in)
		if err != nil || got != want {
			t.Errorf("parseAdvance(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := parseAdvance("soon"); err == nil {
		t.Error("parseAdvance(\"soon\") should fail")
	}
}
