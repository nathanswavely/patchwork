package notifications

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/clock"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// An interim admin is asked first
// (docs/adr/2026-10-19-an-interim-admin-is-asked-first.md).
//
// When inactivity empties a longest-tenure patch, the sweep used to make its
// three longest-standing present members admins on the spot and tell them
// afterwards. That was the one path left that started a role without asking
// the person (F-085): docs/adr/098 asks an invitee, docs/adr/117 refuses to
// hand a follower a role "without anyone having asked them", and this asked
// nobody. So the sweep records an offer for each person it picks, and only
// their answer makes an admin.
//
// No clock answers for anybody. An offer stands until it is answered, the
// same rule as holdover read the other way round: no clock removes anybody,
// and no clock appoints anybody either.

// interimSeats is the shipped succession plan's bus factor: "the three
// longest-tenured active members become interim admins".
const interimSeats = 3

// ErrNoInterimOffer is answering with no open offer on that patch. The
// handler turns it into one 404 for everybody, so the route says nothing
// about who was asked.
var ErrNoInterimOffer = errors.New("no interim offer to answer")

// ErrNoLongerHere is answering after leaving the patch. The offer is closed
// and passed on, since somebody who has gone cannot step in.
var ErrNoLongerHere = errors.New("no longer an active member of this patch")

func skipSet(userIDs []string) map[string]bool {
	m := make(map[string]bool, len(userIDs))
	for _, u := range userIDs {
		m[u] = true
	}
	return m
}

// presentCandidates is who may be asked, longest-standing first. It skips
// anyone this sweep just vacated, anyone who is not here either, and anyone
// already asked since the council was last filled.
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
// interim admin is, on the day they accept, a person this sweep would not
// vacate.
//
// Presence here is wider than the participation an admin is measured by, and
// the asymmetry is deliberate. A seat is lost by not governing, because a seat
// is a governing job. A seat is offered to somebody who is *around* — who
// posted an event, put up a notice, replied to one, or joined recently —
// because the question is not whether they have been governing (they had no
// seat to govern from) but whether asking them reaches a person.
//
// The third is what stops a decline coming straight back: somebody who said
// no is not asked again until the council has been filled and emptied again,
// which is when sweepNode clears the answered offers.
func presentCandidates(db *database.DB, nodeID string, inactivityDays int, skip map[string]bool) []string {
	presentSince := daysAgo(inactivityDays * 2)
	rows, err := db.Query(`
		SELECT m.user_id, MAX(COALESCE(act.at, m.joined_at)) AS last_at
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
		  AND NOT EXISTS (SELECT 1 FROM interim_admin_offers o
		                  WHERE o.node_id = m.node_id AND o.user_id = m.user_id)
		GROUP BY m.id, m.user_id
		HAVING MAX(COALESCE(act.at, m.joined_at)) >= ?
		ORDER BY m.joined_at ASC`, nodeID, presentSince)
	if err != nil {
		log.Printf("succession: %s: who to ask: %v", nodeID, err)
		return nil
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var userID, lastAt string
		if rows.Scan(&userID, &lastAt) != nil || skip[userID] {
			continue
		}
		out = append(out, userID)
	}
	return out
}

// offerInterim asks up to `want` present members and reports how many it
// asked. The question says plainly that nothing changes unless they accept,
// because the old notice told them it already had.
func offerInterim(n *Notifier, nodeID, slug, name string, inactivityDays, want int, skip map[string]bool) int {
	db := n.DB
	asked := 0
	for _, userID := range presentCandidates(db, nodeID, inactivityDays, skip) {
		if asked == want {
			break
		}
		if _, err := db.Exec(`INSERT INTO interim_admin_offers (id, node_id, user_id) VALUES (?, ?, ?)`,
			auth.NewUUIDv7(), nodeID, userID); err != nil {
			continue
		}
		asked++
		auth.LogAuditEvent(db, "", "membership.interim_offered", "node", nodeID,
			`{"user_id":"`+userID+`","policy":"longest_tenure"}`, "")
		n.Notify(Event{
			Type: MembershipInterimOffer, NodeID: nodeID, NodeSlug: slug, NodeName: name,
			TargetID: userID,
			Title:    "Will you step in as an interim admin of " + name + "?",
			Body:     "Its admin seats were vacated for inactivity, and its succession policy asks its longest-standing active members to step in. Nothing changes unless you accept.",
			Link:     weblink.PatchGovernance(slug),
		})
	}
	if asked > 0 {
		log.Printf("succession: %s asked %d member(s) to step in", slug, asked)
	}
	return asked
}

// settleInterimOffer answers an open offer exactly once. The update is
// conditional on the offer still being open, so a second answer racing the
// first (a double click, two tabs) finds it already answered and changes
// nothing. Reports whether this call was the one that answered it.
func settleInterimOffer(db *database.DB, offerID, answer, now string) (bool, error) {
	res, err := db.Exec(`UPDATE interim_admin_offers SET answered_at = ?, answer = ?
	                     WHERE id = ? AND answered_at IS NULL`, now, answer, offerID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// AnswerInterimOffer records a person's answer to their open offer on a patch.
//
// Accepting makes them an admin through makeAdmin, so their inactivity clock
// starts the day they said yes (migration 072), and audits it as
// `membership.succession` with them as the actor: the row the Record already
// reads as "Stepped in as an interim admin", which is now literally what
// happened.
//
// Declining passes the offer on while the patch still has no admins. Once
// somebody has accepted, the patch is governed again and nobody else is
// recruited; the offers already out stay answerable, as the old rule would
// have made all three admins at once.
func AnswerInterimOffer(n *Notifier, nodeID, userID string, accept bool, ip string) error {
	db := n.DB
	var offerID string
	err := db.QueryRow(`SELECT id FROM interim_admin_offers
	                    WHERE node_id = ? AND user_id = ? AND answered_at IS NULL`,
		nodeID, userID).Scan(&offerID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNoInterimOffer
	}
	if err != nil {
		return err
	}

	var slug, name, gcJSON string
	if err := db.QueryRow(`SELECT slug, name, COALESCE(governance_config,'{}') FROM nodes WHERE id = ?`, nodeID).
		Scan(&slug, &name, &gcJSON); err != nil {
		return err
	}
	var rules governanceRules
	if err := json.Unmarshal([]byte(gcJSON), &rules); err != nil {
		// Unreadable rules ask nobody further, which is the safe reading:
		// the instance admin route below still catches an empty council.
		log.Printf("succession: %s: governance_config: %v", slug, err)
	}

	var memID, role string
	err = db.QueryRow(`SELECT id, role FROM memberships WHERE node_id = ? AND user_id = ? AND status = 'active'`,
		nodeID, userID).Scan(&memID, &role)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	now := clock.Now()

	switch {
	case memID == "" || role == "follower":
		// Gone since they were asked. A person who is not here cannot step in,
		// so the offer closes as a decline and moves on.
		if _, err := settleInterimOffer(db, offerID, "declined", now); err != nil {
			return err
		}
		passInterimOn(n, nodeID, slug, name, rules)
		return ErrNoLongerHere
	case role == "admin":
		// Made an admin some other way while the question was open. Nothing
		// left to decide; the offer is spent.
		_, err := settleInterimOffer(db, offerID, "accepted", now)
		return err
	}

	if accept {
		settled, err := settleInterimOffer(db, offerID, "accepted", now)
		if err != nil {
			return err
		}
		if !settled {
			return ErrNoInterimOffer
		}
		if !makeAdmin(db, memID) {
			// Reopen rather than leave an answered question with nobody
			// made an admin by it.
			if _, err := db.Exec(`UPDATE interim_admin_offers SET answered_at = NULL, answer = NULL WHERE id = ?`, offerID); err != nil {
				log.Printf("succession: %s: reopen offer %s: %v", slug, offerID, err)
			}
			return errors.New("could not make the membership an admin")
		}
		auth.LogAuditEvent(db, userID, "membership.succession", "membership", memID,
			`{"node_id":"`+nodeID+`","policy":"longest_tenure","asked":true}`, ip)
		log.Printf("succession: %s gained an interim admin who accepted", slug)
		return nil
	}

	settled, err := settleInterimOffer(db, offerID, "declined", now)
	if err != nil {
		return err
	}
	if !settled {
		return ErrNoInterimOffer
	}
	auth.LogAuditEvent(db, userID, "membership.interim_declined", "node", nodeID,
		`{"user_id":"`+userID+`"}`, ip)
	passInterimOn(n, nodeID, slug, name, rules)
	return nil
}

// passInterimOn asks the next present member after a decline, while the patch
// still has no admins. When nobody is left to ask and nobody is still
// deciding, it says so to an instance admin, the route back that already
// exists for a patch nobody here can run.
func passInterimOn(n *Notifier, nodeID, slug, name string, rules governanceRules) {
	db := n.DB
	var admins int
	if err := db.QueryRow(`SELECT COUNT(*) FROM memberships WHERE node_id = ? AND role = 'admin' AND status = 'active'`,
		nodeID).Scan(&admins); err != nil {
		log.Printf("succession: %s: count admins: %v", slug, err)
		return
	}
	if admins > 0 {
		return
	}
	// Still deciding: an open offer to somebody who is still a member here.
	var open int
	if err := db.QueryRow(`SELECT COUNT(*) FROM interim_admin_offers o
	             JOIN memberships m ON m.node_id = o.node_id AND m.user_id = o.user_id
	                              AND m.status = 'active' AND m.role = 'member'
	             WHERE o.node_id = ? AND o.answered_at IS NULL`, nodeID).Scan(&open); err != nil {
		log.Printf("succession: %s: count open offers: %v", slug, err)
		return
	}
	if want := interimSeats - open; want > 0 {
		if offerInterim(n, nodeID, slug, name, rules.InactivityDays, want, nil) > 0 {
			return
		}
	}
	if open > 0 {
		return
	}
	raiseSuccessionNeeded(n, nodeID, slug, name,
		"Its last admin seat was vacated for inactivity, and every member who has been around recently was asked to step in and said no.")
}
