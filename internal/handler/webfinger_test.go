package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

func TestWebFinger_ValidUser(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	user, _ := createTestUser(t, db, "alice", "member")

	r := httptest.NewRequest("GET", "/.well-known/webfinger?resource=acct:alice@test.example.com", nil)
	w := servePublicMux(t, "GET", "/.well-known/webfinger", handler.WebFinger(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/jrd+json" {
		t.Errorf("expected Content-Type application/jrd+json, got %s", ct)
	}

	result := decodeJSON(t, w)

	if result["subject"] != "acct:alice@test.example.com" {
		t.Errorf("expected subject=acct:alice@test.example.com, got %v", result["subject"])
	}

	links, ok := result["links"].([]interface{})
	if !ok || len(links) == 0 {
		t.Fatal("expected non-empty links array")
	}

	link, ok := links[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected link object")
	}
	if link["rel"] != "self" {
		t.Errorf("expected rel=self, got %v", link["rel"])
	}
	if link["type"] != "application/activity+json" {
		t.Errorf("expected type=application/activity+json, got %v", link["type"])
	}
	href, ok := link["href"].(string)
	if !ok || href == "" {
		t.Fatal("expected non-empty href")
	}
	// The href should point to the user's AP actor URL.
	expectedHref := "https://test.example.com/ap/users/" + user.ID
	if href != expectedHref {
		t.Errorf("expected href=%s, got %s", expectedHref, href)
	}
}

func TestWebFinger_NodeSlug(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	owner, _ := createTestUser(t, db, "wfnodeowner", "member")
	nodeID := createTestNode(t, db, owner.ID, "Webfinger Patch", "webfinger-patch", "open")

	r := httptest.NewRequest("GET", "/.well-known/webfinger?resource=acct:webfinger-patch@test.example.com", nil)
	w := servePublicMux(t, "GET", "/.well-known/webfinger", handler.WebFinger(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	result := decodeJSON(t, w)

	if result["subject"] != "acct:webfinger-patch@test.example.com" {
		t.Errorf("expected subject=acct:webfinger-patch@test.example.com, got %v", result["subject"])
	}

	links, ok := result["links"].([]interface{})
	if !ok || len(links) == 0 {
		t.Fatal("expected non-empty links array")
	}
	link, ok := links[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected link object")
	}
	expectedHref := "https://test.example.com/ap/nodes/" + nodeID
	if link["href"] != expectedHref {
		t.Errorf("expected href=%s, got %v", expectedHref, link["href"])
	}
}

func TestWebFinger_UserTakesPrecedenceOverNode(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	user, _ := createTestUser(t, db, "sharedname", "member")
	createTestNode(t, db, user.ID, "Shared Name Patch", "sharedname", "open")

	r := httptest.NewRequest("GET", "/.well-known/webfinger?resource=acct:sharedname@test.example.com", nil)
	w := servePublicMux(t, "GET", "/.well-known/webfinger", handler.WebFinger(db), r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	result := decodeJSON(t, w)
	links := result["links"].([]interface{})
	link := links[0].(map[string]interface{})
	expectedHref := "https://test.example.com/ap/users/" + user.ID
	if link["href"] != expectedHref {
		t.Errorf("expected user actor href=%s, got %v", expectedHref, link["href"])
	}
}

func TestWebFinger_UserNotFound(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	r := httptest.NewRequest("GET", "/.well-known/webfinger?resource=acct:nobody@test.example.com", nil)
	w := servePublicMux(t, "GET", "/.well-known/webfinger", handler.WebFinger(db), r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWebFinger_MissingResource(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	r := httptest.NewRequest("GET", "/.well-known/webfinger", nil)
	w := servePublicMux(t, "GET", "/.well-known/webfinger", handler.WebFinger(db), r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWebFinger_WrongDomain(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	createTestUser(t, db, "alice2", "member")

	r := httptest.NewRequest("GET", "/.well-known/webfinger?resource=acct:alice2@wrong.domain", nil)
	w := servePublicMux(t, "GET", "/.well-known/webfinger", handler.WebFinger(db), r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWebFinger_InvalidAcctFormat(t *testing.T) {
	db := setupTestDB(t)
	ap.SetDomain("test.example.com")
	defer ap.SetDomain("")

	r := httptest.NewRequest("GET", "/.well-known/webfinger?resource=https://example.com/user", nil)
	w := servePublicMux(t, "GET", "/.well-known/webfinger", handler.WebFinger(db), r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
