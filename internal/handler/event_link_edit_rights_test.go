package handler_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// Edit rights on an event link (docs/adr/2026-10-19-co-hosts-read-as-
// equals-and-the-owner-can-share-the-pen.md): the owner's side grants and
// revokes them, they do anything only on a confirmed link, and a co-host
// holding them edits every field but visibility and never deletes.

func setEditRights(t *testing.T, db *database.DB, token, eventID, nodeID string, canEdit bool) (*httptest.ResponseRecorder, model.EventLink) {
	t.Helper()
	r := authedRequest("PATCH", "/api/v1/events/"+eventID+"/links/"+nodeID, map[string]any{"can_edit": canEdit}, token)
	w := serveMux(t, db, "PATCH", "/api/v1/events/{id}/links/{nodeId}", handler.SetEventLinkEditRights(db), r)
	var l model.EventLink
	json.Unmarshal(w.Body.Bytes(), &l)
	return w, l
}

func requestLinkWithRights(t *testing.T, db *database.DB, token, eventID, target string) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("POST", "/api/v1/events/"+eventID+"/links", map[string]any{"target": target, "can_edit": true}, token)
	return serveMux(t, db, "POST", "/api/v1/events/{id}/links", handler.CreateEventLink(db, linksCfg()), r)
}

func editEvent(t *testing.T, db *database.DB, token, eventID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	r := authedRequest("PATCH", "/api/v1/events/"+eventID, body, token)
	return serveMux(t, db, "PATCH", "/api/v1/events/{id}", handler.UpdateEvent(db), r)
}

func eventColumn(t *testing.T, db *database.DB, eventID, col string) string {
	t.Helper()
	var v string
	db.QueryRow(`SELECT `+col+` FROM events WHERE id = ?`, eventID).Scan(&v)
	return v
}

// coHostFixture: a venue holds the event, a band is linked to it and
// confirmed. Returns the tokens of the venue's admin, the band's admin, and
// a plain member of the band.
type coHostFixture struct {
	eventID, venueID, bandID            string
	venueToken, bandToken, bandMemberTk string
}

func newCoHostFixture(t *testing.T, db *database.DB) coHostFixture {
	t.Helper()
	venueAdmin, venueToken := createTestUser(t, db, "venue-admin", "member")
	bandAdmin, bandToken := createTestUser(t, db, "band-admin", "member")
	bandMember, bandMemberTk := createTestUser(t, db, "band-member", "member")

	venueID := createTestNode(t, db, venueAdmin.ID, "American Bar", "american-bar", "open")
	createTestMembership(t, db, venueAdmin.ID, venueID, "admin", "active")
	bandID := createTestNode(t, db, bandAdmin.ID, "Bad Hints", "bad-hints", "invite_only")
	createTestMembership(t, db, bandAdmin.ID, bandID, "admin", "active")
	createTestMembership(t, db, bandMember.ID, bandID, "member", "active")

	eventID := insertActiveEvent(t, db, venueID, venueAdmin.ID, "Album Release")
	if w, _ := requestLink(t, db, venueToken, eventID, "bad-hints", ""); w.Code != 201 {
		t.Fatalf("request link: code=%d body=%s", w.Code, w.Body.String())
	}
	if w := confirmLink(t, db, bandToken, eventID, bandID, ""); w.Code != 200 {
		t.Fatalf("confirm link: code=%d body=%s", w.Code, w.Body.String())
	}
	return coHostFixture{eventID, venueID, bandID, venueToken, bandToken, bandMemberTk}
}

func TestEditRightsAreTheOwnersToGrant(t *testing.T) {
	db := setupTestDB(t)
	f := newCoHostFixture(t, db)

	// A plain link edits nothing.
	if w := editEvent(t, db, f.bandToken, f.eventID, map[string]any{"title": "Typo fixed"}); w.Code != 403 {
		t.Fatalf("co-host without rights edited: code=%d, want 403", w.Code)
	}

	// The co-host cannot hand the rights to itself.
	if w, _ := setEditRights(t, db, f.bandToken, f.eventID, f.bandID, true); w.Code != 403 {
		t.Fatalf("co-host granted itself edit rights: code=%d, want 403", w.Code)
	}

	// The owner's side grants them.
	w, l := setEditRights(t, db, f.venueToken, f.eventID, f.bandID, true)
	if w.Code != 200 || !l.CanEdit {
		t.Fatalf("owner grant: code=%d can_edit=%v body=%s", w.Code, l.CanEdit, w.Body.String())
	}
	if w := editEvent(t, db, f.bandToken, f.eventID, map[string]any{"title": "Typo fixed"}); w.Code != 200 {
		t.Fatalf("co-host with rights: code=%d body=%s", w.Code, w.Body.String())
	}
	if got := eventColumn(t, db, f.eventID, "title"); got != "Typo fixed" {
		t.Fatalf("title=%q, want the co-host's edit", got)
	}

	// And takes them back.
	if w, _ := setEditRights(t, db, f.venueToken, f.eventID, f.bandID, false); w.Code != 200 {
		t.Fatalf("owner revoke: code=%d", w.Code)
	}
	if w := editEvent(t, db, f.bandToken, f.eventID, map[string]any{"title": "Again"}); w.Code != 403 {
		t.Fatalf("revoked co-host edited: code=%d, want 403", w.Code)
	}
}

// The rights reach whoever speaks for the linked patch, which is its
// admins: a member of the band is not speaking for it.
func TestEditRightsReachTheCoHostsAdminsOnly(t *testing.T) {
	db := setupTestDB(t)
	f := newCoHostFixture(t, db)
	setEditRights(t, db, f.venueToken, f.eventID, f.bandID, true)

	if w := editEvent(t, db, f.bandMemberTk, f.eventID, map[string]any{"title": "Member edit"}); w.Code != 403 {
		t.Fatalf("band member edited through the band's rights: code=%d, want 403", w.Code)
	}
}

func TestCoHostEditsEverythingButVisibility(t *testing.T) {
	db := setupTestDB(t)
	f := newCoHostFixture(t, db)
	setEditRights(t, db, f.venueToken, f.eventID, f.bandID, true)

	// The form spreads every field it loaded, visibility included: sending
	// the stored value back is no change and must pass.
	w := editEvent(t, db, f.bandToken, f.eventID, map[string]any{
		"title": "Doors at 7", "location": "American Bar and Grill", "visibility": "public",
	})
	if w.Code != 200 {
		t.Fatalf("co-host edit with unchanged visibility: code=%d body=%s", w.Code, w.Body.String())
	}

	// Changing who the event is for is the owner's call.
	if w := editEvent(t, db, f.bandToken, f.eventID, map[string]any{"visibility": "members"}); w.Code != 403 {
		t.Fatalf("co-host changed visibility: code=%d, want 403", w.Code)
	}
	if got := eventColumn(t, db, f.eventID, "visibility"); got != "public" {
		t.Fatalf("visibility=%q after a refused change, want public", got)
	}
	// And a refused field refuses the whole edit, rather than saving the rest.
	if w := editEvent(t, db, f.bandToken, f.eventID, map[string]any{"title": "Sneaky", "visibility": "followers"}); w.Code != 403 {
		t.Fatalf("mixed edit: code=%d, want 403", w.Code)
	}
	if got := eventColumn(t, db, f.eventID, "title"); got != "Doors at 7" {
		t.Fatalf("title=%q, want the refused edit to have saved nothing", got)
	}

	// The owner still sets it.
	if w := editEvent(t, db, f.venueToken, f.eventID, map[string]any{"visibility": "members"}); w.Code != 200 {
		t.Fatalf("owner changed visibility: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCoHostNeverDeletes(t *testing.T) {
	db := setupTestDB(t)
	f := newCoHostFixture(t, db)
	setEditRights(t, db, f.venueToken, f.eventID, f.bandID, true)

	r := authedRequest("DELETE", "/api/v1/events/"+f.eventID, nil, f.bandToken)
	if w := serveMux(t, db, "DELETE", "/api/v1/events/{id}", handler.DeleteEvent(db), r); w.Code != 403 {
		t.Fatalf("co-host deleted the event: code=%d, want 403", w.Code)
	}
	if got := eventColumn(t, db, f.eventID, "title"); got == "" {
		t.Fatalf("event is gone")
	}
}

// A grant made in the link request waits for the other side's consent like
// the link does: it does nothing while pending.
func TestEditRightsWaitForConfirmation(t *testing.T) {
	db := setupTestDB(t)
	venueAdmin, venueToken := createTestUser(t, db, "venue-admin", "member")
	bandAdmin, bandToken := createTestUser(t, db, "band-admin", "member")
	venueID := createTestNode(t, db, venueAdmin.ID, "American Bar", "american-bar", "open")
	createTestMembership(t, db, venueAdmin.ID, venueID, "admin", "active")
	bandID := createTestNode(t, db, bandAdmin.ID, "Bad Hints", "bad-hints", "invite_only")
	createTestMembership(t, db, bandAdmin.ID, bandID, "admin", "active")
	eventID := insertActiveEvent(t, db, venueID, venueAdmin.ID, "Album Release")

	if w := requestLinkWithRights(t, db, venueToken, eventID, "bad-hints"); w.Code != 201 {
		t.Fatalf("owner request with rights: code=%d body=%s", w.Code, w.Body.String())
	}
	if w := editEvent(t, db, bandToken, eventID, map[string]any{"title": "Early"}); w.Code != 403 {
		t.Fatalf("pending co-host edited: code=%d, want 403", w.Code)
	}
	confirmLink(t, db, bandToken, eventID, bandID, "")
	if w := editEvent(t, db, bandToken, eventID, map[string]any{"title": "Now"}); w.Code != 200 {
		t.Fatalf("confirmed co-host with rights: code=%d body=%s", w.Code, w.Body.String())
	}

	// Removing the link takes the rights with it.
	r := authedRequest("DELETE", "/api/v1/events/"+eventID+"/links/"+bandID, nil, bandToken)
	serveMux(t, db, "DELETE", "/api/v1/events/{id}/links/{nodeId}", handler.RemoveEventLink(db), r)
	if w := editEvent(t, db, bandToken, eventID, map[string]any{"title": "After"}); w.Code != 403 {
		t.Fatalf("removed co-host edited: code=%d, want 403", w.Code)
	}
}

// The linked side cannot ask for edit rights in its own request. Refused
// rather than quietly downgraded to a plain link.
func TestLinkedSideCannotRequestEditRights(t *testing.T) {
	db := setupTestDB(t)
	venueAdmin, _ := createTestUser(t, db, "venue-admin", "member")
	bandAdmin, bandToken := createTestUser(t, db, "band-admin", "member")
	venueID := createTestNode(t, db, venueAdmin.ID, "American Bar", "american-bar", "open")
	createTestMembership(t, db, venueAdmin.ID, venueID, "admin", "active")
	bandID := createTestNode(t, db, bandAdmin.ID, "Bad Hints", "bad-hints", "invite_only")
	createTestMembership(t, db, bandAdmin.ID, bandID, "admin", "active")
	eventID := insertActiveEvent(t, db, venueID, venueAdmin.ID, "Album Release")

	if w := requestLinkWithRights(t, db, bandToken, eventID, "bad-hints"); w.Code != 403 {
		t.Fatalf("linked side asked for edit rights: code=%d, want 403", w.Code)
	}
	if s := linkStatusInDB(t, db, eventID, bandID); s != "" {
		t.Fatalf("a refused request left a link behind: status=%q", s)
	}
}

// Rights do not pass one hop at a time: a co-host holding them cannot
// grant them to another co-host.
func TestEditRightsAreNotTransitive(t *testing.T) {
	db := setupTestDB(t)
	f := newCoHostFixture(t, db)
	setEditRights(t, db, f.venueToken, f.eventID, f.bandID, true)

	otherAdmin, otherToken := createTestUser(t, db, "other-admin", "member")
	otherID := createTestNode(t, db, otherAdmin.ID, "Nick Cove", "nick-cove", "invite_only")
	createTestMembership(t, db, otherAdmin.ID, otherID, "admin", "active")
	if w, _ := requestLink(t, db, f.venueToken, f.eventID, "nick-cove", ""); w.Code != 201 {
		t.Fatalf("second link: code=%d", w.Code)
	}
	confirmLink(t, db, otherToken, f.eventID, otherID, "")

	if w, _ := setEditRights(t, db, f.bandToken, f.eventID, otherID, true); w.Code != 403 {
		t.Fatalf("co-host granted rights onward: code=%d, want 403", w.Code)
	}
}

// An imported event stays read-only to a co-host as it is to everyone,
// because the next sync would undo the edit (docs/adr/031).
func TestCoHostCannotEditImportedEvent(t *testing.T) {
	db := setupTestDB(t)
	f := newCoHostFixture(t, db)
	setEditRights(t, db, f.venueToken, f.eventID, f.bandID, true)

	sourceID := auth.NewUUIDv7()
	var owner string
	db.QueryRow(`SELECT created_by FROM events WHERE id = ?`, f.eventID).Scan(&owner)
	if _, err := db.Exec(
		`INSERT INTO event_sources (id, node_id, type, url, added_by) VALUES (?, ?, 'ics', 'https://cal.example/a.ics', ?)`,
		sourceID, f.venueID, owner); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	db.Exec(`UPDATE events SET source_id = ?, source_uid = 'uid-1' WHERE id = ?`, sourceID, f.eventID)

	if w := editEvent(t, db, f.bandToken, f.eventID, map[string]any{"title": "Overwritten next hour"}); w.Code != 403 {
		t.Fatalf("co-host edited an imported event: code=%d, want 403", w.Code)
	}
}

// Who may edit is back-office: stated to the two sides of the link, read
// as false by everyone else, and the event says which door this viewer
// would edit through.
func TestEditRightsAreStatedOnlyToTheTwoSides(t *testing.T) {
	db := setupTestDB(t)
	f := newCoHostFixture(t, db)
	setEditRights(t, db, f.venueToken, f.eventID, f.bandID, true)
	_, strangerToken := createTestUser(t, db, "stranger", "member")

	linkCanEdit := func(ev map[string]any) bool {
		links, _ := ev["links"].([]any)
		if len(links) != 1 {
			t.Fatalf("links=%v, want one", ev["links"])
		}
		v, _ := links[0].(map[string]any)["can_edit"].(bool)
		return v
	}

	for _, c := range []struct {
		name, token      string
		canEdit, coEdits bool
	}{
		{"anonymous", "", false, false},
		{"stranger", strangerToken, false, false},
		{"band member", f.bandMemberTk, false, false},
		{"owner admin", f.venueToken, true, false},
		{"co-host admin", f.bandToken, true, true},
	} {
		ev := getEventPublic(t, db, f.eventID, c.token)
		if got := linkCanEdit(ev); got != c.canEdit {
			t.Errorf("%s: link can_edit=%v, want %v", c.name, got, c.canEdit)
		}
		if got, _ := ev["viewer_co_edits"].(bool); got != c.coEdits {
			t.Errorf("%s: viewer_co_edits=%v, want %v", c.name, got, c.coEdits)
		}
	}
}
