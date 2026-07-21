package handler_test

import (
	"bytes"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// TestRequestMagicLinkSendFailureLogged verifies that a failed SMTP send still
// returns the generic 200 (anti-enumeration) while logging the error so
// operators can diagnose a broken SMTP config from the server log.
func TestRequestMagicLinkSendFailureLogged(t *testing.T) {
	db := setupTestDB(t)

	// Grab a port that is guaranteed closed so the SMTP dial fails fast.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	cfg := &config.Config{}
	cfg.SMTP = config.SMTP{Host: "127.0.0.1", Port: port, From: "test@example.com"}
	cfg.Instance.Domain = "example.com"

	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link",
		strings.NewReader(`{"email":"smtp-fail-test@example.com"}`))
	w := httptest.NewRecorder()
	handler.RequestMagicLink(db, cfg)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 regardless of send failure", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, `"ok"`) {
		t.Errorf("body = %q, want generic ok response", body)
	}
	logged := logBuf.String()
	if !strings.Contains(logged, "magic link") || !strings.Contains(logged, "smtp-fail-test@example.com") {
		t.Errorf("send failure not logged; log output: %q", logged)
	}
}
