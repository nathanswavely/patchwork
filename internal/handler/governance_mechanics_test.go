package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// The governance-mechanics decisions of 2026-10-20, one block per finding.

// ---- F-042: a member can stand for a vacant seat ---------------------------

func standForSeat(t *testing.T, db *database.DB, slug, userID, token string) (int, string) {
	t.Helper()
	body := map[string]interface{}{
		"title": "Standing for the council", "proposal_type": "membership", "target_user_id": userID,
	}
	r := authedRequest("POST", "/api/v1/nodes/"+slug+"/proposals", body, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/proposals", handler.CreateProposal(db), r)
	return w.Code, w.Body.String()
}

// A member names themselves, the members ratify it, and the chair is theirs
// with the term it already carried. Between contests this is the door that
// was missing (docs/adr/2026-11-11-a-member-can-stand-for-a-vacant-seat.md).
func TestStanding_MemberStandsAndIsRatifiedIntoTheChair(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "standadmin1", "member")
	nodeID := electedNode(t, db, admin.ID, "Stand One", "stand-one", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	sam, samToken := createTestUser(t, db, "standsam1", "member")
	createTestMembership(t, db, sam.ID, nodeID, "member", "active")
	seat := makeVacantSeat(t, db, nodeID, "2027-08-15")

	code, body := standForSeat(t, db, "stand-one", sam.ID, samToken)
	if code != http.StatusCreated {
		t.Fatalf("a member standing for a vacant seat: expected 201, got %d: %s", code, body)
	}
	var proposalID string
	if err := json.Unmarshal([]byte(body), &struct {
		ID *string `json:"id"`
	}{&proposalID}); err != nil || proposalID == "" {
		t.Fatalf("no proposal id in %s", body)
	}
	// Standing grants nothing by itself.
	if got := roleOf(t, db, sam.ID, nodeID); got != "member" {
		t.Fatalf("standing must not promote, got %q", got)
	}

	vr := authedRequest("POST", "/api/v1/proposals/"+proposalID+"/vote",
		map[string]interface{}{"value": "approve"}, adminToken)
	vw := serveMux(t, db, "POST", "/api/v1/proposals/{id}/vote", handler.VoteOnProposal(db), vr)
	if vw.Code != http.StatusOK && vw.Code != http.StatusCreated {
		t.Fatalf("vote: got %d: %s", vw.Code, vw.Body.String())
	}
	expireProposal(t, db, proposalID)
	gr := authedRequest("GET", "/api/v1/proposals/"+proposalID, nil, adminToken)
	serveMux(t, db, "GET", "/api/v1/proposals/{id}", handler.GetProposal(db), gr)

	if got := roleOf(t, db, sam.ID, nodeID); got != "admin" {
		t.Fatalf("a ratified candidacy should seat them, got role %q", got)
	}
	holder, termEnds := seatRow(t, db, seat)
	if holder != sam.ID {
		t.Errorf("expected the chair to be theirs, holder = %q", holder)
	}
	if termEnds != "2027-08-15" {
		t.Errorf("they serve out the chair's term, not a fresh one: got %q", termEnds)
	}
}

// The conditions, each refused before anything is written.
func TestStanding_Conditions(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "standadmin2", "member")
	nodeID := electedNode(t, db, admin.ID, "Stand Two", "stand-two", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	member, memberToken := createTestUser(t, db, "standmember2", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")
	other, _ := createTestUser(t, db, "standother2", "member")
	createTestMembership(t, db, other.ID, nodeID, "member", "active")
	follower, followerToken := createTestUser(t, db, "standfollower2", "member")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")

	// No vacant chair: the date of the next contest, not a vote that could
	// only end unseatable.
	if code, body := standForSeat(t, db, "stand-two", member.ID, memberToken); code != http.StatusConflict {
		t.Errorf("standing with no vacant seat: expected 409, got %d: %s", code, body)
	}

	makeVacantSeat(t, db, nodeID, "2027-08-15")

	// A follower is not on the ladder.
	if code, body := standForSeat(t, db, "stand-two", follower.ID, followerToken); code == http.StatusCreated {
		t.Errorf("a follower standing: expected a refusal, got 201: %s", body)
	}
	// Standing is for yourself. Putting somebody else forward is still a
	// nomination, and an admin raises those.
	if code, body := standForSeat(t, db, "stand-two", other.ID, memberToken); code != http.StatusConflict ||
		!strings.Contains(body, "only an admin") {
		t.Errorf("a member nominating someone else: expected 409 naming admins, got %d: %s", code, body)
	}

	var n int
	_ = db.QueryRow("SELECT COUNT(*) FROM proposals WHERE node_id = ?", nodeID).Scan(&n)
	if n != 0 {
		t.Errorf("refused candidacies must leave no record, found %d proposals", n)
	}
}

// A council with nobody on it is filled by its contest (docs/adr/108), where
// any member may already stand. A standing proposal racing that contest for
// the same chair would seat somebody the ballot then unseats.
func TestStanding_RefusedOnAnEmptyCouncil(t *testing.T) {
	db := setupTestDB(t)
	founder, _ := createTestUser(t, db, "standfounder3", "member")
	nodeID := electedNode(t, db, founder.ID, "Stand Three", "stand-three", 0, 12)
	createTestMembership(t, db, founder.ID, nodeID, "member", "active")
	makeVacantSeat(t, db, nodeID, "2027-08-15")
	member, memberToken := createTestUser(t, db, "standmember3", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	code, body := standForSeat(t, db, "stand-three", member.ID, memberToken)
	if code != http.StatusConflict || !strings.Contains(body, "next election") {
		t.Errorf("standing on a council with no admins: expected 409 naming the election, got %d: %s", code, body)
	}
}

// ---- F-057: a member can file the minutes -----------------------------------

func createDoc(t *testing.T, db *database.DB, slug, token string, body map[string]interface{}) (int, string) {
	t.Helper()
	r := authedRequest("POST", "/api/v1/nodes/"+slug+"/governance", body, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/governance", handler.CreateGovernanceDoc(db), r)
	return w.Code, w.Body.String()
}

func TestMemberFilesMinutes_OnAPatchThatMeetsElsewhere(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "minutesadmin1", "member")
	nodeID := elsewhereNode(t, db, admin.ID, "Minutes One", "minutes-one")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	harriet, harrietToken := createTestUser(t, db, "minutesharriet1", "member")
	createTestMembership(t, db, harriet.ID, nodeID, "member", "active")
	follower, followerToken := createTestUser(t, db, "minutesfollower1", "member")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")

	code, body := createDoc(t, db, "minutes-one", harrietToken, map[string]interface{}{
		"title": "Minutes of 28 September", "body": "Present: ...",
	})
	if code != http.StatusCreated {
		t.Fatalf("a member filing minutes: expected 201, got %d: %s", code, body)
	}
	var visibility, createdBy string
	_ = db.QueryRow(`SELECT visibility, created_by FROM governance_docs WHERE node_id = ? AND title = 'Minutes of 28 September'`, nodeID).
		Scan(&visibility, &createdBy)
	if visibility != "members" || createdBy != harriet.ID {
		t.Errorf("expected a members-only doc by the member, got visibility=%q created_by=%q", visibility, createdBy)
	}

	// Publishing stays an admin's decision.
	if code, body := createDoc(t, db, "minutes-one", harrietToken, map[string]interface{}{
		"title": "Minutes of 5 October", "visibility": "public",
	}); code != http.StatusForbidden {
		t.Errorf("a member publishing: expected 403, got %d: %s", code, body)
	}
	// A follower is not in the room that keeps the minutes.
	if code, body := createDoc(t, db, "minutes-one", followerToken, map[string]interface{}{
		"title": "Minutes of 12 October",
	}); code != http.StatusForbidden {
		t.Errorf("a follower filing: expected 403, got %d: %s", code, body)
	}
}

// Where everything is decided in Patchwork the shelf is still the admins'.
func TestMemberFilesMinutes_NotWhereEverythingIsDecidedHere(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "minutesadmin2", "member")
	nodeID := electedNode(t, db, admin.ID, "Minutes Two", "minutes-two", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	member, memberToken := createTestUser(t, db, "minutesmember2", "member")
	createTestMembership(t, db, member.ID, nodeID, "member", "active")

	if code, body := createDoc(t, db, "minutes-two", memberToken, map[string]interface{}{
		"title": "Minutes",
	}); code != http.StatusForbidden {
		t.Errorf("a member creating a doc where nothing is decided elsewhere: expected 403, got %d: %s", code, body)
	}
}

// ---- F-066: the council changing size is recorded, not announced ------------

type recordItem struct {
	Kind           string   `json:"kind"`
	Title          string   `json:"title"`
	Outcome        string   `json:"outcome"`
	Actor          string   `json:"actor"`
	Seats          *int     `json:"seats"`
	Names          []string `json:"names"`
	StoodDown      []string `json:"stood_down"`
	EnteredInError bool     `json:"entered_in_error"`
}

func readRecordItems(t *testing.T, db *database.DB, slug, token string) []recordItem {
	t.Helper()
	r := authedRequest("GET", "/api/v1/nodes/"+slug+"/governance/record", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/governance/record", handler.GovernanceRecord(db), r)
	var out struct {
		Items []recordItem `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("record: %v: %s", err, w.Body.String())
	}
	return out.Items
}

func TestCouncilSize_RecordedAndRingsNoBell(t *testing.T) {
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })
	admin, adminToken := createTestUser(t, db, "sizeadmin1", "member")
	nodeID := electedNode(t, db, admin.ID, "Size One", "size-one", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	ana, anaToken := createTestUser(t, db, "sizeana1", "member")
	createTestMembership(t, db, ana.ID, nodeID, "member", "active")

	r := authedRequest("POST", "/api/v1/nodes/size-one/seats", nil, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/seats", handler.AddSeat(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("add seat: %d %s", w.Code, w.Body.String())
	}
	seatID := decodeJSON(t, w)["id"].(string)
	dr := authedRequest("DELETE", "/api/v1/nodes/size-one/seats/"+seatID, nil, adminToken)
	dw := serveMux(t, db, "DELETE", "/api/v1/nodes/{slug}/seats/{id}", handler.RemoveSeat(db), dr)
	if dw.Code != http.StatusNoContent {
		t.Fatalf("remove seat: %d %s", dw.Code, dw.Body.String())
	}

	// Furniture carries no obligation, so nobody's bell rings for it.
	if n := countNotifications(t, db, ana.ID, notifications.GovernanceRulesChanged, 0); n != 0 {
		t.Errorf("adding and removing a seat rang a member's bell %d times", n)
	}

	// And the Record, which is where a member goes looking, has both.
	var added, removed *recordItem
	items := readRecordItems(t, db, "size-one", anaToken)
	for i := range items {
		switch items[i].Outcome {
		case "seat_added":
			added = &items[i]
		case "seat_removed":
			removed = &items[i]
		}
	}
	if added == nil || removed == nil {
		t.Fatalf("expected a seat added and a seat removed on the Record, got %+v", items)
	}
	if added.Seats == nil || *added.Seats != 1 {
		t.Errorf("the added entry should say the council went to 1 seat, got %v", added.Seats)
	}
	if removed.Seats == nil || *removed.Seats != 0 {
		t.Errorf("the removed entry should say the council went to 0 seats, got %v", removed.Seats)
	}
	if added.Actor == "" {
		t.Errorf("inside the room the entry names who added the seat")
	}
}

// A term end moving still notifies: it changes when a member can stand.
func TestCouncilSize_TermEndStillAnnounced(t *testing.T) {
	db := setupTestDB(t)
	handler.SetNotifier(notifications.NewNotifier(db))
	t.Cleanup(func() { handler.SetNotifier(nil) })
	admin, adminToken := createTestUser(t, db, "sizeadmin2", "member")
	nodeID := electedNode(t, db, admin.ID, "Size Two", "size-two", 0, 12)
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	ana, _ := createTestUser(t, db, "sizeana2", "member")
	createTestMembership(t, db, ana.ID, nodeID, "member", "active")
	seat := makeVacantSeat(t, db, nodeID, "2027-08-15")

	r := authedRequest("PATCH", "/api/v1/nodes/size-two/seats/"+seat,
		map[string]interface{}{"term_ends_at": "2027-11-01"}, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/seats/{id}", handler.SetSeatTerm(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("set term: %d %s", w.Code, w.Body.String())
	}
	if n := countNotifications(t, db, ana.ID, notifications.GovernanceRulesChanged, 1); n != 1 {
		t.Errorf("a term end moving should still reach the members, got %d", n)
	}
}

// ---- F-078: who stood down, and a record struck as entered in error ----------

type attestationListing struct {
	Items []struct {
		ID             string `json:"id"`
		SupersededBy   string `json:"superseded_by"`
		EnteredInError bool   `json:"entered_in_error"`
		StruckByName   string `json:"struck_by_name"`
		Names          []struct {
			DisplayName string `json:"display_name"`
		} `json:"names"`
		StoodDown []struct {
			DisplayName string `json:"display_name"`
		} `json:"stood_down"`
	} `json:"items"`
}

func listAttestations(t *testing.T, db *database.DB, slug, token string) attestationListing {
	t.Helper()
	r := authedRequest("GET", "/api/v1/nodes/"+slug+"/attestations", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/attestations", handler.ListAttestations(db), r)
	var out attestationListing
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("list: %v: %s", err, w.Body.String())
	}
	return out
}

func strike(t *testing.T, db *database.DB, slug, id, token string) (int, string) {
	t.Helper()
	r := authedRequest("POST", "/api/v1/nodes/"+slug+"/attestations/"+id+"/strike", nil, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/attestations/{id}/strike", handler.StrikeAttestation(db), r)
	return w.Code, w.Body.String()
}

func idOf(t *testing.T, res *httpRecorderResult) string {
	t.Helper()
	var out struct {
		ID string `json:"id"`
	}
	if res.Code != http.StatusCreated || json.Unmarshal([]byte(res.Body), &out) != nil || out.ID == "" {
		t.Fatalf("record: %d %s", res.Code, res.Body)
	}
	return out.ID
}

func TestAttestation_RecordsWhoStoodDown(t *testing.T) {
	db := setupTestDB(t)
	omar, omarToken := createTestUser(t, db, "stoodomar1", "member")
	nodeID := elsewhereNode(t, db, omar.ID, "Stood One", "stood-one")
	createTestMembership(t, db, omar.ID, nodeID, "admin", "active")
	clyde, _ := createTestUser(t, db, "stoodclyde1", "member")
	createTestMembership(t, db, clyde.ID, nodeID, "admin", "active")
	harriet, _ := createTestUser(t, db, "stoodharriet1", "member")
	createTestMembership(t, db, harriet.ID, nodeID, "member", "active")

	idOf(t, recordAttestation(t, db, "stood-one", omarToken, map[string]interface{}{
		"decided_at": "2026-09-28",
		"names":      []map[string]interface{}{{"user_id": omar.ID}, {"user_id": harriet.ID}},
		"stood_down": []map[string]interface{}{{"user_id": clyde.ID}, {"display_name": "Former Treasurer"}},
	}))

	got := listAttestations(t, db, "stood-one", omarToken)
	if len(got.Items) != 1 || len(got.Items[0].Names) != 2 || len(got.Items[0].StoodDown) != 2 {
		t.Fatalf("expected two seated and two stood down, got %+v", got.Items)
	}
	if got := roleOf(t, db, clyde.ID, nodeID); got != "member" {
		t.Errorf("the person the meeting stood down should no longer hold admin, got %q", got)
	}
	if got := roleOf(t, db, harriet.ID, nodeID); got != "admin" {
		t.Errorf("the person the meeting put in place should hold admin, got %q", got)
	}

	// The Record says both sides.
	items := readRecordItems(t, db, "stood-one", omarToken)
	var council *recordItem
	for i := range items {
		if items[i].Kind == "council" {
			council = &items[i]
		}
	}
	if council == nil || len(council.StoodDown) != 2 || len(council.Names) != 2 {
		t.Fatalf("expected the council entry to name both sides, got %+v", items)
	}

	// Nobody is on both sides of one decision.
	res := recordAttestation(t, db, "stood-one", omarToken, map[string]interface{}{
		"decided_at": "2026-10-05",
		"names":      []map[string]interface{}{{"user_id": omar.ID}, {"user_id": clyde.ID}},
		"stood_down": []map[string]interface{}{{"user_id": clyde.ID}},
	})
	if res.Code != http.StatusBadRequest {
		t.Errorf("one person on both sides: expected 400, got %d: %s", res.Code, res.Body)
	}
}

// Omar pressed Record twice. The duplicate is crossed out, stays readable,
// counts for nothing, and moves nobody.
func TestAttestation_StrikeADuplicate(t *testing.T) {
	db := setupTestDB(t)
	omar, omarToken := createTestUser(t, db, "strikeomar1", "member")
	nodeID := elsewhereNode(t, db, omar.ID, "Strike One", "strike-one")
	createTestMembership(t, db, omar.ID, nodeID, "admin", "active")
	harriet, harrietToken := createTestUser(t, db, "strikeharriet1", "member")
	createTestMembership(t, db, harriet.ID, nodeID, "member", "active")

	council := map[string]interface{}{
		"decided_at": "2026-09-28",
		"names":      []map[string]interface{}{{"user_id": omar.ID}, {"user_id": harriet.ID}},
	}
	idOf(t, recordAttestation(t, db, "strike-one", omarToken, council))
	dup := idOf(t, recordAttestation(t, db, "strike-one", omarToken, council))

	// A member cannot strike: it can move the admin role. Harriet is an admin
	// now, so a member without the role is needed.
	plain, plainToken := createTestUser(t, db, "strikeplain1", "member")
	createTestMembership(t, db, plain.ID, nodeID, "member", "active")
	if code, body := strike(t, db, "strike-one", dup, plainToken); code != http.StatusForbidden {
		t.Errorf("a member striking: expected 403, got %d: %s", code, body)
	}

	if code, body := strike(t, db, "strike-one", dup, harrietToken); code != http.StatusOK {
		t.Fatalf("strike: expected 200, got %d: %s", code, body)
	}
	for _, who := range []string{omar.ID, harriet.ID} {
		if got := roleOf(t, db, who, nodeID); got != "admin" {
			t.Errorf("striking a duplicate should move nobody, %s is now %q", who, got)
		}
	}

	got := listAttestations(t, db, "strike-one", omarToken)
	if len(got.Items) != 2 {
		t.Fatalf("nothing is deleted: expected both records listed, got %d", len(got.Items))
	}
	var struck int
	for _, it := range got.Items {
		if it.EnteredInError {
			struck++
			if it.ID != dup || it.StruckByName == "" {
				t.Errorf("expected the duplicate struck with who struck it, got %+v", it)
			}
		}
	}
	if struck != 1 {
		t.Errorf("expected exactly one record struck, got %d", struck)
	}

	// The Record keeps it, crossed out.
	var onRecord, crossed int
	for _, it := range readRecordItems(t, db, "strike-one", omarToken) {
		if it.Kind == "council" {
			onRecord++
			if it.EnteredInError {
				crossed++
			}
		}
	}
	if onRecord != 2 || crossed != 1 {
		t.Errorf("expected both on the Record with one crossed out, got %d and %d", onRecord, crossed)
	}

	// Struck once is struck.
	if code, _ := strike(t, db, "strike-one", dup, omarToken); code != http.StatusConflict {
		t.Errorf("striking twice: expected 409, got %d", code)
	}
	// And a struck record is not something to correct.
	res := recordAttestation(t, db, "strike-one", omarToken, map[string]interface{}{
		"decided_at": "2026-09-28", "supersedes_id": dup,
		"names": []map[string]interface{}{{"user_id": omar.ID}},
	})
	if res.Code != http.StatusConflict {
		t.Errorf("correcting a struck record: expected 409, got %d: %s", res.Code, res.Body)
	}
}

// A wrong record struck hands the patch back to the decision before it, and
// a struck correction stops correcting.
func TestAttestation_StrikeAWrongRecordRestoresTheOneBefore(t *testing.T) {
	db := setupTestDB(t)
	omar, omarToken := createTestUser(t, db, "strikeomar2", "member")
	nodeID := elsewhereNode(t, db, omar.ID, "Strike Two", "strike-two")
	createTestMembership(t, db, omar.ID, nodeID, "admin", "active")
	harriet, _ := createTestUser(t, db, "strikeharriet2", "member")
	createTestMembership(t, db, harriet.ID, nodeID, "member", "active")
	bea, _ := createTestUser(t, db, "strikebea2", "member")
	createTestMembership(t, db, bea.ID, nodeID, "member", "active")

	first := idOf(t, recordAttestation(t, db, "strike-two", omarToken, map[string]interface{}{
		"decided_at": "2026-09-28",
		"names":      []map[string]interface{}{{"user_id": omar.ID}, {"user_id": harriet.ID}},
	}))
	// The wrong one: a correction naming Bea instead of Harriet.
	wrong := idOf(t, recordAttestation(t, db, "strike-two", omarToken, map[string]interface{}{
		"decided_at": "2026-09-28", "supersedes_id": first,
		"names": []map[string]interface{}{{"user_id": omar.ID}, {"user_id": bea.ID}},
	}))
	if roleOf(t, db, bea.ID, nodeID) != "admin" || roleOf(t, db, harriet.ID, nodeID) != "member" {
		t.Fatalf("setup: the correction should have seated Bea in Harriet's place")
	}

	if code, body := strike(t, db, "strike-two", wrong, omarToken); code != http.StatusOK {
		t.Fatalf("strike: %d %s", code, body)
	}
	if got := roleOf(t, db, harriet.ID, nodeID); got != "admin" {
		t.Errorf("the record before the struck one is in force again, so Harriet holds admin; got %q", got)
	}
	if got := roleOf(t, db, bea.ID, nodeID); got != "member" {
		t.Errorf("the struck record counts for nothing, so Bea steps down; got %q", got)
	}
	for _, it := range listAttestations(t, db, "strike-two", omarToken).Items {
		if it.ID == first && it.SupersededBy != "" {
			t.Errorf("a struck correction corrects nothing, but the first record still reads superseded by %q", it.SupersededBy)
		}
	}
}

// The gate: a patch that chooses its admins here has nothing to strike.
func TestAttestation_StrikeRefusedWhereDecidedInPatchwork(t *testing.T) {
	db := setupTestDB(t)
	omar, omarToken := createTestUser(t, db, "strikeomar3", "member")
	nodeID := elsewhereNode(t, db, omar.ID, "Strike Three", "strike-three")
	createTestMembership(t, db, omar.ID, nodeID, "admin", "active")
	rec := idOf(t, recordAttestation(t, db, "strike-three", omarToken, map[string]interface{}{
		"decided_at": "2026-09-28", "names": []map[string]interface{}{{"user_id": omar.ID}},
	}))
	_, _ = db.Exec(`UPDATE nodes SET governance_config = replace(governance_config, '"leadership_venue":"elsewhere"', '"leadership_venue":"patchwork"') WHERE id = ?`, nodeID)
	if code, body := strike(t, db, "strike-three", rec, omarToken); code != http.StatusConflict {
		t.Errorf("striking where admins are chosen here: expected 409, got %d: %s", code, body)
	}
}

// ---- F-034: where a patch that meets elsewhere decides ----------------------

func TestVenueNote_ReadBackOnTheRules(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "venueadmin1", "member")
	nodeID := createTestNode(t, db, admin.ID, "Venue One", "venue-one", "open")
	_, _ = db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","leadership_model":"maintainer","proposal_venue":"elsewhere","venue_note":"First Tuesday of the month, 7pm, the church hall"}`,
		nodeID)

	r := authedRequest("GET", "/api/v1/nodes/venue-one/governance/rules", nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/governance/rules", handler.GetGovernanceRules(db), r)
	var rules struct {
		VenueNote string `json:"venue_note"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rules); err != nil {
		t.Fatalf("rules: %v", err)
	}
	// The editor spreads what it loaded back into its submission, so a
	// field this drops is a field the next unrelated edit erases.
	if rules.VenueNote != "First Tuesday of the month, 7pm, the church hall" {
		t.Errorf("expected the venue note on the rules, got %q (%d: %s)", rules.VenueNote, w.Code, w.Body.String())
	}
}
