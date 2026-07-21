package governance_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/governance"
)

func TestGitHTTPHandler_InfoRefs(t *testing.T) {
	dir := tempDataDir(t)

	// Init instance repo and fork for a node.
	if err := governance.InitInstanceRepo(dir); err != nil {
		t.Fatalf("init: %v", err)
	}
	nodeID := "test-node-http"
	if err := governance.ForkForNode(dir, nodeID, "casual"); err != nil {
		t.Fatalf("fork: %v", err)
	}
	governance.SetDataDir(dir)
	defer governance.SetDataDir("")

	handler := governance.GitHTTPHandler(func(slug string) string {
		if slug == "test-patch" {
			return nodeID
		}
		return ""
	})

	req := httptest.NewRequest("GET", "/api/v1/nodes/test-patch/governance.git/info/refs?service=git-upload-pack", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/x-git-upload-pack-advertisement" {
		t.Errorf("expected git content type, got %s", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, "git-upload-pack") {
		t.Errorf("response should contain service header, got: %s", body[:min(100, len(body))])
	}
	if !strings.Contains(body, "refs/heads/") {
		t.Errorf("response should contain refs, got: %s", body[:min(200, len(body))])
	}
}

func TestGitHTTPHandler_InfoRefs_NotFound(t *testing.T) {
	dir := tempDataDir(t)
	governance.SetDataDir(dir)
	defer governance.SetDataDir("")

	handler := governance.GitHTTPHandler(func(slug string) string {
		return "" // not found
	})

	req := httptest.NewRequest("GET", "/api/v1/nodes/nonexistent/governance.git/info/refs?service=git-upload-pack", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGitHTTPHandler_InfoRefs_WrongService(t *testing.T) {
	dir := tempDataDir(t)
	if err := governance.InitInstanceRepo(dir); err != nil {
		t.Fatalf("init: %v", err)
	}
	nodeID := "test-node-svc"
	governance.ForkForNode(dir, nodeID, "casual")
	governance.SetDataDir(dir)
	defer governance.SetDataDir("")

	handler := governance.GitHTTPHandler(func(slug string) string {
		return nodeID
	})

	req := httptest.NewRequest("GET", "/api/v1/nodes/test/governance.git/info/refs?service=git-receive-pack", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for receive-pack, got %d", w.Code)
	}
}

func TestGitHTTPHandler_InvalidPath(t *testing.T) {
	handler := governance.GitHTTPHandler(func(slug string) string { return "x" })

	req := httptest.NewRequest("GET", "/api/v1/nodes/test/bad-path", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad path, got %d", w.Code)
	}
}

// tempDataDir is defined in governance_test.go (same package)
