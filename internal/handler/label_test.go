package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// The Label (docs/adr/023): floor of one, consent to appear, and the
// cross-quilt summary that never carries handles.

func getLabelJSON(t *testing.T, db *database.DB) map[string]any {
	t.Helper()
	r := httptest.NewRequest("GET", "/api/v1/label", nil)
	w := httptest.NewRecorder()
	handler.GetLabel(db)(w, r)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("label response: %v", err)
	}
	return body
}

func serveAuthed(db *database.DB, method, pattern string, h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc(method+" "+pattern, middleware.AuthRequired(db, h))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestLabelFloorOfOne(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "boss", "admin")

	// Publishing with zero listed stewards is refused: the buck has to
	// stop somewhere.
	r := authedRequest("PATCH", "/api/v1/admin/label", map[string]any{"published": true}, adminToken)
	w := serveAdmin(db, "PATCH", "/api/v1/admin/label", handler.AdminUpdateLabel(db), r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("publish with no stewards: got %d, want 400", w.Code)
	}

	// Adding yourself lists you immediately — consent is in the act.
	r = authedRequest("POST", "/api/v1/admin/label/stewards", map[string]string{"username": "boss"}, adminToken)
	w = serveAdmin(db, "POST", "/api/v1/admin/label/stewards", handler.AdminAddLabelSteward(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("add self: got %d: %s", w.Code, w.Body.String())
	}

	r = authedRequest("PATCH", "/api/v1/admin/label", map[string]any{"published": true, "prose": "hi, I run this"}, adminToken)
	w = serveAdmin(db, "PATCH", "/api/v1/admin/label", handler.AdminUpdateLabel(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("publish: got %d: %s", w.Code, w.Body.String())
	}

	if body := getLabelJSON(t, db); body["published"] != true {
		t.Fatalf("label should be published: %v", body)
	}

	// The last listed steward unlisting themselves unpublishes the Label:
	// the floor holds at all times, not just at publish.
	r = authedRequest("PATCH", "/api/v1/users/me/steward", map[string]any{"listed": false}, adminToken)
	w = serveAuthed(db, "PATCH", "/api/v1/users/me/steward", handler.UpdateMyStewardListing(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("unlist self: got %d: %s", w.Code, w.Body.String())
	}
	if body := getLabelJSON(t, db); body["published"] != false {
		t.Fatalf("label should auto-unpublish when the last steward unlists: %v", body)
	}
}

func TestLabelStewardConsent(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "boss", "admin")
	_, otherToken := createTestUser(t, db, "helper", "member")

	// Adding someone else creates an unlisted invitation.
	r := authedRequest("POST", "/api/v1/admin/label/stewards", map[string]string{"username": "helper"}, adminToken)
	w := serveAdmin(db, "POST", "/api/v1/admin/label/stewards", handler.AdminAddLabelSteward(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("invite: got %d: %s", w.Code, w.Body.String())
	}
	var listed int
	db.QueryRow(`SELECT listed FROM label_stewards`).Scan(&listed)
	if listed != 0 {
		t.Fatal("inviting someone else must not list them — only they can accept")
	}

	// Only the person can accept, and their blurb is their own words.
	r = authedRequest("PATCH", "/api/v1/users/me/steward",
		map[string]any{"listed": true, "blurb": "keeps the lights on"}, otherToken)
	w = serveAuthed(db, "PATCH", "/api/v1/users/me/steward", handler.UpdateMyStewardListing(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("accept: got %d: %s", w.Code, w.Body.String())
	}
	db.QueryRow(`SELECT listed FROM label_stewards`).Scan(&listed)
	if listed != 1 {
		t.Fatal("accepting should list the steward")
	}
}

func TestLabelSummaryNeverCarriesHandles(t *testing.T) {
	db := setupTestDB(t)
	cfg := testConfig()
	_, adminToken := createTestUser(t, db, "boss", "admin")

	// List a steward, add costs, publish.
	r := authedRequest("POST", "/api/v1/admin/label/stewards", map[string]string{"username": "boss"}, adminToken)
	serveAdmin(db, "POST", "/api/v1/admin/label/stewards", handler.AdminAddLabelSteward(db), r)

	today := time.Now().Format("2006-01-02")
	r = authedRequest("PUT", "/api/v1/admin/label/costs", map[string]any{
		"items": []map[string]any{
			{"service": "Hetzner CX22", "purpose": "the server", "amount_minor": 451, "period": "monthly", "stated_on": today},
			{"service": "Domain", "amount_minor": 1200, "period": "yearly", "stated_on": today},
		},
	}, adminToken)
	w := serveAdmin(db, "PUT", "/api/v1/admin/label/costs", handler.AdminPutLabelCosts(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("costs: got %d: %s", w.Code, w.Body.String())
	}
	r = authedRequest("PATCH", "/api/v1/admin/label", map[string]any{"published": true, "currency": "eur"}, adminToken)
	serveAdmin(db, "PATCH", "/api/v1/admin/label", handler.AdminUpdateLabel(db), r)

	// The instance endpoint gets the summary: count and total, no names —
	// consenting to a page is not consenting to a scrapeable directory.
	pub := httptest.NewRequest("GET", "/api/v1/instance", nil)
	pw := httptest.NewRecorder()
	handler.Instance(db, cfg)(pw, pub)
	var inst struct {
		Label handler.LabelSummary `json:"label"`
	}
	if err := json.Unmarshal(pw.Body.Bytes(), &inst); err != nil {
		t.Fatalf("instance response: %v", err)
	}
	if !inst.Label.Published || inst.Label.StewardCount != 1 {
		t.Fatalf("summary = %+v, want published with steward_count 1", inst.Label)
	}
	// 451 monthly + 1200/12 yearly = 551 minor units per month.
	if inst.Label.TotalMonthlyMinor != 551 {
		t.Fatalf("total = %d, want 551 (yearly normalized to monthly)", inst.Label.TotalMonthlyMinor)
	}
	if inst.Label.Currency != "EUR" {
		t.Fatalf("currency = %q, want EUR", inst.Label.Currency)
	}
	if inst.Label.Stale {
		t.Fatal("fresh figures should not be stale")
	}
	for _, forbidden := range []string{`"stewards"`, `"boss"`} {
		if strings.Contains(pw.Body.String(), forbidden) {
			t.Fatalf("instance summary must never carry steward handles; found %s", forbidden)
		}
	}

	// The full roster lives on the Label itself, one click away.
	if body := getLabelJSON(t, db); len(body["stewards"].([]any)) != 1 {
		t.Fatalf("public label should list the steward: %v", body)
	}
}
