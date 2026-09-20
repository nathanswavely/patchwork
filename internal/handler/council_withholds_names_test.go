package handler_test

import (
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// A council is the half of a patch's roster that names the people with power,
// so it answers the roster's question and not one of its own.
//
// The overview used to answer both at once: `admins_withheld: true` beside a
// seats array carrying the withheld admin's username, display name and term
// dates, to a caller with no account. A board member of another organization
// found it by signing out to check what she would be forwarding, and the
// damage was not only the names — the page told her in a sentence that it was
// not naming them, and she was using that sentence to decide what was safe to
// send.
//
// What these tests hold: the shape of the council stays public (ADR 100 put it
// there on purpose, and none of it names anybody), the names follow the same
// two rules the admin listing follows, and a withheld chair never reads as an
// empty one.

func councilFixture(t *testing.T, db *database.DB, slug string) (nodeID, holderID, memberToken string) {
	t.Helper()
	holder, _ := createTestUser(t, db, slug+"-holder", "member")
	member, memberToken := createTestUser(t, db, slug+"-member", "member")

	nodeID = createTestNode(t, db, holder.ID, "Council "+slug, slug, "open")
	createTestMembership(t, db, holder.ID, nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	seedSeatFor(t, db, nodeID, holder.ID, "2027-08-15")
	makeVacantSeat(t, db, nodeID, "2027-08-15")
	return nodeID, holder.ID, memberToken
}

func seatsAsViewer(t *testing.T, db *database.DB, slug, token string) []map[string]interface{} {
	t.Helper()
	r := authedRequest("GET", "/api/v1/nodes/"+slug+"/governance/overview", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/governance/overview", handler.GovernanceOverview(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("overview: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	raw, ok := decodeJSON(t, w)["seats"].([]interface{})
	if !ok {
		t.Fatal("overview: expected a seats array")
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, s := range raw {
		out = append(out, s.(map[string]interface{}))
	}
	return out
}

// heldSeat is the one chair in the fixture that somebody is sitting in.
func heldSeat(t *testing.T, seats []map[string]interface{}) map[string]interface{} {
	t.Helper()
	for _, s := range seats {
		if v, _ := s["vacant"].(bool); !v {
			return s
		}
	}
	t.Fatal("expected one held seat; the council read as entirely vacant")
	return nil
}

func TestCouncil_ClosedRosterWithholdsTheHoldersName(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	nodeID, _, _ := councilFixture(t, db, "closed-roster")
	setPublicMemberList(t, db, nodeID, "nobody")

	seat := heldSeat(t, overviewSeats(t, db, "closed-roster"))

	if got := seat["username"]; got != nil && got != "" {
		t.Errorf("a closed roster still named the holder: username = %v", got)
	}
	if got := seat["holder_id"]; got != nil && got != "" {
		t.Errorf("a closed roster still handed out the holder's id: %v", got)
	}
	if got := seat["display_name"]; got != handler.HiddenMemberName {
		t.Errorf("display_name = %v, want the substitute %q", got, handler.HiddenMemberName)
	}
	if withheld, _ := seat["holder_withheld"].(bool); !withheld {
		t.Error("holder_withheld was not set, so a client cannot tell this chair from a vacant one")
	}
}

// The chair is still held, and the council is still a council. Withholding a
// name must not report a seated patch as leaderless — the failure the admin
// list's own withheld flag exists to prevent, one field over.
func TestCouncil_AWithheldChairIsStillHeld(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	nodeID, _, _ := councilFixture(t, db, "still-held")
	setPublicMemberList(t, db, nodeID, "nobody")

	seats := overviewSeats(t, db, "still-held")
	if len(seats) != 2 {
		t.Fatalf("expected the council's two chairs, got %d", len(seats))
	}
	seat := heldSeat(t, seats)
	if v, _ := seat["vacant"].(bool); v {
		t.Error("a withheld holder made the chair read as vacant")
	}
	if seat["term_ends_at"] != "2027-08-15" {
		t.Errorf("the term end went with the name: %v", seat["term_ends_at"])
	}
	if seat["fill"] == nil || seat["fill"] == "" {
		t.Error("the chair lost its answer to what happens next")
	}
}

func TestCouncil_TheRoomSeesItsOwnCouncil(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	nodeID, _, memberToken := councilFixture(t, db, "the-room")
	setPublicMemberList(t, db, nodeID, "nobody")

	seat := heldSeat(t, seatsAsViewer(t, db, "the-room", memberToken))
	if seat["username"] != "the-room-holder" {
		t.Errorf("a member of the patch was not shown their own council: username = %v", seat["username"])
	}
	if withheld, _ := seat["holder_withheld"].(bool); withheld {
		t.Error("holder_withheld was set for somebody inside the room")
	}
}

func TestCouncil_AnOpenRosterStillNamesTheHolder(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	nodeID, _, _ := councilFixture(t, db, "open-roster")
	setPublicMemberList(t, db, nodeID, "everyone")

	seat := heldSeat(t, overviewSeats(t, db, "open-roster"))
	if seat["username"] != "open-roster-holder" {
		t.Errorf("an open roster stopped naming its council: username = %v", seat["username"])
	}
}

// The member's own switch subtracts on top, the one direction no patch
// setting may overrule (docs/adr/006).
func TestCouncil_AHiddenMembershipWithholdsThatHolderAnyway(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	nodeID, holderID, _ := councilFixture(t, db, "hidden-holder")
	setPublicMemberList(t, db, nodeID, "everyone")
	if _, err := db.Exec("UPDATE memberships SET visible = 0 WHERE user_id = ? AND node_id = ?",
		holderID, nodeID); err != nil {
		t.Fatalf("hide the membership: %v", err)
	}

	seat := heldSeat(t, overviewSeats(t, db, "hidden-holder"))
	if got := seat["username"]; got != nil && got != "" {
		t.Errorf("a member who hid was named by their chair: %v", got)
	}
	if withheld, _ := seat["holder_withheld"].(bool); !withheld {
		t.Error("holder_withheld was not set for a hidden membership")
	}
}

// The contradiction itself, stated as one assertion: the payload may not say
// it is withholding the admins and name one of them in the same breath.
func TestCouncil_WithheldAdminsAreNotNamedBySeats(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	nodeID, _, _ := councilFixture(t, db, "one-answer")
	setPublicMemberList(t, db, nodeID, "nobody")

	r := authedRequest("GET", "/api/v1/nodes/one-answer/governance/overview", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/governance/overview", handler.GovernanceOverview(db), r)
	body := decodeJSON(t, w)

	if withheld, _ := body["admins_withheld"].(bool); !withheld {
		t.Fatal("fixture is wrong: admins_withheld should be set on a closed roster")
	}
	_ = nodeID
	for _, s := range body["seats"].([]interface{}) {
		seat := s.(map[string]interface{})
		if u, _ := seat["username"].(string); u == "one-answer-holder" {
			t.Fatal("the overview said it was withholding the admins and named one in its seats array")
		}
	}
}
