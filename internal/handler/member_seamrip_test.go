package handler_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// The member seamrip endpoint (docs/adr/089, docs/adr/012 affordance 2).
// What each row of the bundle contains is the boundary's business and is
// tested in internal/seamrip; what is tested here is that the route is a
// member's to use, that it is rationed, that it says who took it, and that
// the zip is the archive cmd/import reads.

func requestMemberSeamrip(t *testing.T, db *database.DB, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("GET", "/api/v1/users/me/seamrip", nil, token)
	return serveMux(t, db, "GET", "/api/v1/users/me/seamrip",
		handler.MemberSeamrip(db, exportCfg()), r)
}

// bundleFiles unpacks the zip into name → bytes.
func bundleFiles(t *testing.T, w *httptest.ResponseRecorder) map[string][]byte {
	t.Helper()
	body := w.Body.Bytes()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		files[f.Name] = data
	}
	return files
}

func bundleRows(t *testing.T, files map[string][]byte, name string) []map[string]any {
	t.Helper()
	data, ok := files[name]
	if !ok {
		t.Fatalf("bundle has no %s", name)
	}
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return rows
}

func TestMemberSeamrip_RequiresAuth(t *testing.T) {
	db := setupTestDB(t)

	w := requestMemberSeamrip(t, db, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated member seamrip: got %d, want 401", w.Code)
	}
}

func TestMemberSeamrip_CarriesTheViewersQuiltAndSaysWhoTookIt(t *testing.T) {
	db := setupTestDB(t)
	weaver, token := createTestUser(t, db, "weaver", "member")
	other, _ := createTestUser(t, db, "binder", "member")
	if _, err := db.Exec(`UPDATE users SET email = ? WHERE id = ?`, "binder@example.com", other.ID); err != nil {
		t.Fatal(err)
	}

	open := createTestNode(t, db, other.ID, "Gallery Row", "gallery-row", "open")
	shut := createTestNode(t, db, other.ID, "Back Room", "back-room", "open")
	if _, err := db.Exec(`UPDATE nodes SET visibility = 'private' WHERE id = ?`, shut); err != nil {
		t.Fatal(err)
	}
	createTestMembership(t, db, weaver.ID, open, "member", "active")
	createTestMembership(t, db, other.ID, open, "admin", "active")
	createTestMembership(t, db, other.ID, shut, "admin", "active")

	// A notice in the room the viewer belongs to. The room is the members'
	// (docs/adr/081); a bundle is not a way to carry it out.
	if _, err := db.Exec(
		`INSERT INTO notices (id, node_id, author_id, title, body) VALUES (?, ?, ?, 'PA is broken', 'Again.')`,
		auth.NewUUIDv7(), open, weaver.ID); err != nil {
		t.Fatal(err)
	}

	w := requestMemberSeamrip(t, db, token)
	if w.Code != http.StatusOK {
		t.Fatalf("member seamrip: got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("content type: %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); cd == "" {
		t.Error("no filename offered for the download")
	}

	files := bundleFiles(t, w)
	for _, want := range []string{"manifest.json", "instance.json", "README.txt", "nodes.json", "users.json"} {
		if _, ok := files[want]; !ok {
			t.Errorf("bundle has no %s", want)
		}
	}
	if _, failed := files["EXPORT_FAILED.txt"]; failed {
		t.Fatalf("the export aborted: %s", files["EXPORT_FAILED.txt"])
	}

	var manifest map[string]any
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest["kind"] != "member-seamrip" {
		t.Errorf("manifest kind: %v", manifest["kind"])
	}
	if manifest["requested_by"] != "weaver" {
		t.Errorf("manifest does not name who took it: %v", manifest["requested_by"])
	}
	if manifest["exported_at"] == nil {
		t.Error("manifest does not say when it was taken")
	}

	nodes := bundleRows(t, files, "nodes.json")
	if len(nodes) != 1 {
		t.Errorf("expected the one visible patch, got %d", len(nodes))
	}
	for _, n := range nodes {
		if n["slug"] == "back-room" {
			t.Error("a private patch the viewer holds no role on reached the bundle")
		}
	}

	if rows := bundleRows(t, files, "notices.json"); len(rows) != 0 {
		t.Errorf("the noticeboard reached the bundle: %d rows", len(rows))
	}

	users := bundleRows(t, files, "users.json")
	if len(users) == 0 {
		t.Fatal("nobody reached the bundle")
	}
	for _, u := range users {
		if u["email"] != nil {
			t.Errorf("an email address reached the bundle: %v", u["email"])
		}
	}
}

func TestMemberSeamrip_WritesAnAuditRow(t *testing.T) {
	db := setupTestDB(t)
	weaver, token := createTestUser(t, db, "weaver", "member")

	if w := requestMemberSeamrip(t, db, token); w.Code != http.StatusOK {
		t.Fatalf("member seamrip: got %d", w.Code)
	}

	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE user_id = ? AND action = 'user.seamrip'`,
		weaver.ID).Scan(&n)
	if n != 1 {
		t.Errorf("audit rows for user.seamrip: got %d, want 1", n)
	}
}

func TestMemberSeamrip_RateLimitTrips(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "weaver", "member")

	// Two a day is the budget; the third asks the person to come back.
	for i := 0; i < 2; i++ {
		if w := requestMemberSeamrip(t, db, token); w.Code != http.StatusOK {
			t.Fatalf("download %d: got %d", i+1, w.Code)
		}
	}
	w := requestMemberSeamrip(t, db, token)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("third download: got %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("a refusal with no Retry-After leaves the client guessing")
	}
}
