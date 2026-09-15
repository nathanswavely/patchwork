package governance_test

import (
	"os"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/governance"
)

// A repo rebuilt from the canonical rows has `main` and nothing else
// (docs/adr/084), so a pending amendment's branch is gone. The proposed text
// is not gone — it is a column on the proposal — and EnsureBranch is what
// turns the canonical text back into the ref the merge needs.
func TestEnsureBranch_RecreatesAMissingBranchFromCanonicalText(t *testing.T) {
	db := repairDB(t)
	dataDir := t.TempDir()
	original := "# Community Standards\n\nThe text as adopted.\n"
	seedPatch(t, db, "Community Standards", original)

	if err := governance.InitInstanceRepo(dataDir); err != nil {
		t.Fatal(err)
	}
	if err := governance.ForkForNode(dataDir, repairNodeID, "casual"); err != nil {
		t.Fatal(err)
	}

	// The amendment is raised while the repo is whole, and then the machine
	// it lived on is replaced: the repo is gone and only the database arrives.
	proposed := "# Community Standards\n\nApproval required to join.\n"
	if _, err := governance.CreateBranch(dataDir, repairNodeID, "amendment-abc12345",
		"community-standards.md", proposed, "Nell", "nell@example.test", "Proposed amendment"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(governance.NodeRepoPath(dataDir, repairNodeID)); err != nil {
		t.Fatal(err)
	}
	if _, err := governance.Repair(db, dataDir, governance.RepairOptions{}); err != nil {
		t.Fatalf("repair: %v", err)
	}

	created, err := governance.EnsureBranch(dataDir, repairNodeID, "amendment-abc12345",
		"community-standards.md", proposed)
	if err != nil {
		t.Fatalf("ensure branch: %v", err)
	}
	if !created {
		t.Fatal("expected the missing branch to be recreated")
	}

	// And the whole point: the amendment can be applied after the rebuild.
	if _, err := governance.MergeBranch(dataDir, repairNodeID, "amendment-abc12345",
		"Nell", "nell@example.test"); err != nil {
		t.Fatalf("merge after rebuild: %v", err)
	}
	got, err := governance.GetDocument(dataDir, repairNodeID, "community-standards.md")
	if err != nil {
		t.Fatal(err)
	}
	if got != proposed {
		t.Errorf("document after merge = %q, want the proposed text", got)
	}

	// The commit is repair-signed, so the charter history says the text came
	// from the database rather than passing a reconstruction off as a commit
	// somebody made.
	history, err := governance.GetHistory(dataDir, repairNodeID, "community-standards.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) == 0 {
		t.Fatal("no history after merge")
	}
	if history[0].Repair != governance.RepairRestored {
		t.Errorf("top commit repair marker = %q, want %q", history[0].Repair, governance.RepairRestored)
	}
	if history[0].AuthorName != governance.RepairAuthorName {
		t.Errorf("top commit author = %q, want the repair identity", history[0].AuthorName)
	}
}

// It fills an absence and never writes over what is there: a branch that
// survived carries the proposer's own commit, and the reconstruction must not
// replace it (docs/adr/084's asymmetry).
func TestEnsureBranch_LeavesAnExistingBranchAlone(t *testing.T) {
	db := repairDB(t)
	dataDir := t.TempDir()
	seedPatch(t, db, "Community Standards", "# Community Standards\n\nAs adopted.\n")

	if err := governance.InitInstanceRepo(dataDir); err != nil {
		t.Fatal(err)
	}
	if err := governance.ForkForNode(dataDir, repairNodeID, "casual"); err != nil {
		t.Fatal(err)
	}

	proposed := "# Community Standards\n\nThe proposer's own words.\n"
	if _, err := governance.CreateBranch(dataDir, repairNodeID, "amendment-def67890",
		"community-standards.md", proposed, "Nell", "nell@example.test", "Proposed amendment"); err != nil {
		t.Fatal(err)
	}

	created, err := governance.EnsureBranch(dataDir, repairNodeID, "amendment-def67890",
		"community-standards.md", "something else entirely")
	if err != nil {
		t.Fatalf("ensure branch: %v", err)
	}
	if created {
		t.Fatal("an existing branch was rewritten; it must be left exactly as it is")
	}

	if _, err := governance.MergeBranch(dataDir, repairNodeID, "amendment-def67890",
		"Nell", "nell@example.test"); err != nil {
		t.Fatal(err)
	}
	got, err := governance.GetDocument(dataDir, repairNodeID, "community-standards.md")
	if err != nil {
		t.Fatal(err)
	}
	if got != proposed {
		t.Errorf("document after merge = %q, want the branch's own text", got)
	}
}

// With no repo at all there is nothing to create a branch in. That is the
// repair pass's job, not this one's, and saying so beats writing a repo on
// the strength of one missing ref.
func TestEnsureBranch_RefusesWhenTheRepoIsGone(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := governance.EnsureBranch(dataDir, repairNodeID, "amendment-abc12345",
		"community-standards.md", "text"); err == nil {
		t.Fatal("expected an error when the repo does not exist")
	}
}
