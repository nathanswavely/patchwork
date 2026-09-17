package handler_test

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// docs/adr/116. The charters key hands a follower the charters a patch chose
// not to publish, and it shipped on: on by column default, by DefaultRules,
// and by three of the four templates. Following costs nothing and asks nobody,
// so on an invite-only patch that made the private shelf readable to anyone
// who clicked Follow. These tests hold the default down in each of the places
// it lived, because one of them staying true puts the whole thing back.

func chartersFromRow(t *testing.T, db *database.DB, nodeID string) bool {
	t.Helper()
	var raw string
	if err := db.QueryRow("SELECT COALESCE(follower_permissions,'') FROM nodes WHERE id = ?", nodeID).Scan(&raw); err != nil {
		t.Fatalf("read follower_permissions: %v", err)
	}
	var fp model.FollowerPermissions
	json.Unmarshal([]byte(raw), &fp)
	return fp.Charters
}

func TestShippedDefaultsWithholdMembersOnlyCharters(t *testing.T) {
	if governance.DefaultRules().FollowerPermissions.Charters {
		t.Error("DefaultRules still grants followers the members-only shelf")
	}
	for _, name := range []string{"minimal", "casual", "collaborative", "formal"} {
		rules, err := governance.TemplateRules(name)
		if err != nil {
			t.Fatalf("template %s: %v", name, err)
		}
		if rules.FollowerPermissions.Charters {
			t.Errorf("template %q still grants followers the members-only shelf", name)
		}
	}
}

// Migration 075 closes the rows, and moves only the charters key. Re-run
// against a legacy-shaped row: the shipped default is what every patch that
// existed before this carried, because migration 012's ALTER TABLE wrote it
// into all of them.
func TestMigration075ClosesOnlyTheChartersGrant(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "fc-owner", "member")
	nodeID := createTestNode(t, db, owner.ID, "Inherited", "inherited", "invite_only")

	legacy := `{"events":true,"proposals":true,"charters":true,"members":true}`
	if _, err := db.Exec("UPDATE nodes SET follower_permissions = ? WHERE id = ?", legacy, nodeID); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	sql, err := patchwork.MigrationsFS.ReadFile("migrations/075_follower_charters_off_by_default.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	run := func() {
		t.Helper()
		if _, err := db.Exec(string(sql)); err != nil {
			t.Fatalf("run migration 075: %v", err)
		}
	}

	run()
	if chartersFromRow(t, db, nodeID) {
		t.Error("migration 075 left the inherited grant in place")
	}

	var raw string
	db.QueryRow("SELECT follower_permissions FROM nodes WHERE id = ?", nodeID).Scan(&raw)
	var fp model.FollowerPermissions
	json.Unmarshal([]byte(raw), &fp)
	if !fp.Events || !fp.Proposals || !fp.Members {
		t.Errorf("migration 075 touched a key that was not charters: %s", raw)
	}

	// Migrations run once, but a re-run must not be destructive: this one is
	// a plain UPDATE and the suite asserts that rather than assuming it.
	run()
	if chartersFromRow(t, db, nodeID) {
		t.Error("migration 075 is not idempotent")
	}
}

// The startup pass is the backstop and the durable half: the column is a cache
// of the rules file, so closing only the row would be undone by the next
// amendment. It must also be safe to run on every boot.
func TestCloseFollowerChartersDefaultIsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	owner, _ := createTestUser(t, db, "fc-owner3", "member")
	nodeID := createTestNode(t, db, owner.ID, "Reopened", "reopened", "open")

	// Something put the grant back: a rules sync from a repo that still says
	// true, a restore, an old import.
	if _, err := db.Exec(
		`UPDATE nodes SET follower_permissions = '{"events":true,"proposals":true,"charters":true,"members":true}' WHERE id = ?`,
		nodeID); err != nil {
		t.Fatalf("reopen: %v", err)
	}

	closed, err := handler.CloseFollowerChartersDefault(db)
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if closed != 1 {
		t.Errorf("expected the pass to close 1 patch, closed %d", closed)
	}
	if chartersFromRow(t, db, nodeID) {
		t.Error("the pass ran and the grant is still there")
	}

	closedAgain, err := handler.CloseFollowerChartersDefault(db)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if closedAgain != 0 {
		t.Errorf("the pass is not idempotent: closed %d on a second run", closedAgain)
	}
}

// The end of it, stated as the thing a person would notice: a follower on an
// invite-only patch, which is the case that made this worth fixing.
func TestFollowerOnAnInviteOnlyPatchCannotReadItsPrivateCharters(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "fc-admin", "member")
	fran, franToken := createTestUser(t, db, "fc-fran", "member")
	nodeID := createTestNode(t, db, admin.ID, "Closed Doors", "closed-doors", "invite_only")
	createTestMembership(t, db, admin.ID, nodeID, "admin", "active")
	createTestMembership(t, db, fran.ID, nodeID, "follower", "active")

	docID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO governance_docs (id, node_id, title, body, created_by, visibility) VALUES (?, ?, 'House Rules', 'members only body', ?, 'members')`,
		docID, nodeID, admin.ID); err != nil {
		t.Fatalf("insert doc: %v", err)
	}

	r := authedRequest("GET", "/api/v1/governance/"+docID, nil, franToken)
	w := serveMux(t, db, "GET", "/api/v1/governance/{id}", handler.GetGovernanceDoc(db), r)
	if w.Code != http.StatusNotFound {
		t.Errorf("a follower read an invite-only patch's unpublished charter: %d %s", w.Code, w.Body.String())
	}

	// The grant is still a grant: a patch that asks for it gets it.
	if _, err := db.Exec(
		`UPDATE nodes SET follower_permissions = '{"events":true,"proposals":true,"charters":true,"members":true}' WHERE id = ?`,
		nodeID); err != nil {
		t.Fatalf("grant: %v", err)
	}
	r2 := authedRequest("GET", "/api/v1/governance/"+docID, nil, franToken)
	w2 := serveMux(t, db, "GET", "/api/v1/governance/{id}", handler.GetGovernanceDoc(db), r2)
	if w2.Code != http.StatusOK {
		t.Errorf("a patch that grants charters should still share them: %d %s", w2.Code, w2.Body.String())
	}
}

// Migration 012's column DEFAULT still grants charters and is deliberately not
// rewritten: changing a default in SQLite means rebuilding `nodes`, which is 42
// columns and 18 inbound foreign keys, and no insert path relies on it. This is
// what makes that claim true rather than hopeful. An INSERT INTO nodes that
// omits the column silently reopens the grant until the next boot, so a new one
// fails here instead.
func TestNoInsertIntoNodesInheritsTheColumnDefault(t *testing.T) {
	roots := []string{"../../internal", "../../cmd", "../../migrations"}
	var offenders []string

	for _, root := range roots {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			if !strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, ".sql") {
				return nil
			}
			// The rebuild in 009 predates the column, and the tests here are
			// the ones asserting the rule.
			if strings.HasSuffix(name, "_test.go") || name == "009_flatten_and_simplify.sql" {
				return nil
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			text := string(body)
			for _, stmt := range insertsIntoNodes(text) {
				if !strings.Contains(stmt, "follower_permissions") {
					offenders = append(offenders, filepath.ToSlash(path)+": "+firstLine(stmt))
				}
			}
			return nil
		})
	}

	for _, o := range offenders {
		t.Errorf("INSERT INTO nodes without follower_permissions — it would inherit migration 012's charters:true (docs/adr/116):\n  %s", o)
	}
}

// insertsIntoNodes returns each `INSERT INTO nodes (...)` column list in text.
func insertsIntoNodes(text string) []string {
	var out []string
	re := regexp.MustCompile(`(?is)INSERT\s+(?:OR\s+\w+\s+)?INTO\s+nodes\s*\(([^)]*)\)`)
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	return out
}

func firstLine(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	return s
}
