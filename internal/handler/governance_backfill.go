package handler

import (
	"encoding/json"
	"fmt"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
)

// BackfillNodeGovernanceRepos creates governance repos for live nodes that
// don't have one, built from that node's own canonical governance_docs rows
// (docs/adr/011: the row is canonical, the repo file is its history mirror).
// Nodes normally get their repo at creation time; a missing repo means either
// that creation failed at runtime — e.g. the distroless container with no git
// binary, before repo init went pure go-git — or that the instance was
// restored from a database backup alone, which carries no repos at all
// (docs/adr/084). Returns the number of repos created.
//
// Scoped to active nodes only — unclaimed patches carry no governance repo
// at all (docs/adr/039); one is created only when a claim's setup completes.
//
// This is the strictly-create-missing half of governance.Repair: it never
// writes inside a repo that already exists, which is what makes it safe on
// every boot. The other half — adding a document an existing repo lacks and
// bringing a stale file current — is the operator's `-repair-governance`
// pass, because it commits into live history and should be somebody's
// decision rather than something a restart does.
func BackfillNodeGovernanceRepos(db *database.DB) (int, error) {
	rep, err := governance.Repair(db, governance.GetDataDir(), governance.RepairOptions{})
	if err != nil {
		return 0, err
	}
	_, created, _, _ := rep.Counts()
	return created, nil
}

// BackfillGovernanceConfig fills the governance_config cache for live nodes
// that have none. Until this sync existed, CreateNode forked the template's
// rules file but never cached it, so every DB read path saw voting defaults
// and the admin-decides fast-track never fired (docs/adr/041). For each such
// node the DB's live membership_policy — and follower_permissions where they
// were explicitly set — are absorbed into the rules file first: they were
// the enforced values while the cache was empty, and a blind sync would
// clobber them with template values. Nodes with a populated cache are left
// alone; their git and DB stores are already kept in sync by the amendment
// apply paths. Must run after BackfillNodeGovernanceRepos so every node has
// a repo. Active nodes only — unclaimed patches carry no governance repo at
// all (docs/adr/039), so there are no rules to sync until a claim's setup
// completes. Returns the number of nodes synced.
func BackfillGovernanceConfig(db *database.DB) (int, error) {
	dataDir := governance.GetDataDir()
	if dataDir == "" {
		return 0, fmt.Errorf("governance data dir not set")
	}

	// The two shapes a never-synced config can wear. Rows created before
	// migration 041 were stamped by it with the 013 schema DEFAULT plus the
	// default leadership block; rows created after carry the raw 013 DEFAULT
	// (SQLite can't change an existing column's default) whenever the
	// creation-time sync failed. The stamped string is also what a sync of
	// pure-default rules marshals, so a default-rules node re-syncs on each
	// startup — WriteRules skips the no-op git write, and the DB rewrite is
	// same-values, so the repeat is cheap and changes nothing.
	const migration013DefaultGC = `{"decision_method":"majority","quorum_percent":0,"default_vote_duration_hours":72,"amendment_threshold":"majority","amendment_auto_apply":true,"succession_policy":"longest_tenure","min_voting_tenure_days":0}`
	const migration041DefaultGC = `{"decision_method":"majority","quorum_percent":0,"default_vote_duration_hours":72,"amendment_threshold":"majority","amendment_auto_apply":true,"succession_policy":"longest_tenure","min_voting_tenure_days":0,"leadership_model":"maintainer","succession_method":"admin_nominate","max_admins":3,"inactivity_days":90}`

	rows, err := db.Query(`SELECT id, membership_policy, COALESCE(follower_permissions,'')
		FROM nodes WHERE status = 'active' AND removed_at IS NULL
		AND (governance_config IS NULL OR governance_config = '' OR governance_config = '{}' OR governance_config = ? OR governance_config = ?)`,
		migration013DefaultGC, migration041DefaultGC)
	if err != nil {
		return 0, fmt.Errorf("list nodes: %w", err)
	}
	type nodeRow struct {
		id, membershipPolicy, fpJSON string
	}
	var nodes []nodeRow
	for rows.Next() {
		var n nodeRow
		if err := rows.Scan(&n.id, &n.membershipPolicy, &n.fpJSON); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan node: %w", err)
		}
		nodes = append(nodes, n)
	}
	rows.Close()

	synced := 0
	for _, n := range nodes {
		rules, err := governance.ReadRules(dataDir, n.id)
		if err != nil {
			return synced, fmt.Errorf("read rules for node %s: %w", n.id, err)
		}
		if n.membershipPolicy != "" {
			rules.MembershipPolicy = n.membershipPolicy
		}
		if n.fpJSON != "" && n.fpJSON != "{}" {
			json.Unmarshal([]byte(n.fpJSON), &rules.FollowerPermissions)
		}
		if _, err := governance.WriteRules(dataDir, n.id, rules, "Backfill: absorb live membership settings"); err != nil {
			return synced, fmt.Errorf("write rules for node %s: %w", n.id, err)
		}
		if err := governance.SyncRulesToDB(db, dataDir, n.id); err != nil {
			return synced, fmt.Errorf("sync rules for node %s: %w", n.id, err)
		}
		synced++
	}
	return synced, nil
}
