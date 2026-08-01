package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Amendments adopted elsewhere (docs/adr/053).
//
// The correctness-sensitive parts are the two refusals and the direction of
// the write. The rules file and the lining must stay unattestable — one closes
// a route around the leadership gate, the other is docs/adr/037's hard rule —
// and an attestation must *replace* the charter's text rather than merge into
// it, because Patchwork's copy is the stale side.

// proposalsElsewhereNode is a patch that decides its proposals at meetings.
func proposalsElsewhereNode(t *testing.T, db *database.DB, ownerID, name, slug string) string {
	t.Helper()
	nodeID := createTestNode(t, db, ownerID, name, slug, "open")
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":0,"default_vote_duration_hours":72,`+
			`"proposal_venue":"elsewhere","min_voting_tenure_days":0}`, nodeID)
	return nodeID
}

func seedDoc(t *testing.T, db *database.DB, nodeID, createdBy, title, body, kind string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO governance_docs (id, node_id, title, body, kind, visibility, version, created_by)
		 VALUES (?, ?, ?, ?, ?, 'members', 1, ?)`,
		id, nodeID, title, body, kind, createdBy,
	); err != nil {
		t.Fatalf("seed doc: %v", err)
	}
	return id
}

func recordAdoption(t *testing.T, db *database.DB, slug, token string, body map[string]interface{}) *httpRecorderResult {
	t.Helper()
	r := authedRequest("POST", "/api/v1/nodes/"+slug+"/amendment-attestations", body, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/amendment-attestations",
		handler.CreateAmendmentAttestation(db), r)
	return &httpRecorderResult{Code: w.Code, Body: w.Body.String()}
}

func docBody(t *testing.T, db *database.DB, docID string) (string, int) {
	t.Helper()
	var body string
	var version int
	db.QueryRow("SELECT body, version FROM governance_docs WHERE id = ?", docID).Scan(&body, &version)
	return body, version
}

// The gate. On a patch that decides here, recording an adoption would be a way
// around the vote — the same reason docs/adr/052 gated the leadership half.
func TestAmendmentAttestation_RefusedWhereDecidedInPatchwork(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "amatt1", "member")
	nodeID := createTestNode(t, db, admin.ID, "Am One", "am-one", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	docID := seedDoc(t, db, nodeID, admin.ID, "Bylaws", "Old text", "charter")

	res := recordAdoption(t, db, "am-one", adminToken, map[string]interface{}{
		"doc_id": docID, "decided_at": "2026-03-14", "adopted_body": "New text",
	})
	if res.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", res.Code, res.Body)
	}
	if body, _ := docBody(t, db, docID); body != "Old text" {
		t.Errorf("a refused record must not touch the charter, got %q", body)
	}
}

// Recording is an admin act. A member of a patch that decides at meetings is
// still not the person who says what the meeting decided.
func TestAmendmentAttestation_MemberMayNotRecord(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "amatt2", "member")
	nodeID := proposalsElsewhereNode(t, db, admin.ID, "Am Two", "am-two")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	member, memberToken := createTestUser(t, db, "amatt2m", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	docID := seedDoc(t, db, nodeID, admin.ID, "Bylaws", "Old text", "charter")

	res := recordAdoption(t, db, "am-two", memberToken, map[string]interface{}{
		"doc_id": docID, "decided_at": "2026-03-14", "adopted_body": "New text",
	})
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", res.Code, res.Body)
	}
}

// The happy path, and the direction that matters: the adopted text replaces
// the charter whole. Patchwork's copy is a possibly-stale cache being
// corrected, not a base to build on (docs/adr/053), so nothing about the old
// text survives and no conflict check can refuse the truth.
func TestAmendmentAttestation_ReplacesTheCharterWhole(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "amatt3", "member")
	nodeID := proposalsElsewhereNode(t, db, admin.ID, "Am Three", "am-three")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	docID := seedDoc(t, db, nodeID, admin.ID, "Bylaws",
		"Article I. The shipped template, amended at three AGMs since and recorded nowhere.", "charter")

	adopted := "Article I. What the members actually agreed.\n\nArticle II. And this."
	res := recordAdoption(t, db, "am-three", adminToken, map[string]interface{}{
		"doc_id": docID, "decided_at": "2026-03-14",
		"summary": "Annual meeting", "adopted_body": adopted,
	})
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", res.Code, res.Body)
	}

	body, version := docBody(t, db, docID)
	if body != adopted {
		t.Errorf("expected the charter to hold the adopted text, got %q", body)
	}
	if version != 2 {
		t.Errorf("expected the version to move, got %d", version)
	}

	// The record carries the text too, so it stays checkable after the next
	// adoption replaces the charter.
	var storedBody, decidedAt, targetDoc string
	db.QueryRow(`SELECT adopted_body, decided_at, target_doc FROM amendment_attestations
	             WHERE node_id = ?`, nodeID).Scan(&storedBody, &decidedAt, &targetDoc)
	if storedBody != adopted {
		t.Errorf("expected the record to carry the adopted text, got %q", storedBody)
	}
	if decidedAt != "2026-03-14" {
		t.Errorf("expected the decision's own date, got %q", decidedAt)
	}
	if targetDoc != "bylaws.md" {
		t.Errorf("expected the git filename as the durable key, got %q", targetDoc)
	}

	// And it reads back publicly, which is the whole point: an attestation
	// Patchwork cannot check is worth what the people in the room can check.
	lr := authedRequest("GET", "/api/v1/nodes/am-three/amendment-attestations", nil, adminToken)
	lw := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/amendment-attestations",
		handler.ListAmendmentAttestations(db), lr)
	if lw.Code != http.StatusOK {
		t.Fatalf("expected 200 listing, got %d: %s", lw.Code, lw.Body.String())
	}
	if !strings.Contains(lw.Body.String(), "Annual meeting") {
		t.Errorf("expected the record in the listing, got %s", lw.Body.String())
	}
}

// The lining changes only by a passed amendment proposal (docs/adr/037),
// wherever else a patch decides. If an assertion could diverge one, the
// "Amended lining" badge would look identical whether a community had voted or
// one admin had claimed a meeting happened — and the anti-discrimination
// baseline is what that badge guards.
func TestAmendmentAttestation_LiningIsNotAttestable(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "amatt4", "member")
	nodeID := proposalsElsewhereNode(t, db, admin.ID, "Am Four", "am-four")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	liningID := seedDoc(t, db, nodeID, admin.ID, "Community Standards", "The baseline.", "lining")

	// By id...
	res := recordAdoption(t, db, "am-four", adminToken, map[string]interface{}{
		"doc_id": liningID, "decided_at": "2026-03-14", "adopted_body": "No discrimination clause.",
	})
	if res.Code != http.StatusConflict {
		t.Fatalf("by id: expected 409, got %d: %s", res.Code, res.Body)
	}
	// ...and by the name that resolves to it, which is the way around the
	// first check if only the first check exists.
	res = recordAdoption(t, db, "am-four", adminToken, map[string]interface{}{
		"title": "Community Standards", "decided_at": "2026-03-14", "adopted_body": "No discrimination clause.",
	})
	if res.Code != http.StatusConflict {
		t.Fatalf("by title: expected 409, got %d: %s", res.Code, res.Body)
	}

	if body, _ := docBody(t, db, liningID); body != "The baseline." {
		t.Errorf("the lining's body must be untouched, got %q", body)
	}
}

// Prose is attestable; the machine configuration is not (docs/adr/053), and
// this is what closes the two-step route around the leadership gate: attest a
// rules amendment flipping `leadership_venue` to elsewhere, then attest a
// council — one admin's assertion, and the gate docs/adr/052 built is gone.
//
// Asserted as the property rather than as a refusal, because the exclusion is
// structural: an attestation writes `governanceFilename(title)`, which always
// ends in `.md`, and never calls the rules sync. There is no request that
// reaches the rules through here, so the test that matters is that no request
// moves `governance_config`, however it is dressed up.
func TestAmendmentAttestation_CannotReachTheRules(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "amatt5", "member")
	nodeID := proposalsElsewhereNode(t, db, admin.ID, "Am Five", "am-five")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	var before string
	db.QueryRow("SELECT governance_config FROM nodes WHERE id = ?", nodeID).Scan(&before)

	for _, title := range []string{"governance rules.json", "Governance Rules", "governance-rules"} {
		recordAdoption(t, db, "am-five", adminToken, map[string]interface{}{
			"title": title, "decided_at": "2026-03-14",
			"adopted_body": `{"leadership_venue":"elsewhere","decision_method":"admin"}`,
		})
	}

	var after string
	db.QueryRow("SELECT governance_config FROM nodes WHERE id = ?", nodeID).Scan(&after)
	if after != before {
		t.Errorf("an attestation moved the rules:\n before %s\n after  %s", before, after)
	}

	// And whatever it wrote, it wrote as prose.
	rows, _ := db.Query("SELECT target_doc FROM amendment_attestations WHERE node_id = ?", nodeID)
	defer rows.Close()
	for rows.Next() {
		var target string
		rows.Scan(&target)
		if !strings.HasSuffix(target, ".md") {
			t.Errorf("expected every attested document to be prose, got %q", target)
		}
	}
}

// A meeting can adopt a charter this instance was never templated with.
// Refusing would mean a community may only record amendments to documents
// Patchwork happened to guess at.
func TestAmendmentAttestation_CreatesAMissingDocument(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "amatt6", "member")
	nodeID := proposalsElsewhereNode(t, db, admin.ID, "Am Six", "am-six")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	res := recordAdoption(t, db, "am-six", adminToken, map[string]interface{}{
		"title": "Operating Agreement", "decided_at": "2026-03-14",
		"adopted_body": "Section 1.",
	})
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", res.Code, res.Body)
	}

	var title, body, visibility string
	if err := db.QueryRow(
		"SELECT title, body, visibility FROM governance_docs WHERE node_id = ? AND title = ?",
		nodeID, "Operating Agreement").Scan(&title, &body, &visibility); err != nil {
		t.Fatalf("expected the document to be created: %v", err)
	}
	if body != "Section 1." {
		t.Errorf("expected the adopted text, got %q", body)
	}
	// Publishing stays a deliberate act (docs/adr/036).
	if visibility != "members" {
		t.Errorf("expected a new charter to be members-only, got %q", visibility)
	}
}

// Recording by name lands on the charter that name already means, rather than
// minting a second one beside it — same identity rule as docs/adr/011.
func TestAmendmentAttestation_MatchesAnExistingDocumentByName(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "amatt7", "member")
	nodeID := proposalsElsewhereNode(t, db, admin.ID, "Am Seven", "am-seven")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	docID := seedDoc(t, db, nodeID, admin.ID, "Bylaws", "Old text", "charter")

	res := recordAdoption(t, db, "am-seven", adminToken, map[string]interface{}{
		"title": "bylaws", "decided_at": "2026-03-14", "adopted_body": "New text",
	})
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", res.Code, res.Body)
	}

	var docs int
	db.QueryRow("SELECT COUNT(*) FROM governance_docs WHERE node_id = ?", nodeID).Scan(&docs)
	if docs != 1 {
		t.Errorf("expected the existing charter to be used, got %d documents", docs)
	}
	if body, _ := docBody(t, db, docID); body != "New text" {
		t.Errorf("expected the existing charter to carry the adopted text, got %q", body)
	}
}

// A record of what happened has to say when, and a record of an adopted text
// has to carry the text.
func TestAmendmentAttestation_RequiresWhenAndWhat(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "amatt8", "member")
	nodeID := proposalsElsewhereNode(t, db, admin.ID, "Am Eight", "am-eight")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	docID := seedDoc(t, db, nodeID, admin.ID, "Bylaws", "Old text", "charter")

	for _, tc := range []struct {
		name string
		body map[string]interface{}
	}{
		{"no date", map[string]interface{}{"doc_id": docID, "adopted_body": "New text"}},
		{"no text", map[string]interface{}{"doc_id": docID, "decided_at": "2026-03-14"}},
		{"no document", map[string]interface{}{"decided_at": "2026-03-14", "adopted_body": "New text"}},
	} {
		res := recordAdoption(t, db, "am-eight", adminToken, tc.body)
		if res.Code == http.StatusCreated {
			t.Errorf("%s: expected a refusal, got 201", tc.name)
		}
	}
	if body, _ := docBody(t, db, docID); body != "Old text" {
		t.Errorf("a refused record must not touch the charter, got %q", body)
	}
}

// A community correcting Patchwork's stale copy of a members-only charter
// publishes nothing (docs/adr/054).
//
// This is the load-bearing property of the federation decision, and the only
// part of it worth a test — the federation itself is one line of pre-existing
// code. An attestation is the one write path that can *create* a governance
// document, so it is also the one that could invent a public charter out of a
// meeting nobody outside the patch was at. It cannot: a document born of a
// first attestation is members-only (docs/adr/036), and the broadcast is gated
// on the charter already being public.
func TestAmendmentAttestation_MembersOnlyCharterFederatesNothing(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "amfed1", "member")
	nodeID := proposalsElsewhereNode(t, db, admin.ID, "Am Fed", "am-fed")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	// A public patch with a remote follower — everything delivery needs, so a
	// silent outbox means the gate held rather than that nothing was wired up.
	db.Exec(`UPDATE nodes SET visibility = 'public' WHERE id = ?`, nodeID)
	db.Exec(`INSERT INTO ap_followers (id, local_actor_type, local_actor_id, remote_actor_id, remote_inbox, accepted)
	         VALUES (?, 'node', ?, 'https://remote.example/ap/instance', 'https://remote.example/ap/inbox', 1)`,
		auth.NewUUIDv7(), nodeID)

	members := seedDoc(t, db, nodeID, admin.ID, "House Rules", "Old text", "charter")
	if res := recordAdoption(t, db, "am-fed", adminToken, map[string]interface{}{
		"doc_id": members, "decided_at": "2026-03-14", "adopted_body": "What the meeting adopted",
	}); res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", res.Code, res.Body)
	}

	var queued int
	db.QueryRow("SELECT COUNT(*) FROM ap_outbox_queue").Scan(&queued)
	if queued != 0 {
		t.Errorf("a members-only charter reached the wire: %d queued activities", queued)
	}

	// The other half, and the half that makes the first half mean anything: a
	// public charter *does* reach the wire through the same call. Without this
	// the test above passes for a broadcast that never happens at all — which
	// is exactly what it did when the queue write was still racing it.
	public := seedDoc(t, db, nodeID, admin.ID, "Charter", "Old text", "charter")
	db.Exec(`UPDATE governance_docs SET visibility = 'public' WHERE id = ?`, public)
	if res := recordAdoption(t, db, "am-fed", adminToken, map[string]interface{}{
		"doc_id": public, "decided_at": "2026-03-14", "adopted_body": "Adopted at the meeting",
	}); res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", res.Code, res.Body)
	}
	db.QueryRow("SELECT COUNT(*) FROM ap_outbox_queue").Scan(&queued)
	if queued != 1 {
		t.Fatalf("a public charter must federate: %d queued activities, want 1", queued)
	}

	// And a document this instance never had is born members-only, so a first
	// attestation cannot publish a text by creating it. Measured as "no new
	// activity" against the row the public charter just queued, not as zero.
	before := queued
	res := recordAdoption(t, db, "am-fed", adminToken, map[string]interface{}{
		"title": "Operating Agreement", "decided_at": "2026-03-14", "adopted_body": "Section 1.",
	})
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", res.Code, res.Body)
	}
	var visibility string
	db.QueryRow("SELECT visibility FROM governance_docs WHERE node_id = ? AND title = ?",
		nodeID, "Operating Agreement").Scan(&visibility)
	if visibility != "members" {
		t.Errorf("a charter created by attestation must be members-only, got %q", visibility)
	}
	db.QueryRow("SELECT COUNT(*) FROM ap_outbox_queue").Scan(&queued)
	if queued != before {
		t.Errorf("creating a charter by attestation reached the wire: %d queued, want %d", queued, before)
	}
}
