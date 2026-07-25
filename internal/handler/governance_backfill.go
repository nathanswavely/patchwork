package handler

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
)

// BackfillNodeGovernanceRepos creates governance repos for live nodes that
// don't have one, then mirrors each node's DB-canonical governance docs into
// the fresh repo (docs/adr/011: the governance_docs row is canonical, the git
// file is its history mirror). Nodes normally get their repo at creation
// time; a missing repo means creation failed at runtime — e.g. the gitless
// distroless container before repo init went pure go-git. Returns the number
// of repos created.
//
// Scoped to active nodes only — unclaimed patches carry no governance repo
// at all (docs/adr/039); one is created only when a claim's setup completes.
//
// The template chosen at node creation is not persisted, so backfilled repos
// start from the default template; admins can re-run governance setup to
// change the rules.
func BackfillNodeGovernanceRepos(db *database.DB) (int, error) {
	dataDir := governance.GetDataDir()
	if dataDir == "" {
		return 0, fmt.Errorf("governance data dir not set")
	}

	rows, err := db.Query(`SELECT id FROM nodes WHERE status = 'active' AND removed_at IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("list nodes: %w", err)
	}
	var nodeIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan node: %w", err)
		}
		nodeIDs = append(nodeIDs, id)
	}
	rows.Close()

	created := 0
	for _, nodeID := range nodeIDs {
		if _, err := os.Stat(governance.NodeRepoPath(dataDir, nodeID)); err == nil {
			continue
		}
		if err := governance.ForkForNode(dataDir, nodeID, ""); err != nil {
			return created, fmt.Errorf("fork for node %s: %w", nodeID, err)
		}
		created++

		if err := mirrorDocsToRepo(db, dataDir, nodeID); err != nil {
			return created, err
		}
	}
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

// mirrorDocsToRepo writes a node's governance_docs bodies into its git repo
// wherever the repo content differs from the canonical DB row.
func mirrorDocsToRepo(db *database.DB, dataDir, nodeID string) error {
	rows, err := db.Query(`SELECT title, body FROM governance_docs WHERE node_id = ?`, nodeID)
	if err != nil {
		return fmt.Errorf("list governance docs for node %s: %w", nodeID, err)
	}
	defer rows.Close()

	type doc struct{ title, body string }
	var docs []doc
	for rows.Next() {
		var d doc
		if err := rows.Scan(&d.title, &d.body); err != nil {
			return fmt.Errorf("scan governance doc: %w", err)
		}
		docs = append(docs, d)
	}

	for _, d := range docs {
		filename := governanceFilename(d.title)
		if cur, err := governance.GetDocument(dataDir, nodeID, filename); err == nil && cur == d.body {
			continue
		}
		_, err := governance.DirectEdit(dataDir, nodeID, filename, d.body,
			"Patchwork System", "system@patchwork.local", "Backfill "+d.title+" from database")
		if err != nil {
			return fmt.Errorf("mirror doc %q for node %s: %w", d.title, nodeID, err)
		}
	}
	return nil
}
