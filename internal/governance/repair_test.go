package governance_test

import (
	"io/fs"
	"os"
	"testing"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"

	"github.com/go-git/go-git/v5"
)

// --- Fixtures ---

func repairDB(t *testing.T) *database.DB {
	t.Helper()
	tmp, err := os.CreateTemp("", "patchwork-repair-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(tmp.Name(), migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

const (
	repairUserID = "019f0000-0000-7000-8000-000000000001"
	repairNodeID = "019f0000-0000-7000-8000-000000000002"
	repairSlug   = "selvage"
)

// seedPatch creates one active patch with one governance doc, and returns the
// doc's canonical body.
func seedPatch(t *testing.T, db *database.DB, title, body string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO users (id, username, display_name, role)
		VALUES (?, 'weaver', 'Weaver', 'member')`, repairUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO nodes (id, slug, name, owner_id, status)
		VALUES (?, ?, 'The Selvage', ?, 'active')`, repairNodeID, repairSlug, repairUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO governance_docs (id, node_id, title, body, created_by)
		VALUES (?, ?, ?, ?, ?)`,
		"019f0000-0000-7000-8000-000000000003", repairNodeID, title, body, repairUserID); err != nil {
		t.Fatal(err)
	}
}

// headSHA reads a repo's current HEAD commit.
func headSHA(t *testing.T, dataDir, nodeID string) string {
	t.Helper()
	repo, err := git.PlainOpen(governance.NodeRepoPath(dataDir, nodeID))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	ref, err := repo.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	return ref.Hash().String()
}

func fullRepair(t *testing.T, db *database.DB, dataDir string) *governance.Report {
	t.Helper()
	rep, err := governance.Repair(db, dataDir, governance.RepairOptions{RestoreExisting: true})
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}
	return rep
}

// --- A healthy instance is left alone ---

func TestRepair_HealthyInstanceUntouched(t *testing.T) {
	dataDir := tempDataDir(t)
	db := repairDB(t)
	seedPatch(t, db, governance.DefaultLiningTitle, governance.CurrentLiningBody())
	initInstanceAndNode(t, dataDir, repairNodeID)

	// Bring the repo in line the way node creation does, so the fixture is a
	// genuinely healthy instance rather than one that merely has a repo.
	if _, err := governance.DirectEdit(dataDir, repairNodeID, "community-standards.md",
		governance.CurrentLiningBody(), "Patchwork System", "system@patchwork.local",
		"Create lining"); err != nil {
		t.Fatalf("DirectEdit: %v", err)
	}

	before := headSHA(t, dataDir, repairNodeID)

	rep := fullRepair(t, db, dataDir)

	if after := headSHA(t, dataDir, repairNodeID); after != before {
		t.Errorf("HEAD moved on a healthy repo: %s -> %s", before, after)
	}
	checked, created, updated, unchanged := rep.Counts()
	if checked != 1 || created != 0 || updated != 0 || unchanged != 1 {
		t.Errorf("counts = checked %d, created %d, updated %d, unchanged %d; want 1/0/0/1",
			checked, created, updated, unchanged)
	}
	if rep.InstanceRepo != governance.RepoUnchanged {
		t.Errorf("instance repo status = %q, want unchanged", rep.InstanceRepo)
	}

	// And a second pass changes nothing either.
	fullRepair(t, db, dataDir)
	if after := headSHA(t, dataDir, repairNodeID); after != before {
		t.Errorf("HEAD moved on the second pass: %s -> %s", before, after)
	}
}

// --- A missing repo is rebuilt from the database ---

func TestRepair_MissingRepoRebuiltFromDatabase(t *testing.T) {
	dataDir := tempDataDir(t)
	db := repairDB(t)
	const body = "The community agreed this text, and only the database still has it.\n"
	seedPatch(t, db, "Operating Agreement", body)

	// A database-only restore: rows, no repos at all.
	rep := fullRepair(t, db, dataDir)

	if rep.InstanceRepo != governance.RepoCreated {
		t.Errorf("instance repo status = %q, want created", rep.InstanceRepo)
	}
	if _, created, _, _ := rep.Counts(); created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}
	if rep.Repos[0].Slug != repairSlug {
		t.Errorf("report slug = %q, want %q", rep.Repos[0].Slug, repairSlug)
	}

	got, err := governance.GetDocument(dataDir, repairNodeID, "operating-agreement.md")
	if err != nil {
		t.Fatalf("GetDocument after rebuild: %v", err)
	}
	if got != body {
		t.Errorf("rebuilt body = %q, want %q", got, body)
	}

	// The rules file rides along, so amendment writes and ReadRules work.
	if _, err := governance.GetDocument(dataDir, repairNodeID, "governance-rules.json"); err != nil {
		t.Errorf("rebuilt repo has no rules file: %v", err)
	}

	// A second pass is a no-op.
	before := headSHA(t, dataDir, repairNodeID)
	fullRepair(t, db, dataDir)
	if after := headSHA(t, dataDir, repairNodeID); after != before {
		t.Errorf("second pass moved HEAD: %s -> %s", before, after)
	}
}

// A rebuilt repo must be readable as rebuilt — the whole point of not doing
// this silently.
func TestRepair_RebuiltMarkerInHistory(t *testing.T) {
	dataDir := tempDataDir(t)
	db := repairDB(t)
	seedPatch(t, db, "Operating Agreement", "Body.\n")

	fullRepair(t, db, dataDir)

	history, err := governance.GetHistory(dataDir, repairNodeID, "operating-agreement.md")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history has %d entries, want 1 — a rebuild is one commit, not a fabricated run of them", len(history))
	}
	c := history[0]
	if c.Repair != governance.RepairRebuilt {
		t.Errorf("repair marker = %q, want %q", c.Repair, governance.RepairRebuilt)
	}
	if c.AuthorName != governance.RepairAuthorName {
		t.Errorf("author = %q, want %q", c.AuthorName, governance.RepairAuthorName)
	}
	if c.Message != governance.RebuildMessage {
		t.Errorf("message = %q, want %q", c.Message, governance.RebuildMessage)
	}
}

// The rules a patch was actually running come back, not the template's.
func TestRepair_RebuildKeepsTheRulesInForce(t *testing.T) {
	dataDir := tempDataDir(t)
	db := repairDB(t)
	seedPatch(t, db, "Operating Agreement", "Body.\n")
	if _, err := db.Exec(
		`UPDATE nodes SET governance_config = ?, membership_policy = 'approval_required' WHERE id = ?`,
		`{"leadership_model":"elected","quorum_percent":40,"admin_term_months":12}`, repairNodeID,
	); err != nil {
		t.Fatal(err)
	}

	fullRepair(t, db, dataDir)

	rules, err := governance.ReadRules(dataDir, repairNodeID)
	if err != nil {
		t.Fatalf("ReadRules: %v", err)
	}
	if rules.LeadershipModel != "elected" {
		t.Errorf("leadership_model = %q, want elected — a rebuild that reset this would end the patch's elections", rules.LeadershipModel)
	}
	if rules.QuorumPercent != 40 {
		t.Errorf("quorum_percent = %d, want 40", rules.QuorumPercent)
	}
	if rules.MembershipPolicy != "approval_required" {
		t.Errorf("membership_policy = %q, want approval_required", rules.MembershipPolicy)
	}
	// Untouched fields keep their defaults rather than going empty.
	if rules.DecisionMethod != "majority" {
		t.Errorf("decision_method = %q, want majority", rules.DecisionMethod)
	}
}

// --- An existing repo that has drifted ---

func TestRepair_StaleFileBroughtCurrent(t *testing.T) {
	dataDir := tempDataDir(t)
	db := repairDB(t)
	const current = "The text the community actually voted in.\n"
	seedPatch(t, db, governance.DefaultLiningTitle, current)
	initInstanceAndNode(t, dataDir, repairNodeID)

	// The repo carries the shipped baseline; the canonical row has moved on.
	stale, err := governance.GetDocument(dataDir, repairNodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if stale == current {
		t.Fatal("fixture is not stale")
	}
	before := headSHA(t, dataDir, repairNodeID)

	rep := fullRepair(t, db, dataDir)

	if _, _, updated, _ := rep.Counts(); updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}
	got, err := governance.GetDocument(dataDir, repairNodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("GetDocument after repair: %v", err)
	}
	if got != current {
		t.Errorf("body after repair = %q, want %q", got, current)
	}

	history, err := governance.GetHistory(dataDir, repairNodeID, "community-standards.md")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) < 2 {
		t.Fatalf("history has %d entries, want the original plus the restore", len(history))
	}
	if history[0].Repair != governance.RepairRestored {
		t.Errorf("newest entry repair = %q, want %q", history[0].Repair, governance.RepairRestored)
	}
	if history[0].AuthorName != governance.RepairAuthorName {
		t.Errorf("author = %q, want %q", history[0].AuthorName, governance.RepairAuthorName)
	}
	// The history that existed is still there, unlabelled.
	if history[len(history)-1].Repair != "" {
		t.Errorf("oldest entry carries a repair marker it should not: %q", history[len(history)-1].Repair)
	}
	if history[len(history)-1].SHA != before {
		t.Errorf("original commit lost: oldest is %s, was %s", history[len(history)-1].SHA, before)
	}
}

// A document the database has and the repo does not is added; the repo's own
// files survive.
func TestRepair_MissingDocumentAddedWithoutLosingOthers(t *testing.T) {
	dataDir := tempDataDir(t)
	db := repairDB(t)
	seedPatch(t, db, governance.DefaultLiningTitle, governance.CurrentLiningBody())
	initInstanceAndNode(t, dataDir, repairNodeID)
	if _, err := governance.DirectEdit(dataDir, repairNodeID, "community-standards.md",
		governance.CurrentLiningBody(), "Patchwork System", "system@patchwork.local",
		"Create lining"); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`INSERT INTO governance_docs (id, node_id, title, body, created_by)
		VALUES (?, ?, 'Conflict Resolution', 'How we handle it.', ?)`,
		"019f0000-0000-7000-8000-000000000004", repairNodeID, repairUserID); err != nil {
		t.Fatal(err)
	}

	fullRepair(t, db, dataDir)

	got, err := governance.GetDocument(dataDir, repairNodeID, "conflict-resolution.md")
	if err != nil {
		t.Fatalf("added doc not in repo: %v", err)
	}
	if got != "How we handle it." {
		t.Errorf("added body = %q", got)
	}
	// The casual template's own file is still there — repair never deletes.
	if _, err := governance.GetDocument(dataDir, repairNodeID, "operating-agreement.md"); err != nil {
		t.Errorf("repair dropped a file it did not put there: %v", err)
	}
	// And the document that was already current gained no commit.
	lining, err := governance.GetHistory(dataDir, repairNodeID, "community-standards.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range lining {
		if c.Repair != "" {
			t.Errorf("an already-current document was committed to by the repair: %q", c.Message)
		}
	}
}

// --- The startup half ---

// The create-missing pass is what runs on every boot, so it must never write
// into a repo that is already there, however stale it is.
func TestRepair_CreateMissingOnlyLeavesExistingRepos(t *testing.T) {
	dataDir := tempDataDir(t)
	db := repairDB(t)
	seedPatch(t, db, governance.DefaultLiningTitle, "A body the repo does not have.\n")
	initInstanceAndNode(t, dataDir, repairNodeID)
	before := headSHA(t, dataDir, repairNodeID)

	rep, err := governance.Repair(db, dataDir, governance.RepairOptions{})
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}

	if after := headSHA(t, dataDir, repairNodeID); after != before {
		t.Errorf("create-missing pass wrote into an existing repo: %s -> %s", before, after)
	}
	if _, created, updated, unchanged := rep.Counts(); created != 0 || updated != 0 || unchanged != 1 {
		t.Errorf("counts = created %d, updated %d, unchanged %d; want 0/0/1", created, updated, unchanged)
	}
}

func TestRepair_CreateMissingStillCreates(t *testing.T) {
	dataDir := tempDataDir(t)
	db := repairDB(t)
	seedPatch(t, db, "Operating Agreement", "Body.\n")

	rep, err := governance.Repair(db, dataDir, governance.RepairOptions{})
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}
	if _, created, _, _ := rep.Counts(); created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}
	if _, err := governance.GetDocument(dataDir, repairNodeID, "operating-agreement.md"); err != nil {
		t.Errorf("GetDocument after startup heal: %v", err)
	}
}

// Unclaimed patches carry no governance repo at all (docs/adr/039); the
// repair must not mint one for them.
func TestRepair_SkipsUnclaimedPatches(t *testing.T) {
	dataDir := tempDataDir(t)
	db := repairDB(t)
	seedPatch(t, db, "Operating Agreement", "Body.\n")
	if _, err := db.Exec(`UPDATE nodes SET status = 'unclaimed' WHERE id = ?`, repairNodeID); err != nil {
		t.Fatal(err)
	}

	rep := fullRepair(t, db, dataDir)

	if checked, _, _, _ := rep.Counts(); checked != 0 {
		t.Errorf("checked = %d, want 0", checked)
	}
	if _, err := os.Stat(governance.NodeRepoPath(dataDir, repairNodeID)); err == nil {
		t.Error("repair created a repo for an unclaimed patch")
	}
}

// The repair is a governance write path like any other and must work in the
// distroless image, which ships no git binary.
func TestRepair_Gitless(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("GIT_EXEC_PATH", "")

	dataDir := tempDataDir(t)
	db := repairDB(t)
	seedPatch(t, db, "Operating Agreement", "Body.\n")

	fullRepair(t, db, dataDir)

	if _, err := governance.GetDocument(dataDir, repairNodeID, "operating-agreement.md"); err != nil {
		t.Errorf("GetDocument after gitless rebuild: %v", err)
	}
}

func TestFilename(t *testing.T) {
	cases := map[string]string{
		"Community Standards":  "community-standards.md",
		"Operating Agreement!": "operating-agreement.md",
		"Bylaws 2026":          "bylaws-2026.md",
	}
	for title, want := range cases {
		if got := governance.Filename(title); got != want {
			t.Errorf("Filename(%q) = %q, want %q", title, got, want)
		}
	}
}
