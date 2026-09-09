package main

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"testing"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
)

// A seeded patch's governance_config cache says what its rules file says.
//
// The seed used to write the cache as a JSON literal per membership policy
// and fork a template chosen by the same policy — two copies of one answer,
// and they drifted. The invite-only literal said "consensus" while the
// minimal template it forked said "admin"; the approval-required literal
// said succession by "longest_tenure" while collaborative said "nomination".
// Nothing failed: the row inserted, the repo forked, and BackfillGovernanceConfig
// only fills empty caches, so the two stores disagreed for the life of the
// demo — the proposal flow read voting rules the rules editor never showed.
//
// The seed now fills the cache from the forked file the way CreateNode
// does. This test seeds a fresh database and checks every node twice: the
// cache against the node's own rules file, and the rules file's decision
// and leadership fields against the template the policy forks. The first
// is the invariant; the second is what keeps "forks minimal" from quietly
// becoming "forks casual" through ForkForNode's unknown-template fallback.
func TestSeededGovernanceConfigMatchesForkedTemplate(t *testing.T) {
	dataDir := t.TempDir()
	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("migrations fs: %v", err)
	}
	db, err := database.Open(filepath.Join(dataDir, "patchwork.db"), migrations)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ap.SetDomain("localhost")
	governance.SetDataDir(dataDir)
	if err := governance.InitInstanceRepo(dataDir); err != nil {
		t.Fatalf("init instance repo: %v", err)
	}

	s := newSeeder(db, dataDir, artsProfile())
	s.seedUsers()
	s.seedTags()
	s.seedNodes()

	rows, err := db.Query(`SELECT id, slug, membership_policy, COALESCE(governance_config,'') FROM nodes WHERE status = 'active'`)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	defer rows.Close()

	checked := 0
	policiesSeen := map[string]bool{}
	for rows.Next() {
		var id, slug, policy, cached string
		if err := rows.Scan(&id, &slug, &policy, &cached); err != nil {
			t.Fatalf("scan node: %v", err)
		}
		checked++
		policiesSeen[policy] = true

		// The invariant: the cache is what the node's own rules file marshals to.
		fileRules, err := governance.ReadRules(dataDir, id)
		if err != nil {
			t.Fatalf("%s: read rules: %v", slug, err)
		}
		fromFile, err := governance.MarshalConfig(fileRules)
		if err != nil {
			t.Fatalf("%s: marshal rules: %v", slug, err)
		}
		if cached != fromFile {
			t.Errorf("%s (%s): governance_config disagrees with governance-rules.json\n  cache: %s\n  file:  %s",
				slug, policy, cached, fromFile)
		}

		// The file carries the template the policy forks, in the fields that
		// decide how a proposal resolves and how an admin is made. Membership
		// policy and follower permissions are excluded on purpose: those are
		// the seed's choices, absorbed into the file exactly as the creation
		// form's are.
		templateName := templateForPolicy(policy)
		want, err := governance.TemplateRules(templateName)
		if err != nil {
			t.Fatalf("%s: template %q: %v", slug, templateName, err)
		}
		want.MembershipPolicy = fileRules.MembershipPolicy
		want.FollowerPermissions = fileRules.FollowerPermissions
		if got, exp := mustJSON(t, fileRules), mustJSON(t, want); got != exp {
			t.Errorf("%s (%s): governance-rules.json is not the %s template's\n  file:     %s\n  template: %s",
				slug, policy, templateName, got, exp)
		}

		// And the file agrees with the DB about the seed's own choices.
		if fileRules.MembershipPolicy != policy {
			t.Errorf("%s: rules file says membership_policy %q, row says %q", slug, fileRules.MembershipPolicy, policy)
		}
	}
	if checked == 0 {
		t.Fatal("seeded no active nodes; the check has nothing to stand on")
	}
	for _, p := range []string{"open", "approval_required", "invite_only"} {
		if !policiesSeen[p] {
			t.Errorf("no seeded patch uses membership_policy %q, so its template branch went unchecked", p)
		}
	}
}

// Every template the seed can name exists. ForkForNode falls back to casual
// for a name it does not know, so a typo here would fork the wrong rules and
// the test above would then faithfully confirm the cache matches them.
func TestSeedTemplatesExist(t *testing.T) {
	for _, policy := range []string{"open", "approval_required", "invite_only", ""} {
		name := templateForPolicy(policy)
		if _, err := governance.TemplateRules(name); err != nil {
			t.Errorf("policy %q maps to template %q: %v", policy, name, err)
		}
	}
	if _, err := governance.TemplateRules("no-such-template"); err == nil {
		t.Error("TemplateRules accepted an unknown template; the fallback belongs to ForkForNode alone")
	}
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
