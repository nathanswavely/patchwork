package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

func dummyHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestCORS_EnabledOnGET(t *testing.T) {
	cfg := &config.Config{MultiQuilt: true}
	handler := middleware.CORS(cfg, dummyHandler())

	r := httptest.NewRequest("GET", "/api/v1/nodes", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS origin header on GET when multi_quilt=true")
	}
	if w.Header().Get("Access-Control-Max-Age") != "86400" {
		t.Error("expected Max-Age header")
	}
}

func TestCORS_DisabledWhenFalse(t *testing.T) {
	cfg := &config.Config{MultiQuilt: false}
	handler := middleware.CORS(cfg, dummyHandler())

	r := httptest.NewRequest("GET", "/api/v1/nodes", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no CORS headers when multi_quilt=false")
	}
}

func TestCORS_NeverOnPOST(t *testing.T) {
	cfg := &config.Config{MultiQuilt: true}
	handler := middleware.CORS(cfg, dummyHandler())

	r := httptest.NewRequest("POST", "/api/v1/nodes", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS headers should never appear on POST")
	}
}

func TestCORS_NeverOnDELETE(t *testing.T) {
	cfg := &config.Config{MultiQuilt: true}
	handler := middleware.CORS(cfg, dummyHandler())

	r := httptest.NewRequest("DELETE", "/api/v1/edges/123", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS headers should never appear on DELETE")
	}
}

func TestCORS_NeverOnPATCH(t *testing.T) {
	cfg := &config.Config{MultiQuilt: true}
	handler := middleware.CORS(cfg, dummyHandler())

	r := httptest.NewRequest("PATCH", "/api/v1/nodes/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS headers should never appear on PATCH")
	}
}

func TestCORS_OptionsPreflight(t *testing.T) {
	cfg := &config.Config{MultiQuilt: true}
	handler := middleware.CORS(cfg, dummyHandler())

	r := httptest.NewRequest("OPTIONS", "/api/v1/nodes", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for preflight, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS origin header on OPTIONS")
	}
	if w.Header().Get("Access-Control-Max-Age") != "86400" {
		t.Error("expected Max-Age on preflight")
	}
}

func TestCORS_NotOnNonAPIPath(t *testing.T) {
	cfg := &config.Config{MultiQuilt: true}
	handler := middleware.CORS(cfg, dummyHandler())

	r := httptest.NewRequest("GET", "/some/other/path", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS headers should not appear on non-API paths")
	}
}
