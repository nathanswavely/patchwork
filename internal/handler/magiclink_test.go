package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
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

// A throttled request answers the same 200 as a sent one, by design, so the
// server log has to be the place that says nothing was issued. Without it a
// developer watching the log for the printed link reuses the previous one
// and gets "already used" with no clue why (#222).
func TestRequestMagicLinkThrottleIsLogged(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{}
	cfg.Instance.Domain = "example.com"

	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	// The limiters are process-wide; a distinct client address keeps this
	// test's four requests out of the per-IP bucket the other tests share.
	send := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link",
			strings.NewReader(`{"email":"throttle-test@example.com"}`))
		req.RemoteAddr = "203.0.113.7:4321"
		w := httptest.NewRecorder()
		handler.RequestMagicLink(db, cfg)(w, req)
		return w
	}
	for i := 0; i < 3; i++ {
		if w := send(); w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i+1, w.Code)
		}
	}
	if strings.Contains(logBuf.String(), "throttled") {
		t.Fatalf("throttle logged before the limit was reached:\n%s", logBuf.String())
	}
	issuedBefore := strings.Count(logBuf.String(), "Magic link for")

	w := send()
	if w.Code != http.StatusOK {
		t.Errorf("throttled status = %d, want the same 200 as a sent request", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, `"ok"`) {
		t.Errorf("throttled body = %q, want the generic ok response", body)
	}
	logged := logBuf.String()
	if !strings.Contains(logged, "throttled") || !strings.Contains(logged, "throttle-test@example.com") {
		t.Errorf("log should name the throttled address, got:\n%s", logged)
	}
	if got := strings.Count(logged, "Magic link for"); got != issuedBefore {
		t.Errorf("a throttled request printed a link: %d before, %d after", issuedBefore, got)
	}
}

// --- The sign-in code ---------------------------------------------------

// requestCode asks for a magic link with no SMTP configured and reads the
// code back out of the server log, which is exactly the channel an operator
// without SMTP has (#222).
func requestCode(t *testing.T, db *database.DB, email string) string {
	t.Helper()
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	cfg := &config.Config{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link",
		strings.NewReader(`{"email":`+strconv.Quote(email)+`}`))
	req.RemoteAddr = "198.51.100.9:5555"
	w := httptest.NewRecorder()
	handler.RequestMagicLink(db, cfg)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("magic link request: status = %d, want 200", w.Code)
	}

	m := regexp.MustCompile(`code: ([0-9]{6})`).FindStringSubmatch(logBuf.String())
	if m == nil {
		t.Fatalf("no sign-in code in the log; without SMTP the log is the delivery channel:\n%s", logBuf.String())
	}
	return m[1]
}

func postCode(t *testing.T, db *database.DB, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link/verify", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler.VerifyMagicCode(db)(w, req)
	return w
}

// The log must carry the link and the code, since without SMTP nothing else
// does — a client that signs in by code would otherwise have nowhere to read
// one on a dev or SMTP-less instance.
func TestRequestMagicLinkLogsCodeUnderTheLink(t *testing.T) {
	db := setupTestDB(t)
	code := requestCode(t, db, "logged-code@example.com")

	var stored sql.NullString
	if err := db.QueryRow(`SELECT code_hash FROM magic_links WHERE email = ?`, "logged-code@example.com").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !stored.Valid || stored.String == "" {
		t.Error("no code_hash stored beside the link")
	}
	if len(code) != 6 {
		t.Errorf("logged code = %q, want six digits", code)
	}
}

func TestVerifyMagicCodeSignsInAndSetsTheSessionCookie(t *testing.T) {
	db := setupTestDB(t)
	email := "code-signin@example.com"
	user, _ := createTestUser(t, db, "code-signin", "member")
	if _, err := db.Exec(`UPDATE users SET email = ? WHERE id = ?`, email, user.ID); err != nil {
		t.Fatal(err)
	}

	code := requestCode(t, db, email)
	w := postCode(t, db, `{"email":"`+email+`","code":"`+code+`"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}
	var got map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v (%s)", err, w.Body.String())
	}
	if got["id"] != user.ID {
		t.Errorf("signed in as %v, want %s", got["id"], user.ID)
	}

	var sessionSet bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "patchwork_session" && c.Value != "" {
			sessionSet = true
		}
	}
	if !sessionSet {
		t.Error("no patchwork_session cookie on the response")
	}

	// Audited as its own method, so the log says how somebody got in.
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM audit_log WHERE user_id = ? AND action = 'user.login' AND metadata LIKE '%magic_code%'`,
		user.ID,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("magic_code login audit rows = %d, want 1", n)
	}
}

// Spaces are a reading aid the email adds; the endpoint takes the code
// either way.
func TestVerifyMagicCodeAcceptsSpaces(t *testing.T) {
	db := setupTestDB(t)
	email := "code-spaces@example.com"
	code := requestCode(t, db, email)

	w := postCode(t, db, `{"email":"`+email+`","code":"`+code[:3]+` `+code[3:]+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}
}

// An address with no account gets a signup token, exactly as the link's JSON
// branch does (docs/adr/013).
func TestVerifyMagicCodeUnknownAddressNeedsAUsername(t *testing.T) {
	db := setupTestDB(t)
	email := "code-newcomer@example.com"
	code := requestCode(t, db, email)

	w := postCode(t, db, `{"email":"`+email+`","code":"`+code+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}
	var got struct {
		Status      string `json:"status"`
		SignupToken string `json:"signup_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "username_required" {
		t.Errorf("status = %q, want username_required", got.Status)
	}
	if got.SignupToken == "" {
		t.Error("no signup token for an address with no account")
	}
}

// Every refusal is the same 400 with the same words, so the endpoint cannot
// be read for whether an address has ever been used here.
func TestVerifyMagicCodeRefusalsAreOneGenericError(t *testing.T) {
	db := setupTestDB(t)
	email := "code-generic@example.com"
	requestCode(t, db, email)

	for _, body := range []string{
		`{"email":"` + email + `","code":"000000"}`,
		`{"email":"nobody-asked@example.com","code":"000000"}`,
		`{"email":"not-an-address","code":"000000"}`,
		`{"email":"` + email + `","code":""}`,
		`not json`,
	} {
		w := postCode(t, db, body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, w.Code)
		}
		if got := strings.TrimSpace(w.Body.String()); got != `{"error":"invalid or expired code"}` {
			t.Errorf("body %s: response = %s, want the one generic error", body, got)
		}
	}
}

// Five wrong codes spend the row, and the row is the emailed link too.
func TestVerifyMagicCodeFiveWrongCodesBurnTheLink(t *testing.T) {
	db := setupTestDB(t)
	email := "code-burned@example.com"
	code := requestCode(t, db, email)

	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for i := 0; i < 5; i++ {
		if w := postCode(t, db, `{"email":"`+email+`","code":"`+wrong+`"}`); w.Code != http.StatusBadRequest {
			t.Fatalf("guess %d: status = %d, want 400", i+1, w.Code)
		}
	}

	if w := postCode(t, db, `{"email":"`+email+`","code":"`+code+`"}`); w.Code != http.StatusBadRequest {
		t.Errorf("the right code after five wrong ones: status = %d, want 400", w.Code)
	}
	var used int
	if err := db.QueryRow(`SELECT used FROM magic_links WHERE email = ?`, email).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if used != 1 {
		t.Errorf("used = %d, want the emailed link spent along with the code", used)
	}
}
