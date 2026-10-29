package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// A post says whether AI made it
// (docs/adr/2026-10-20-a-post-says-whether-ai-made-it.md): three answers and
// NULL, nothing preselected, only `generated` shown, a switch that makes the
// question required, a patch-level declaration with a personal discovery
// filter, and a report whose first remedy is the label.

func setAIDisclosure(t *testing.T, db *database.DB, mode string) {
	t.Helper()
	if err := settings.Set(db, settings.KeyAIDisclosure, mode); err != nil {
		t.Fatalf("set ai_disclosure: %v", err)
	}
}

// storedAnswer reads a nullable answer column; "" means NULL.
func storedAnswer(t *testing.T, db *database.DB, table, column, id string) string {
	t.Helper()
	var v *string
	if err := db.QueryRow(`SELECT `+column+` FROM `+table+` WHERE id = ?`, id).Scan(&v); err != nil {
		t.Fatalf("read %s.%s: %v", table, column, err)
	}
	if v == nil {
		return ""
	}
	return *v
}

func postEvent(t *testing.T, db *database.DB, token string, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("POST", "/api/v1/events", body, token)
	return serveMux(t, db, "POST", "/api/v1/events", handler.CreateEvent(db, &config.Config{}), r)
}

func eventFixture(t *testing.T) (*database.DB, string, string) {
	t.Helper()
	db := setupTestDB(t)
	admin, token := createTestUser(t, db, "aievadmin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	return db, nodeID, token
}

func TestAIDisclosure_OptionalLeavesAnUnansweredEventNull(t *testing.T) {
	db, nodeID, token := eventFixture(t)

	w := postEvent(t, db, token, map[string]interface{}{
		"node_id": nodeID, "title": "Opening", "starts_at": "2030-06-01T19:00:00Z",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %s", w.Code, w.Body.String())
	}
	var e model.Event
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode: %v: %s", err, w.Body.String())
	}
	if e.AIUse != nil || e.ImageAI != nil {
		t.Errorf("an unanswered event should carry null answers: %s", w.Body.String())
	}
	if got := storedAnswer(t, db, "events", "ai_use", e.ID); got != "" {
		t.Errorf("stored ai_use = %q, want NULL: silence is not an answer", got)
	}
}

func TestAIDisclosure_EventStoresAndReturnsTheAnswers(t *testing.T) {
	db, nodeID, token := eventFixture(t)

	w := postEvent(t, db, token, map[string]interface{}{
		"node_id": nodeID, "title": "Opening", "starts_at": "2030-06-01T19:00:00Z",
		"ai_use": "assisted", "image_url": "https://example.com/flyer.jpg", "image_alt": "A flyer",
		"image_ai": "generated",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %s", w.Code, w.Body.String())
	}
	var e model.Event
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode: %v: %s", err, w.Body.String())
	}
	if e.AIUse == nil || *e.AIUse != "assisted" || e.ImageAI == nil || *e.ImageAI != "generated" {
		t.Errorf("create response answers wrong: %s", w.Body.String())
	}

	r := authedRequest("GET", "/api/v1/events/"+e.ID, nil, token)
	w = serveMux(t, db, "GET", "/api/v1/events/{id}", handler.GetEvent(db), r)
	if !bodyContains(w.Body.Bytes(), `"ai_use":"assisted"`) || !bodyContains(w.Body.Bytes(), `"image_ai":"generated"`) {
		t.Errorf("GetEvent does not carry the answers: %s", w.Body.String())
	}
}

func TestAIDisclosure_RejectsAnAnswerThatIsNotOne(t *testing.T) {
	db, nodeID, token := eventFixture(t)
	w := postEvent(t, db, token, map[string]interface{}{
		"node_id": nodeID, "title": "Opening", "starts_at": "2030-06-01T19:00:00Z", "ai_use": "maybe",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("ai_use maybe: got %d, want 400", w.Code)
	}
}

// Required means an API client cannot skip what the form asks: a 400 that
// names the field.
func TestAIDisclosure_RequiredRefusesACreateWithoutAnAnswer(t *testing.T) {
	db, nodeID, token := eventFixture(t)
	setAIDisclosure(t, db, "required")

	w := postEvent(t, db, token, map[string]interface{}{
		"node_id": nodeID, "title": "Opening", "starts_at": "2030-06-01T19:00:00Z",
	})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "ai_use") {
		t.Errorf("required, no answer: got %d %s, want a 400 naming ai_use", w.Code, w.Body.String())
	}

	// The image is asked only when there is one.
	w = postEvent(t, db, token, map[string]interface{}{
		"node_id": nodeID, "title": "Opening", "starts_at": "2030-06-01T19:00:00Z", "ai_use": "none",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("required, answered, no image: got %d %s", w.Code, w.Body.String())
	}
	w = postEvent(t, db, token, map[string]interface{}{
		"node_id": nodeID, "title": "Second", "starts_at": "2030-06-02T19:00:00Z", "ai_use": "none",
		"image_url": "https://example.com/flyer.jpg", "image_alt": "A flyer",
	})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "image_ai") {
		t.Errorf("required, image without answer: got %d %s, want a 400 naming image_ai", w.Code, w.Body.String())
	}
}

// A PATCH that does not mention the answer leaves it alone, because the edit
// form spreads what it loaded; clearing the image takes its answer with it.
func TestAIDisclosure_EventEditKeepsTheAnswer(t *testing.T) {
	db, nodeID, token := eventFixture(t)
	w := postEvent(t, db, token, map[string]interface{}{
		"node_id": nodeID, "title": "Opening", "starts_at": "2030-06-01T19:00:00Z", "ai_use": "generated",
		"image_url": "https://example.com/flyer.jpg", "image_alt": "A flyer", "image_ai": "none",
	})
	var e model.Event
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode: %v: %s", err, w.Body.String())
	}

	patch := func(body map[string]interface{}) *httptest.ResponseRecorder {
		r := authedRequest("PATCH", "/api/v1/events/"+e.ID, body, token)
		return serveMux(t, db, "PATCH", "/api/v1/events/{id}", handler.UpdateEvent(db), r)
	}
	if w := patch(map[string]interface{}{"title": "Opening night"}); w.Code != http.StatusOK {
		t.Fatalf("patch: got %d %s", w.Code, w.Body.String())
	}
	if got := storedAnswer(t, db, "events", "ai_use", e.ID); got != "generated" {
		t.Errorf("an unrelated edit changed ai_use to %q", got)
	}
	if w := patch(map[string]interface{}{"image_url": "", "image_alt": ""}); w.Code != http.StatusOK {
		t.Fatalf("clear image: got %d %s", w.Code, w.Body.String())
	}
	if got := storedAnswer(t, db, "events", "image_ai", e.ID); got != "" {
		t.Errorf("a cleared image kept its answer %q", got)
	}
	if w := patch(map[string]interface{}{"ai_use": "sometimes"}); w.Code != http.StatusBadRequest {
		t.Errorf("bad ai_use on PATCH: got %d, want 400", w.Code)
	}
}

// Content written elsewhere carries provenance: the CSV takes an optional
// answer and a blank cell stays NULL even where answers are required.
func TestAIDisclosure_CSVUploadTakesAnOptionalAnswer(t *testing.T) {
	db, nodeID, token := eventFixture(t)
	setAIDisclosure(t, db, "required")

	r := authedRequest("POST", "/api/v1/nodes/gallery-row/events/bulk", map[string]interface{}{
		"events": []map[string]interface{}{
			{"title": "One", "starts_at": "2030-06-01T19:00:00Z", "ai_use": "generated"},
			{"title": "Two", "starts_at": "2030-06-02T19:00:00Z"},
		},
	}, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/events/bulk", handler.BulkCreateEvents(db), r)
	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("bulk: got %d %s", w.Code, w.Body.String())
	}
	var one, two *string
	if err := db.QueryRow(`SELECT ai_use FROM events WHERE node_id = ? AND title = 'One'`, nodeID).Scan(&one); err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := db.QueryRow(`SELECT ai_use FROM events WHERE node_id = ? AND title = 'Two'`, nodeID).Scan(&two); err != nil {
		t.Fatalf("read: %v", err)
	}
	if one == nil || *one != "generated" {
		t.Errorf("row One ai_use = %v, want generated", one)
	}
	if two != nil {
		t.Errorf("row Two ai_use = %q, want NULL", *two)
	}

	r = authedRequest("POST", "/api/v1/nodes/gallery-row/events/bulk", map[string]interface{}{
		"events": []map[string]interface{}{{"title": "Three", "starts_at": "2030-06-03T19:00:00Z", "ai_use": "lots"}},
	}, token)
	if w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/events/bulk", handler.BulkCreateEvents(db), r); w.Code != http.StatusBadRequest {
		t.Errorf("bad CSV ai_use: got %d, want 400", w.Code)
	}
}

// A federated generated post says so in its content, the only place
// ActivityPub has for it; assisted is never shown.
func TestAIDisclosure_FederatedContentCarriesTheLine(t *testing.T) {
	generated, assisted := "generated", "assisted"
	obj := ap.EventToObject(model.Event{ID: "e1", Description: "Doors at seven.", AIUse: &generated, ImageAI: &generated}, "example.org")
	if !strings.HasSuffix(obj.Content, "Made with AI. Image made with AI.") {
		t.Errorf("generated event content = %q", obj.Content)
	}
	obj = ap.EventToObject(model.Event{ID: "e2", Description: "Doors at seven.", AIUse: &assisted}, "example.org")
	if obj.Content != "Doors at seven." {
		t.Errorf("assisted event content = %q, want it unchanged", obj.Content)
	}
}

func TestAIDisclosure_PatchDescriptionAndDeclaration(t *testing.T) {
	db := setupTestDB(t)
	user, token := createTestUser(t, db, "aicreator", "member")

	r := authedRequest("POST", "/api/v1/nodes", map[string]interface{}{
		"name": "Model Weekly", "ai_use": "generated", "ai_work": true,
	}, token)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/nodes", middleware.AuthRequired(db, handler.CreateNode(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d %s", w.Code, w.Body.String())
	}
	var n model.Node
	if err := json.Unmarshal(w.Body.Bytes(), &n); err != nil {
		t.Fatalf("decode: %v: %s", err, w.Body.String())
	}
	if n.AIUse == nil || *n.AIUse != "generated" || n.AIWork == nil || !*n.AIWork {
		t.Errorf("create response answers wrong: %s", w.Body.String())
	}

	// Dropping the badge is audited on its own line.
	if w := patchNode(t, db, n.Slug, token, map[string]interface{}{"ai_work": false}); w.Code != http.StatusOK {
		t.Fatalf("patch ai_work: got %d %s", w.Code, w.Body.String())
	}
	var audits int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'node.ai_work_set' AND entity_id = ? AND user_id = ?`, n.ID, user.ID).Scan(&audits); err != nil {
		t.Fatalf("read: %v", err)
	}
	if audits != 1 {
		t.Errorf("node.ai_work_set audit rows = %d, want 1", audits)
	}
	// Setting it to what it already is is not a change.
	patchNode(t, db, n.Slug, token, map[string]interface{}{"ai_work": false})
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'node.ai_work_set' AND entity_id = ?`, n.ID).Scan(&audits); err != nil {
		t.Fatalf("read: %v", err)
	}
	if audits != 1 {
		t.Errorf("an unchanged ai_work was audited again: %d rows", audits)
	}
	if w := patchNode(t, db, n.Slug, token, map[string]interface{}{"ai_work": "yes"}); w.Code != http.StatusBadRequest {
		t.Errorf("ai_work string: got %d, want 400", w.Code)
	}
}

// An unclaimed patch carries no declaration; whoever claims it answers.
func TestAIDisclosure_UnclaimedPatchTakesNoDeclaration(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "aisiteadmin", "admin")
	owner, _ := createTestUser(t, db, "aiowner", "member")
	nodeID := createTestNode(t, db, owner.ID, "Corner Bar", "corner-bar", "open")
	if _, err := db.Exec(`UPDATE nodes SET status = 'unclaimed' WHERE id = ?`, nodeID); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if w := patchNode(t, db, "corner-bar", adminToken, map[string]interface{}{"ai_work": true}); w.Code != http.StatusBadRequest {
		t.Errorf("ai_work on an unclaimed patch: got %d, want 400", w.Code)
	}
}

// The tree carries the declaration so cards can wear the badge, and the
// person who asked to hide declared work stops seeing it in discovery. Only
// that person: there is no instance-wide twin.
func TestAIDisclosure_HideAIWorkFiltersDiscoveryForThatPersonOnly(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "aiowner2", "member")
	hider, hiderToken := createTestUser(t, db, "aihider", "member")
	_, otherToken := createTestUser(t, db, "aiother", "member")
	declared := createTestNode(t, db, owner.ID, "Model Weekly", "model-weekly", "open")
	createTestNode(t, db, owner.ID, "Gallery Row", "gallery-row", "open")
	if _, err := db.Exec(`UPDATE nodes SET ai_work = 1 WHERE id = ?`, declared); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := db.Exec(`UPDATE users SET hide_ai_work = 1 WHERE id = ?`, hider.ID); err != nil {
		t.Fatalf("setup: %v", err)
	}

	tree := func(token string) string {
		r := authedRequest("GET", "/api/v1/nodes/tree", nil, token)
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/nodes/tree", middleware.AuthOptional(db, handler.NodeTree(db)))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Body.String()
	}
	if body := tree(otherToken); !strings.Contains(body, "model-weekly") || !strings.Contains(body, `"ai_work":true`) {
		t.Errorf("another person's tree should carry the declared patch and its badge: %s", body)
	}
	if body := tree(hiderToken); strings.Contains(body, "model-weekly") || !strings.Contains(body, "gallery-row") {
		t.Errorf("the hider's tree should drop only the declared patch: %s", body)
	}

	list := func(token string) string {
		r := authedRequest("GET", "/api/v1/nodes", nil, token)
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/nodes", middleware.AuthOptional(db, handler.ListNodes(db)))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Body.String()
	}
	if body := list(hiderToken); strings.Contains(body, "model-weekly") {
		t.Errorf("the hider's patch list still has the declared patch: %s", body)
	}
	if body := list(otherToken); !strings.Contains(body, "model-weekly") {
		t.Errorf("another person's patch list lost the declared patch: %s", body)
	}
}

func TestAIDisclosure_HideAIWorkIsTheirSwitch(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "aiswitch", "member")
	r := authedRequest("PATCH", "/api/v1/auth/me", map[string]interface{}{"hide_ai_work": true}, token)
	w := serveMux(t, db, "PATCH", "/api/v1/auth/me", handler.UpdateMe(db), r)
	if w.Code != http.StatusOK || !bodyContains(w.Body.Bytes(), `"hide_ai_work":true`) {
		t.Errorf("PATCH hide_ai_work: got %d %s", w.Code, w.Body.String())
	}
}

func TestAIDisclosure_ProposalAndNoticeCarryTheAnswer(t *testing.T) {
	rm := newRoom(t)

	r := authedRequest("POST", "/api/v1/nodes/room/proposals", map[string]interface{}{
		"title": "Buy a kiln", "body": "We need one.", "proposal_type": "action", "ai_use": "generated",
	}, rm.memberTok)
	w := serveMux(t, rm.db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(rm.db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create proposal: got %d %s", w.Code, w.Body.String())
	}
	var p model.Proposal
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode: %v: %s", err, w.Body.String())
	}
	if got := storedAnswer(t, rm.db, "proposals", "ai_use", p.ID); got != "generated" {
		t.Errorf("proposal ai_use = %q, want generated", got)
	}
	// The detail page reads the answer from GetProposal, which builds its
	// payload by hand, and the list reads it from ListProposals.
	r = authedRequest("GET", "/api/v1/proposals/"+p.ID, nil, rm.memberTok)
	w = serveMux(t, rm.db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(rm.db), r)
	if !bodyContains(w.Body.Bytes(), `"ai_use":"generated"`) {
		t.Errorf("GetProposal does not carry ai_use: %s", w.Body.String())
	}
	r = authedRequest("GET", "/api/v1/nodes/room/proposals", nil, rm.memberTok)
	w = serveMux(t, rm.db, "GET", "/api/v1/nodes/{slug}/proposals", handler.ListProposals(rm.db), r)
	if !bodyContains(w.Body.Bytes(), `"ai_use":"generated"`) {
		t.Errorf("ListProposals does not carry ai_use: %s", w.Body.String())
	}

	setAIDisclosure(t, rm.db, "required")
	r = authedRequest("POST", "/api/v1/nodes/room/proposals", map[string]interface{}{
		"title": "Buy a second kiln", "body": "Still need one.", "proposal_type": "action",
	}, rm.memberTok)
	if w := serveMux(t, rm.db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(rm.db), r); w.Code != http.StatusBadRequest {
		t.Errorf("required, unanswered proposal: got %d, want 400", w.Code)
	}

	w = rm.do(t, "POST", "/api/v1/nodes/{slug}/notices", "/api/v1/nodes/room/notices",
		map[string]interface{}{"title": "Kiln repair Saturday", "ai_use": "none"}, rm.memberTok, handler.CreateNotice(rm.db))
	if w.Code != http.StatusCreated || !bodyContains(w.Body.Bytes(), `"ai_use":"none"`) {
		t.Fatalf("create notice: got %d %s", w.Code, w.Body.String())
	}
	var notice model.Notice
	if err := json.Unmarshal(w.Body.Bytes(), &notice); err != nil {
		t.Fatalf("decode: %v: %s", err, w.Body.String())
	}
	w = rm.do(t, "PATCH", "/api/v1/notices/{id}", "/api/v1/notices/"+notice.ID,
		map[string]interface{}{"body": "Bring gloves."}, rm.memberTok, handler.UpdateNotice(rm.db))
	if w.Code != http.StatusOK {
		t.Fatalf("edit notice: got %d %s", w.Code, w.Body.String())
	}
	if got := storedAnswer(t, rm.db, "notices", "ai_use", notice.ID); got != "none" {
		t.Errorf("a notice edit that did not mention ai_use changed it to %q", got)
	}
}

// The report: unlabelled_ai on a proposal, and label as the first remedy,
// which sets the answer and records who applied it.
func TestAIDisclosure_LabelIsTheFirstRemedy(t *testing.T) {
	rm := newRoom(t)
	if _, err := rm.db.Exec(`UPDATE nodes SET public_governance_record = 'everyone' WHERE id = ?`, rm.nodeID); err != nil {
		t.Fatalf("setup: %v", err)
	}
	proposalID := createTestProposal(t, rm.db, rm.nodeID, rm.memberID)

	r := authedRequest("POST", "/api/v1/reports", map[string]interface{}{
		"entity_type": "proposal", "entity_id": proposalID, "reason": "unlabelled_ai",
	}, rm.strangerTok)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/reports", middleware.AuthRequired(rm.db, handler.CreateReport(rm.db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("report proposal: got %d %s", w.Code, w.Body.String())
	}
	var reportID string
	if err := rm.db.QueryRow(`SELECT id FROM content_reports WHERE entity_id = ?`, proposalID).Scan(&reportID); err != nil {
		t.Fatalf("read: %v", err)
	}

	r = authedRequest("PATCH", "/api/v1/admin/reports/"+reportID,
		map[string]interface{}{"status": "resolved", "action": "label"}, rm.siteAdminTok)
	w = serveMux(t, rm.db, "PATCH", "/api/v1/admin/reports/{id}", middleware.AdminRequired(rm.db, handler.UpdateReport(rm.db)), r)
	if w.Code != http.StatusOK {
		t.Fatalf("label: got %d %s", w.Code, w.Body.String())
	}
	if got := storedAnswer(t, rm.db, "proposals", "ai_use", proposalID); got != "generated" {
		t.Errorf("labelled proposal ai_use = %q, want generated", got)
	}
	var audits int
	if err := rm.db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'admin.ai_label' AND entity_id = ?`, proposalID).Scan(&audits); err != nil {
		t.Fatalf("read: %v", err)
	}
	if audits != 1 {
		t.Errorf("admin.ai_label audit rows = %d, want 1", audits)
	}
}

// A proposal on a closed record cannot be reported by someone outside it,
// the same 404 the proposal itself answers.
func TestAIDisclosure_ProposalReportFollowsTheRecord(t *testing.T) {
	rm := newRoom(t)
	if _, err := rm.db.Exec(`UPDATE nodes SET public_governance_record = 'nobody' WHERE id = ?`, rm.nodeID); err != nil {
		t.Fatalf("setup: %v", err)
	}
	proposalID := createTestProposal(t, rm.db, rm.nodeID, rm.memberID)

	r := authedRequest("POST", "/api/v1/reports", map[string]interface{}{
		"entity_type": "proposal", "entity_id": proposalID, "reason": "unlabelled_ai",
	}, rm.strangerTok)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/reports", middleware.AuthRequired(rm.db, handler.CreateReport(rm.db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("stranger reporting a closed record's proposal: got %d, want 404", w.Code)
	}
}

func TestAIDisclosure_PatchQueueLabelsANotice(t *testing.T) {
	rm := newRoom(t)
	created := rm.post(t, rm.memberTok, map[string]interface{}{"title": "Free kiln time"})
	noticeID, _ := created["id"].(string)

	reportID := auth.NewUUIDv7()
	if _, err := rm.db.Exec(`INSERT INTO content_reports (id, reporter_id, entity_type, entity_id, reason, details, node_id)
		VALUES (?, ?, 'notice', ?, 'unlabelled_ai', '', ?)`, reportID, rm.otherID, noticeID, rm.nodeID); err != nil {
		t.Fatalf("setup: %v", err)
	}

	w := rm.do(t, "PATCH", "/api/v1/nodes/{slug}/reports/{id}", "/api/v1/nodes/room/reports/"+reportID,
		map[string]interface{}{"action": "label"}, rm.adminTok, handler.UpdatePatchReport(rm.db))
	if w.Code != http.StatusOK {
		t.Fatalf("label notice: got %d %s", w.Code, w.Body.String())
	}
	if got := storedAnswer(t, rm.db, "notices", "ai_use", noticeID); got != "generated" {
		t.Errorf("labelled notice ai_use = %q, want generated", got)
	}
}

// The AI page says which way the switch is set, as the privacy policy says
// which way visitor counting is.
func TestAIDisclosure_AIPageRendersTheSwitch(t *testing.T) {
	db := setupTestDB(t)
	page := func() string {
		md, _ := getAIPage(t, db)["project_markdown"].(string)
		return md
	}
	if md := page(); !strings.Contains(md, "Answering is optional") || strings.Contains(md, "{ai_disclosure}") {
		t.Errorf("default page should render the optional paragraph: %s", md)
	}
	setAIDisclosure(t, db, "required")
	if md := page(); !strings.Contains(md, "It needs an answer") {
		t.Errorf("required page should say an answer is needed: %s", md)
	}
	setAIDisclosure(t, db, "off")
	if md := page(); !strings.Contains(md, "does not ask people whether AI made") || strings.Contains(md, "{quilt_name}") {
		t.Errorf("off page should say the question is off, with the quilt named: %s", md)
	}
}

func TestAIDisclosure_AdminSetsTheSwitch(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "aiadmin", "admin")
	patch := func(mode string) *httptest.ResponseRecorder {
		r := authedRequest("PATCH", "/api/v1/admin/settings", map[string]interface{}{"ai_disclosure": mode}, adminToken)
		return serveAdminMux(t, db, "PATCH", "/api/v1/admin/settings", handler.AdminUpdateSettings(db, testConfig()), r)
	}
	if w := patch("sometimes"); w.Code != http.StatusBadRequest {
		t.Errorf("bad mode: got %d, want 400", w.Code)
	}
	if w := patch("required"); w.Code != http.StatusOK {
		t.Fatalf("required: got %d %s", w.Code, w.Body.String())
	}
	if got := settings.AIDisclosure(db); got != "required" {
		t.Errorf("stored mode = %q, want required", got)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/instance", handler.Instance(db, testConfig()))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/instance", nil))
	if !bodyContains(w.Body.Bytes(), `"ai_disclosure":"required"`) {
		t.Errorf("instance endpoint does not report the mode: %s", w.Body.String())
	}
}
