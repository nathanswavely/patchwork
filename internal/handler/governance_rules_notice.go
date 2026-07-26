package handler

import (
	"fmt"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// syncRulesAndNotify writes a merged rules change into the node's config cache
// and tells the patch its rules moved.
//
// `governance.rules_changed` was declared in the notification registry when
// the registry was written and never once fired, so changing a patch's rules
// notified nobody — the one governance event that changes what every future
// vote means was the only one that went out silently.
//
// It also carries the news a vote in flight cannot: since docs/adr/047, an
// open proposal is judged by the terms it opened with, so a rules change does
// not reach the votes already running. Both halves of that need saying. The
// people who just lost eligibility will notice on their own; the people who
// just gained it and find no vote buttons on the proposals in front of them
// have been told the truth twice and shown a contradiction, and they are who
// this message is really for.
//
// Only the governance paths call this — a proposal applying, an amendment
// auto-applying, an admin's direct change. Setup-time syncs (claiming a patch,
// choosing a template, the startup backfill) establish rules rather than
// change them, and firing there would mail every member on every boot.
func syncRulesAndNotify(db *database.DB, dataDir, nodeID, actorID string) error {
	var before string
	db.QueryRow("SELECT COALESCE(governance_config,'') FROM nodes WHERE id = ?", nodeID).Scan(&before)

	if err := governance.SyncRulesToDB(db, dataDir, nodeID); err != nil {
		return err
	}

	var after string
	db.QueryRow("SELECT COALESCE(governance_config,'') FROM nodes WHERE id = ?", nodeID).Scan(&after)
	if after == before {
		// A merge that touched no rule is not a rules change. Amendments to
		// prose documents come through here too.
		return nil
	}

	var slug, name string
	db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&slug, &name)

	notify(notifications.Event{
		Type:     notifications.GovernanceRulesChanged,
		NodeID:   nodeID,
		NodeSlug: slug,
		NodeName: name,
		ActorID:  actorID,
		EntityID: nodeID,
		Title:    "Rules changed in " + name,
		Body:     rulesChangeBody(db, nodeID, after),
		Link:     weblink.PatchGovernance(slug),
	})
	return nil
}

// rulesChangeBody states the change, and names the votes it does not reach.
func rulesChangeBody(db *database.DB, nodeID, newConfig string) string {
	stillRunning := proposalsUnderOldTerms(db, nodeID, newConfig)
	if stillRunning == 0 {
		return "This patch's governance rules have changed."
	}
	if stillRunning == 1 {
		return "This patch's governance rules have changed. One vote is already " +
			"open and keeps the rules it started under, including who may vote in it."
	}
	return fmt.Sprintf(
		"This patch's governance rules have changed. %d votes are already open and "+
			"keep the rules they started under, including who may vote in them.",
		stillRunning,
	)
}

// proposalsUnderOldTerms counts the open votes whose terms no longer match the
// patch's rules — the ones a member reading the new rules would wrongly expect
// to be governed by them (docs/adr/047).
//
// A proposal with no terms of its own is not counted: it has nothing to
// diverge from, and it follows the live config like everything else.
func proposalsUnderOldTerms(db *database.DB, nodeID, newConfig string) int {
	rows, err := db.Query(
		`SELECT COALESCE(voting_terms,'') FROM proposals WHERE node_id = ? AND status = 'open'`,
		nodeID,
	)
	if err != nil {
		return 0
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var terms string
		if rows.Scan(&terms) != nil {
			continue
		}
		if terms != "" && terms != newConfig {
			count++
		}
	}
	return count
}
