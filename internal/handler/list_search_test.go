package handler_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// A list a person searches asks the server for the matches (search.go). The
// workspace finder used to filter one page in the browser, so on a patch
// with more than twenty events, proposals or members a search never found
// anything past the twentieth row. These tests put the match past row 20 on
// purpose: a search that only reads page one fails them.

func itemField(t *testing.T, resp map[string]interface{}, field string) []string {
	t.Helper()
	raw, _ := resp["items"].([]interface{})
	out := make([]string, 0, len(raw))
	for _, it := range raw {
		m, _ := it.(map[string]interface{})
		s, _ := m[field].(string)
		out = append(out, s)
	}
	return out
}

func TestEventSearchFindsPastRowTwenty(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "search-ev-owner", "member")
	nodeID := createTestNode(t, db, owner.ID, "Search Venue", "search-venue", "open")
	base := time.Now().UTC().Add(-400 * 24 * time.Hour)
	for i := 0; i < 30; i++ {
		insertEvent(t, db, nodeID, owner.ID, fmt.Sprintf("Open mic %02d", i), base.Add(time.Duration(i)*24*time.Hour))
	}
	insertEvent(t, db, nodeID, owner.ID, "Jazz Brunch", base.Add(40*24*time.Hour))

	titles := listTitles(t, db, "?node_slug=search-venue&include_past=true&search=jazz")
	if len(titles) != 1 || titles[0] != "Jazz Brunch" {
		t.Fatalf("search=jazz: want [Jazz Brunch], got %v", titles)
	}

	// The term's own wildcards are literal: "%" matches only a title that
	// carries one, not every row.
	if got := listTitles(t, db, "?node_slug=search-venue&include_past=true&search=%25"); len(got) != 0 {
		t.Errorf("search=%%: want no matches, got %v", got)
	}
}

func TestEventSearchStaysInsideTheTierGate(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "search-tier-owner", "member")
	nodeID := createTestNode(t, db, owner.ID, "Tier Venue", "tier-venue", "open")
	insertEvent(t, db, nodeID, owner.ID, "Secret rehearsal", time.Now().UTC().Add(48*time.Hour), func(id string) {
		if _, err := db.Exec(`UPDATE events SET visibility = 'members' WHERE id = ?`, id); err != nil {
			t.Fatalf("tier: %v", err)
		}
	})
	if got := listTitles(t, db, "?node_slug=tier-venue&search=rehearsal"); len(got) != 0 {
		t.Errorf("an anonymous search reached a members-only event: %v", got)
	}
}

func TestProposalSearchCountsMatches(t *testing.T) {
	db := setupTestDB(t)
	admin, token := createTestUser(t, db, "search-prop-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Search Coop", "search-coop", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	for i := 0; i < 25; i++ {
		createTestProposal(t, db, nodeID, admin.ID)
	}
	target := createTestProposal(t, db, nodeID, admin.ID)
	if _, err := db.Exec(`UPDATE proposals SET title = 'Buy a kiln' WHERE id = ?`, target); err != nil {
		t.Fatal(err)
	}
	// The oldest proposal is the one past row twenty on a newest-first list.
	if _, err := db.Exec(`UPDATE proposals SET id = '00000000-0000-7000-8000-000000000001' WHERE id = ?`, target); err != nil {
		t.Fatal(err)
	}

	r := authedRequest("GET", "/api/v1/nodes/search-coop/proposals?search=KILN", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/proposals", handler.ListProposals(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	resp := decodeJSON(t, w)
	titles := itemField(t, resp, "title")
	if len(titles) != 1 || titles[0] != "Buy a kiln" {
		t.Fatalf("search=KILN: want [Buy a kiln], got %v", titles)
	}
	if total, _ := resp["total"].(float64); total != 1 {
		t.Errorf("total should count the matches, got %v", resp["total"])
	}
}

func TestMemberSearchNeverFindsAHiddenMember(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "search-mem-admin", "member")
	nodeID := createTestNode(t, db, admin.ID, "Search Band", "search-band", "open")
	openMemberList(t, db, nodeID)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	for i := 0; i < 25; i++ {
		u, _ := createTestUser(t, db, fmt.Sprintf("filler%02d", i), "member")
		createTestMembership(t, db, u.ID, nodeID, "member", "active")
	}
	shown, _ := createTestUser(t, db, "zelda-visible", "member")
	createTestMembership(t, db, shown.ID, nodeID, "member", "active")
	hidden, _ := createTestUser(t, db, "zelda-hidden", "member")
	mid := createTestMembership(t, db, hidden.ID, nodeID, "member", "active")
	if _, err := db.Exec(`UPDATE memberships SET visible = 0 WHERE id = ?`, mid); err != nil {
		t.Fatal(err)
	}

	r, _ := http.NewRequest("GET", "/api/v1/nodes/search-band/members?search=zelda", nil)
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/members", handler.ListMembers(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	names := itemField(t, decodeJSON(t, w), "username")
	if len(names) != 1 || names[0] != "zelda-visible" {
		t.Fatalf("outsider search=zelda: want [zelda-visible], got %v", names)
	}
}

func TestAdminClaimsNarrowToOnePatch(t *testing.T) {
	db := setupTestDB(t)
	_, adminToken := createTestUser(t, db, "claims-admin", "admin")
	claimant, _ := createTestUser(t, db, "claims-claimant", "member")
	mine := createTestNode(t, db, claimant.ID, "Mine", "claims-mine", "open")
	other := createTestNode(t, db, claimant.ID, "Other", "claims-other", "open")
	insertClaim := func(nodeID string) {
		if _, err := db.Exec(`INSERT INTO claim_requests (id, node_id, user_id, method, status) VALUES (?, ?, ?, 'admin', 'pending')`,
			auth.NewUUIDv7(), nodeID, claimant.ID); err != nil {
			t.Fatalf("insert claim: %v", err)
		}
	}
	insertClaim(mine)
	insertClaim(other)

	r := authedRequest("GET", "/api/v1/admin/claims?status=pending&node_id="+mine, nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/admin/claims", handler.ListClaims(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	ids := itemField(t, decodeJSON(t, w), "node_id")
	if len(ids) != 1 || ids[0] != mine {
		t.Fatalf("node_id filter: want only %s, got %v", mine, ids)
	}
}

func TestSubmissionAndReportSearch(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "sub-admin", "admin")
	for _, name := range []string{"Gallery Row", "The Selvage", "Row House"} {
		if _, err := db.Exec(`INSERT INTO nodes (id, owner_id, name, slug, description, website, address, submitted_by, node_type, visibility, membership_policy, status, follower_permissions)
			VALUES (?, ?, ?, ?, '', '', '', ?, 'leaf', 'public', 'open', 'pending_review', '{}')`,
			auth.NewUUIDv7(), admin.ID, name, auth.NewUUIDv7(), admin.ID); err != nil {
			t.Fatalf("insert submission: %v", err)
		}
	}
	r := authedRequest("GET", "/api/v1/admin/submissions?search=row", nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/admin/submissions", handler.ListSubmissions(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("submissions: %d %s", w.Code, w.Body.String())
	}
	if names := itemField(t, decodeJSON(t, w), "name"); len(names) != 2 {
		t.Errorf("submissions search=row: want 2, got %v", names)
	}

	for _, reason := range []string{"spam", "harassment"} {
		if _, err := db.Exec(`INSERT INTO content_reports (id, reporter_id, entity_type, entity_id, reason, details) VALUES (?, ?, 'user', ?, ?, '')`,
			auth.NewUUIDv7(), admin.ID, admin.ID, reason); err != nil {
			t.Fatalf("insert report: %v", err)
		}
	}
	r = authedRequest("GET", "/api/v1/admin/reports?search=spam", nil, adminToken)
	w = serveMux(t, db, "GET", "/api/v1/admin/reports", handler.ListReports(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("reports: %d %s", w.Code, w.Body.String())
	}
	if reasons := itemField(t, decodeJSON(t, w), "reason"); len(reasons) != 1 || reasons[0] != "spam" {
		t.Errorf("reports search=spam: want [spam], got %v", reasons)
	}
}
