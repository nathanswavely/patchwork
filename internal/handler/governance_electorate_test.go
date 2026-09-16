package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// The facts the rules editor states back to a founder (docs/adr/104).
//
// A sentence about what a quorum comes to is only worth printing if it is the
// same arithmetic the resolver runs, so what this endpoint sends has to be
// the electorate as `electorateFilter` defines it: active admins and members,
// their tenure in whole days, and the patch's own age, which caps the bar.

func electorateVia(t *testing.T, db *database.DB, slug, token string) (int, map[string]interface{}) {
	t.Helper()
	r := authedRequest("GET", "/api/v1/nodes/"+slug+"/governance/electorate", nil, token)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/governance/electorate",
		handler.GovernanceElectorate(db), r)
	if w.Code != http.StatusOK {
		return w.Code, nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode electorate: %v", err)
	}
	return w.Code, out
}

func joinedDaysAgo(t *testing.T, db *database.DB, userID, nodeID string, days int) {
	t.Helper()
	when := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02T15:04:05.000Z")
	if _, err := db.Exec(`UPDATE memberships SET joined_at = ? WHERE user_id = ? AND node_id = ?`,
		when, userID, nodeID); err != nil {
		t.Fatalf("backdate membership: %v", err)
	}
}

func TestElectorate_CountsTheRoomAndItsTenures(t *testing.T) {
	db := setupTestDB(t)
	founder, founderToken := createTestUser(t, db, "elecfacts", "member")
	nodeID := createTestNode(t, db, founder.ID, "Facts Co-op", "facts-coop", "open")
	createTestMembership(t, db, founder.ID, nodeID, "admin", "active")
	joinedDaysAgo(t, db, founder.ID, nodeID, 200)
	db.Exec(`UPDATE nodes SET founded_at = ? WHERE id = ?`,
		time.Now().UTC().AddDate(0, 0, -200).Format("2006-01-02"), nodeID)

	old, _ := createTestUser(t, db, "elecfacts-old", "member")
	createTestMembership(t, db, old.ID, nodeID, "member", "active")
	joinedDaysAgo(t, db, old.ID, nodeID, 45)

	fresh, _ := createTestUser(t, db, "elecfacts-new", "member")
	createTestMembership(t, db, fresh.ID, nodeID, "member", "active")

	// Neither of these is in the electorate (docs/adr/044): a follower has no
	// vote, and a pending request is not a membership yet.
	follower, followerToken := createTestUser(t, db, "elecfacts-fol", "member")
	createTestMembership(t, db, follower.ID, nodeID, "follower", "active")
	waiting, _ := createTestUser(t, db, "elecfacts-wait", "member")
	createTestMembership(t, db, waiting.ID, nodeID, "member", "pending")

	code, facts := electorateVia(t, db, "facts-coop", founderToken)
	if code != http.StatusOK {
		t.Fatalf("expected 200 for a member, got %d", code)
	}
	if got := facts["members"].(float64); got != 3 {
		t.Errorf("expected the three admins and members, got %v", got)
	}
	raw, _ := facts["tenure_days"].([]interface{})
	var tenures []int
	for _, v := range raw {
		tenures = append(tenures, int(v.(float64)))
	}
	if len(tenures) != 3 {
		t.Fatalf("expected three tenures, got %v", tenures)
	}
	if tenures[0] < tenures[1] || tenures[1] < tenures[2] {
		t.Errorf("expected longest tenure first, got %v", tenures)
	}
	if tenures[0] < 199 || tenures[1] < 44 || tenures[2] != 0 {
		t.Errorf("tenures should be whole days since joining, got %v", tenures)
	}
	if age := facts["patch_age_days"].(float64); age < 199 {
		t.Errorf("expected the patch's own age, got %v", age)
	}

	// A follower can read the rules and cannot see who votes on them.
	if code, _ := electorateVia(t, db, "facts-coop", followerToken); code != http.StatusForbidden {
		t.Errorf("expected 403 for a follower, got %d", code)
	}
}

// A brand-new patch reports its own age, which is what lets the editor say
// the tenure bar is not in force yet rather than reporting an electorate of
// nobody (docs/adr/098).
func TestElectorate_YoungPatchReportsItsAge(t *testing.T) {
	db := setupTestDB(t)
	founder, token := createTestUser(t, db, "elecyoung", "member")
	nodeID := createTestNode(t, db, founder.ID, "Young Press", "young-press", "open")
	createTestMembership(t, db, founder.ID, nodeID, "admin", "active")

	code, facts := electorateVia(t, db, "young-press", token)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if age := facts["patch_age_days"].(float64); age != 0 {
		t.Errorf("a patch made today is 0 days old, got %v", age)
	}
	if got := facts["members"].(float64); got != 1 {
		t.Errorf("expected the founder alone, got %v", got)
	}
}
