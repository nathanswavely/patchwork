package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// An interim admin is asked first
// (docs/adr/2026-10-19-an-interim-admin-is-asked-first.md). The answer is the
// person's own: only they can give it, and anybody else is told nothing about
// whether anyone was asked.
func TestInterimOffer_OnlyThePersonAskedCanAnswer(t *testing.T) {
	db := setupTestDB(t)
	founder, _ := createTestUser(t, db, "interim-founder", "member")
	nodeID := createTestNode(t, db, founder.ID, "Interim", "interim-patch", "open")
	asked, askedToken := createTestUser(t, db, "interim-asked", "member")
	createTestMembership(t, db, asked.ID, nodeID, "member", "active")
	other, otherToken := createTestUser(t, db, "interim-other", "member")
	createTestMembership(t, db, other.ID, nodeID, "member", "active")
	stranger, strangerToken := createTestUser(t, db, "interim-stranger", "member")
	_ = stranger

	if _, err := db.Exec(`INSERT INTO interim_admin_offers (id, node_id, user_id) VALUES (?, ?, ?)`,
		auth.NewUUIDv7(), nodeID, asked.ID); err != nil {
		t.Fatalf("seed offer: %v", err)
	}

	answer := func(verb, token string) (int, string) {
		r := authedRequest("POST", "/api/v1/nodes/interim-patch/interim-offer/"+verb, nil, token)
		var h http.HandlerFunc = handler.AcceptInterimOffer(db)
		if verb == "decline" {
			h = handler.DeclineInterimOffer(db)
		}
		w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/interim-offer/"+verb, h, r)
		return w.Code, w.Body.String()
	}

	// A member nobody asked, and a stranger, get the same 404 an unknown
	// patch gets: the route does not say who was asked.
	codeOther, bodyOther := answer("accept", otherToken)
	codeStranger, bodyStranger := answer("accept", strangerToken)
	r := authedRequest("POST", "/api/v1/nodes/no-such-patch/interim-offer/accept", nil, strangerToken)
	wNone := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/interim-offer/accept", handler.AcceptInterimOffer(db), r)
	for _, got := range []struct {
		who  string
		code int
		body string
	}{{"another member", codeOther, bodyOther}, {"a stranger", codeStranger, bodyStranger}, {"an unknown patch", wNone.Code, wNone.Body.String()}} {
		if got.code != http.StatusNotFound {
			t.Errorf("%s: expected 404, got %d", got.who, got.code)
		}
		if got.body != bodyStranger {
			t.Errorf("%s: every refusal must read the same, got %q vs %q", got.who, got.body, bodyStranger)
		}
	}
	var role string
	if err := db.QueryRow(`SELECT role FROM memberships WHERE node_id = ? AND user_id = ?`, nodeID, other.ID).Scan(&role); err != nil {
		t.Fatalf("read: %v", err)
	}
	if role != "member" {
		t.Errorf("somebody else's answer must change nothing, got %q", role)
	}

	// Their own list shows the open question; nobody else's does.
	list := func(token string) int {
		r := authedRequest("GET", "/api/v1/users/me/interim-offers", nil, token)
		w := serveMux(t, db, "GET", "/api/v1/users/me/interim-offers", handler.ListMyInterimOffers(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("list: %d %s", w.Code, w.Body.String())
		}
		items, _ := decodeJSON(t, w)["items"].([]interface{})
		return len(items)
	}
	if got := list(askedToken); got != 1 {
		t.Errorf("the person asked should see their question, got %d", got)
	}
	if got := list(otherToken); got != 0 {
		t.Errorf("nobody else sees it, got %d", got)
	}

	// The hub says questions are out, so it does not tell the patch only an
	// instance admin can help; it says nothing about who was asked.
	overviewOpen := func() interface{} {
		r := authedRequest("GET", "/api/v1/nodes/interim-patch/governance/overview", nil, otherToken)
		w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/governance/overview", handler.GovernanceOverview(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("overview: %d %s", w.Code, w.Body.String())
		}
		return decodeJSON(t, w)["interim_offers_open"]
	}
	if got := overviewOpen(); got != true {
		t.Errorf("an open question should show on the hub, got %v", got)
	}

	// The person asked says yes, and is an admin.
	if code, body := answer("accept", askedToken); code != http.StatusOK {
		t.Fatalf("accept: %d %s", code, body)
	}
	if err := db.QueryRow(`SELECT role FROM memberships WHERE node_id = ? AND user_id = ?`, nodeID, asked.ID).Scan(&role); err != nil {
		t.Fatalf("read: %v", err)
	}
	if role != "admin" {
		t.Errorf("accepting should make them an admin, got %q", role)
	}
	if got := list(askedToken); got != 0 {
		t.Errorf("an answered question leaves the list, got %d", got)
	}
	if got := overviewOpen(); got != false {
		t.Errorf("an answered question is no longer out, got %v", got)
	}
}
