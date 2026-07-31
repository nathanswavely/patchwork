package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// The rules endpoint has to send the whole rule set, because the editor's
// submission is built by spreading what it loaded.
//
// It didn't. leadership_model, both venues, subject_recusal, the term and
// inactivity numbers and the nomination window were all omitted, so they never
// reached the editor to be spread back into its submission, and SyncRulesToDB
// read the resulting file over DefaultRules(). Changing a quorum silently
// reset an elected patch to `maintainer` and a patch that decides at meetings
// back to deciding in Patchwork — governance nobody chose, applied by an edit
// to something else, and invisible because the diff only knew its own labels.
//
// Written against the marshalled rule set rather than a list of field names:
// a field added to GovernanceRules and forgotten here would reintroduce the
// bug, and the point is that nothing can be forgotten.
func TestGovernanceRules_SendsEveryFieldItStores(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "grr1", "member")
	nodeID := createTestNode(t, db, admin.ID, "Rules Roundtrip", "rules-roundtrip", "open")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")

	// A patch that has chosen almost everything away from its defaults, so a
	// dropped field shows up as a value reverting rather than as a match by
	// coincidence.
	setNodeRules(t, db, nodeID, `{
		"decision_method":"supermajority","quorum_percent":40,
		"default_vote_duration_hours":168,"amendment_threshold":"consensus",
		"amendment_auto_apply":false,"min_voting_tenure_days":30,
		"subject_recusal":true,"leadership_model":"elected",
		"leadership_venue":"elsewhere","proposal_venue":"elsewhere",
		"nomination_days":21,"succession_method":"admin_nominate",
		"succession_policy":"freeze","admin_term_months":24,
		"max_admins":7,"inactivity_days":120}`)

	r := authedRequest("GET", "/api/v1/nodes/rules-roundtrip/governance/rules", nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/governance/rules", handler.GetGovernanceRules(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Every key the rules file itself carries has to be in the payload. This
	// is the invariant: what the editor loads must be able to become what the
	// editor saves.
	shipped, err := json.Marshal(governance.DefaultRules())
	if err != nil {
		t.Fatalf("marshal defaults: %v", err)
	}
	var keys map[string]interface{}
	json.Unmarshal(shipped, &keys)
	for key := range keys {
		if _, ok := got[key]; !ok {
			t.Errorf("the rules payload drops %q, so an edit to anything else would reset it", key)
		}
	}

	// And the values are this patch's, not the shipped defaults.
	for key, want := range map[string]interface{}{
		"leadership_model":  "elected",
		"leadership_venue":  "elsewhere",
		"proposal_venue":    "elsewhere",
		"subject_recusal":   true,
		"nomination_days":   float64(21),
		"admin_term_months": float64(24),
	} {
		if got[key] != want {
			t.Errorf("%s: expected %v, got %v", key, want, got[key])
		}
	}
}
