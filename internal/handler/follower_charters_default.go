package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// CloseFollowerChartersDefault turns off `follower_permissions.charters` on
// every patch still carrying it from the shipped default, in the rules file
// as well as the cache column, and tells each patch's admins (docs/adr/116).
//
// Why a startup pass and not migration 075 alone: nodes.follower_permissions
// is a cache of governance-rules.json in the patch's own repo. SyncRulesToDB
// rewrites the column from that file whenever a rules change lands, so a
// migration that touched only the row would be quietly undone by the next
// unrelated amendment — the rules file is where a patch's rules live.
//
// Modelled on AutoUpdateLinings (docs/adr/037): a default the project owns
// moved, so it moves on every patch, at startup, announced rather than asked.
// The commit is authored "Patchwork System" like every other rules write the
// product makes, so the history does not show an admin editing rules at 3am.
//
// Idempotent by construction. WriteRules skips a file that already holds the
// content, and the second pass finds nothing to change, so this runs on every
// boot and does its work once.
//
// It never touches events, proposals or members. Those three govern what a
// follower sees of a patch's public life and are each patch's own business;
// charters is the one that hands over what the patch chose not to publish.
func CloseFollowerChartersDefault(db *database.DB) (int, error) {
	dataDir := governance.GetDataDir()

	rows, err := db.Query(
		`SELECT id, slug, name, COALESCE(follower_permissions, '')
		   FROM nodes
		  WHERE status = 'active' AND removed_at IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("list nodes: %w", err)
	}
	type patch struct{ id, slug, name, fp string }
	var all []patch
	for rows.Next() {
		var p patch
		if err := rows.Scan(&p.id, &p.slug, &p.name, &p.fp); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan node: %w", err)
		}
		all = append(all, p)
	}
	rows.Close()

	closed := 0
	var told []notifications.Event
	for _, p := range all {
		// The row and the rules file are checked separately: migration 075
		// has already fixed the rows, so on an upgraded instance the file is
		// the one still saying true, and on a restored-from-SQLite instance
		// (docs/adr/084) there may be no repo at all.
		rowGrants := false
		if p.fp != "" {
			var fp model.FollowerPermissions
			if json.Unmarshal([]byte(p.fp), &fp) == nil {
				rowGrants = fp.Charters
			}
		}

		fileGrants := false
		var rules *governance.GovernanceRules
		if dataDir != "" {
			if r, err := governance.ReadRules(dataDir, p.id); err == nil {
				rules = r
				fileGrants = r.FollowerPermissions.Charters
			}
		}

		if !rowGrants && !fileGrants {
			continue
		}

		if fileGrants && rules != nil {
			rules.FollowerPermissions.Charters = false
			if _, err := governance.WriteRules(dataDir, p.id, rules,
				"Followers no longer read members-only charters by default (shipped with Patchwork)"); err != nil {
				// Best effort, like every other DB→git write here. The row is
				// still closed below, which is what the gate reads.
				log.Printf("follower charters: git write for node %s: %v", p.id, err)
			}
		}

		if _, err := db.Exec(
			`UPDATE nodes SET follower_permissions = json_set(
			     CASE WHEN follower_permissions IS NULL OR follower_permissions = ''
			          THEN '{}' ELSE follower_permissions END,
			     '$.charters', json('false'))
			  WHERE id = ?`, p.id); err != nil {
			return closed, fmt.Errorf("close charters on node %s: %w", p.id, err)
		}
		closed++

		// No audit entry, for AutoUpdateLinings' reason: audit_log.user_id
		// references users and no user did this. The commit and the notice
		// are the record.
		told = append(told, notifications.Event{
			Type:     notifications.GovernanceFollowerChartersClosed,
			NodeID:   p.id,
			NodeSlug: p.slug,
			NodeName: p.name,
			// One literal each, not concatenated: the copy ledger reviews
			// what it can see, and a title assembled from three pieces
			// reviews as three fragments instead of the sentence somebody
			// reads.
			Title: fmt.Sprintf("Followers can no longer read %s's members-only charters", p.name),
			Body: "This patch was sharing its unpublished charters with followers, from a default Patchwork shipped rather than a choice this patch made. That default is now off. Published charters are unaffected, and you can grant it again under Follower Permissions, at the bottom of Propose a change to these rules. " +
				stillOpenToFollowers(rules, p.fp),
			Link: weblink.PatchGovernance(p.slug),
		})
	}

	notifyFollowerChartersClosed(told)
	return closed, nil
}

// notifyFollowerChartersClosed delivers the rollout's events with at most one
// notification per person, the way notifyLiningUpdates does: an admin of six
// patches hears once, with the count, not six times.
func notifyFollowerChartersClosed(events []notifications.Event) {
	if pkgNotifier == nil || len(events) == 0 {
		return
	}
	pkgNotifier.NotifyCoalesced(events, func(userEvents []notifications.Event) notifications.Event {
		if len(userEvents) == 1 {
			return userEvents[0]
		}
		e := userEvents[0]
		e.Title = fmt.Sprintf("Followers can no longer read members-only charters in %d of your patches", len(userEvents))
		return e
	})
}

// stillOpenToFollowers names the three keys this rollout did not touch.
//
// The notice said only what moved, and the admin who read it went looking
// for the rest: "Nobody sent me a notice about the other three. Followers
// can see our member list. That one I would want the co-op to have actually
// decided, and I have no memory of anybody deciding it. It sat there
// checked the whole time and no notice was ever sent about it. The one
// thing the site chose to write to me about is the one of the four I care
// least about."
//
// He is right that the other three were never decided either — they are
// migration 012's column default, the same default this rollout is closing
// for charters. Closing them is not this change's call to make, and three
// of them govern what a follower sees of a patch's *public* life rather
// than its private shelf. What the notice can do is stop leaving them
// unsaid, so the reader learns the state of the shelf rather than one tin
// on it.
//
// Reads the file's rules where there are any and the row otherwise, which
// is the same order the closing above uses.
func stillOpenToFollowers(rules *governance.GovernanceRules, rowFP string) string {
	fp := model.FollowerPermissions{}
	if rules != nil {
		fp = model.FollowerPermissions{
			Events:    rules.FollowerPermissions.Events,
			Proposals: rules.FollowerPermissions.Proposals,
			Members:   rules.FollowerPermissions.Members,
		}
	} else if rowFP != "" {
		json.Unmarshal([]byte(rowFP), &fp)
	}
	var open []string
	if fp.Events {
		open = append(open, "this patch's events")
	}
	if fp.Proposals {
		open = append(open, "its proposals")
	}
	if fp.Members {
		open = append(open, "its member list")
	}
	if len(open) == 0 {
		return "Followers here can now see only what any visitor can."
	}
	return "Followers here can still see " + joinWithAnd(open) + ", each of which is a separate switch in the same place."
}

// joinWithAnd reads a short list aloud.
func joinWithAnd(parts []string) string {
	switch len(parts) {
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
	}
}
