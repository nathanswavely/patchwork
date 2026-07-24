package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

func csrfHandler() http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return middleware.CSRF(ok)
}

func TestCSRF_GetExempt(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/nodes", nil)
	w := httptest.NewRecorder()
	csrfHandler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("GET should pass, got %d", w.Code)
	}
}

func TestCSRF_MutationWithoutHeaderRejected(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/v1/nodes", nil)
	w := httptest.NewRecorder()
	csrfHandler().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("POST without header should be 403, got %d", w.Code)
	}
}

func TestCSRF_MutationWithHeaderAllowed(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/v1/nodes", nil)
	r.Header.Set("X-Patchwork-Request", "true")
	w := httptest.NewRecorder()
	csrfHandler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("POST with header should pass, got %d", w.Code)
	}
}

// Federation endpoints are reached by remote servers that cannot send the
// browser-only CSRF header; they must bypass CSRF (they are authenticated by
// HTTP Signatures / git transport instead).
func TestCSRF_FederationPathsExempt(t *testing.T) {
	paths := []string{
		"/ap/nodes/abc/inbox",
		"/ap/users/abc/inbox",
		"/.well-known/webfinger",
		"/api/v1/nodes/some-slug/governance.git/git-upload-pack",
	}
	for _, p := range paths {
		r := httptest.NewRequest("POST", p, nil) // no X-Patchwork-Request header
		w := httptest.NewRecorder()
		csrfHandler().ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("federation path %q should bypass CSRF, got %d", p, w.Code)
		}
	}
}
