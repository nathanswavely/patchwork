package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Where a client learns that a per-patch grant reaches it
// (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar.md,
// decision 2). The signed-in user's `trusted_contributor` flag only ever
// means quilt-wide, so a client reading nothing else shows a person trusted
// on one patch no link control on that patch's own events (docs/adr/057).
// Two payloads answer it: the event carries `viewer_trusted` for its own
// patch, and `auth/me` carries `trusted_patches`, the reach a client cannot
// enumerate from memberships because an unclaimed patch has none.

// The event payload states whether the viewer's grant reaches the event's
// own patch — at either scope, and never on an active patch.
func TestEventPayloadViewerTrusted(t *testing.T) {
	db := setupTestDB(t)
	siteAdmin, _ := createTestUser(t, db, "site-admin", "admin")
	owner, _ := createTestUser(t, db, "owner", "member")
	scoped, scopedToken := createTestUser(t, db, "scoped", "member")
	wide, wideToken := createTestUser(t, db, "wide", "member")
	stranger, strangerToken := createTestUser(t, db, "stranger", "member")
	_ = stranger
	makeTrusted(t, db, wide.ID)

	granted := createTestNode(t, db, siteAdmin.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, granted)
	other := createTestNode(t, db, siteAdmin.ID, "Selvage", "selvage", "open")
	makeUnclaimed(t, db, other)
	active := createTestNode(t, db, owner.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, owner.ID, active, "admin", "active")

	grantOnPatch(t, db, scoped.ID, granted, siteAdmin.ID)
	// A grant row pointing at a claimed patch decides nothing.
	grantOnPatch(t, db, scoped.ID, active, siteAdmin.ID)

	onGranted := insertActiveEvent(t, db, granted, siteAdmin.ID, "Zine Fair")
	onOther := insertActiveEvent(t, db, other, siteAdmin.ID, "Basement Show")
	onActive := insertActiveEvent(t, db, active, owner.ID, "Opening")

	cases := []struct {
		name    string
		eventID string
		token   string
		want    bool
	}{
		{"per-patch grant on its own patch", onGranted, scopedToken, true},
		{"per-patch grant on another unclaimed patch", onOther, scopedToken, false},
		{"per-patch grant row on an active patch", onActive, scopedToken, false},
		{"quilt-wide flag on any unclaimed patch", onOther, wideToken, true},
		{"quilt-wide flag on an active patch", onActive, wideToken, false},
		{"no grant at all", onGranted, strangerToken, false},
		{"anonymous", onGranted, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := getEventPublic(t, db, c.eventID, c.token)
			got, present := out["viewer_trusted"]
			if !present {
				t.Fatalf("viewer_trusted absent from event payload: %v", out)
			}
			if got != c.want {
				t.Fatalf("viewer_trusted = %v, want %v", got, c.want)
			}
		})
	}
}

// A link row names its patch's status, so the client can decide the linked
// side of the handshake by the rule the server uses (docs/adr/057): the
// person trusted on the unclaimed linked patch is the one who confirms an
// owner-initiated request, and there is no admin of an unclaimed patch to
// offer the button to instead.
func TestEventLinkRowsCarryNodeStatus(t *testing.T) {
	db := setupTestDB(t)
	siteAdmin, _ := createTestUser(t, db, "site-admin", "admin")
	owner, ownerToken := createTestUser(t, db, "owner", "member")
	scoped, scopedToken := createTestUser(t, db, "scoped", "member")

	venue := createTestNode(t, db, owner.ID, "The Selvage", "the-selvage", "open")
	createTestMembership(t, db, owner.ID, venue, "admin", "active")
	listing := createTestNode(t, db, siteAdmin.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, listing)
	grantOnPatch(t, db, scoped.ID, listing, siteAdmin.ID)

	eventID := insertActiveEvent(t, db, venue, owner.ID, "Selvage Show")
	// The venue's admin requests the link from the owner side; it waits on
	// the listing's side, which is the per-patch trusted person. (An instance
	// admin would speak for both sides and confirm it in one step.)
	if w, l := requestLink(t, db, ownerToken, eventID, "spark-hall", ""); w.Code != http.StatusCreated || l.Status != "pending" {
		t.Fatalf("owner-initiated request: code=%d status=%q body=%s", w.Code, l.Status, w.Body.String())
	}

	links, _ := getEventPublic(t, db, eventID, scopedToken)["links"].([]interface{})
	if len(links) != 1 {
		t.Fatalf("pending link invisible to the person who can confirm it: %v", links)
	}
	row := links[0].(map[string]interface{})
	if row["node_status"] != "unclaimed" || row["initiated_by"] != "owner" {
		t.Fatalf("link row = %v, want node_status unclaimed and initiated_by owner", row)
	}
}

// auth/me lists the patches a per-patch grant reaches, filtered to the ones
// still unclaimed, so a client can offer them as things this person may
// speak for from any event page — and never one whose claim has already
// ended the grant.
func TestMeListsTrustedPatches(t *testing.T) {
	db := setupTestDB(t)
	siteAdmin, _ := createTestUser(t, db, "site-admin", "admin")
	scoped, scopedToken := createTestUser(t, db, "scoped", "member")
	plain, plainToken := createTestUser(t, db, "plain", "member")
	_ = plain

	granted := createTestNode(t, db, siteAdmin.ID, "Spark Hall", "spark-hall", "open")
	makeUnclaimed(t, db, granted)
	claimed := createTestNode(t, db, siteAdmin.ID, "Gallery Row", "gallery-row", "open")
	grantOnPatch(t, db, scoped.ID, granted, siteAdmin.ID)
	grantOnPatch(t, db, scoped.ID, claimed, siteAdmin.ID)

	r := authedRequest("GET", "/api/v1/auth/me", nil, scopedToken)
	w := serveMux(t, db, "GET", "/api/v1/auth/me", handler.Me(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("me: %d %s", w.Code, w.Body.String())
	}
	patches, _ := decodeJSON(t, w)["trusted_patches"].([]interface{})
	if len(patches) != 1 {
		t.Fatalf("trusted_patches = %v, want exactly the unclaimed one", patches)
	}
	p := patches[0].(map[string]interface{})
	if p["slug"] != "spark-hall" || p["name"] != "Spark Hall" || p["id"] != granted {
		t.Fatalf("trusted patch = %v, want spark-hall", p)
	}
	for _, k := range []string{"role", "status"} {
		if _, has := p[k]; has {
			t.Fatalf("trusted patch carries %q — a grant is not a membership", k)
		}
	}

	r = authedRequest("GET", "/api/v1/auth/me", nil, plainToken)
	w = serveMux(t, db, "GET", "/api/v1/auth/me", handler.Me(db), r)
	if _, has := decodeJSON(t, w)["trusted_patches"]; has {
		t.Fatalf("trusted_patches present for a person holding no grant: %s", w.Body.String())
	}
}
