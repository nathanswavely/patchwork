package governance_test

import (
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/governance"
)

// A direct charter edit records its AI answer as a git trailer, and the
// history view reads it back as a field with the message left as written
// (docs/adr/2026-10-20-a-post-says-whether-ai-made-it.md).
func TestDirectEdit_AIUseTrailerRoundTrips(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "trailer-node"
	initInstanceAndNode(t, dataDir, nodeID)

	generated := "generated"
	if _, err := governance.DirectEdit(dataDir, nodeID, "house-rules.md", "Be kind.",
		"Ada", "ada@example.org", governance.WithAIUseTrailer("Update House Rules (v2)", &generated)); err != nil {
		t.Fatalf("DirectEdit: %v", err)
	}
	if _, err := governance.DirectEdit(dataDir, nodeID, "house-rules.md", "Be kind, always.",
		"Ada", "ada@example.org", governance.WithAIUseTrailer("Update House Rules (v3)", nil)); err != nil {
		t.Fatalf("DirectEdit: %v", err)
	}

	history, err := governance.GetHistory(dataDir, nodeID, "house-rules.md")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history has %d entries, want 2", len(history))
	}
	latest, earlier := history[0], history[1]
	if latest.AIUse != "" || latest.Message != "Update House Rules (v3)" {
		t.Errorf("unanswered edit: ai_use %q, message %q", latest.AIUse, latest.Message)
	}
	if earlier.AIUse != "generated" || earlier.Message != "Update House Rules (v2)" {
		t.Errorf("answered edit: ai_use %q, message %q", earlier.AIUse, earlier.Message)
	}
}
