package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	// A later decision, deliberately not this one: listing an app for
	// passkeys must not also claim every URL on the domain.
	if _, present := doc["applinks"]; present {
		t.Error("the Apple file carries an applinks block, which no decision has made")
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
