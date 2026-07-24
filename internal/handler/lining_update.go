package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// NodeLiningStatuses returns node_id → lining status (pristine/stale/diverged)
// for every lining row. Nodes without a lining row are absent; treat absent as
// pristine (AutoUpdateLinings creates missing rows at startup, so absence is a
// transient state, not a divergence).
func NodeLiningStatuses(db *database.DB) map[string]string {
	rows, err := db.Query("SELECT node_id, body FROM governance_docs WHERE kind = 'lining'")
	if err != nil {
		return map[string]string{}
	}
	defer rows.Close()
	statuses := map[string]string{}
	for rows.Next() {
		var nodeID, body string
		if rows.Scan(&nodeID, &body) != nil {
			continue
		}
		statuses[nodeID] = governance.LiningStatus(body)
	}
	return statuses
}

// hideAmendedLinings reports whether this request's discovery surfaces should
// omit amended-lining patches (docs/adr/037). Strictest wins: quilt policy
// (instance setting) hides for everyone including signed-out visitors; the
// per-user switch can hide when policy shows, never the reverse.
func hideAmendedLinings(db *database.DB, r *http.Request) bool {
	if v, ok := settings.Get(db, settings.KeyHideAmendedLinings); ok && v == "true" {
		return true
	}
	if user := middleware.UserFromContext(r.Context()); user != nil {
		var hide int
		db.QueryRow("SELECT hide_amended_linings FROM users WHERE id = ?", user.ID).Scan(&hide)
		return hide == 1
	}
	return false
}

// GetInstanceLining handles GET /api/v1/instance/lining — the current shipped
// lining text. Public: its most important reader is deciding whether to
// create a patch, and adoption should never be a surprise (docs/adr/037).
func GetInstanceLining(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"title":   DefaultLiningTitle,
			"body":    governance.CurrentLiningBody(),
			"version": governance.CurrentLiningVersion(),
		})
	}
}

// AutoUpdateLinings brings every live patch's lining to the current shipped
// text (docs/adr/037). Runs at startup, after the governance repo backfill.
// Two passes:
//
//  1. Patches with no lining row at all (nodes created before the lining
//     existed, or inserted by paths that bypass CreateNode) get one, pristine.
//  2. Stale linings — bodies matching an older shipped version or a legacy
//     pre-lineage draft — are rewritten to the current text, with a version
//     bump, a git mirror commit, and a notification to the patch's members.
//     Notified, never asked: a pristine patch agreed to the baseline,
//     whatever it currently says.
//
// Diverged linings (bodies matching no shipped version) are never touched —
// that patch amended its lining and owns the divergence.
//
// Returns (created, updated).
func AutoUpdateLinings(db *database.DB) (int, int, error) {
	rows, err := db.Query(
		`SELECT n.id, n.owner_id, n.slug, n.name FROM nodes n
		 WHERE n.status IN ('active','unclaimed') AND n.removed_at IS NULL
		   AND NOT EXISTS (SELECT 1 FROM governance_docs g WHERE g.node_id = n.id AND g.kind = 'lining')`)
	if err != nil {
		return 0, 0, fmt.Errorf("list nodes missing linings: %w", err)
	}
	type nodeRef struct{ id, ownerID, slug, name string }
	var missing []nodeRef
	for rows.Next() {
		var n nodeRef
		if err := rows.Scan(&n.id, &n.ownerID, &n.slug, &n.name); err != nil {
			rows.Close()
			return 0, 0, fmt.Errorf("scan node: %w", err)
		}
		missing = append(missing, n)
	}
	rows.Close()

	created := 0
	for _, n := range missing {
		CreateDefaultLining(db, n.id, n.ownerID)
		created++
	}

	rows, err = db.Query(
		`SELECT g.id, g.node_id, g.body, g.version, n.slug, n.name FROM governance_docs g
		 JOIN nodes n ON n.id = g.node_id AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL
		 WHERE g.kind = 'lining'`)
	if err != nil {
		return created, 0, fmt.Errorf("list linings: %w", err)
	}
	type staleRef struct {
		docID, nodeID, nodeSlug, nodeName string
		version                           int
	}
	var stale []staleRef
	for rows.Next() {
		var docID, nodeID, body, slug, name string
		var version int
		if err := rows.Scan(&docID, &nodeID, &body, &version, &slug, &name); err != nil {
			rows.Close()
			return created, 0, fmt.Errorf("scan lining: %w", err)
		}
		if governance.LiningStatus(body) == governance.LiningStale {
			stale = append(stale, staleRef{docID, nodeID, slug, name, version})
		}
	}
	rows.Close()

	current := governance.CurrentLiningBody()
	commitMsg := "The lining, v" + strconv.Itoa(governance.CurrentLiningVersion()) + " (shipped with Patchwork)"
	updated := 0
	for _, s := range stale {
		_, err := db.Exec(
			`UPDATE governance_docs SET body = ?, visibility = 'public', version = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
			current, s.version+1, s.docID,
		)
		if err != nil {
			return created, updated, fmt.Errorf("update lining %s: %w", s.docID, err)
		}
		updated++

		// Mirror to git, best effort like every other DB→git write.
		if dataDir := governance.GetDataDir(); dataDir != "" {
			if _, gitErr := governance.DirectEdit(dataDir, s.nodeID,
				governanceFilename(DefaultLiningTitle), current,
				"Patchwork", "patchwork@"+s.nodeSlug+".local", commitMsg); gitErr != nil {
				log.Printf("lining: git mirror for node %s: %v", s.nodeID, gitErr)
			}
		}

		// No audit entry: audit_log.user_id references users, and this is a
		// system action with no user. The git commit and the notification are
		// the record.
		notify(notifications.Event{
			Type:     notifications.LiningUpdated,
			NodeID:   s.nodeID,
			NodeSlug: s.nodeSlug,
			NodeName: s.nodeName,
			EntityID: s.docID,
			Title:    "The lining was updated",
			Link:     "/patches/" + s.nodeSlug + "/governance/docs/" + s.docID,
		})
	}

	return created, updated, nil
}
