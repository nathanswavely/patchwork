package notifications

import (
	"encoding/json"
	"log"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// Inactivity and succession (docs/adr/051).
//
// The shipped succession plan says what happens to an admin who stops turning
// up: "Council members who have not participated in governance (votes,
// proposals, discussions) for 30 consecutive days are contacted... Day 60: if
// still inactive, the seat is declared vacant and succession procedures
// begin." Both `inactivity_days` and `succession_policy` were among the six
// fields docs/adr/049 found stored, rendered, and read by nothing.
//
// The two are a pair, and that is what makes `succession_policy` reachable at
// last. The last-admin floor stops a patch being *voluntarily* stranded —
// nobody may leave, be banned, or be demoted out of the last seat. Inactivity
// is the opposite case: the patch is already stranded, its one admin having
// vanished months ago, and refusing to vacate the seat protects nothing but
// the absence. So inactivity may empty a patch, and succession is what
// catches it in the same sweep.

// governanceRules is the slice of a node's config this sweep needs.
type governanceRules struct {
	InactivityDays   int    `json:"inactivity_days"`
	SuccessionPolicy string `json:"succession_policy"`
}

// SweepInactiveAdmins warns admins who have gone quiet, vacates the seats of
// those who stayed quiet twice as long, and runs succession on any patch left
// without an admin. Exported so tests can trigger it directly, the same shape
// as ExpireStaleClaims.
func SweepInactiveAdmins(n *Notifier) {
	db := n.DB
	rows, err := db.Query(`SELECT id, slug, name, COALESCE(governance_config,'{}')
	                       FROM nodes WHERE status = 'active' AND removed_at IS NULL`)
	if err != nil {
		log.Printf("inactivity: list nodes: %v", err)
		return
	}
	type nodeRow struct{ id, slug, name, gc string }
	var nodes []nodeRow
	for rows.Next() {
		var nr nodeRow
		if rows.Scan(&nr.id, &nr.slug, &nr.name, &nr.gc) == nil {
			nodes = append(nodes, nr)
		}
	}
	rows.Close()

	for _, nr := range nodes {
		var rules governanceRules
		if json.Unmarshal([]byte(nr.gc), &rules) != nil || rules.InactivityDays <= 0 {
			continue
		}
		sweepNode(n, nr.id, nr.slug, nr.name, rules)
	}
}

func sweepNode(n *Notifier, nodeID, slug, name string, rules governanceRules) {
	db := n.DB
	warnBefore := time.Now().UTC().AddDate(0, 0, -rules.InactivityDays).Format("2006-01-02T15:04:05.000Z")
	vacateBefore := time.Now().UTC().AddDate(0, 0, -rules.InactivityDays*2).Format("2006-01-02T15:04:05.000Z")

	// Last time each admin took part in governance. The succession plan names
	// the three things that count — votes, proposals, discussions — so posting
	// events or editing the patch is deliberately not participation here. It
	// is a seat on the council that goes quiet, not an account.
	//
	// joined_at is the floor: someone who joined last week has not been absent
	// for thirty days, whatever their empty activity record suggests.
	adminRows, err := db.Query(`
		SELECT m.id, m.user_id, MAX(COALESCE(act.at, m.joined_at)) AS last_at
		FROM memberships m
		LEFT JOIN (
			SELECT v.user_id, p.node_id, v.created_at AS at FROM votes v
			  JOIN proposals p ON p.id = v.proposal_id
			UNION ALL
			SELECT author_id, node_id, created_at FROM proposals
			UNION ALL
			SELECT c.author_id, p2.node_id, c.created_at
			  FROM proposal_comments c JOIN proposals p2 ON p2.id = c.proposal_id
		) act ON act.user_id = m.user_id AND act.node_id = m.node_id
		WHERE m.node_id = ? AND m.role = 'admin' AND m.status = 'active'
		GROUP BY m.id, m.user_id`, nodeID)
	if err != nil {
		log.Printf("inactivity: %s: %v", slug, err)
		return
	}
	type adminRow struct{ memID, userID, lastAt string }
	var admins []adminRow
	for adminRows.Next() {
		var a adminRow
		if adminRows.Scan(&a.memID, &a.userID, &a.lastAt) == nil {
			admins = append(admins, a)
		}
	}
	adminRows.Close()

	var vacatedUsers []string
	for _, a := range admins {
		switch {
		case a.lastAt < vacateBefore:
			if vacateSeat(n, nodeID, slug, name, a.memID, a.userID) {
				vacatedUsers = append(vacatedUsers, a.userID)
			}
		case a.lastAt < warnBefore:
			warnInactive(n, nodeID, slug, name, a.memID, a.userID, rules.InactivityDays)
		default:
			// Back in the room: clear the warning so a future absence starts
			// its own cycle rather than being silently skipped.
			db.Exec(`DELETE FROM notification_reminders_sent
			         WHERE entity_type = 'membership' AND entity_id = ? AND reminder_type = 'inactivity_warning'`, a.memID)
		}
	}

	if len(vacatedUsers) == 0 {
		return
	}
	var remaining int
	db.QueryRow(`SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'`, nodeID).Scan(&remaining)
	if remaining == 0 {
		runSuccession(n, nodeID, slug, name, rules.SuccessionPolicy, vacatedUsers)
	}
}

// warnInactive tells someone their seat is at risk, once per absence.
func warnInactive(n *Notifier, nodeID, slug, name, memID, userID string, days int) {
	var already int
	n.DB.QueryRow(`SELECT COUNT(*) FROM notification_reminders_sent
	               WHERE entity_type = 'membership' AND entity_id = ? AND reminder_type = 'inactivity_warning'`, memID).Scan(&already)
	if already > 0 {
		return
	}
	n.DB.Exec(`INSERT OR IGNORE INTO notification_reminders_sent (id, entity_type, entity_id, reminder_type)
	           VALUES (?, 'membership', ?, 'inactivity_warning')`, auth.NewUUIDv7(), memID)

	n.Notify(Event{
		Type: GovernanceInactivityWarning, NodeID: nodeID, NodeSlug: slug, NodeName: name,
		TargetID: userID,
		Title:    "Your admin seat in " + name + " is inactive",
		Body:     "You have not taken part in governance here for " + itoaDays(days) + ". Vote, propose, or comment to keep the seat; otherwise it is declared vacant after twice that long.",
		Link:     weblink.PatchGovernance(slug),
	})
}

// vacateSeat declares a seat vacant. Reports whether it actually went.
//
// The last admin is not exempt here, unlike every voluntary path. A patch
// whose only admin stopped participating two inactivity periods ago has no
// working administrator already; keeping the seat filled on paper protects
// the absence rather than the patch, and succession is what runs next.
func vacateSeat(n *Notifier, nodeID, slug, name, memID, userID string) bool {
	res, err := n.DB.Exec(`UPDATE memberships SET role = 'member' WHERE id = ? AND role = 'admin' AND status = 'active'`, memID)
	if err != nil {
		return false
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return false
	}
	n.DB.Exec(`DELETE FROM notification_reminders_sent
	           WHERE entity_type = 'membership' AND entity_id = ? AND reminder_type = 'inactivity_warning'`, memID)
	auth.LogAuditEvent(n.DB, "", "membership.seat_vacated", "membership", memID,
		`{"node_id":"`+nodeID+`","reason":"inactivity"}`, "")

	n.Notify(Event{
		Type: MembershipRoleChanged, NodeID: nodeID, NodeSlug: slug, NodeName: name,
		TargetID: userID,
		Title:    "Your admin seat in " + name + " was declared vacant",
		Body:     "The seat was vacant for inactivity. You are still a member and can take part again at any time.",
		Link:     weblink.PatchGovernance(slug),
	})
	return true
}

// runSuccession fills a patch that inactivity has emptied, by whatever rule
// the patch chose.
func runSuccession(n *Notifier, nodeID, slug, name, policy string, justVacated []string) {
	switch policy {
	case "instance_admin":
		// The one policy Patchwork cannot carry out itself: a person with
		// site-wide responsibility has to decide who runs this patch.
		n.Notify(Event{
			Type: GovernanceSuccessionNeeded, NodeID: nodeID, NodeSlug: slug, NodeName: name,
			EntityID: nodeID,
			Title:    name + " has no admins left",
			Body:     "Its last admin seat was vacated for inactivity, and its succession policy asks an instance admin to step in.",
			Link:     weblink.PatchGovernance(slug),
		})
	case "freeze":
		// The patch chose to stop rather than be reassigned. Nothing to do,
		// but say so somewhere a person will find it.
		log.Printf("succession: %s has no admins and its policy is freeze", slug)
	default:
		// "longest_tenure" and anything unrecognised: the shipped succession
		// plan's bus-factor rule — "the three longest-tenured active members
		// become interim admins".
		promoteLongestTenured(n, nodeID, slug, name, 3, justVacated)
	}
}

// promoteLongestTenured installs interim admins, skipping anyone this sweep
// just vacated.
//
// Without that exclusion the rule reinstates exactly the person it removed: a
// vacated admin is a member again, and having been there since the beginning
// they are the *longest-tenured* member in the patch. The seat would come
// straight back to the one person known to have stopped turning up.
func promoteLongestTenured(n *Notifier, nodeID, slug, name string, howMany int, justVacated []string) {
	skip := make(map[string]bool, len(justVacated))
	for _, u := range justVacated {
		skip[u] = true
	}
	rows, err := n.DB.Query(`SELECT id, user_id FROM memberships
	                         WHERE node_id = ? AND role = 'member' AND status = 'active'
	                         ORDER BY joined_at ASC`, nodeID)
	if err != nil {
		return
	}
	type pick struct{ memID, userID string }
	var picks []pick
	for rows.Next() {
		var p pick
		if rows.Scan(&p.memID, &p.userID) != nil || skip[p.userID] {
			continue
		}
		picks = append(picks, p)
		if len(picks) == howMany {
			break
		}
	}
	rows.Close()

	if len(picks) == 0 {
		log.Printf("succession: %s has no admins and nobody to promote", slug)
		return
	}
	for _, p := range picks {
		if _, err := n.DB.Exec(`UPDATE memberships SET role = 'admin' WHERE id = ?`, p.memID); err != nil {
			continue
		}
		auth.LogAuditEvent(n.DB, "", "membership.succession", "membership", p.memID,
			`{"node_id":"`+nodeID+`","policy":"longest_tenure"}`, "")
		n.Notify(Event{
			Type: MembershipRoleChanged, NodeID: nodeID, NodeSlug: slug, NodeName: name,
			TargetID: p.userID,
			Title:    "You are now an interim admin of " + name,
			Body:     "Its admin seats were vacated for inactivity, and this patch's succession policy hands them to its longest-standing members.",
			Link:     weblink.PatchGovernance(slug),
		})
	}
	log.Printf("succession: %s promoted %d longest-tenured member(s)", slug, len(picks))
}

func itoaDays(d int) string {
	if d == 1 {
		return "1 day"
	}
	digits := ""
	n := d
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	if digits == "" {
		digits = "0"
	}
	return digits + " days"
}
