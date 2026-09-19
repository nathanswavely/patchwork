package notifications

import (
	"encoding/json"
	"log"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/clock"
	"github.com/patchwork-toolkit/patchwork/internal/database"
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
//
// # What a simulated quiet year found, and what it changed
//
// Five patches advanced 320 days with nobody acting produced 1,348
// `membership.succession` rows and 1,347 `membership.seat_vacated` rows.
// Every founder had been stripped of admin, and the role had walked through
// the whole membership of every patch, over and over, ringing a bell at every
// step. Three separate faults, and they compounded:
//
//  1. **Promotion did not reset the clock.** Absence was measured from the
//     later of a person's governance activity and their *joining*, and
//     somebody promoted to fill an absence has usually been a member for
//     longer than the vacate threshold — so they were absent the instant they
//     were appointed. Migration 072 records `memberships.role_since` and it
//     is a floor here beside `joined_at`.
//  2. **Vacating an admin left their seat held.** The council and the admin
//     roll disagreed, which is the divergence docs/adr/100 exists to prevent,
//     reached through a different door. Emptying a chair now empties the
//     chair — the chair itself stays, because a seat outlives its holder.
//  3. **Succession ignored the leadership model.** It promoted the
//     longest-tenured member on every patch, including `elected` ones, which
//     is exactly what docs/adr/100 forbids of the role dropdown and what
//     docs/adr/051 answers with nomination and ratification. Filling now
//     follows the model: see fillEmptyCouncil.
//
// A fourth thing was true and is the reason the loop never damped: an interim
// admin was chosen for *tenure alone*, so a patch where nobody had done
// anything for a year kept handing the role to people who were not there.
// The shipped plan says "the three longest-tenured **active** members", and
// this reads that word as the plan means it: still around in this patch. To
// lose a seat you must stop governing; to be handed one you must still be
// here.

// governanceRules is the slice of a node's config this sweep needs.
type governanceRules struct {
	InactivityDays   int    `json:"inactivity_days"`
	SuccessionPolicy string `json:"succession_policy"`
	// How admins are made here, and where (docs/adr/051, docs/adr/052). The
	// sweep may empty a chair under any model; what refills it is the
	// patch's own mechanic, and under two of the three that is not a
	// promotion this code may perform.
	LeadershipModel string `json:"leadership_model"`
	LeadershipVenue string `json:"leadership_venue"`
	AdminTermMonths int    `json:"admin_term_months"`
}

// electsHere is the `elected` model actually conducted in Patchwork. Where the
// venue is elsewhere the community keeps its own calendar and records the
// result, and Patchwork conducts nothing (docs/adr/052).
func (g governanceRules) electsHere() bool {
	return g.LeadershipModel == "elected" && g.LeadershipVenue != "elsewhere"
}

// SweepInactiveAdmins warns admins who have gone quiet, vacates the seats of
// those who stayed quiet twice as long, and fills any patch left without an
// admin the way that patch fills seats. Exported so tests can trigger it
// directly, the same shape as ExpireStaleClaims.
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
	warnBefore := daysAgo(rules.InactivityDays)
	vacateBefore := daysAgo(rules.InactivityDays * 2)

	// Last time each admin took part in governance. The succession plan names
	// the three things that count — votes, proposals, discussions — so posting
	// events or editing the patch is deliberately not participation here. It
	// is a seat on the council that goes quiet, not an account.
	//
	// Two floors, both read as a date this person was demonstrably present:
	// `joined_at`, because someone who joined last week has not been absent
	// for thirty days whatever their empty activity record says; and
	// `role_since`, because the same is true of someone who was handed the
	// seat last week. The second is migration 072 and is what stops the role
	// walking round the patch for ever. `role_since` is NULL on every
	// membership older than that migration, where it reads as `joined_at` —
	// the floor those rows already had.
	//
	// The aggregate and the scalar are scanned separately and compared in Go
	// rather than nested as max(max(...), ...), because one of those two is
	// an aggregate and the other is not and the reader should not have to
	// work out which.
	adminRows, err := db.Query(`
		SELECT m.id, m.user_id,
		       MAX(COALESCE(act.at, m.joined_at)) AS last_act,
		       MAX(COALESCE(m.role_since, m.joined_at), m.joined_at) AS floor_at
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
		var lastAct, floorAt string
		if adminRows.Scan(&a.memID, &a.userID, &lastAct, &floorAt) != nil {
			continue
		}
		a.lastAt = later(lastAct, floorAt)
		admins = append(admins, a)
	}
	adminRows.Close()

	// A council with anybody in it is not an empty one, so the notice about
	// emptiness is spent and may be sent again if this patch ever empties.
	if len(admins) > 0 {
		forgetTold(db, "node", nodeID, "council_empty")
		forgetTold(db, "node", nodeID, "succession_needed")
	}

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
			// Back in the room, or newly seated: clear both notices so a
			// future absence starts its own cycle rather than being silently
			// skipped. A person promoted since the last pass lands here,
			// because their `role_since` is today — which is also how the
			// record of "they were told their seat went" is cleared when the
			// seat comes back.
			forgetTold(db, "membership", a.memID, "inactivity_warning")
			forgetTold(db, "membership", a.memID, "inactivity_vacated")
		}
	}

	if len(vacatedUsers) == 0 {
		return
	}
	var remaining int
	db.QueryRow(`SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'`, nodeID).Scan(&remaining)
	if remaining == 0 {
		fillEmptyCouncil(n, nodeID, slug, name, rules, vacatedUsers)
	}
}

// warnInactive tells someone their seat is at risk, once per absence.
func warnInactive(n *Notifier, nodeID, slug, name, memID, userID string, days int) {
	if !tellOnce(n.DB, "membership", memID, "inactivity_warning") {
		return
	}
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
//
// **Emptying a chair empties the chair.** This used to change the membership
// role and leave the `seats` row alone, so a co-op's council still recorded
// its chair as held by somebody the admin roll said was an ordinary member —
// two numbers for one council, which is the state docs/adr/100 exists to end.
// The chair itself stays: a seat outlives its holder (docs/adr/051), and what
// refills it is the patch's own mechanic, never this sweep.
func vacateSeat(n *Notifier, nodeID, slug, name, memID, userID string) bool {
	db := n.DB
	res, err := db.Exec(`UPDATE memberships SET role = 'member' WHERE id = ? AND role = 'admin' AND status = 'active'`, memID)
	if err != nil {
		return false
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return false
	}
	emptyChairsOf(db, nodeID, userID)
	forgetTold(db, "membership", memID, "inactivity_warning")
	auth.LogAuditEvent(db, "", "membership.seat_vacated", "membership", memID,
		`{"node_id":"`+nodeID+`","reason":"inactivity"}`, "")

	// Told once. The clock fix above makes a second pass over the same person
	// impossible without a promotion in between, and a promotion clears this
	// row; the guard is here so that no future change to the measurement can
	// turn this notice into the 1,347 the simulation sent.
	if tellOnce(db, "membership", memID, "inactivity_vacated") {
		n.Notify(Event{
			Type: MembershipRoleChanged, NodeID: nodeID, NodeSlug: slug, NodeName: name,
			TargetID: userID,
			Title:    "Your admin seat in " + name + " was declared vacant",
			Body:     "The seat was vacant for inactivity. You are still a member and can take part again at any time.",
			Link:     weblink.PatchGovernance(slug),
		})
	}
	return true
}

// emptyChairsOf takes somebody out of every chair they hold on a patch,
// leaving the chairs. Audited per chair, with no actor: nobody removed them,
// the calendar found them gone.
func emptyChairsOf(db *database.DB, nodeID, userID string) {
	rows, err := db.Query(`SELECT id FROM seats WHERE node_id = ? AND holder_id = ?`, nodeID, userID)
	if err != nil {
		return
	}
	var seatIDs []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			seatIDs = append(seatIDs, id)
		}
	}
	rows.Close()

	for _, id := range seatIDs {
		if _, err := db.Exec(`UPDATE seats SET holder_id = NULL WHERE id = ?`, id); err != nil {
			continue
		}
		auth.LogAuditEvent(db, "", "seat.vacated", "seat", id,
			`{"node_id":"`+nodeID+`","holder_id":"`+userID+`","reason":"inactivity"}`, "")
	}
}

// fillEmptyCouncil answers "who runs this patch now" the way the patch itself
// answers it (docs/adr/051's table), and it is deliberately not one answer.
//
//	maintainer     — the named successor, else succession_policy
//	meritocratic   — nomination and ratification: nobody is promoted here
//	elected        — a contest: nobody is promoted here
//	venue elsewhere— Patchwork conducts nothing: vacate and record
//
// The old code ran `longest_tenure` on all four, which put unelected admins on
// an elected council — the precise act docs/adr/100 refuses the role dropdown,
// arriving through a side door and outranking the election the patch's own
// page promises.
func fillEmptyCouncil(n *Notifier, nodeID, slug, name string, rules governanceRules, justVacated []string) {
	// Where leadership is decided elsewhere (docs/adr/052), Patchwork
	// conducts nothing and records instead. The seat went because nobody was
	// using it; who takes it is a decision at a venue this software was not
	// at, and the community brings back an attestation.
	if rules.LeadershipVenue == "elsewhere" {
		announceCouncilEmpty(n, nodeID, slug, name,
			"This patch records its leadership decisions elsewhere, so Patchwork has appointed nobody. When the community decides who runs it, an admin records that decision here.")
		raiseSuccessionNeeded(n, nodeID, slug, name,
			"Its seats were vacated for inactivity, and this patch decides its leadership elsewhere, so Patchwork appointed nobody.")
		return
	}

	switch rules.LeadershipModel {
	case "elected":
		// The chairs are empty and they are still chairs. What fills them is
		// the contest the calendar runs — so bring that contest forward to
		// now rather than promoting anybody into a seat nobody voted for.
		//
		// Only ever forward. docs/adr/051 put the clock on the seat so that
		// "appointment can fill a gap but can never manufacture a mandate",
		// and SetSeatTerm holds the same line: a held chair's date may be
		// brought forward and never pushed back. Here every chair is empty,
		// so there is no mandate to shorten at all.
		//
		// Every chair, which is safe because every chair is empty — the
		// condition this branch already stands on. It had to be: a contest
		// opened for *some* of a council's chairs used to refill them in
		// created order and vacate the rest, unseating colleagues the
		// electorate never voted out. docs/adr/103 fixed that by having a
		// contest name its chairs, so a partial contest is now an ordinary
		// thing; this branch still moves them all, because here they are all
		// empty and all equally overdue.
		if advanceContests(n.DB, nodeID, rules) > 0 {
			announceCouncilEmpty(n, nodeID, slug, name,
				"Every seat was vacated for inactivity, so this patch has no admins. It elects its council, and that election is now due: it opens shortly, and any member of this patch may stand.")
			return
		}
		announceCouncilEmpty(n, nodeID, slug, name,
			"Every seat was vacated for inactivity, so this patch has no admins. It elects its council, and no election can be scheduled — its seats carry no term end. An instance admin has been told.")
		raiseSuccessionNeeded(n, nodeID, slug, name,
			"Its seats were vacated for inactivity. It elects its council and no contest can be scheduled, so nothing will refill it on its own.")

	case "meritocratic":
		// Admins are nominated and the community ratifies (docs/adr/051), and
		// a nomination is raised by an admin. With none left there is nobody
		// to raise one, so this is the model that genuinely needs a hand.
		announceCouncilEmpty(n, nodeID, slug, name,
			"Its admin seats were vacated for inactivity, so this patch has no admins. Here a new admin is nominated by an admin and ratified by the members, so an instance admin has been told.")
		raiseSuccessionNeeded(n, nodeID, slug, name,
			"Its admin seats were vacated for inactivity. Here admins are nominated by an admin and ratified by the members, so with none left nobody can start one.")

	default:
		// `maintainer`, and any patch whose config names no model. One person
		// runs it and names who inherits it, so the designation is tried
		// first — it is the mechanic this model's own description promises —
		// and `succession_policy` is the fallback the shipped succession plan
		// describes for a patch with nobody named.
		if promoteSuccessor(n, nodeID, slug, name, justVacated) {
			return
		}
		runSuccessionPolicy(n, nodeID, slug, name, rules, justVacated)
	}
}

// advanceContests brings every dated chair's term end forward to today, so
// that the calendar finds this council overdue on its next pass and opens the
// contest that refills it. Reports how many chairs it moved; zero means no
// contest is coming and the caller has to say so.
//
// It writes only `seats.term_ends_at`, because that column *is* the election
// calendar (docs/adr/051: dueness is derived, never stored). Nothing here
// opens an election — `ScheduleDueElections` does, in internal/handler, which
// this package cannot import and does not need to: the two sweeps run on the
// same hourly worker pattern and the calendar reads the seats.
func advanceContests(db *database.DB, nodeID string, rules governanceRules) int {
	// A patch that sets no term length has a council serving until the next
	// election and never schedules one; `ScheduleDueElections` skips it, so
	// moving its dates would be writing a calendar nobody would read.
	if !rules.electsHere() || rules.AdminTermMonths <= 0 {
		return 0
	}
	today := time.Now().UTC().Format("2006-01-02")
	res, err := db.Exec(`UPDATE seats SET term_ends_at = ?
	                     WHERE node_id = ? AND term_ends_at IS NOT NULL AND term_ends_at > ?`,
		today, nodeID, today)
	if err != nil {
		return 0
	}
	moved, _ := res.RowsAffected()

	// A chair whose term had already run out is due as it stands — holdover
	// was carrying it and now there is nobody to hold over. Count those too,
	// or a council that was already overdue reads here as having no contest
	// coming when it has the soonest one of all.
	var dated int
	db.QueryRow(`SELECT COUNT(*) FROM seats WHERE node_id = ? AND term_ends_at IS NOT NULL`, nodeID).Scan(&dated)
	if dated == 0 {
		return 0
	}
	if moved > 0 {
		auth.LogAuditEvent(db, "", "seat.contest_advanced", "node", nodeID,
			`{"seats":`+itoa(int(moved))+`,"term_ends_at":"`+today+`","reason":"council_empty"}`, "")
	}
	return dated
}

// promoteSuccessor hands a maintainer patch to the person its maintainer named
// (docs/adr/051: "the maintainer designates a successor"). Reports whether it
// happened.
//
// The designation is re-checked rather than trusted: the named person must
// still be an active member or admin here, and must not be somebody this very
// sweep judged absent. It is spent on use, exactly as it is when a maintainer
// leaves through the door — re-naming belongs to whoever runs the patch now.
func promoteSuccessor(n *Notifier, nodeID, slug, name string, justVacated []string) bool {
	db := n.DB
	var successorID string
	db.QueryRow(`SELECT COALESCE(designated_successor_id,'') FROM nodes WHERE id = ?`, nodeID).Scan(&successorID)
	if successorID == "" {
		return false
	}
	for _, u := range justVacated {
		if u == successorID {
			return false
		}
	}
	var memID string
	err := db.QueryRow(`SELECT id FROM memberships
	                    WHERE user_id = ? AND node_id = ? AND status = 'active' AND role IN ('admin','member')`,
		successorID, nodeID).Scan(&memID)
	if err != nil || memID == "" {
		return false
	}
	if !makeAdmin(db, memID) {
		return false
	}
	db.Exec(`UPDATE nodes SET designated_successor_id = NULL WHERE id = ?`, nodeID)
	auth.LogAuditEvent(db, "", "node.succession", "node", nodeID,
		`{"successor_id":"`+successorID+`","reason":"inactivity"}`, "")
	n.Notify(Event{
		Type: MembershipRoleChanged, NodeID: nodeID, NodeSlug: slug, NodeName: name,
		TargetID: successorID,
		Title:    "You now run " + name,
		Body:     "Its admin seat was vacated for inactivity, and you were named as the successor. You can hand the patch on, or step down, from Settings.",
		Link:     weblink.Patch(slug),
	})
	log.Printf("succession: %s passed to its designated successor", slug)
	return true
}

// runSuccessionPolicy is the shipped succession plan's rule for a patch with
// nobody named: whatever `succession_policy` says.
func runSuccessionPolicy(n *Notifier, nodeID, slug, name string, rules governanceRules, justVacated []string) {
	switch rules.SuccessionPolicy {
	case "instance_admin":
		// The one policy Patchwork cannot carry out itself: a person with
		// site-wide responsibility has to decide who runs this patch.
		announceCouncilEmpty(n, nodeID, slug, name,
			"Its admin seat was vacated for inactivity, so this patch has no admins. Its succession policy asks an instance admin to step in, and one has been told.")
		raiseSuccessionNeeded(n, nodeID, slug, name,
			"Its last admin seat was vacated for inactivity, and its succession policy asks an instance admin to step in.")
	case "freeze":
		// The patch chose to stop rather than be reassigned. Nothing to do
		// about the role, but the members are owed the sentence.
		announceCouncilEmpty(n, nodeID, slug, name,
			"Its admin seat was vacated for inactivity, so this patch has no admins. Its succession policy is to stop rather than hand the patch to somebody else, so nobody has been appointed.")
		log.Printf("succession: %s has no admins and its policy is freeze", slug)
	default:
		// "longest_tenure" and anything unrecognised: the shipped succession
		// plan's bus-factor rule — "the three longest-tenured active members
		// become interim admins".
		if promoteLongestTenured(n, nodeID, slug, name, 3, justVacated) > 0 {
			return
		}
		announceCouncilEmpty(n, nodeID, slug, name,
			"Its admin seat was vacated for inactivity, so this patch has no admins. Its succession policy hands the patch to its longest-standing active members, and nobody here has taken part recently enough to be one. An instance admin has been told.")
		raiseSuccessionNeeded(n, nodeID, slug, name,
			"Its last admin seat was vacated for inactivity, and nobody in the patch has taken part recently enough to be made an interim admin.")
	}
}

// promoteLongestTenured installs interim admins and reports how many. Skips
// anyone this sweep just vacated, and anyone who is not here either.
//
// The first exclusion stops the rule reinstating exactly the person it removed:
// a vacated admin is a member again, and having been there since the beginning
// they are the *longest-tenured* member in the patch.
//
// The second is what makes this sweep settle. "The three longest-tenured
// **active** members" is the plan's own sentence, and reading "active" as
// `status = 'active'` — a row that is not pending, banned or departed — put
// the patch in the hands of people who had not been seen in a year. They
// lapsed on the same clock, the next three were promoted, and the role walked
// round the membership for as long as the simulation ran. Read as the plan
// means it — somebody who has taken part in this patch inside the window that
// costs an admin their seat — the rule terminates by construction: every
// interim admin is, on the day they are appointed, a person this sweep would
// not vacate.
//
// Presence here is wider than the participation an admin is measured by, and
// the asymmetry is deliberate. A seat is lost by not governing, because a seat
// is a governing job. A seat is offered to somebody who is *around* — who
// posted an event, put up a notice, replied to one, or joined recently —
// because the question is not whether they have been governing (they had no
// seat to govern from) but whether handing them one reaches a person.
func promoteLongestTenured(n *Notifier, nodeID, slug, name string, howMany int, justVacated []string) int {
	db := n.DB
	skip := make(map[string]bool, len(justVacated))
	for _, u := range justVacated {
		skip[u] = true
	}
	var rules governanceRules
	var gcJSON string
	db.QueryRow(`SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?`, nodeID).Scan(&gcJSON)
	json.Unmarshal([]byte(gcJSON), &rules)
	presentSince := daysAgo(rules.InactivityDays * 2)

	rows, err := db.Query(`
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
			UNION ALL
			SELECT created_by, node_id, created_at FROM events
			UNION ALL
			SELECT author_id, node_id, created_at FROM notices
			UNION ALL
			SELECT rp.author_id, nt.node_id, rp.created_at
			  FROM notice_replies rp JOIN notices nt ON nt.id = rp.notice_id
		) act ON act.user_id = m.user_id AND act.node_id = m.node_id
		WHERE m.node_id = ? AND m.role = 'member' AND m.status = 'active'
		GROUP BY m.id, m.user_id
		HAVING MAX(COALESCE(act.at, m.joined_at)) >= ?
		ORDER BY m.joined_at ASC`, nodeID, presentSince)
	if err != nil {
		return 0
	}
	type pick struct{ memID, userID string }
	var picks []pick
	for rows.Next() {
		var p pick
		var lastAt string
		if rows.Scan(&p.memID, &p.userID, &lastAt) != nil || skip[p.userID] {
			continue
		}
		picks = append(picks, p)
		if len(picks) == howMany {
			break
		}
	}
	rows.Close()

	if len(picks) == 0 {
		log.Printf("succession: %s has no admins and nobody present to promote", slug)
		return 0
	}
	promoted := 0
	for _, p := range picks {
		if !makeAdmin(db, p.memID) {
			continue
		}
		promoted++
		auth.LogAuditEvent(db, "", "membership.succession", "membership", p.memID,
			`{"node_id":"`+nodeID+`","policy":"longest_tenure"}`, "")
		n.Notify(Event{
			Type: MembershipRoleChanged, NodeID: nodeID, NodeSlug: slug, NodeName: name,
			TargetID: p.userID,
			Title:    "You are now an interim admin of " + name,
			Body:     "Its admin seats were vacated for inactivity, and this patch's succession policy hands them to its longest-standing active members.",
			Link:     weblink.PatchGovernance(slug),
		})
	}
	log.Printf("succession: %s promoted %d longest-tenured member(s)", slug, promoted)
	return promoted
}

// makeAdmin promotes one membership and starts that person's own inactivity
// clock (migration 072). Every path that makes an admin has to do this; the
// one that did not is how a promotion became a removal on the next pass.
func makeAdmin(db *database.DB, memID string) bool {
	res, err := db.Exec(`UPDATE memberships
	                     SET role = 'admin', role_since = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	                     WHERE id = ? AND status = 'active'`, memID)
	if err != nil {
		return false
	}
	rows, _ := res.RowsAffected()
	return rows > 0
}

// announceCouncilEmpty tells the patch's members, once, that nobody is running
// it and what happens next.
//
// It follows an obligation rather than reporting the world (docs/adr/093): who
// decides in a member's name is a term of that membership, and this is that
// term changing. The members are also the only people who can do anything
// about it under two of the three models — they are the electorate, and they
// are who a nomination names.
func announceCouncilEmpty(n *Notifier, nodeID, slug, name, body string) {
	if !tellOnce(n.DB, "node", nodeID, "council_empty") {
		return
	}
	auth.LogAuditEvent(n.DB, "", "node.council_empty", "node", nodeID, `{"reason":"inactivity"}`, "")
	n.Notify(Event{
		Type: GovernanceCouncilEmpty, NodeID: nodeID, NodeSlug: slug, NodeName: name,
		EntityID: nodeID,
		Title:    name + " has no admins",
		Body:     body,
		Link:     weblink.PatchGovernance(slug),
	})
}

// raiseSuccessionNeeded tells the instance admins, once per emptying. It is
// the route back for a patch whose own mechanic cannot start without an admin
// — and, because a patch with no admins has no mechanism left to outrank,
// UpdateMember lets an instance admin restore one there.
func raiseSuccessionNeeded(n *Notifier, nodeID, slug, name, body string) {
	if !tellOnce(n.DB, "node", nodeID, "succession_needed") {
		return
	}
	n.Notify(Event{
		Type: GovernanceSuccessionNeeded, NodeID: nodeID, NodeSlug: slug, NodeName: name,
		EntityID: nodeID,
		Title:    name + " has no admins left",
		Body:     body,
		Link:     weblink.PatchGovernance(slug),
	})
}

// tellOnce claims the right to send one notice. True the first time and false
// for ever after, until forgetTold releases it.
//
// The UNIQUE on (entity_type, entity_id, reminder_type) does the work, so the
// claim is one statement rather than a read and a write with a gap between
// them.
func tellOnce(db *database.DB, entityType, entityID, kind string) bool {
	res, err := db.Exec(`INSERT OR IGNORE INTO notification_reminders_sent (id, entity_type, entity_id, reminder_type)
	                     VALUES (?, ?, ?, ?)`, auth.NewUUIDv7(), entityType, entityID, kind)
	if err != nil {
		return false
	}
	rows, _ := res.RowsAffected()
	return rows > 0
}

// forgetTold releases a tellOnce claim, so the same notice may be sent about a
// genuinely new occasion.
func forgetTold(db *database.DB, entityType, entityID, kind string) {
	db.Exec(`DELETE FROM notification_reminders_sent
	         WHERE entity_type = ? AND entity_id = ? AND reminder_type = ?`, entityType, entityID, kind)
}

// daysAgo is a stored timestamp that many days back.
func daysAgo(days int) string {
	return clock.Format(time.Now().AddDate(0, 0, -days))
}

// later is the newer of two stored timestamps. They are ISO, so a string
// compare is a time compare.
func later(a, b string) string {
	if b > a {
		return b
	}
	return a
}

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func itoaDays(d int) string {
	if d == 1 {
		return "1 day"
	}
	return itoa(d) + " days"
}
