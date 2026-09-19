package handler

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// A governance repo is handed over whole, so a commit's author travels with it
// (docs/adr/110, docs/adr/116). Four call sites passed user.Email into the
// signature, which put members' real addresses in a file the transport serves —
// the one thing the personal export, the member seamrip and every API surface
// each refuse to hand over.
func TestCommitIdentityNeverCarriesARealAddress(t *testing.T) {
	u := &model.User{
		Username:    "harriet",
		DisplayName: "Harriet",
		Email:       "harriet@her-real-mailbox.example",
	}
	name, email := commitIdentity(u)
	if name != "Harriet" {
		t.Errorf("name: want the display name, got %q", name)
	}
	if strings.Contains(email, "her-real-mailbox") {
		t.Errorf("commit email leaks the real address: %q", email)
	}
	if email != "harriet@patchwork.local" {
		t.Errorf("email: want harriet@patchwork.local, got %q", email)
	}

	// A tombstoned account has its display name emptied and keeps its
	// username, so the fallback is the one that has to hold (docs/adr/086).
	deleted := &model.User{Username: "gone", Email: ""}
	name, email = commitIdentity(deleted)
	if name != "gone" || email != "gone@patchwork.local" {
		t.Errorf("tombstone fallback: got %q / %q", name, email)
	}
}

// And the rule is kept by every caller, not just the ones fixed once. A commit
// author is a per-person value that no reviewer would look twice at.
func TestNoGovernanceCommitIsAuthoredWithAUserEmail(t *testing.T) {
	call := regexp.MustCompile(`governance\.(DirectEdit|CreateBranch|MergeBranch)\((?s).*?\)`)
	leak := regexp.MustCompile(`\b\w+\.Email\b`)

	filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for _, stmt := range call.FindAllString(string(body), -1) {
			if leak.MatchString(stmt) {
				t.Errorf("%s: a governance commit is authored with a real address — use commitIdentity (docs/adr/116):\n  %s",
					filepath.ToSlash(path), strings.Join(strings.Fields(stmt), " "))
			}
		}
		return nil
	})
}
