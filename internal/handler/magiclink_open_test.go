package handler_test

import (
	"bytes"
	"encoding/json"
	"log"
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

// A sign-in link knows which browser asked for it
// (docs/adr/2026-09-28-a-link-knows-where-it-was-asked-for.md). These walk
// the three answers the link's page can get: signed in (the browser that
// asked, or "Sign in on this device"), the code (anywhere else), and one
// refusal for a link that is spent, expired or unknown.

type issuedLink struct {
	token  string
	code   string
	cookie *http.Cookie // the binding cookie the browser holds after asking
}

// requestSignInLink asks for a link with no SMTP configured, optionally from a
// browser already holding a binding cookie, and reads the token and code back
// out of the log. Each caller passes its own client address so the
// process-wide per-IP limiter never couples these tests.
func requestSignInLink(t *testing.T, db *database.DB, email, remoteAddr string, cookie *http.Cookie) issuedLink {
	t.Helper()
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link",
		strings.NewReader(`{"email":`+strconv.Quote(email)+`}`))
	req.RemoteAddr = remoteAddr
	if cookie != nil {
		req.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
	w := httptest.NewRecorder()
	handler.RequestMagicLink(db, &config.Config{})(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("magic link request: status = %d, want 200", w.Code)
	}

	token := regexp.MustCompile(`/login/link/([0-9a-f]+)`).FindStringSubmatch(logBuf.String())
	code := regexp.MustCompile(`code: ([0-9]{6})`).FindStringSubmatch(logBuf.String())
	if token == nil || code == nil {
		t.Fatalf("no link or code in the log:\n%s", logBuf.String())
	}

	out := issuedLink{token: token[1], code: code[1], cookie: cookie}
	for _, c := range w.Result().Cookies() {
		if c.Name == "patchwork_signin_request" {
			out.cookie = c
		}
	}
	return out
}

func openLink(t *testing.T, db *database.DB, token string, here bool, cookie *http.Cookie) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	body := `{"token":` + strconv.Quote(token) + `,"here":` + strconv.FormatBool(here) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link/open", strings.NewReader(body))
	if cookie != nil {
		req.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
	w := httptest.NewRecorder()
	handler.OpenMagicLink(db)(w, req)
	var got map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	return w, got
}

func sessionCookieSet(w *httptest.ResponseRecorder) bool {
	for _, c := range w.Result().Cookies() {
		if c.Name == "patchwork_session" && c.Value != "" {
			return true
		}
	}
	return false
}

func linkUsed(t *testing.T, db *database.DB, email string) bool {
	t.Helper()
	var used int
	if err := db.QueryRow(`SELECT used FROM magic_links WHERE email = ? ORDER BY created_at DESC LIMIT 1`, email).Scan(&used); err != nil {
		t.Fatal(err)
	}
	return used != 0
}

func existingAccount(t *testing.T, db *database.DB, username, email string) string {
	t.Helper()
	user, _ := createTestUser(t, db, username, "member")
	if _, err := db.Exec(`UPDATE users SET email = ? WHERE id = ?`, email, user.ID); err != nil {
		t.Fatal(err)
	}
	return user.ID
}

func TestRequestMagicLinkSetsABindingCookie(t *testing.T) {
	db := setupTestDB(t)
	link := requestSignInLink(t, db, "binding@example.com", "192.0.2.10:1000", nil)
	c := link.cookie
	if c == nil {
		t.Fatal("no patchwork_signin_request cookie on the response")
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie flags: HttpOnly=%v Secure=%v SameSite=%v", c.HttpOnly, c.Secure, c.SameSite)
	}
	if c.Path != "/api/v1/auth/magic-link" {
		t.Errorf("cookie path = %q, want it scoped to the magic-link routes", c.Path)
	}
	if c.MaxAge != 15*60 {
		t.Errorf("cookie MaxAge = %d, want the link's own fifteen minutes", c.MaxAge)
	}
}

func TestOpenInTheBrowserThatAskedSignsIn(t *testing.T) {
	db := setupTestDB(t)
	email := "same-browser@example.com"
	userID := existingAccount(t, db, "same-browser", email)
	link := requestSignInLink(t, db, email, "192.0.2.11:1000", nil)

	w, got := openLink(t, db, link.token, false, link.cookie)
	if w.Code != http.StatusOK || got["status"] != "signed_in" {
		t.Fatalf("status %d body %s, want signed_in", w.Code, w.Body.String())
	}
	if user, _ := got["user"].(map[string]interface{}); user == nil || user["id"] != userID {
		t.Errorf("signed in as %v, want %s", got["user"], userID)
	}
	if !sessionCookieSet(w) {
		t.Error("no session cookie")
	}
	if !linkUsed(t, db, email) {
		t.Error("the row was not spent")
	}
}

func TestOpenElsewhereShowsTheCodeAndSpendsNothing(t *testing.T) {
	db := setupTestDB(t)
	email := "elsewhere@example.com"
	userID := existingAccount(t, db, "elsewhere", email)
	link := requestSignInLink(t, db, email, "192.0.2.12:1000", nil)

	for _, tc := range []struct {
		name   string
		cookie *http.Cookie
	}{
		{"no cookie", nil},
		{"another browser's cookie", &http.Cookie{Name: "patchwork_signin_request", Value: strings.Repeat("ab", 32)}},
	} {
		w, got := openLink(t, db, link.token, false, tc.cookie)
		if w.Code != http.StatusOK || got["status"] != "code" {
			t.Fatalf("%s: status %d body %s, want the code", tc.name, w.Code, w.Body.String())
		}
		if got["code"] != link.code {
			t.Errorf("%s: code = %v, want %s", tc.name, got["code"], link.code)
		}
		if got["email"] != email {
			t.Errorf("%s: email = %v, want %s", tc.name, got["email"], email)
		}
		if sessionCookieSet(w) {
			t.Errorf("%s: a session was set for a browser that did not ask", tc.name)
		}
	}
	if linkUsed(t, db, email) {
		t.Fatal("showing the code spent the row")
	}

	// The code the page showed signs in the device that asked.
	w := postCode(t, db, `{"email":"`+email+`","code":"`+link.code+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("redeeming the shown code: status %d body %s", w.Code, w.Body.String())
	}
	var user map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &user)
	if user["id"] != userID {
		t.Errorf("code signed in %v, want %s", user["id"], userID)
	}

	// And the link is spent with it.
	if w, _ := openLink(t, db, link.token, false, nil); w.Code != http.StatusBadRequest {
		t.Errorf("opening a spent link: status %d, want 400", w.Code)
	}
}

func TestSignInOnThisDeviceSpendsTheRowAndKillsTheCode(t *testing.T) {
	db := setupTestDB(t)
	email := "here@example.com"
	existingAccount(t, db, "here", email)
	link := requestSignInLink(t, db, email, "192.0.2.13:1000", nil)

	w, got := openLink(t, db, link.token, true, nil)
	if w.Code != http.StatusOK || got["status"] != "signed_in" {
		t.Fatalf("status %d body %s, want signed_in", w.Code, w.Body.String())
	}
	if !sessionCookieSet(w) {
		t.Error("no session cookie")
	}

	if w := postCode(t, db, `{"email":"`+email+`","code":"`+link.code+`"}`); w.Code != http.StatusBadRequest {
		t.Errorf("the code still worked after signing in here: status %d", w.Code)
	}
}

func TestOpenNewAddressInTheBrowserThatAskedAsksForAUsername(t *testing.T) {
	db := setupTestDB(t)
	link := requestSignInLink(t, db, "brand-new@example.com", "192.0.2.14:1000", nil)

	w, got := openLink(t, db, link.token, false, link.cookie)
	if w.Code != http.StatusOK || got["status"] != "username_required" {
		t.Fatalf("status %d body %s, want username_required", w.Code, w.Body.String())
	}
	if tok, _ := got["signup_token"].(string); tok == "" {
		t.Error("no signup token")
	}
	if sessionCookieSet(w) {
		t.Error("a session was set before an account exists (docs/adr/013)")
	}
}

// A browser that asks twice keeps its cookie, so either link opens here.
func TestAskingTwiceBindsBothLinks(t *testing.T) {
	db := setupTestDB(t)
	email := "twice@example.com"
	existingAccount(t, db, "twice", email)
	first := requestSignInLink(t, db, email, "192.0.2.15:1000", nil)
	second := requestSignInLink(t, db, email, "192.0.2.15:1000", first.cookie)
	if second.cookie.Value != first.cookie.Value {
		t.Fatal("the second request replaced a live binding cookie")
	}

	w, got := openLink(t, db, first.token, false, first.cookie)
	if w.Code != http.StatusOK || got["status"] != "signed_in" {
		t.Fatalf("the first link in the asking browser: status %d body %s", w.Code, w.Body.String())
	}
}

func TestOpenRefusalsAreOneSentence(t *testing.T) {
	db := setupTestDB(t)
	email := "refusals@example.com"
	existingAccount(t, db, "refusals", email)
	spent := requestSignInLink(t, db, email, "192.0.2.16:1000", nil)
	if w, _ := openLink(t, db, spent.token, true, nil); w.Code != http.StatusOK {
		t.Fatalf("spending the link: status %d", w.Code)
	}
	expired := requestSignInLink(t, db, "expired@example.com", "192.0.2.16:1000", nil)
	if _, err := db.Exec(`UPDATE magic_links SET expires_at = '2000-01-01T00:00:00Z' WHERE email = ?`, "expired@example.com"); err != nil {
		t.Fatal(err)
	}

	var bodies []string
	for _, tok := range []string{spent.token, expired.token, strings.Repeat("0", 64)} {
		w, _ := openLink(t, db, tok, false, nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("token %s: status %d, want 400", tok[:8], w.Code)
		}
		bodies = append(bodies, strings.TrimSpace(w.Body.String()))
	}
	for _, b := range bodies[1:] {
		if b != bodies[0] {
			t.Errorf("refusals differ: %q vs %q", bodies[0], b)
		}
	}
}

// A GET never spends a link: mail scanners fetch links before a person does.
func TestOldVerifyURLRedirectsWithoutSpending(t *testing.T) {
	db := setupTestDB(t)
	email := "old-mail@example.com"
	existingAccount(t, db, "old-mail", email)
	link := requestSignInLink(t, db, email, "192.0.2.17:1000", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify/"+link.token, nil)
	req.SetPathValue("token", link.token)
	w := httptest.NewRecorder()
	handler.MagicLinkRedirect()(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status %d, want 302", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login/link/"+link.token {
		t.Errorf("Location = %q, want the link's page", loc)
	}
	if sessionCookieSet(w) {
		t.Error("a GET set a session")
	}
	if linkUsed(t, db, email) {
		t.Error("a GET spent the link")
	}
}
