package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// An image is a reference, never bytes (docs/adr/007).
//
// The binary never fetches it, so nothing here can be about the picture. What
// the rules protect is the two ways a reference fails a reader: an http URL
// that every browser blocks as mixed content, and a missing description, which
// is what remains when somebody else's host stops serving the file.

func TestImageRef_RefusesWhatNobodyWillSee(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "img1", "member")
	nodeID := createTestNode(t, db, admin.ID, "Img One", "img-one", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	update := func(body map[string]interface{}) (int, string) {
		r := authedRequest("PATCH", "/api/v1/nodes/img-one", body, adminToken)
		w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r)
		return w.Code, w.Body.String()
	}

	for _, tc := range []struct {
		name, wantIn string
		body         map[string]interface{}
	}{
		{
			// A patch page on https that pulls an image over http gets it
			// blocked by the browser. Refusing at the form beats a blank frame
			// on every visitor's screen.
			name: "plain http", wantIn: "https",
			body: map[string]interface{}{"image_url": "http://example.com/flyer.jpg", "image_alt": "A flyer"},
		},
		{
			name: "no description", wantIn: "description",
			body: map[string]interface{}{"image_url": "https://example.com/flyer.jpg", "image_alt": ""},
		},
		{
			name: "not an address", wantIn: "image address",
			body: map[string]interface{}{"image_url": "flyer.jpg", "image_alt": "A flyer"},
		},
	} {
		code, resp := update(tc.body)
		if code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d: %s", tc.name, code, resp)
		}
		if !strings.Contains(resp, tc.wantIn) {
			t.Errorf("%s: expected the refusal to mention %q, got %s", tc.name, tc.wantIn, resp)
		}
	}

	// The good case, and clearing it again. Clearing must not demand a
	// description for a picture that no longer exists.
	if code, resp := update(map[string]interface{}{
		"image_url": "https://example.com/flyer.jpg", "image_alt": "Flyer for the March show",
	}); code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", code, resp)
	}
	if code, resp := update(map[string]interface{}{"image_url": "", "image_alt": ""}); code != http.StatusOK {
		t.Errorf("clearing an image must be allowed, got %d: %s", code, resp)
	}
}

// A PATCH carrying half the pair is judged against the stored other half.
// Without that, sending only the URL reads as a picture with no description
// and gets refused, and sending only the description skips the check.
func TestImageRef_PartialPatchSeesWhatIsAlreadyThere(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "img2", "member")
	nodeID := createTestNode(t, db, admin.ID, "Img Two", "img-two", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	db.Exec(`UPDATE nodes SET image_url = 'https://example.com/a.jpg', image_alt = 'A flyer' WHERE id = ?`, nodeID)

	update := func(body map[string]interface{}) int {
		r := authedRequest("PATCH", "/api/v1/nodes/img-two", body, adminToken)
		return serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}", handler.UpdateNode(db), r).Code
	}

	// Swapping the picture alone is fine: a description is already stored.
	if code := update(map[string]interface{}{"image_url": "https://example.com/b.jpg"}); code != http.StatusOK {
		t.Errorf("changing only the address should pass, got %d", code)
	}
	// Emptying only the description leaves a picture nobody can read.
	if code := update(map[string]interface{}{"image_alt": ""}); code != http.StatusBadRequest {
		t.Errorf("emptying only the description should be refused, got %d", code)
	}
}

// The instance embeds an image it does not host, so pulling the reference is
// the whole of the remedy available (docs/adr/007). It takes down the picture,
// never the patch behind it.
func TestImageRef_ModeratorCanPullTheReference(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "img3", "admin")
	reporter, _ := createTestUser(t, db, "img3r", "member")
	nodeID := createTestNode(t, db, reporter.ID, "Img Three", "img-three", "open")
	db.Exec(`UPDATE nodes SET image_url = 'https://example.com/bad.jpg', image_alt = 'x' WHERE id = ?`, nodeID)

	reportID := auth.NewUUIDv7()
	db.Exec(`INSERT INTO content_reports (id, reporter_id, entity_type, entity_id, reason, details)
	         VALUES (?, ?, 'node', ?, 'spam', '')`, reportID, reporter.ID, nodeID)

	r := authedRequest("PATCH", "/api/v1/admin/reports/"+reportID,
		map[string]interface{}{"status": "resolved", "action": "remove_image"}, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/admin/reports/{id}",
		middleware.AdminRequired(db, handler.UpdateReport(db)), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var url, alt, removedAt string
	db.QueryRow("SELECT image_url, image_alt, COALESCE(removed_at,'') FROM nodes WHERE id = ?", nodeID).
		Scan(&url, &alt, &removedAt)
	if url != "" || alt != "" {
		t.Errorf("expected the reference pulled, got url=%q alt=%q", url, alt)
	}
	if removedAt != "" {
		t.Error("removing an image must not remove the patch")
	}
}
