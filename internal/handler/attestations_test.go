package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// Decisions can happen elsewhere, and be recorded here (docs/adr/052).
//
// Two rules carry the design and these tests hold both: attestation is offered
// only where the patch declared that venue, and the record may name anyone
// while the effect lands only on members.

func elsewhereNode(t *testing.T, db *database.DB, ownerID, name, slug string) string {
	t.Helper()
	nodeID := createTestNode(t, db, ownerID, name, slug, "open")
	db.Exec(`UPDATE nodes SET governance_config = ? WHERE id = ?`,
		`{"decision_method":"majority","quorum_percent":0,"leadership_model":"elected","leadership_venue":"elsewhere","min_voting_tenure_days":0}`,
		nodeID)
	return nodeID
}

func recordAttestation(t *testing.T, db *database.DB, slug, token string, body map[string]interface{}) *httpRecorderResult {
	t.Helper()
	r := authedRequest("POST", "/api/v1/nodes/"+slug+"/attestations", body, token)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/attestations", handler.CreateAttestation(db), r)
	return &httpRecorderResult{Code: w.Code, Body: w.Body.String()}
}

type httpRecorderResult struct {
	Code int
	Body string
}

// The gate. Somewhere that decides here, attesting would be a way around the
// vote — docs/adr/049's disease with the polarity reversed.
func TestAttestation_RefusedWhereDecidedInPatchwork(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "attadmin1", "member")
	nodeID := meritocraticNode(t, db, admin.ID, "Att One", "att-one")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	res := recordAttestation(t, db, "att-one", adminToken, map[string]interface{}{
		"decided_at": "2026-03-14", "summary": "AGM",
	})
	if res.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", res.Code, res.Body)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM attestations WHERE node_id = ?", nodeID).Scan(&count)
	if count != 0 {
		t.Errorf("expected nothing recorded, got %d", count)
	}
}

// The record may name anyone; the effect lands only on members. A co-op
// arriving with a board of seven records all seven on day one, and the ones who
// have not joined hold nothing.
func TestAttestation_RecordNamesAnyoneEffectHitsMembersOnly(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "attadmin2", "member")
	nodeID := elsewhereNode(t, db, admin.ID, "Att Two", "att-two")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	joined, _ := createTestUser(t, db, "attjoined2", "member")
	createTestMembership(t, db, joined.ID, nodeID, "member", "active")

	res := recordAttestation(t, db, "att-two", adminToken, map[string]interface{}{
		"decided_at": "2026-03-14",
		"summary":    "Annual meeting elected the council",
		"names": []map[string]interface{}{
			{"user_id": admin.ID},
			{"user_id": joined.ID},
			{"display_name": "Dana Okonkwo"},
			{"display_name": "Sam Whitfield"},
		},
	})
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", res.Code, res.Body)
	}

	// All four names are on the record — the community's own statement.
	var names int
	db.QueryRow(`SELECT COUNT(*) FROM attestation_names n JOIN attestations a ON a.id = n.attestation_id WHERE a.node_id = ?`, nodeID).Scan(&names)
	if names != 4 {
		t.Errorf("expected all four names recorded, got %d", names)
	}
	// Two of them are unrealized: no user, and so no role anywhere.
	var unrealized int
	db.QueryRow(`SELECT COUNT(*) FROM attestation_names n JOIN attestations a ON a.id = n.attestation_id WHERE a.node_id = ? AND n.user_id IS NULL`, nodeID).Scan(&unrealized)
	if unrealized != 2 {
		t.Errorf("expected two unrealized names, got %d", unrealized)
	}
	// The member who was named now holds admin.
	if got := roleOf(t, db, joined.ID, nodeID); got != "admin" {
		t.Errorf("expected the named member promoted, got %q", got)
	}
	// And no membership row was invented for anyone who never joined.
	var members int
	db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ?", nodeID).Scan(&members)
	if members != 2 {
		t.Errorf("expected exactly the two real memberships, got %d", members)
	}
}

// Naming a user who is not in the patch does not smuggle them in — the name is
// kept as the community's words, the link is not.
func TestAttestation_NamingANonMemberLeavesThemUnrealized(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "attadmin3", "member")
	nodeID := elsewhereNode(t, db, admin.ID, "Att Three", "att-three")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	outsider, _ := createTestUser(t, db, "attoutsider3", "member")

	res := recordAttestation(t, db, "att-three", adminToken, map[string]interface{}{
		"decided_at": "2026-03-14",
		"names": []map[string]interface{}{
			{"user_id": admin.ID},
			{"user_id": outsider.ID, "display_name": "Outsider Person"},
		},
	})
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", res.Code, res.Body)
	}
	var linked int
	db.QueryRow(`SELECT COUNT(*) FROM attestation_names WHERE user_id = ?`, outsider.ID).Scan(&linked)
	if linked != 0 {
		t.Errorf("an outsider must not be linked by naming, got %d", linked)
	}
	if roleOf(t, db, outsider.ID, nodeID) != "" {
		t.Error("an outsider must not gain a membership from being named")
	}
}

// An election decides who is on the council, which is also a decision about
// who is off it.
func TestAttestation_UnnamedAdminStepsDown(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "attadmin4", "member")
	nodeID := elsewhereNode(t, db, admin.ID, "Att Four", "att-four")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	outgoing, _ := createTestUser(t, db, "attoutgoing4", "member")
	createTestMembership(t, db, outgoing.ID, nodeID, "admin", "active")

	res := recordAttestation(t, db, "att-four", adminToken, map[string]interface{}{
		"decided_at": "2026-03-14",
		"names":      []map[string]interface{}{{"user_id": admin.ID}},
	})
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", res.Code, res.Body)
	}
	if got := roleOf(t, db, outgoing.ID, nodeID); got != "member" {
		t.Errorf("expected the unnamed admin to step down, got %q", got)
	}
}

// No record may leave a patch with nobody able to administer it.
func TestAttestation_NeverRemovesTheLastAdmin(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "attadmin5", "member")
	nodeID := elsewhereNode(t, db, admin.ID, "Att Five", "att-five")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	other, _ := createTestUser(t, db, "attother5", "member")
	createTestMembership(t, db, other.ID, nodeID, "member", "active")

	// Names only the member, which would strip the sole sitting admin.
	res := recordAttestation(t, db, "att-five", adminToken, map[string]interface{}{
		"decided_at": "2026-03-14",
		"names":      []map[string]interface{}{{"user_id": other.ID}},
	})
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", res.Code, res.Body)
	}
	var admins int
	db.QueryRow("SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'", nodeID).Scan(&admins)
	if admins < 1 {
		t.Fatal("a patch must never be left with no admin")
	}
	if got := roleOf(t, db, other.ID, nodeID); got != "admin" {
		t.Errorf("expected the newly named person promoted, got %q", got)
	}
}

// Corrections supersede; a record is never edited.
func TestAttestation_CorrectionSupersedes(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "attadmin6", "member")
	nodeID := elsewhereNode(t, db, admin.ID, "Att Six", "att-six")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	first := recordAttestation(t, db, "att-six", adminToken, map[string]interface{}{
		"decided_at": "2026-03-14", "summary": "typo",
		"names": []map[string]interface{}{{"user_id": admin.ID}},
	})
	if first.Code != http.StatusCreated {
		t.Fatalf("first: got %d: %s", first.Code, first.Body)
	}
	var firstID string
	db.QueryRow("SELECT id FROM attestations WHERE node_id = ?", nodeID).Scan(&firstID)

	second := recordAttestation(t, db, "att-six", adminToken, map[string]interface{}{
		"decided_at": "2026-03-14", "summary": "corrected",
		"supersedes_id": firstID,
		"names":         []map[string]interface{}{{"user_id": admin.ID}},
	})
	if second.Code != http.StatusCreated {
		t.Fatalf("second: got %d: %s", second.Code, second.Body)
	}

	// Both survive — the corrected one is not erased.
	var total int
	db.QueryRow("SELECT COUNT(*) FROM attestations WHERE node_id = ?", nodeID).Scan(&total)
	if total != 2 {
		t.Errorf("expected both records kept, got %d", total)
	}

	// The same record cannot be corrected twice; correct the correction.
	third := recordAttestation(t, db, "att-six", adminToken, map[string]interface{}{
		"decided_at": "2026-03-14", "supersedes_id": firstID,
		"names": []map[string]interface{}{{"user_id": admin.ID}},
	})
	if third.Code != http.StatusConflict {
		t.Errorf("expected 409 correcting an already-corrected record, got %d: %s", third.Code, third.Body)
	}
}

// A record of what happened has to say when it happened.
func TestAttestation_RequiresADecidedDate(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "attadmin7", "member")
	nodeID := elsewhereNode(t, db, admin.ID, "Att Seven", "att-seven")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	res := recordAttestation(t, db, "att-seven", adminToken, map[string]interface{}{"summary": "no date"})
	if res.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", res.Code, res.Body)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM attestations WHERE node_id = ?", nodeID).Scan(&count)
	if count != 0 {
		t.Errorf("expected a dateless record to be refused outright, got %d", count)
	}
}

// Where admins are chosen elsewhere, Patchwork does not make them by hand —
// the record is what promotes.
func TestPromoteToAdmin_RefusedWhereLeadershipIsElsewhere(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "attadmin8", "member")
	nodeID := elsewhereNode(t, db, admin.ID, "Att Eight", "att-eight")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	hopeful, _ := createTestUser(t, db, "atthopeful8", "member")
	createTestMembership(t, db, hopeful.ID, nodeID, "member", "active")

	r := authedRequest("PATCH", "/api/v1/nodes/att-eight/members/"+hopeful.ID,
		map[string]interface{}{"role": "admin"}, adminToken)
	w := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/members/{userId}", handler.UpdateMember(db), r)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "elsewhere") {
		t.Errorf("expected the refusal to point at the venue, got %s", w.Body.String())
	}
}

// Linking an unrealized name is the moment someone the record already named
// takes up the role it said they held.
func TestAttestation_LinkingAnUnrealizedNamePromotes(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "attadmin9", "member")
	nodeID := elsewhereNode(t, db, admin.ID, "Att Nine", "att-nine")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	res := recordAttestation(t, db, "att-nine", adminToken, map[string]interface{}{
		"decided_at": "2026-03-14",
		"names": []map[string]interface{}{
			{"user_id": admin.ID},
			{"display_name": "Dana Okonkwo"},
		},
	})
	if res.Code != http.StatusCreated {
		t.Fatalf("record: got %d: %s", res.Code, res.Body)
	}
	var nameID string
	db.QueryRow("SELECT id FROM attestation_names WHERE user_id IS NULL").Scan(&nameID)
	if nameID == "" {
		t.Fatal("expected an unrealized name")
	}

	// Dana arrives.
	dana, _ := createTestUser(t, db, "attdana9", "member")
	createTestMembership(t, db, dana.ID, nodeID, "member", "active")

	lr := authedRequest("PATCH", "/api/v1/nodes/att-nine/attestation-names/"+nameID,
		map[string]interface{}{"user_id": dana.ID}, adminToken)
	lw := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/attestation-names/{id}", handler.LinkAttestationName(db), lr)

	if lw.Code != http.StatusOK {
		t.Fatalf("link: got %d: %s", lw.Code, lw.Body.String())
	}
	if got := roleOf(t, db, dana.ID, nodeID); got != "admin" {
		t.Errorf("expected linking to apply the role the record already named, got %q", got)
	}
}

// A name can only be linked to someone who is actually in the patch.
func TestAttestation_LinkRefusesANonMember(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "attadmin10", "member")
	nodeID := elsewhereNode(t, db, admin.ID, "Att Ten", "att-ten")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	recordAttestation(t, db, "att-ten", adminToken, map[string]interface{}{
		"decided_at": "2026-03-14",
		"names": []map[string]interface{}{
			{"user_id": admin.ID}, {"display_name": "Dana Okonkwo"},
		},
	})
	var nameID string
	db.QueryRow("SELECT id FROM attestation_names WHERE user_id IS NULL").Scan(&nameID)

	outsider, _ := createTestUser(t, db, "attoutsider10", "member")
	lr := authedRequest("PATCH", "/api/v1/nodes/att-ten/attestation-names/"+nameID,
		map[string]interface{}{"user_id": outsider.ID}, adminToken)
	lw := serveMux(t, db, "PATCH", "/api/v1/nodes/{slug}/attestation-names/{id}", handler.LinkAttestationName(db), lr)

	if lw.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", lw.Code, lw.Body.String())
	}
	if roleOf(t, db, outsider.ID, nodeID) != "" {
		t.Error("a refused link must not create a membership")
	}
}

// Reading is public: the whole value is that the people who were in the room
// can check it.
func TestAttestation_ListIsPublic(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "attadmin11", "member")
	nodeID := elsewhereNode(t, db, admin.ID, "Att Eleven", "att-eleven")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	recordAttestation(t, db, "att-eleven", adminToken, map[string]interface{}{
		"decided_at": "2026-03-14", "summary": "AGM",
		"names": []map[string]interface{}{{"user_id": admin.ID}, {"display_name": "Dana Okonkwo"}},
	})

	r := authedRequest("GET", "/api/v1/nodes/att-eleven/attestations", nil, "")
	w := servePublicMux(t, "GET", "/api/v1/nodes/{slug}/attestations", handler.ListAttestations(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for an anonymous read, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Dana Okonkwo") {
		t.Errorf("expected the unrealized name to be readable, got %s", body)
	}
	if !strings.Contains(body, `"realized":false`) {
		t.Errorf("expected unrealized names marked as such, got %s", body)
	}
}
