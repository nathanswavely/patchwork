package handler

import (
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
// The template chosen at node creation is not persisted, so backfilled repos
// start from the default template; admins can re-run governance setup to
// change the rules.
func BackfillNodeGovernanceRepos(db *database.DB) (int, error) {
	dataDir := governance.GetDataDir()
	if dataDir == "" {
		return 0, fmt.Errorf("governance data dir not set")
	}

	rows, err := db.Query(`SELECT id FROM nodes WHERE status IN ('active','unclaimed') AND removed_at IS NULL`)
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
