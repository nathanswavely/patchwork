package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

func testConfig() *config.Config {
	return &config.Config{
		Instance: config.Instance{
			Name:        "Yaml Quilt",
			Domain:      "quilt.example.com",
			Description: "from yaml",
		},
	}
}

func serveAdmin(db *database.DB, method, pattern string, h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc(method+" "+pattern, middleware.AdminRequired(db, h))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestQuiltRenameOverridesYaml(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	_, token := createTestUser(t, db, "boss", "admin")

	// Rename via PATCH.
	r := authedRequest("PATCH", "/api/v1/admin/settings",
		map[string]string{"name": "Renamed Quilt", "description": "new desc"}, token)
	w := serveAdmin(db, "PATCH", "/api/v1/admin/settings", handler.AdminUpdateSettings(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("rename: got %d: %s", w.Code, w.Body.String())
	}

	if got := settings.EffectiveName(db, cfg); got != "Renamed Quilt" {
		t.Fatalf("effective name = %q, want Renamed Quilt", got)
	}

	// Public instance endpoint reflects the override and exposes icon_url.
	pub := httptest.NewRequest("GET", "/api/v1/instance", nil)
	pw := httptest.NewRecorder()
	handler.Instance(db, cfg)(pw, pub)
	var resp struct {
		Name    string `json:"name"`
		IconURL string `json:"icon_url"`
	}
	json.NewDecoder(pw.Body).Decode(&resp)
	if resp.Name != "Renamed Quilt" {
		t.Errorf("instance name = %q, want Renamed Quilt", resp.Name)
	}
	if resp.IconURL != "/api/v1/instance/icon" {
		t.Errorf("icon_url = %q", resp.IconURL)
	}

	// Empty name is refused.
	r = authedRequest("PATCH", "/api/v1/admin/settings", map[string]string{"name": "  "}, token)
	w = serveAdmin(db, "PATCH", "/api/v1/admin/settings", handler.AdminUpdateSettings(db, cfg), r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty name: got %d, want 400", w.Code)
	}
}

// Explicit null clears an override so patchwork.yaml is authoritative
// again — the same three-way contract icon_design already uses. Absent must
// stay distinguishable from null, or saving the name would silently reset
// the description on every identity save.
func TestQuiltIdentityResetsToYaml(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	_, token := createTestUser(t, db, "boss", "admin")

	patch := func(body map[string]interface{}) *httptest.ResponseRecorder {
		r := authedRequest("PATCH", "/api/v1/admin/settings", body, token)
		return serveAdmin(db, "PATCH", "/api/v1/admin/settings", handler.AdminUpdateSettings(db, cfg), r)
	}

	if w := patch(map[string]interface{}{"name": "Renamed Quilt", "description": "new desc"}); w.Code != http.StatusOK {
		t.Fatalf("override: got %d: %s", w.Code, w.Body.String())
	}

	// Resetting the name leaves the description override standing.
	w := patch(map[string]interface{}{"name": nil})
	if w.Code != http.StatusOK {
		t.Fatalf("reset name: got %d: %s", w.Code, w.Body.String())
	}
	if got := settings.EffectiveName(db, cfg); got != cfg.Instance.Name {
		t.Errorf("effective name = %q, want the yaml default %q", got, cfg.Instance.Name)
	}
	if got := settings.EffectiveDescription(db, cfg); got != "new desc" {
		t.Errorf("description = %q, want it untouched by the name reset", got)
	}

	var resp struct {
		NameOverridden bool `json:"name_overridden"`
		DescOverridden bool `json:"description_overridden"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.NameOverridden {
		t.Error("name_overridden stayed true after a reset")
	}
	if !resp.DescOverridden {
		t.Error("description_overridden went false without being reset")
	}

	// A blank description resets too: Get reads "" as absent, so storing it
	// would leave a row claiming an override no reader ever sees.
	if w := patch(map[string]interface{}{"description": "   "}); w.Code != http.StatusOK {
		t.Fatalf("blank description: got %d: %s", w.Code, w.Body.String())
	}
	if _, overridden := settings.Get(db, settings.KeyDescription); overridden {
		t.Error("a blank description left an override row behind")
	}
	if got := settings.EffectiveDescription(db, cfg); got != cfg.Instance.Description {
		t.Errorf("description = %q, want the yaml default %q", got, cfg.Instance.Description)
	}
}

func getIcon(db *database.DB, cfg *config.Config, ifNoneMatch string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "/api/v1/instance/icon", nil)
	if ifNoneMatch != "" {
		r.Header.Set("If-None-Match", ifNoneMatch)
	}
	w := httptest.NewRecorder()
	handler.InstanceIcon(db, cfg)(w, r)
	return w
}

// The quilt icon is drafted in the block drafter (docs/adr/043): the
// admin saves a design, the public endpoint renders it to SVG.
func TestQuiltIconIsDrafted(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	_, token := createTestUser(t, db, "boss", "admin")

	// Undesigned: a starter block assigned from the quilt's name.
	w := getIcon(db, cfg, "")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("assigned icon: code %d, type %q", w.Code, w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), "<path") {
		t.Fatal("assigned icon has no pieces")
	}
	assigned := w.Body.String()

	// The settings payload offers starters and reports the icon unchosen.
	sr := authedRequest("GET", "/api/v1/admin/settings", nil, token)
	sw := serveAdmin(db, "GET", "/api/v1/admin/settings", handler.AdminGetSettings(db, cfg), sr)
	var settingsResp struct {
		Icon struct {
			Chosen bool `json:"chosen"`
			Design struct {
				Block  map[string]interface{} `json:"block"`
				Bundle []string               `json:"bundle"`
			} `json:"design"`
		} `json:"icon"`
		Starters []struct {
			Key   string                 `json:"key"`
			Name  string                 `json:"name"`
			Block map[string]interface{} `json:"block"`
		} `json:"icon_starters"`
	}
	if err := json.NewDecoder(sw.Body).Decode(&settingsResp); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if settingsResp.Icon.Chosen {
		t.Error("a fresh quilt reports its icon as chosen")
	}
	if len(settingsResp.Icon.Design.Block) == 0 || len(settingsResp.Icon.Design.Bundle) == 0 {
		t.Error("settings did not describe the effective icon design")
	}
	if len(settingsResp.Starters) == 0 {
		t.Fatal("no starter blocks offered")
	}

	// Save a design.
	design := map[string]interface{}{
		"block": map[string]interface{}{
			"grid":   2,
			"seams":  [][]int{{0, 0, 8, 8}},
			"colors": map[string][]int{"0,0": {1, 0}},
		},
		"bundle": []string{"#EC341C", "#F2EEE4"},
	}
	r := authedRequest("PATCH", "/api/v1/admin/settings", map[string]interface{}{"icon_design": design}, token)
	if w := serveAdmin(db, "PATCH", "/api/v1/admin/settings", handler.AdminUpdateSettings(db, cfg), r); w.Code != http.StatusOK {
		t.Fatalf("save design: got %d: %s", w.Code, w.Body.String())
	}

	w = getIcon(db, cfg, "")
	drafted := w.Body.String()
	if drafted == assigned {
		t.Error("the drafted icon renders the same as the assigned one")
	}
	if !strings.Contains(drafted, "#EC341C") {
		t.Error("the design's fabric did not reach the SVG")
	}
	if etag := w.Header().Get("ETag"); etag == "" {
		t.Error("icon served without an ETag")
	} else if again := getIcon(db, cfg, etag); again.Code != http.StatusNotModified {
		t.Errorf("conditional request: got %d, want 304", again.Code)
	}

	// A malformed design is refused, and the saved one survives.
	bad := map[string]interface{}{"block": map[string]interface{}{"grid": 99}}
	r = authedRequest("PATCH", "/api/v1/admin/settings", map[string]interface{}{"icon_design": bad}, token)
	if w := serveAdmin(db, "PATCH", "/api/v1/admin/settings", handler.AdminUpdateSettings(db, cfg), r); w.Code != http.StatusBadRequest {
		t.Errorf("grid 99: got %d, want 400", w.Code)
	}
	if got := getIcon(db, cfg, "").Body.String(); got != drafted {
		t.Error("a refused design changed the served icon")
	}

	// Explicit null goes back to the assigned starter.
	r = authedRequest("PATCH", "/api/v1/admin/settings", map[string]interface{}{"icon_design": nil}, token)
	if w := serveAdmin(db, "PATCH", "/api/v1/admin/settings", handler.AdminUpdateSettings(db, cfg), r); w.Code != http.StatusOK {
		t.Fatalf("clear design: got %d: %s", w.Code, w.Body.String())
	}
	if got := getIcon(db, cfg, "").Body.String(); got != assigned {
		t.Error("clearing the design did not restore the assigned block")
	}
}

func TestQuiltWipe(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	admin, token := createTestUser(t, db, "boss", "admin")
	nodeID := createTestNode(t, db, admin.ID, "Band", "band", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// Wrong confirmation name refuses and deletes nothing.
	r := authedRequest("POST", "/api/v1/admin/wipe", map[string]string{"confirm_name": "wrong"}, token)
	w := serveAdmin(db, "POST", "/api/v1/admin/wipe", handler.AdminWipe(db, cfg), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("wrong name: got %d, want 400", w.Code)
	}
	// Nothing deleted: the admin plus the _system sentinel remain.
	var users int
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&users)
	if users != 2 {
		t.Fatalf("wrong-name wipe changed data: %d users, want 2", users)
	}

	// Correct name wipes everything.
	r = authedRequest("POST", "/api/v1/admin/wipe", map[string]string{"confirm_name": "Yaml Quilt"}, token)
	w = serveAdmin(db, "POST", "/api/v1/admin/wipe", handler.AdminWipe(db, cfg), r)
	if w.Code != http.StatusOK {
		t.Fatalf("wipe: got %d: %s", w.Code, w.Body.String())
	}

	for _, table := range []string{"nodes", "memberships", "sessions", "instance_settings"} {
		var n int
		db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n)
		if n != 0 {
			t.Errorf("table %s still has %d rows after wipe", table, n)
		}
	}

	// Only the re-seeded _system sentinel remains, so the next real account
	// becomes the instance admin again (bootstrap rule).
	var remaining string
	if err := db.QueryRow("SELECT username FROM users").Scan(&remaining); err != nil || remaining != "_system" {
		t.Errorf("after wipe users = %q (err %v), want just _system", remaining, err)
	}

	// Schema and migration history survive.
	var migrations int
	db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&migrations)
	if migrations == 0 {
		t.Error("schema_migrations was wiped — it must survive")
	}
}
