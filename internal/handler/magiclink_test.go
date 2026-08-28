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

// A malformed address is refused at the door with a 400. The blanket 200 on
// this endpoint exists to keep account existence unanswerable; address
// syntax says nothing about the instance, so refusing it leaks nothing —
// and answering "ok" would send someone off to wait for mail that was never
// going to be sent.
func TestRequestMagicLinkRejectsMalformedAddress(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{}
	cfg.Instance.Domain = "example.com"

	for _, body := range []string{
		`{"email":"not-an-address"}`,
		`{"email":"Bob <bob@example.com>"}`,
		`{"email":"bob@exam ple.com"}`,
		`{"email":"bob@example.com, carol@example.com"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link", strings.NewReader(body))
		w := httptest.NewRecorder()
		handler.RequestMagicLink(db, cfg)(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, w.Code)
		}
		// Nothing was staged, so no link can be consumed later.
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM magic_links`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("body %s: %d magic links stored for a malformed address", body, n)
		}
	}
}

// An empty or whitespace-only address keeps the old silent 200 — there is no
// address to say anything about, and the form has nothing to correct.
func TestRequestMagicLinkEmptyAddressStaysGeneric(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{}
	cfg.Instance.Domain = "example.com"

	for _, body := range []string{`{"email":""}`, `{"email":"   "}`, `{}`, `not json`} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link", strings.NewReader(body))
		w := httptest.NewRecorder()
		handler.RequestMagicLink(db, cfg)(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("body %s: status = %d, want 200", body, w.Code)
		}
	}
}

// The dotless dev address must survive the new gate, or `admin@localhost`
// (cmd/seed's dev admin, and its marker for a demo database) can no longer
// sign in. No SMTP configured, so this takes the log-the-link branch.
func TestRequestMagicLinkAcceptsDotlessDevAddress(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link",
		strings.NewReader(`{"email":"admin@localhost"}`))
	w := httptest.NewRecorder()
	handler.RequestMagicLink(db, cfg)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var stored string
	if err := db.QueryRow(`SELECT email FROM magic_links`).Scan(&stored); err != nil {
		t.Fatalf("no magic link stored for admin@localhost: %v", err)
	}
	if stored != "admin@localhost" {
		t.Errorf("magic_links.email = %q, want %q", stored, "admin@localhost")
	}
}

// Case variants share one rate-limit bucket. Keyed on the raw string they
// would not, making the per-email limit only one capitalization deep.
func TestRequestMagicLinkStoresNormalizedAddress(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link",
		strings.NewReader(`{"email":"  Bob@Example.COM  "}`))
	w := httptest.NewRecorder()
	handler.RequestMagicLink(db, cfg)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var stored string
	if err := db.QueryRow(`SELECT email FROM magic_links`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "bob@example.com" {
		t.Errorf("magic_links.email = %q, want %q", stored, "bob@example.com")
	}
}
