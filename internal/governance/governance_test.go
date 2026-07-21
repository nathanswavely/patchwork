package governance_test

import (
	"os"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/governance"
)

func tempDataDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "patchwork-gov-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// initInstanceAndNode is a helper that creates an instance repo and forks a node.
func initInstanceAndNode(t *testing.T, dataDir, nodeID string) {
	t.Helper()
	if err := governance.InitInstanceRepo(dataDir); err != nil {
		t.Fatalf("InitInstanceRepo: %v", err)
	}
	if err := governance.ForkForNode(dataDir, nodeID, "casual"); err != nil {
		t.Fatalf("ForkForNode: %v", err)
	}
}

// --- Gitless runtime ---

// TestGitlessRuntime guards the distroless deployment: the production image
// ships no git binary, so every governance operation must work without one.
// Emptying PATH makes any exec of git (e.g. go-git's file transport, which
// runs `git upload-pack` for local-path clones) fail loudly here instead of
// in production.
func TestGitlessRuntime(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("GIT_EXEC_PATH", "")

	dataDir := tempDataDir(t)
	nodeID := "gitless-node"
	initInstanceAndNode(t, dataDir, nodeID)

	// Full amendment lifecycle: branch with proposed change, merge, history.
	content, err := governance.GetDocument(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	amended := content + "\n## Amendment\n\nAdded without a git binary.\n"

	if _, err := governance.CreateBranch(dataDir, nodeID, "amendment-1", "community-standards.md",
		amended, "Test User", "test@patchwork.local", "Propose amendment"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if _, err := governance.MergeBranch(dataDir, nodeID, "amendment-1", "Test User", "test@patchwork.local"); err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}

	got, err := governance.GetDocument(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("GetDocument after merge: %v", err)
	}
	if got != amended {
		t.Errorf("merged content mismatch:\ngot:\n%s\nwant:\n%s", got, amended)
	}

	history, err := governance.GetHistory(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) < 2 {
		t.Errorf("expected at least 2 commits in history, got %d", len(history))
	}
}

// --- Instance Repo ---

func TestInitInstanceRepo(t *testing.T) {
	dataDir := tempDataDir(t)
	if err := governance.InitInstanceRepo(dataDir); err != nil {
		t.Fatalf("InitInstanceRepo: %v", err)
	}

	path := governance.InstanceRepoPath(dataDir)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("repo path does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory at %s", path)
	}
}

func TestInitInstanceRepo_Idempotent(t *testing.T) {
	dataDir := tempDataDir(t)
	if err := governance.InitInstanceRepo(dataDir); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := governance.InitInstanceRepo(dataDir); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestInstanceRepo_HasDefaultDocs(t *testing.T) {
	dataDir := tempDataDir(t)
	if err := governance.InitInstanceRepo(dataDir); err != nil {
		t.Fatal(err)
	}

	// ListDocuments uses NodeRepoPath(dataDir, nodeID) which equals InstanceRepoPath
	// when nodeID is "instance".
	docs, err := governance.ListDocuments(dataDir, "instance")
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}

	found := false
	for _, d := range docs {
		if d.Filename == "community-standards.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected community-standards.md in instance docs, got %v", docs)
	}
}

// --- Fork ---

func TestForkForNode(t *testing.T) {
	dataDir := tempDataDir(t)
	if err := governance.InitInstanceRepo(dataDir); err != nil {
		t.Fatal(err)
	}

	nodeID := "test-node-1"
	if err := governance.ForkForNode(dataDir, nodeID, "casual"); err != nil {
		t.Fatalf("ForkForNode: %v", err)
	}

	path := governance.NodeRepoPath(dataDir, nodeID)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("node repo does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory at %s", path)
	}
}

func TestForkForNode_Idempotent(t *testing.T) {
	dataDir := tempDataDir(t)
	if err := governance.InitInstanceRepo(dataDir); err != nil {
		t.Fatal(err)
	}

	nodeID := "test-node-2"
	if err := governance.ForkForNode(dataDir, nodeID, "casual"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := governance.ForkForNode(dataDir, nodeID, "casual"); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestForkForNode_InheritsDocuments(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "inherit-test"
	initInstanceAndNode(t, dataDir, nodeID)

	nodeDocs, err := governance.ListDocuments(dataDir, nodeID)
	if err != nil {
		t.Fatalf("node ListDocuments: %v", err)
	}

	// Casual template should have: community-standards.md, governance-rules.json, operating-agreement.md
	if len(nodeDocs) < 2 {
		t.Fatalf("expected at least 2 docs in fork, got %d", len(nodeDocs))
	}

	nodeMap := make(map[string]bool)
	for _, d := range nodeDocs {
		nodeMap[d.Filename] = true
	}

	// Must have the lining
	if !nodeMap["community-standards.md"] {
		t.Error("fork missing community-standards.md (the lining)")
	}
	// Must have governance rules
	if !nodeMap["governance-rules.json"] {
		t.Error("fork missing governance-rules.json")
	}
}

func TestForkForNode_FormalTemplate(t *testing.T) {
	dataDir := tempDataDir(t)
	if err := governance.InitInstanceRepo(dataDir); err != nil {
		t.Fatalf("InitInstanceRepo: %v", err)
	}
	nodeID := "formal-test"
	if err := governance.ForkForNode(dataDir, nodeID, "formal"); err != nil {
		t.Fatalf("ForkForNode: %v", err)
	}

	nodeDocs, err := governance.ListDocuments(dataDir, nodeID)
	if err != nil {
		t.Fatalf("node ListDocuments: %v", err)
	}

	nodeMap := make(map[string]bool)
	for _, d := range nodeDocs {
		nodeMap[d.Filename] = true
	}

	expected := []string{"community-standards.md", "governance-rules.json", "charter.md", "bylaws.md", "financial-transparency.md", "conflict-resolution.md", "succession-plan.md"}
	for _, f := range expected {
		if !nodeMap[f] {
			t.Errorf("formal template missing expected doc: %s", f)
		}
	}
}

// --- Documents ---

func TestListDocuments(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "list-docs"
	initInstanceAndNode(t, dataDir, nodeID)

	docs, err := governance.ListDocuments(dataDir, nodeID)
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}

	// Should have at least community-standards.md and governance-rules.json
	if len(docs) == 0 {
		t.Fatal("expected at least one document")
	}

	for _, d := range docs {
		if !strings.HasSuffix(d.Filename, ".md") && d.Filename != "governance-rules.json" {
			t.Errorf("expected .md files or governance-rules.json, got %q", d.Filename)
		}
	}
}

func TestGetDocument(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "get-doc"
	initInstanceAndNode(t, dataDir, nodeID)

	content, err := governance.GetDocument(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}

	// The forked file must be byte-identical to the canonical default
	// lining body (docs/adr/011: one identity, DB canonical, git mirror).
	if content != governance.DefaultLiningBody {
		t.Errorf("expected community-standards.md to equal DefaultLiningBody, got: %s", content[:100])
	}
}

func TestGetDocument_NotFound(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "get-doc-404"
	initInstanceAndNode(t, dataDir, nodeID)

	_, err := governance.GetDocument(dataDir, nodeID, "nonexistent.md")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

// --- Direct Edit ---

func TestDirectEdit(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "direct-edit"
	initInstanceAndNode(t, dataDir, nodeID)

	newContent := "# Updated Standards\n\nNew content here.\n"
	_, err := governance.DirectEdit(dataDir, nodeID, "community-standards.md", newContent, "Test User", "test@example.com", "Update standards")
	if err != nil {
		t.Fatalf("DirectEdit: %v", err)
	}

	got, err := governance.GetDocument(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("GetDocument after edit: %v", err)
	}
	if got != newContent {
		t.Errorf("expected updated content, got: %s", got)
	}
}

func TestDirectEdit_NewFile(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "direct-edit-new"
	initInstanceAndNode(t, dataDir, nodeID)

	newContent := "# Code of Conduct\n\nBe excellent to each other.\n"
	_, err := governance.DirectEdit(dataDir, nodeID, "code-of-conduct.md", newContent, "Test User", "test@example.com", "Add code of conduct")
	if err != nil {
		t.Fatalf("DirectEdit new file: %v", err)
	}

	docs, err := governance.ListDocuments(dataDir, nodeID)
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}

	found := false
	for _, d := range docs {
		if d.Filename == "code-of-conduct.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected code-of-conduct.md in documents, got %v", docs)
	}
}

func TestDirectEdit_History(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "direct-edit-hist"
	initInstanceAndNode(t, dataDir, nodeID)

	// The initial fork has 1 commit for community-standards.md
	// Make 2 edits
	_, err := governance.DirectEdit(dataDir, nodeID, "community-standards.md", "v2", "User", "u@e.com", "Edit 1")
	if err != nil {
		t.Fatalf("edit 1: %v", err)
	}
	_, err = governance.DirectEdit(dataDir, nodeID, "community-standards.md", "v3", "User", "u@e.com", "Edit 2")
	if err != nil {
		t.Fatalf("edit 2: %v", err)
	}

	history, err := governance.GetHistory(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}

	// 1 initial + 2 edits = 3 entries
	if len(history) < 3 {
		t.Errorf("expected at least 3 history entries, got %d", len(history))
	}
}

// --- Version History ---

func TestGetHistory(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "history-order"
	initInstanceAndNode(t, dataDir, nodeID)

	_, err := governance.DirectEdit(dataDir, nodeID, "community-standards.md", "v2", "User", "u@e.com", "Edit 1")
	if err != nil {
		t.Fatal(err)
	}

	history, err := governance.GetHistory(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}

	if len(history) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(history))
	}

	// Newest first: first entry should have the highest version number
	if history[0].VersionNumber <= history[len(history)-1].VersionNumber {
		t.Errorf("expected newest-first order: first version=%d, last version=%d",
			history[0].VersionNumber, history[len(history)-1].VersionNumber)
	}

	// Oldest entry should be version 1
	if history[len(history)-1].VersionNumber != 1 {
		t.Errorf("expected oldest version to be 1, got %d", history[len(history)-1].VersionNumber)
	}
}

func TestGetDocumentAtVersion(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "at-version"
	initInstanceAndNode(t, dataDir, nodeID)

	// Get original content's SHA
	historyBefore, err := governance.GetHistory(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatal(err)
	}
	originalSHA := historyBefore[0].SHA

	// Edit the file
	_, err = governance.DirectEdit(dataDir, nodeID, "community-standards.md", "new content", "User", "u@e.com", "Update")
	if err != nil {
		t.Fatal(err)
	}

	// Read old version by SHA — it must be the pristine default lining.
	oldContent, err := governance.GetDocumentAtVersion(dataDir, nodeID, "community-standards.md", originalSHA)
	if err != nil {
		t.Fatalf("GetDocumentAtVersion: %v", err)
	}

	if oldContent != governance.DefaultLiningBody {
		t.Errorf("expected old version to equal DefaultLiningBody, got: %s", oldContent)
	}

	// Current version should be different
	currentContent, err := governance.GetDocument(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatal(err)
	}
	if currentContent != "new content" {
		t.Errorf("expected current content to be 'new content', got: %s", currentContent)
	}
}

// --- Branching ---

func TestCreateBranch(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "create-branch"
	initInstanceAndNode(t, dataDir, nodeID)

	branchContent := "# Proposed Change\n\nThis is a proposed amendment.\n"
	sha, err := governance.CreateBranch(dataDir, nodeID, "proposal-1", "community-standards.md", branchContent, "Proposer", "p@e.com", "Propose amendment")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if sha == "" {
		t.Fatal("expected non-empty SHA")
	}

	// Main branch should still have original content
	mainContent, err := governance.GetDocument(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatal(err)
	}
	if mainContent == branchContent {
		t.Error("main branch content should not have changed")
	}
}

func TestMergeBranch_FastForward(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "merge-ff"
	initInstanceAndNode(t, dataDir, nodeID)

	branchContent := "# Fast-forward content\n"
	_, err := governance.CreateBranch(dataDir, nodeID, "ff-branch", "community-standards.md", branchContent, "User", "u@e.com", "FF change")
	if err != nil {
		t.Fatal(err)
	}

	// Merge without any changes on main (fast-forward)
	sha, err := governance.MergeBranch(dataDir, nodeID, "ff-branch", "Merger", "m@e.com")
	if err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if sha == "" {
		t.Fatal("expected non-empty SHA")
	}

	// Main should now have branch content
	content, err := governance.GetDocument(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatal(err)
	}
	if content != branchContent {
		t.Errorf("expected merged content %q, got %q", branchContent, content)
	}
}

func TestMergeBranch_MergeCommit(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "merge-commit"
	initInstanceAndNode(t, dataDir, nodeID)

	// Create a branch with changes to community-standards.md
	branchContent := "# Branch version\n"
	_, err := governance.CreateBranch(dataDir, nodeID, "amendment-1", "community-standards.md", branchContent, "Proposer", "p@e.com", "Branch change")
	if err != nil {
		t.Fatal(err)
	}

	// Make a direct edit on main (different file to avoid conflict, or same file -- the merge uses branch tree)
	_, err = governance.DirectEdit(dataDir, nodeID, "community-standards.md", "# Main version\n", "Admin", "a@e.com", "Main change")
	if err != nil {
		t.Fatal(err)
	}

	// Now merge -- should create a merge commit (non-fast-forward)
	sha, err := governance.MergeBranch(dataDir, nodeID, "amendment-1", "Merger", "m@e.com")
	if err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	if sha == "" {
		t.Fatal("expected non-empty SHA")
	}

	// Branch wins in non-ff merge (code uses branch's tree)
	content, err := governance.GetDocument(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatal(err)
	}
	if content != branchContent {
		t.Errorf("expected branch content %q after merge, got %q", branchContent, content)
	}
}

func TestDeleteBranch(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "delete-branch"
	initInstanceAndNode(t, dataDir, nodeID)

	_, err := governance.CreateBranch(dataDir, nodeID, "to-delete", "community-standards.md", "temp", "User", "u@e.com", "Temp branch")
	if err != nil {
		t.Fatal(err)
	}

	if err := governance.DeleteBranch(dataDir, nodeID, "to-delete"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}

	// Attempting to diff from deleted branch should error
	_, err = governance.HasChanges(dataDir, nodeID, "to-delete", "community-standards.md")
	if err == nil {
		t.Error("expected error when accessing deleted branch")
	}
}

// --- Diffing ---

func TestDiffStrings(t *testing.T) {
	oldText := "Hello world\nThis is a test.\n"
	newText := "Hello world\nThis is a modified test.\nNew line added.\n"

	result := governance.DiffStrings(oldText, newText, "aaa", "bbb", "test.md")

	if result.Filename != "test.md" {
		t.Errorf("expected filename test.md, got %s", result.Filename)
	}
	if result.FromSHA != "aaa" || result.ToSHA != "bbb" {
		t.Errorf("unexpected SHAs: from=%s to=%s", result.FromSHA, result.ToSHA)
	}
	if len(result.Hunks) == 0 {
		t.Fatal("expected at least one hunk")
	}

	hasInsert := false
	hasDelete := false
	hasEqual := false
	for _, h := range result.Hunks {
		switch h.Type {
		case "insert":
			hasInsert = true
		case "delete":
			hasDelete = true
		case "equal":
			hasEqual = true
		}
	}
	if !hasInsert {
		t.Error("expected at least one insert hunk")
	}
	if !hasDelete {
		t.Error("expected at least one delete hunk")
	}
	if !hasEqual {
		t.Error("expected at least one equal hunk")
	}
}

func TestGetDiff(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "get-diff"
	initInstanceAndNode(t, dataDir, nodeID)

	// Get initial SHA
	history, err := governance.GetHistory(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatal(err)
	}
	oldSHA := history[0].SHA

	// Edit
	newSHA, err := governance.DirectEdit(dataDir, nodeID, "community-standards.md", "changed content", "User", "u@e.com", "Change it")
	if err != nil {
		t.Fatal(err)
	}

	diff, err := governance.GetDiff(dataDir, nodeID, "community-standards.md", oldSHA, newSHA)
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}

	if diff.FromSHA != oldSHA || diff.ToSHA != newSHA {
		t.Errorf("unexpected SHAs in diff result")
	}
	if len(diff.Hunks) == 0 {
		t.Error("expected non-empty hunks")
	}

	// There should be changes (not all equal)
	allEqual := true
	for _, h := range diff.Hunks {
		if h.Type != "equal" {
			allEqual = false
			break
		}
	}
	if allEqual {
		t.Error("expected some non-equal hunks")
	}
}

func TestGetDiffFromBranch(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "diff-branch"
	initInstanceAndNode(t, dataDir, nodeID)

	branchContent := "# Completely new content\n"
	_, err := governance.CreateBranch(dataDir, nodeID, "diff-test", "community-standards.md", branchContent, "User", "u@e.com", "Branch change")
	if err != nil {
		t.Fatal(err)
	}

	diff, err := governance.GetDiffFromBranch(dataDir, nodeID, "diff-test", "community-standards.md")
	if err != nil {
		t.Fatalf("GetDiffFromBranch: %v", err)
	}

	if diff.Filename != "community-standards.md" {
		t.Errorf("unexpected filename: %s", diff.Filename)
	}

	// Should have changes
	hasNonEqual := false
	for _, h := range diff.Hunks {
		if h.Type != "equal" {
			hasNonEqual = true
			break
		}
	}
	if !hasNonEqual {
		t.Error("expected changes in diff from branch")
	}
}

func TestHasChanges(t *testing.T) {
	dataDir := tempDataDir(t)
	nodeID := "has-changes"
	initInstanceAndNode(t, dataDir, nodeID)

	// Branch with changes
	_, err := governance.CreateBranch(dataDir, nodeID, "changed", "community-standards.md", "different content", "User", "u@e.com", "Change")
	if err != nil {
		t.Fatal(err)
	}

	changed, err := governance.HasChanges(dataDir, nodeID, "changed", "community-standards.md")
	if err != nil {
		t.Fatalf("HasChanges: %v", err)
	}
	if !changed {
		t.Error("expected HasChanges to return true for branch with different content")
	}

	// Branch without changes (same content as main)
	mainContent, err := governance.GetDocument(dataDir, nodeID, "community-standards.md")
	if err != nil {
		t.Fatal(err)
	}
	_, err = governance.CreateBranch(dataDir, nodeID, "unchanged", "community-standards.md", mainContent, "User", "u@e.com", "No real change")
	if err != nil {
		t.Fatal(err)
	}

	unchanged, err := governance.HasChanges(dataDir, nodeID, "unchanged", "community-standards.md")
	if err != nil {
		t.Fatalf("HasChanges: %v", err)
	}
	if unchanged {
		t.Error("expected HasChanges to return false for branch with same content")
	}
}
