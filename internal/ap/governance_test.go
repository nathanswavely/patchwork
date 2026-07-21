package ap_test

import (
	"encoding/json"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

func setTestDomain(t *testing.T) {
	t.Helper()
	ap.SetDomain("test.example.com")
	t.Cleanup(func() { ap.SetDomain("") })
}

func TestGovernanceContext(t *testing.T) {
	setTestDomain(t)

	ctx := ap.GovernanceContext()
	if len(ctx) != 2 {
		t.Fatalf("expected 2-element array, got %d", len(ctx))
	}

	asCtx, ok := ctx[0].(string)
	if !ok || asCtx != "https://www.w3.org/ns/activitystreams" {
		t.Errorf("expected AS context, got %v", ctx[0])
	}

	nsMap, ok := ctx[1].(map[string]string)
	if !ok {
		t.Fatalf("expected map[string]string as second element, got %T", ctx[1])
	}
	gvNS, ok := nsMap["gv"]
	if !ok {
		t.Fatal("expected gv key in namespace map")
	}
	if gvNS != "https://test.example.com/ns/governance#" {
		t.Errorf("unexpected gv namespace: %s", gvNS)
	}
}

func TestProposalToObject(t *testing.T) {
	setTestDomain(t)

	votingEnds := "2026-04-10T12:00:00Z"
	p := model.Proposal{
		ID:            "prop-001",
		NodeID:        "node-001",
		AuthorID:      "user-001",
		Title:         "Add weekly meetup",
		Body:          "We should meet every Thursday.",
		ProposalType:  "action",
		Status:        "open",
		DurationHours: 72,
		VotingEndsAt:  &votingEnds,
		CreatedAt:     "2026-04-07T12:00:00Z",
		UpdatedAt:     "2026-04-07T12:00:00Z",
	}

	obj := ap.ProposalToObject(p, "test.example.com")

	if obj["type"] != "gv:Proposal" {
		t.Errorf("expected type=gv:Proposal, got %v", obj["type"])
	}
	if obj["id"] != "https://test.example.com/ap/proposals/prop-001" {
		t.Errorf("unexpected id: %v", obj["id"])
	}
	if obj["name"] != "Add weekly meetup" {
		t.Errorf("unexpected name: %v", obj["name"])
	}
	if obj["content"] != "We should meet every Thursday." {
		t.Errorf("unexpected content: %v", obj["content"])
	}
	if obj["gv:proposalType"] != "action" {
		t.Errorf("unexpected gv:proposalType: %v", obj["gv:proposalType"])
	}
	if obj["gv:status"] != "open" {
		t.Errorf("unexpected gv:status: %v", obj["gv:status"])
	}
	if obj["gv:votingEndsAt"] != votingEnds {
		t.Errorf("unexpected gv:votingEndsAt: %v", obj["gv:votingEndsAt"])
	}

	// attributedTo should reference user
	attrTo, ok := obj["attributedTo"].(string)
	if !ok {
		t.Fatalf("expected attributedTo to be string, got %T", obj["attributedTo"])
	}
	if attrTo != "https://test.example.com/ap/users/user-001" {
		t.Errorf("unexpected attributedTo: %s", attrTo)
	}

	// context should reference node
	ctx, ok := obj["context"].(string)
	if !ok {
		t.Fatalf("expected context to be string, got %T", obj["context"])
	}
	if ctx != "https://test.example.com/ap/nodes/node-001" {
		t.Errorf("unexpected context: %s", ctx)
	}

	// Should not have amendment fields when not set
	if _, exists := obj["gv:targetDoc"]; exists {
		t.Error("expected gv:targetDoc to be absent for non-amendment proposal")
	}
	if _, exists := obj["gv:proposedBody"]; exists {
		t.Error("expected gv:proposedBody to be absent for non-amendment proposal")
	}

	// Verify JSON round-trips
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["type"] != "gv:Proposal" {
		t.Errorf("type missing in JSON output")
	}
}

func TestProposalToObject_WithAmendment(t *testing.T) {
	setTestDomain(t)

	p := model.Proposal{
		ID:           "prop-amend-001",
		NodeID:       "node-001",
		AuthorID:     "user-001",
		Title:        "Update community standards",
		Body:         "Proposing new language in section 2.",
		ProposalType: "amendment",
		Status:       "open",
		TargetDoc:    "community-standards.md",
		ProposedBody: "# Revised Standards\n\nNew content.",
		GitSHA:       "abc123def",
		CreatedAt:    "2026-04-07T12:00:00Z",
		UpdatedAt:    "2026-04-07T12:00:00Z",
	}

	obj := ap.ProposalToObject(p, "test.example.com")

	if obj["gv:targetDoc"] != "community-standards.md" {
		t.Errorf("expected gv:targetDoc=community-standards.md, got %v", obj["gv:targetDoc"])
	}
	if obj["gv:proposedBody"] != "# Revised Standards\n\nNew content." {
		t.Errorf("unexpected gv:proposedBody: %v", obj["gv:proposedBody"])
	}
	if obj["gv:gitSha"] != "abc123def" {
		t.Errorf("unexpected gv:gitSha: %v", obj["gv:gitSha"])
	}
}

func TestGovernanceDocToObject(t *testing.T) {
	setTestDomain(t)

	doc := model.GovernanceDoc{
		ID:        "doc-001",
		NodeID:    "node-002",
		Title:     "Community Lining",
		Body:      "We are welcoming to all.",
		Version:   3,
		CreatedBy: "user-002",
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-03-15T10:00:00Z",
	}

	obj := ap.GovernanceDocToObject(doc, "test.example.com")

	if obj["type"] != "gv:GovernanceDocument" {
		t.Errorf("expected type=gv:GovernanceDocument, got %v", obj["type"])
	}
	if obj["id"] != "https://test.example.com/ap/governance/doc-001" {
		t.Errorf("unexpected id: %v", obj["id"])
	}
	if obj["name"] != "Community Lining" {
		t.Errorf("unexpected name: %v", obj["name"])
	}
	if obj["content"] != "We are welcoming to all." {
		t.Errorf("unexpected content: %v", obj["content"])
	}
	if obj["gv:version"] != 3 {
		t.Errorf("expected gv:version=3, got %v", obj["gv:version"])
	}
	if obj["attributedTo"] != "https://test.example.com/ap/users/user-002" {
		t.Errorf("unexpected attributedTo: %v", obj["attributedTo"])
	}
	if obj["context"] != "https://test.example.com/ap/nodes/node-002" {
		t.Errorf("unexpected context: %v", obj["context"])
	}
}

func TestVoteToActivity(t *testing.T) {
	setTestDomain(t)

	v := model.Vote{
		ID:         "vote-001",
		ProposalID: "prop-001",
		UserID:     "user-003",
		Value:      "approve",
		CreatedAt:  "2026-04-08T09:00:00Z",
	}

	proposalAPID := "https://test.example.com/ap/proposals/prop-001"
	voterAPID := "https://test.example.com/ap/users/user-003"

	activity := ap.VoteToActivity(v, proposalAPID, voterAPID)

	if activity["type"] != "gv:Vote" {
		t.Errorf("expected type=gv:Vote, got %v", activity["type"])
	}
	if activity["actor"] != voterAPID {
		t.Errorf("expected actor=%s, got %v", voterAPID, activity["actor"])
	}
	if activity["object"] != proposalAPID {
		t.Errorf("expected object=%s, got %v", proposalAPID, activity["object"])
	}
	if activity["gv:value"] != "approve" {
		t.Errorf("expected gv:value=approve, got %v", activity["gv:value"])
	}
	if activity["published"] != "2026-04-08T09:00:00Z" {
		t.Errorf("unexpected published: %v", activity["published"])
	}
}

func TestProposalResolvedActivity(t *testing.T) {
	setTestDomain(t)

	proposalAPID := "https://test.example.com/ap/proposals/prop-001"
	nodeAPID := "https://test.example.com/ap/nodes/node-001"

	activity := ap.ProposalResolvedActivity(proposalAPID, nodeAPID, "approved", 5, 2, 1)

	if activity["type"] != "gv:ResolveProposal" {
		t.Errorf("expected type=gv:ResolveProposal, got %v", activity["type"])
	}
	if activity["actor"] != nodeAPID {
		t.Errorf("expected actor=%s, got %v", nodeAPID, activity["actor"])
	}
	if activity["object"] != proposalAPID {
		t.Errorf("expected object=%s, got %v", proposalAPID, activity["object"])
	}
	if activity["gv:result"] != "approved" {
		t.Errorf("expected gv:result=approved, got %v", activity["gv:result"])
	}
	if activity["gv:approveCount"] != 5 {
		t.Errorf("expected gv:approveCount=5, got %v", activity["gv:approveCount"])
	}
	if activity["gv:rejectCount"] != 2 {
		t.Errorf("expected gv:rejectCount=2, got %v", activity["gv:rejectCount"])
	}
	if activity["gv:abstainCount"] != 1 {
		t.Errorf("expected gv:abstainCount=1, got %v", activity["gv:abstainCount"])
	}

	// Verify context array is present
	ctx, ok := activity["@context"].([]interface{})
	if !ok {
		t.Fatalf("expected @context to be []interface{}, got %T", activity["@context"])
	}
	if len(ctx) != 2 {
		t.Errorf("expected 2-element context, got %d", len(ctx))
	}
}
