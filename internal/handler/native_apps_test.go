package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// The apps a quilt vouches for
// (docs/adr/2026-09-20-an-instance-vouches-for-an-app.md).
//
// Two properties carry most of the weight here. Empty publishes nothing —
// which is the state of every instance that never asked for any of this — and
// listing an app takes a step-up, because publishing an identifier hands that
// app the right to ask a phone for this domain's passkeys.

// The same self-consistent pair internal/auth's tests use: SHA-256 of
// "patchwork-test-cert-6", in the two spellings.
const testAppFingerprint = "B2:7E:E8:4A:DC:DA:D8:69:F1:25:6F:73:64:1E:D2:A8:54:FB:05:AA:3F:F8:7E:90:F8:78:EF:2D:D1:0B:B4:05"

// nativeAppAdmin makes an instance admin whose session already holds a
// step-up window, since every add in this file is about what happens after
// the gate rather than about the gate.
func nativeAppAdmin(t *testing.T, db *database.DB, username string) string {
	t.Helper()
	_, token := createTestUser(t, db, username, "admin")
	if _, err := auth.GrantSudo(db, token); err != nil {
		t.Fatalf("grant sudo: %v", err)
	}
	return token
}

func addNativeApp(t *testing.T, db *database.DB, token string, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("POST", "/api/v1/admin/native-apps", body, token)
	return serveSudoAdmin(db, "POST", "/api/v1/admin/native-apps",
		handler.AdminAddNativeApp(db, nil), r)
}

func listNativeApps(t *testing.T, db *database.DB, cfg *config.Config, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("GET", "/api/v1/admin/native-apps", nil, token)
	return serveAdmin(db, "GET", "/api/v1/admin/native-apps",
		handler.AdminListNativeApps(db, cfg), r)
}

func fetchAssociation(db *database.DB, path string, h http.HandlerFunc) *httptest.ResponseRecorder {
	return servePublic("GET", path, h, httptest.NewRequest("GET", path, nil))
}

// An instance that has listed nothing serves neither file. This is the
// default, and it is what keeps a quilt that wants none of this free of it.
func TestAssociationFilesAre404WhenNothingIsListed(t *testing.T) {
	db := setupTestDB(t)

	apple := fetchAssociation(db, handler.AppleAssociationPath, handler.AppleAppSiteAssociation(db))
	if apple.Code != http.StatusNotFound {
		t.Errorf("the Apple file answered %d on an empty quilt: %s", apple.Code, apple.Body.String())
	}
	android := fetchAssociation(db, handler.AndroidAssociationPath, handler.AssetLinks(db))
	if android.Code != http.StatusNotFound {
		t.Errorf("the Android file answered %d on an empty quilt: %s", android.Code, android.Body.String())
	}
}

// One platform listed does not publish the other platform's file.
func TestAssociationFilesAreIndependentPerPlatform(t *testing.T) {
	db := setupTestDB(t)
	token := nativeAppAdmin(t, db, "apps-admin-apple-only")

	w := addNativeApp(t, db, token, map[string]interface{}{
		"platform":   "apple",
		"identifier": "ABCDE12345.org.example.app",
		"label":      "Our iPhone app",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("add returned %d: %s", w.Code, w.Body.String())
	}

	apple := fetchAssociation(db, handler.AppleAssociationPath, handler.AppleAppSiteAssociation(db))
	if apple.Code != http.StatusOK {
		t.Fatalf("the Apple file answered %d: %s", apple.Code, apple.Body.String())
	}
	if ct := apple.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("the Apple file is served as %q; Apple requires application/json", ct)
	}
	if cc := apple.Header().Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Errorf("Cache-Control is %q", cc)
	}

	var doc map[string]interface{}
	if err := json.Unmarshal(apple.Body.Bytes(), &doc); err != nil {
		t.Fatalf("the Apple file is not JSON: %v", err)
	}
	creds, ok := doc["webcredentials"].(map[string]interface{})
	if !ok {
		t.Fatalf("no webcredentials block: %s", apple.Body.String())
	}
	apps, _ := creds["apps"].([]interface{})
	if len(apps) != 1 || apps[0] != "ABCDE12345.org.example.app" {
		t.Errorf("webcredentials.apps is %v", apps)
	}
	// Listing an app for passkeys must not also claim the quilt's URLs: that
	// is a second switch, off until an admin turns it on
	// (docs/adr/2026-10-18-a-listed-app-may-open-the-quilts-links.md).
	if _, present := doc["applinks"]; present {
		t.Error("the Apple file carries an applinks block although no app opens the quilt's links")
	}

	// Nothing Android was listed, so nothing Android is published.
	android := fetchAssociation(db, handler.AndroidAssociationPath, handler.AssetLinks(db))
	if android.Code != http.StatusNotFound {
		t.Errorf("an Apple listing published the Android file too (%d)", android.Code)
	}
}

func TestAssetLinksDelegatesLoginCredsToEveryListedPackage(t *testing.T) {
	db := setupTestDB(t)
	token := nativeAppAdmin(t, db, "apps-admin-android")

	w := addNativeApp(t, db, token, map[string]interface{}{
		"platform":     "android",
		"identifier":   "org.example.app",
		"fingerprints": []string{strings.ToLower(testAppFingerprint)},
		"label":        "Our Android app",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("add returned %d: %s", w.Code, w.Body.String())
	}

	android := fetchAssociation(db, handler.AndroidAssociationPath, handler.AssetLinks(db))
	if android.Code != http.StatusOK {
		t.Fatalf("the Android file answered %d: %s", android.Code, android.Body.String())
	}
	if ct := android.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("the Android file is served as %q", ct)
	}
	if cc := android.Header().Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Errorf("Cache-Control is %q", cc)
	}

	var statements []struct {
		Relation []string `json:"relation"`
		Target   struct {
			Namespace    string   `json:"namespace"`
			PackageName  string   `json:"package_name"`
			Fingerprints []string `json:"sha256_cert_fingerprints"`
		} `json:"target"`
	}
	if err := json.Unmarshal(android.Body.Bytes(), &statements); err != nil {
		t.Fatalf("the Android file is not a JSON array: %v (%s)", err, android.Body.String())
	}
	if len(statements) != 1 {
		t.Fatalf("got %d statements, want 1", len(statements))
	}
	s := statements[0]
	if len(s.Relation) != 1 || s.Relation[0] != "delegate_permission/common.get_login_creds" {
		t.Errorf("relation is %v", s.Relation)
	}
	if s.Target.Namespace != "android_app" || s.Target.PackageName != "org.example.app" {
		t.Errorf("target is %+v", s.Target)
	}
	// Stored uppercase however it was typed, because a fingerprint in two
	// spellings is two origins for one key.
	if len(s.Target.Fingerprints) != 1 || s.Target.Fingerprints[0] != testAppFingerprint {
		t.Errorf("fingerprints are %v", s.Target.Fingerprints)
	}
}

// The admin listing answers the two addresses as well as the rows: "where do
// I point the app at this" is the next question after listing one.
func TestAdminListAnswersTheTwoWellKnownURLs(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	token := nativeAppAdmin(t, db, "apps-admin-urls")

	w := listNativeApps(t, db, cfg, token)
	if w.Code != http.StatusOK {
		t.Fatalf("list returned %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		NativeApps []map[string]interface{} `json:"native_apps"`
		URLs       map[string]string        `json:"urls"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.NativeApps) != 0 {
		t.Errorf("a fresh quilt lists %d apps", len(body.NativeApps))
	}
	if body.URLs["apple"] != "https://quilt.example.com/.well-known/apple-app-site-association" {
		t.Errorf("apple url is %q", body.URLs["apple"])
	}
	if body.URLs["android"] != "https://quilt.example.com/.well-known/assetlinks.json" {
		t.Errorf("android url is %q", body.URLs["android"])
	}
}

// Listing an app is a grant outward, so it takes the step-up gate
// (docs/adr/017) the way export, wipe, promotion and setting an address do.
// Removing one does not.
func TestAddingAnAppNeedsStepUpAndRemovingDoesNot(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "apps-admin-no-sudo", "admin")

	r := authedRequest("POST", "/api/v1/admin/native-apps", map[string]interface{}{
		"platform":   "apple",
		"identifier": "ABCDE12345.org.example.app",
	}, token)
	w := serveSudoAdmin(db, "POST", "/api/v1/admin/native-apps", handler.AdminAddNativeApp(db, nil), r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("an admin with no step-up window got %d, want 403: %s", w.Code, w.Body.String())
	}
	if code := bodyCode(t, w); code != "sudo_required" && code != "passkey_required" {
		t.Errorf("refusal code is %q, so the page cannot tell which prompt to raise", code)
	}
	if n := countRows(db, "SELECT COUNT(*) FROM native_apps"); n != 0 {
		t.Fatalf("the refused add wrote %d rows", n)
	}

	// Removing is admin-only: taking trust back is the safe direction, and a
	// gate there stands between an admin and undoing a mistake.
	sudoToken := nativeAppAdmin(t, db, "apps-admin-remover")
	added := addNativeApp(t, db, sudoToken, map[string]interface{}{
		"platform":   "apple",
		"identifier": "ABCDE12345.org.example.app",
	})
	if added.Code != http.StatusCreated {
		t.Fatalf("add returned %d: %s", added.Code, added.Body.String())
	}
	var app struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(added.Body.Bytes(), &app); err != nil {
		t.Fatalf("decode created row: %v", err)
	}

	// A plain admin, holding no step-up window at all.
	del := authedRequest("DELETE", "/api/v1/admin/native-apps/"+app.ID, nil, token)
	dw := serveAdmin(db, "DELETE", "/api/v1/admin/native-apps/{id}",
		handler.AdminDeleteNativeApp(db, nil), del)
	if dw.Code != http.StatusOK {
		t.Fatalf("remove returned %d: %s", dw.Code, dw.Body.String())
	}
	var status map[string]string
	if err := json.Unmarshal(dw.Body.Bytes(), &status); err != nil || status["status"] != "ok" {
		t.Errorf("remove answered %q", dw.Body.String())
	}
	if n := countRows(db, "SELECT COUNT(*) FROM native_apps"); n != 0 {
		t.Errorf("%d rows survived the removal", n)
	}
	// And the file goes back to 404, because nothing is listed again.
	if apple := fetchAssociation(db, handler.AppleAssociationPath, handler.AppleAppSiteAssociation(db)); apple.Code != http.StatusNotFound {
		t.Errorf("after removing the last app the Apple file answered %d", apple.Code)
	}
}

func TestAddRejectsMalformedIdentifiersAndFingerprints(t *testing.T) {
	db := setupTestDB(t)
	token := nativeAppAdmin(t, db, "apps-admin-validation")

	cases := []struct {
		name string
		body map[string]interface{}
	}{
		{"no platform", map[string]interface{}{"identifier": "ABCDE12345.org.example.app"}},
		{"unknown platform", map[string]interface{}{"platform": "windows", "identifier": "org.example.app"}},
		{"apple team id too short", map[string]interface{}{"platform": "apple", "identifier": "ABCDE1234.org.example.app"}},
		{"apple lowercase team id", map[string]interface{}{"platform": "apple", "identifier": "abcde12345.org.example.app"}},
		{"apple with no bundle id", map[string]interface{}{"platform": "apple", "identifier": "ABCDE12345"}},
		{"android package with no dot", map[string]interface{}{"platform": "android", "identifier": "example", "fingerprints": []string{testAppFingerprint}}},
		{"android with no fingerprint", map[string]interface{}{"platform": "android", "identifier": "org.example.app"}},
		{"android with only blank fingerprints", map[string]interface{}{"platform": "android", "identifier": "org.example.app", "fingerprints": []string{"   "}}},
		{"android with a malformed fingerprint", map[string]interface{}{"platform": "android", "identifier": "org.example.app", "fingerprints": []string{"AA:BB:CC"}}},
	}
	for _, c := range cases {
		w := addNativeApp(t, db, token, c.body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400: %s", c.name, w.Code, w.Body.String())
		}
	}
	if n := countRows(db, "SELECT COUNT(*) FROM native_apps"); n != 0 {
		t.Errorf("a rejected add wrote %d rows", n)
	}
}

// Trimming and casing are a spelling, not a second app.
func TestAddStoresTheIdentifierAsItWillBePublished(t *testing.T) {
	db := setupTestDB(t)
	token := nativeAppAdmin(t, db, "apps-admin-typed")

	w := addNativeApp(t, db, token, map[string]interface{}{
		"platform":     " Android ",
		"identifier":   "  org.example.app  ",
		"fingerprints": []string{" " + strings.ToLower(testAppFingerprint) + " ", testAppFingerprint},
		"label":        "  Our app\nand its name  ",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("add returned %d: %s", w.Code, w.Body.String())
	}
	var app struct {
		Platform     string   `json:"platform"`
		Identifier   string   `json:"identifier"`
		Fingerprints []string `json:"fingerprints"`
		Label        string   `json:"label"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &app); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if app.Platform != "android" || app.Identifier != "org.example.app" {
		t.Errorf("stored %q / %q", app.Platform, app.Identifier)
	}
	// The same fingerprint twice is one fingerprint.
	if len(app.Fingerprints) != 1 || app.Fingerprints[0] != testAppFingerprint {
		t.Errorf("fingerprints are %v", app.Fingerprints)
	}
	if app.Label != "Our app and its name" {
		t.Errorf("label is %q", app.Label)
	}
}

// An Apple row carries no fingerprints even if some were sent: the file Apple
// reads has nowhere to put them.
func TestApplePlatformIgnoresFingerprints(t *testing.T) {
	db := setupTestDB(t)
	token := nativeAppAdmin(t, db, "apps-admin-apple-prints")

	w := addNativeApp(t, db, token, map[string]interface{}{
		"platform":     "apple",
		"identifier":   "ABCDE12345.org.example.app",
		"fingerprints": []string{testAppFingerprint},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("add returned %d: %s", w.Code, w.Body.String())
	}
	var app struct {
		Fingerprints []string `json:"fingerprints"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &app); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(app.Fingerprints) != 0 {
		t.Errorf("an Apple row stored fingerprints: %v", app.Fingerprints)
	}
}

// Listing an app twice is the same grant typed again, not a second one.
func TestAddRefusesADuplicateWith409(t *testing.T) {
	db := setupTestDB(t)
	token := nativeAppAdmin(t, db, "apps-admin-duplicate")

	body := map[string]interface{}{"platform": "apple", "identifier": "ABCDE12345.org.example.app"}
	if w := addNativeApp(t, db, token, body); w.Code != http.StatusCreated {
		t.Fatalf("first add returned %d: %s", w.Code, w.Body.String())
	}
	w := addNativeApp(t, db, token, body)
	if w.Code != http.StatusConflict {
		t.Fatalf("second add returned %d, want 409: %s", w.Code, w.Body.String())
	}
	if n := countRows(db, "SELECT COUNT(*) FROM native_apps"); n != 1 {
		t.Errorf("%d rows after a refused duplicate", n)
	}

	// The same identifier on the other platform is a different app, and the
	// unique index is on the pair.
	other := map[string]interface{}{
		"platform":     "android",
		"identifier":   "org.example.app",
		"fingerprints": []string{testAppFingerprint},
	}
	if w := addNativeApp(t, db, token, other); w.Code != http.StatusCreated {
		t.Fatalf("an Android listing was refused: %d %s", w.Code, w.Body.String())
	}
}

func TestRemoveAnAppThatIsNotThere(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "apps-admin-missing", "admin")
	r := authedRequest("DELETE", "/api/v1/admin/native-apps/"+auth.NewUUIDv7(), nil, token)
	w := serveAdmin(db, "DELETE", "/api/v1/admin/native-apps/{id}",
		handler.AdminDeleteNativeApp(db, nil), r)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404: %s", w.Code, w.Body.String())
	}
}

// Every add and every remove is in the audit log with the app it named: the
// dispute a year from now is "who told this domain to trust that app".
func TestAddAndRemoveAreAudited(t *testing.T) {
	db := setupTestDB(t)
	token := nativeAppAdmin(t, db, "apps-admin-audited")

	w := addNativeApp(t, db, token, map[string]interface{}{
		"platform":   "apple",
		"identifier": "ABCDE12345.org.example.app",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("add returned %d: %s", w.Code, w.Body.String())
	}
	var app struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &app); err != nil {
		t.Fatalf("decode created row: %v", err)
	}

	var metadata string
	if err := db.QueryRow(
		`SELECT metadata FROM audit_log WHERE action = 'admin.native_app_add'`).Scan(&metadata); err != nil {
		t.Fatalf("no admin.native_app_add entry: %v", err)
	}
	if !strings.Contains(metadata, "ABCDE12345.org.example.app") || !strings.Contains(metadata, "apple") {
		t.Errorf("the add entry does not name the app: %s", metadata)
	}

	r := authedRequest("DELETE", "/api/v1/admin/native-apps/"+app.ID, nil, token)
	if dw := serveAdmin(db, "DELETE", "/api/v1/admin/native-apps/{id}",
		handler.AdminDeleteNativeApp(db, nil), r); dw.Code != http.StatusOK {
		t.Fatalf("remove returned %d: %s", dw.Code, dw.Body.String())
	}
	if n := countRows(db, `SELECT COUNT(*) FROM audit_log WHERE action = 'admin.native_app_remove'`); n != 1 {
		t.Errorf("%d admin.native_app_remove entries", n)
	}
}

// An add and a remove reload the relying party's origin list, so a listed
// Android app can actually sign in without a restart.
func TestAddAndRemoveReloadTheWebAuthnOrigins(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	token := nativeAppAdmin(t, db, "apps-admin-origins")

	wa, err := auth.NewWebAuthnService(db, cfg)
	if err != nil {
		t.Fatalf("webauthn: %v", err)
	}

	r := authedRequest("POST", "/api/v1/admin/native-apps", map[string]interface{}{
		"platform":     "android",
		"identifier":   "org.example.app",
		"fingerprints": []string{testAppFingerprint},
	}, token)
	w := serveSudoAdmin(db, "POST", "/api/v1/admin/native-apps",
		handler.AdminAddNativeApp(db, wa), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("add returned %d: %s", w.Code, w.Body.String())
	}

	origins, err := auth.NativeAppOrigins(db)
	if err != nil {
		t.Fatalf("NativeAppOrigins: %v", err)
	}
	if len(origins) != 1 || !strings.HasPrefix(origins[0], "android:apk-key-hash:") {
		t.Fatalf("the listed app derived %v", origins)
	}

	var app struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &app); err != nil {
		t.Fatalf("decode created row: %v", err)
	}
	del := authedRequest("DELETE", "/api/v1/admin/native-apps/"+app.ID, nil, token)
	if dw := serveAdmin(db, "DELETE", "/api/v1/admin/native-apps/{id}",
		handler.AdminDeleteNativeApp(db, wa), del); dw.Code != http.StatusOK {
		t.Fatalf("remove returned %d: %s", dw.Code, dw.Body.String())
	}
	origins, err = auth.NativeAppOrigins(db)
	if err != nil {
		t.Fatalf("NativeAppOrigins: %v", err)
	}
	if len(origins) != 0 {
		t.Errorf("after removing the app the derived origins are %v", origins)
	}
}

// --- Opening the quilt's links in an app ------------------------------------
// (docs/adr/2026-10-18-a-listed-app-may-open-the-quilts-links.md)

func setOpenLinks(db *database.DB, token, id string, body interface{}) *httptest.ResponseRecorder {
	r := authedRequest("PATCH", "/api/v1/admin/native-apps/"+id, body, token)
	return serveAdmin(db, "PATCH", "/api/v1/admin/native-apps/{id}",
		handler.AdminSetNativeAppOpenLinks(db), r)
}

func addedAppID(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	if w.Code != http.StatusCreated {
		t.Fatalf("add returned %d: %s", w.Code, w.Body.String())
	}
	var app struct {
		ID        string `json:"id"`
		OpenLinks *bool  `json:"open_links"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &app); err != nil {
		t.Fatalf("decode created row: %v", err)
	}
	// Every row says where it stands, and a new one stands at off.
	if app.OpenLinks == nil || *app.OpenLinks {
		t.Fatalf("a freshly listed app answered open_links %v, want false", app.OpenLinks)
	}
	return app.ID
}

type appleLinksDoc struct {
	Applinks *struct {
		Details []struct {
			AppIDs     []string `json:"appIDs"`
			Components []struct {
				Path    string `json:"/"`
				Exclude bool   `json:"exclude"`
			} `json:"components"`
		} `json:"details"`
	} `json:"applinks"`
	Webcredentials struct {
		Apps []string `json:"apps"`
	} `json:"webcredentials"`
}

func fetchAppleDoc(t *testing.T, db *database.DB) appleLinksDoc {
	t.Helper()
	w := fetchAssociation(db, handler.AppleAssociationPath, handler.AppleAppSiteAssociation(db))
	if w.Code != http.StatusOK {
		t.Fatalf("the Apple file answered %d: %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Errorf("Cache-Control is %q", cc)
	}
	var doc appleLinksDoc
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("the Apple file is not JSON: %v", err)
	}
	return doc
}

// appleClaims answers whether an iPhone reading these components would open
// path in the app: the first component whose pattern matches decides, "*" is
// zero or more characters and "?" exactly one, and both cross "/". That is
// Apple's documented matching, done here so the test reads the published
// order rather than trusting it.
func appleClaims(components []struct {
	Path    string `json:"/"`
	Exclude bool   `json:"exclude"`
}, path string) bool {
	for _, c := range components {
		var re strings.Builder
		re.WriteString("^")
		for _, ch := range c.Path {
			switch ch {
			case '*':
				re.WriteString(".*")
			case '?':
				re.WriteString(".")
			default:
				re.WriteString(regexp.QuoteMeta(string(ch)))
			}
		}
		re.WriteString("$")
		if regexp.MustCompile(re.String()).MatchString(path) {
			return !c.Exclude
		}
	}
	return false
}

// The block appears once an admin turns the switch on, names only the apps
// that were turned on, and goes away again when the last one is turned off.
// webcredentials is untouched throughout: holding the passkeys and claiming
// the URLs are two grants.
func TestOpenLinksPublishesApplinksForOptedInAppsOnly(t *testing.T) {
	db := setupTestDB(t)
	token := nativeAppAdmin(t, db, "apps-admin-links")

	first := addedAppID(t, addNativeApp(t, db, token, map[string]interface{}{
		"platform": "apple", "identifier": "ABCDE12345.org.example.app",
	}))
	addedAppID(t, addNativeApp(t, db, token, map[string]interface{}{
		"platform": "apple", "identifier": "VWXYZ67890.org.example.other",
	}))

	if doc := fetchAppleDoc(t, db); doc.Applinks != nil {
		t.Fatal("an applinks block was published before any app was opted in")
	}

	w := setOpenLinks(db, token, first, map[string]interface{}{"open_links": true})
	if w.Code != http.StatusOK {
		t.Fatalf("turning open_links on returned %d: %s", w.Code, w.Body.String())
	}
	var updated struct {
		ID        string `json:"id"`
		OpenLinks bool   `json:"open_links"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil || updated.ID != first || !updated.OpenLinks {
		t.Errorf("the PATCH answered %s", w.Body.String())
	}

	doc := fetchAppleDoc(t, db)
	if len(doc.Webcredentials.Apps) != 2 {
		t.Errorf("webcredentials.apps is %v; opting one app into links must not change who holds passkeys", doc.Webcredentials.Apps)
	}
	if doc.Applinks == nil || len(doc.Applinks.Details) != 1 {
		t.Fatalf("no single applinks detail after opting an app in: %+v", doc.Applinks)
	}
	detail := doc.Applinks.Details[0]
	if len(detail.AppIDs) != 1 || detail.AppIDs[0] != "ABCDE12345.org.example.app" {
		t.Errorf("applinks names %v, want only the opted-in app", detail.AppIDs)
	}

	// Spot checks on the published list itself: an exclusion the decision
	// names, and the patch page it is carved out of.
	index := map[string]int{}
	for i, c := range detail.Components {
		index[c.Path] = i
		switch c.Path {
		case "/patches/?*/settings*", "/login*", "/api/*":
			if !c.Exclude {
				t.Errorf("%q is published as a claim, want an exclusion", c.Path)
			}
		case "/patches/?*", "/events/?*", "/":
			if c.Exclude {
				t.Errorf("%q is published as an exclusion, want a claim", c.Path)
			}
		}
	}
	for _, p := range []string{"/patches/?*/settings*", "/login*", "/patches/?*", "/"} {
		if _, ok := index[p]; !ok {
			t.Errorf("the components carry no %q", p)
		}
	}
	if detail.Components[0].Path != "/api/*" || !detail.Components[0].Exclude {
		t.Errorf("the list opens with %+v; the exclusions come first", detail.Components[0])
	}
	if index["/patches/?*/settings*"] > index["/patches/?*"] {
		t.Error("the settings exclusion comes after the patch claim, so the claim would swallow it")
	}

	// Turning the last one off takes the block away again.
	if w := setOpenLinks(db, token, first, map[string]interface{}{"open_links": false}); w.Code != http.StatusOK {
		t.Fatalf("turning open_links off returned %d: %s", w.Code, w.Body.String())
	}
	if doc := fetchAppleDoc(t, db); doc.Applinks != nil {
		t.Error("the applinks block survived turning the last app off")
	}
}

// Read the published components the way an iPhone does, and check every
// route the decision names: the ones the client renders open in the app, and
// everything else, including web routes the client has never seen, stays the
// browser's.
func TestApplinksClaimMatchesTheClientsRoutes(t *testing.T) {
	db := setupTestDB(t)
	token := nativeAppAdmin(t, db, "apps-admin-claim")
	id := addedAppID(t, addNativeApp(t, db, token, map[string]interface{}{
		"platform": "apple", "identifier": "ABCDE12345.org.example.app",
	}))
	if w := setOpenLinks(db, token, id, map[string]interface{}{"open_links": true}); w.Code != http.StatusOK {
		t.Fatalf("PATCH returned %d: %s", w.Code, w.Body.String())
	}
	doc := fetchAppleDoc(t, db)
	if doc.Applinks == nil || len(doc.Applinks.Details) != 1 {
		t.Fatalf("no applinks detail: %+v", doc.Applinks)
	}
	components := doc.Applinks.Details[0].Components

	claimed := []string{
		"/", "/my", "/map", "/map/my", "/events", "/events/my", "/events/0192-abc",
		"/discover", "/dashboard", "/notifications", "/about", "/label", "/lining",
		"/governance", "/privacy", "/terms",
		"/patches/the-loom", "/patches/the-loom/events", "/patches/the-loom/members",
		"/patches/the-loom/governance", "/patches/the-loom/governance/0192-p",
		"/patches/the-loom/governance/proposals/0192-p", "/patches/the-loom/governance/docs/0192-d",
		"/patches/the-loom/noticeboard", "/patches/the-loom/noticeboard/0192-n",
		"/patches/the-loom/claim", "/patches/the-loom/setup",
		"/users/weaver", "/invite/tok3n", "/quilts/other.example/patches/their-patch",
	}
	for _, p := range claimed {
		if !appleClaims(components, p) {
			t.Errorf("%s stays in the browser, but the client renders it", p)
		}
	}

	notClaimed := []string{
		// Named by the decision.
		"/patches/new", "/patches/the-loom/settings", "/patches/the-loom/settings/members",
		"/patches/the-loom/manage", "/patches/the-loom/manage/members",
		"/patches/the-loom/governance/new", "/patches/the-loom/governance/rules",
		"/patches/the-loom/governance/rules/propose", "/patches/the-loom/governance/docs/new",
		"/patches/the-loom/governance/docs/0192-d/propose", "/patches/the-loom/governance/docs/0192-d/history",
		"/patches/the-loom/governance/record", "/patches/the-loom/noticeboard/new",
		"/events/new", "/events/0192-abc/edit",
		"/login", "/login/link/tok3n", "/signup/complete", "/welcome", "/submit",
		"/claims/verify-email", "/admin", "/admin/settings/apps", "/settings", "/settings/security",
		"/api/v1/nodes/tree", "/.well-known/apple-app-site-association", "/activity",
		// Web routes the client does not render, under prefixes it does.
		"/patches", "/patches/the-loom/governance/docs", "/patches/the-loom/governance/proposals",
		"/patches/the-loom/charters", "/patches/the-loom/proposals/0192-p",
		"/patches/the-loom/events/0192-abc", "/patches/the-loom/governance-setup",
		"/patches/the-loom/", "/users/weaver/extra", "/quilts/other.example/patches/their-patch/events",
		"/map/elsewhere",
	}
	for _, p := range notClaimed {
		if appleClaims(components, p) {
			t.Errorf("%s would open in the app, which does not handle it", p)
		}
	}
}

// The switch is the instance admin's, and it takes no step-up: listing the
// app was the grant that needed proof of presence, and this one names no new
// party. A plain admin session is enough; a member's is not.
func TestOpenLinksIsAdminOnlyWithoutStepUp(t *testing.T) {
	db := setupTestDB(t)
	sudo := nativeAppAdmin(t, db, "apps-admin-lister")
	id := addedAppID(t, addNativeApp(t, db, sudo, map[string]interface{}{
		"platform": "apple", "identifier": "ABCDE12345.org.example.app",
	}))

	_, member := createTestUser(t, db, "apps-member", "member")
	if w := setOpenLinks(db, member, id, map[string]interface{}{"open_links": true}); w.Code != http.StatusForbidden {
		t.Errorf("a member got %d, want 403: %s", w.Code, w.Body.String())
	}
	r := httptest.NewRequest("PATCH", "/api/v1/admin/native-apps/"+id, strings.NewReader(`{"open_links":true}`))
	if w := serveAdmin(db, "PATCH", "/api/v1/admin/native-apps/{id}",
		handler.AdminSetNativeAppOpenLinks(db), r); w.Code != http.StatusUnauthorized {
		t.Errorf("nobody got %d, want 401: %s", w.Code, w.Body.String())
	}
	if n := countRows(db, `SELECT COUNT(*) FROM native_apps WHERE open_links = 1`); n != 0 {
		t.Fatalf("a refused PATCH turned the switch on")
	}

	_, plain := createTestUser(t, db, "apps-admin-plain", "admin")
	if w := setOpenLinks(db, plain, id, map[string]interface{}{"open_links": true}); w.Code != http.StatusOK {
		t.Fatalf("an admin with no step-up window got %d, want 200: %s", w.Code, w.Body.String())
	}
	if n := countRows(db, `SELECT COUNT(*) FROM native_apps WHERE open_links = 1`); n != 1 {
		t.Errorf("after the PATCH %d rows open links", n)
	}
}

// Each change is in the audit log with the app it named and which way it
// went; setting the switch to where it already is changes nothing and says
// nothing.
func TestOpenLinksIsAuditedOnChange(t *testing.T) {
	db := setupTestDB(t)
	token := nativeAppAdmin(t, db, "apps-admin-links-audit")
	id := addedAppID(t, addNativeApp(t, db, token, map[string]interface{}{
		"platform": "apple", "identifier": "ABCDE12345.org.example.app",
	}))

	for _, on := range []bool{true, true, false, false} {
		if w := setOpenLinks(db, token, id, map[string]interface{}{"open_links": on}); w.Code != http.StatusOK {
			t.Fatalf("PATCH %v returned %d: %s", on, w.Code, w.Body.String())
		}
	}
	rows, err := db.Query(`SELECT metadata FROM audit_log WHERE action = 'admin.native_app_open_links' ORDER BY created_at, rowid`)
	if err != nil {
		t.Fatalf("query audit log: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var entries []map[string]interface{}
	for rows.Next() {
		var metadata string
		if err := rows.Scan(&metadata); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(metadata), &m); err != nil {
			t.Fatalf("decode %q: %v", metadata, err)
		}
		entries = append(entries, m)
	}
	if len(entries) != 2 {
		t.Fatalf("%d audit entries for two changes and two no-ops: %v", len(entries), entries)
	}
	if entries[0]["open_links"] != true || entries[1]["open_links"] != false {
		t.Errorf("the entries do not say which way the switch went: %v", entries)
	}
	if entries[0]["identifier"] != "ABCDE12345.org.example.app" || entries[0]["platform"] != "apple" {
		t.Errorf("the entry does not name the app: %v", entries[0])
	}
}

// Android's handle_all_urls is a later decision, so an Android row cannot be
// turned on; turning it off is allowed, since off is where it already is. A
// body without the field is a 400 rather than a quiet "off", and an unknown
// id is a 404.
func TestOpenLinksRefusals(t *testing.T) {
	db := setupTestDB(t)
	token := nativeAppAdmin(t, db, "apps-admin-links-refusals")
	android := addedAppID(t, addNativeApp(t, db, token, map[string]interface{}{
		"platform":     "android",
		"identifier":   "org.example.app",
		"fingerprints": []string{testAppFingerprint},
	}))
	apple := addedAppID(t, addNativeApp(t, db, token, map[string]interface{}{
		"platform": "apple", "identifier": "ABCDE12345.org.example.app",
	}))

	if w := setOpenLinks(db, token, android, map[string]interface{}{"open_links": true}); w.Code != http.StatusBadRequest {
		t.Errorf("an Android row was turned on: %d %s", w.Code, w.Body.String())
	}
	if w := setOpenLinks(db, token, android, map[string]interface{}{"open_links": false}); w.Code != http.StatusOK {
		t.Errorf("turning an Android row off returned %d: %s", w.Code, w.Body.String())
	}
	if w := setOpenLinks(db, token, apple, map[string]interface{}{}); w.Code != http.StatusBadRequest {
		t.Errorf("a body with no open_links returned %d: %s", w.Code, w.Body.String())
	}
	if w := setOpenLinks(db, token, auth.NewUUIDv7(), map[string]interface{}{"open_links": true}); w.Code != http.StatusNotFound {
		t.Errorf("an unknown id returned %d: %s", w.Code, w.Body.String())
	}
	if n := countRows(db, `SELECT COUNT(*) FROM native_apps WHERE open_links = 1`); n != 0 {
		t.Errorf("%d rows open links after only refusals", n)
	}
	// The assetlinks file never grows an app-links relation from any of this.
	w := fetchAssociation(db, handler.AndroidAssociationPath, handler.AssetLinks(db))
	if strings.Contains(w.Body.String(), "handle_all_urls") {
		t.Errorf("the Android file claims URLs: %s", w.Body.String())
	}
}

// The admin list carries the switch on every row, so the page can draw it.
func TestAdminListCarriesOpenLinks(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	token := nativeAppAdmin(t, db, "apps-admin-list-links")
	id := addedAppID(t, addNativeApp(t, db, token, map[string]interface{}{
		"platform": "apple", "identifier": "ABCDE12345.org.example.app",
	}))
	if w := setOpenLinks(db, token, id, map[string]interface{}{"open_links": true}); w.Code != http.StatusOK {
		t.Fatalf("PATCH returned %d: %s", w.Code, w.Body.String())
	}
	w := listNativeApps(t, db, cfg, token)
	var body struct {
		NativeApps []map[string]interface{} `json:"native_apps"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.NativeApps) != 1 || body.NativeApps[0]["open_links"] != true {
		t.Errorf("the list answered %v", body.NativeApps)
	}
}
