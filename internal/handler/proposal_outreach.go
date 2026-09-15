package handler

import (
	"fmt"
	"log"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// A notification follows an obligation (docs/adr/093), and an obligation to
// vote arrives with the standing to cast one — which is not always the
// moment a vote is announced.
//
// `ProposalNew` reaches whoever is a member at the instant a proposal is
// raised, once, and nothing ever spoke to that patch about that vote again
// except a 24-hour warning aimed at everybody. A simulated co-op ran three
// proposals in a fortnight and lapsed all three: seven of its eight members
// had joined after the rules vote opened and were never told it existed, and
// the only nudge the product sent arrived the day before the window shut, to
// voters and non-voters alike, saying nothing about where the vote stood.
//
// This pass answers one question every hour — *who may vote here, and has
// not been told?* — and three notices come out of it:
//
//   - `proposal.open_to_you`, once, to somebody who entered the electorate
//     after the announcement. Joining is one way in; a voting tenure coming
//     due is the other, and it happens with no membership event at all
//     (docs/adr/098), which is why this is a sweep and not a hook on join.
//   - `proposal.turnout`, once, at the window's midpoint, to people who
//     still owe a ballot, and only while quorum is unmet — the one moment
//     turning up still changes the outcome.
//   - `proposal.deadline`, once, in the last 24 hours, to people who still
//     owe a ballot. It used to go to the whole patch.
//
// Everyone here is in the electorate, so a follower is never told anything
// (docs/adr/044), the author is not exempt, and casting a ballot — abstain
// included, an abstention discharges the obligation — takes a person out of
// the remaining notices immediately.

// Dedupe keys in `notification_reminders_sent`, which is UNIQUE on
// (entity_type, entity_id, reminder_type) and needs no migration to carry
// these. The per-person rows key on `proposal_voter` with a composite id,
// because the table's grain is one row per thing-reminded-about and the
// thing here is one person's obligation on one proposal, not the proposal.
const (
	reminderEntityProposal = "proposal"
	reminderEntityVoter    = "proposal_voter"

	// A proposal whose electorate has been written down as already told.
	reminderAudienceSeeded = "audience_seeded"

	reminderAnnounced = "announced"
	reminderTurnout   = "turnout"
	reminderDeadline  = "deadline"
)

// SweepVoteNotices tells the people who still owe a vote. One pass of the
// hourly loop, beside SweepProposals (docs/adr/097) — that one ends windows,
// this one makes sure the people whose windows they are know about them.
func SweepVoteNotices(db *database.DB) {
	nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	rows, err := db.Query(`SELECT p.id, p.node_id, n.slug, n.name, p.title, p.voting_ends_at,
	                              COALESCE(p.duration_hours, 0), COALESCE(p.target_user_id, '')
	                       FROM proposals p
	                       JOIN nodes n ON n.id = p.node_id
	                       WHERE p.status = 'open'
	                         AND COALESCE(p.state, 'voting') = 'voting'
	                         AND COALESCE(p.seats_contested, 0) = 0
	                         AND p.voting_ends_at IS NOT NULL AND p.voting_ends_at > ?
	                         AND n.status IN ('active', 'unclaimed') AND n.removed_at IS NULL`, nowStr)
	if err != nil {
		log.Printf("vote notices: query: %v", err)
		return
	}
	type openBallot struct {
		id, nodeID, slug, name, title, endsAt, subject string
		durationHours                                  int
	}
	var ballots []openBallot
	for rows.Next() {
		var b openBallot
		if rows.Scan(&b.id, &b.nodeID, &b.slug, &b.name, &b.title, &b.endsAt, &b.durationHours, &b.subject) == nil {
			ballots = append(ballots, b)
		}
	}
	rows.Close()

	for _, b := range ballots {
		// The patch's own switch, checked before anything is written: a
		// dedupe row recorded for a notice the config suppressed would
		// silence that person for good once the switch came back on.
		if !notifications.IsCategoryEnabled(db, b.nodeID, notifications.CategoryProposals) {
			continue
		}

		// This vote's own frozen terms, like every other surface that says
		// something about a running vote (docs/adr/047), so the turnout line
		// quotes the quorum the resolver will apply rather than whatever the
		// rules say today.
		gc := votingTerms(db, b.id, b.nodeID)
		recused := recusedSubject(db, b.nodeID, gc, b.subject)
		electorate := electorateUserIDs(db, b.nodeID, gc, recused)
		if len(electorate) == 0 {
			continue
		}

		announced := announceOpenVote(db, b.id, b.nodeID, b.slug, b.name, b.title, b.endsAt, electorate)

		ends, err := parseStoredInstant(b.endsAt)
		if err != nil {
			continue
		}
		remaining := time.Until(ends)
		if remaining <= 0 {
			continue
		}

		var kind string
		switch {
		case remaining <= 24*time.Hour:
			kind = reminderDeadline
		case b.durationHours > 0 && remaining <= time.Duration(b.durationHours)*time.Hour/2:
			kind = reminderTurnout
		default:
			continue
		}

		owed := notVotedYet(db, b.id, electorate)
		if len(owed) == 0 {
			continue
		}

		approve, reject, abstain := tallyProposal(db, b.id)
		cast := approve + reject + abstain
		needed := votesNeededForQuorum(gc, len(electorate))

		// The midpoint notice exists because a vote is about to lapse for
		// want of turnout. Where quorum is already met it has no news, and
		// the last-call notice below is enough.
		if kind == reminderTurnout && quorumReached(gc, cast, len(electorate)) {
			continue
		}

		for _, uid := range owed {
			// Somebody told about this vote for the first time moments ago
			// does not also need chasing about it in the same pass.
			if announced[uid] {
				continue
			}
			if reminderAlreadySent(db, reminderEntityVoter, voterKey(b.id, uid), kind) {
				continue
			}
			markReminderSent(db, reminderEntityVoter, voterKey(b.id, uid), kind)

			title, body := voteChaseCopy(kind, b.title, cast, len(electorate), needed, remaining)
			notifyNow(notifications.Event{
				Type:     reminderNotificationType(kind),
				NodeID:   b.nodeID,
				NodeSlug: b.slug,
				NodeName: b.name,
				TargetID: uid,
				EntityID: b.id,
				Title:    title,
				Body:     body,
				Link:     weblink.Proposal(b.slug, b.id),
			})
		}
	}
}

func reminderNotificationType(kind string) notifications.NotificationType {
	if kind == reminderTurnout {
		return notifications.ProposalTurnout
	}
	return notifications.ProposalDeadline
}

// announceOpenVote tells anyone in the electorate who has not been told that
// this vote is open, and returns the set it reached this pass.
//
// The record of who has been told is written, never inferred. A proposal is
// seeded at the moment it is announced (SeedVoteAudience, from the create and
// open-vote paths) with everyone who could vote on it then; this pass seeds
// any proposal that carries no such record, which is how a vote that was
// already running when this shipped avoids telling its whole patch again.
func announceOpenVote(db *database.DB, proposalID, nodeID, slug, name, title, endsAt string, electorate []string) map[string]bool {
	seeded := reminderAlreadySent(db, reminderEntityProposal, proposalID, reminderAudienceSeeded)
	if !seeded {
		SeedVoteAudience(db, proposalID, electorate)
		return nil
	}

	reached := map[string]bool{}
	for _, uid := range electorate {
		if reminderAlreadySent(db, reminderEntityVoter, voterKey(proposalID, uid), reminderAnnounced) {
			continue
		}
		markReminderSent(db, reminderEntityVoter, voterKey(proposalID, uid), reminderAnnounced)
		reached[uid] = true

		body := "This vote opened before you could take part in it."
		if closes := formatVoteDate(endsAt); closes != "" {
			body += " Voting closes " + closes + "."
		}
		notifyNow(notifications.Event{
			Type:     notifications.ProposalOpenToYou,
			NodeID:   nodeID,
			NodeSlug: slug,
			NodeName: name,
			TargetID: uid,
			EntityID: proposalID,
			Title:    "Open for your vote: " + title,
			Body:     body,
			Link:     weblink.Proposal(slug, proposalID),
		})
	}
	return reached
}

// SeedVoteAudience records the people who were told a vote is open by the
// announcement itself, so the hourly pass can tell the difference between
// "has not been told" and "was in the room when it was said".
//
// Called from the paths that announce a proposal to a patch — creation and
// the maintainer opening an advisory vote — with the electorate as it stands
// at that instant. Writing it down beats working it out: a person's joining
// date does not answer the question, because a voting tenure can fall due
// long after they joined (docs/adr/098), and the notifications table does not
// answer it either, since a member may have switched the category off.
func SeedVoteAudience(db *database.DB, proposalID string, electorate []string) {
	for _, uid := range electorate {
		markReminderSent(db, reminderEntityVoter, voterKey(proposalID, uid), reminderAnnounced)
	}
	markReminderSent(db, reminderEntityProposal, proposalID, reminderAudienceSeeded)
}

// seedVoteAudienceNow reads the electorate for a node under a proposal's own
// terms and seeds it. The shorthand the two announcement paths use; it reads
// the recused subject from the row rather than taking it as an argument, so
// the two callers cannot disagree about who the vote is about.
func seedVoteAudienceNow(db *database.DB, proposalID, nodeID string) {
	var targetUserID string
	db.QueryRow("SELECT COALESCE(target_user_id,'') FROM proposals WHERE id = ?", proposalID).Scan(&targetUserID)
	gc := votingTerms(db, proposalID, nodeID)
	recused := recusedSubject(db, nodeID, gc, targetUserID)
	SeedVoteAudience(db, proposalID, electorateUserIDs(db, nodeID, gc, recused))
}

// notVotedYet narrows an electorate to the people with no ballot on this
// proposal. Any ballot: an abstention is a vote for this purpose, because the
// person turned up and said so, and chasing them would be chasing somebody
// for an answer they have already given.
func notVotedYet(db *database.DB, proposalID string, electorate []string) []string {
	voted := map[string]bool{}
	rows, err := db.Query(`SELECT user_id FROM votes WHERE proposal_id = ?`, proposalID)
	if err != nil {
		return nil
	}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			voted[id] = true
		}
	}
	rows.Close()

	var owed []string
	for _, uid := range electorate {
		if !voted[uid] {
			owed = append(owed, uid)
		}
	}
	return owed
}

// voteChaseCopy writes the two chasing notices. Both open with where the vote
// stands, because "please vote" is a request and "votes cast: 2 of 8, quorum
// needs 4" is a reason. A co-op read the first one twenty-eight times and
// lapsed three proposals.
func voteChaseCopy(kind, title string, cast, eligible, needed int, remaining time.Duration) (string, string) {
	stand := fmt.Sprintf("Votes cast: %d of %d.", cast, eligible)
	if needed > 0 {
		stand += fmt.Sprintf(" Quorum needs %d.", needed)
	}
	if kind == reminderTurnout {
		return "Still needs votes: " + title, stand + " Voting closes in " + humanRemaining(remaining) + "."
	}
	return "Voting ends soon: " + title, stand + " Less than 24 hours left to vote."
}

// humanRemaining rounds a voting window's remainder to the unit a person
// would use. Never below "1 day" — the midpoint notice only fires with more
// than 24 hours left, so hours are never the honest answer here.
func humanRemaining(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days <= 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

// formatVoteDate renders a stored voting deadline for member-facing copy.
// Empty when it cannot be read: a notice is better without a date than with
// a raw timestamp in it.
func formatVoteDate(iso string) string {
	t, err := parseStoredInstant(iso)
	if err != nil {
		return ""
	}
	return t.Format("January 2, 2006")
}

func voterKey(proposalID, userID string) string { return proposalID + ":" + userID }

func reminderAlreadySent(db *database.DB, entityType, entityID, reminderType string) bool {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM notification_reminders_sent
	             WHERE entity_type = ? AND entity_id = ? AND reminder_type = ?`,
		entityType, entityID, reminderType).Scan(&n)
	return n > 0
}

func markReminderSent(db *database.DB, entityType, entityID, reminderType string) {
	db.Exec(`INSERT OR IGNORE INTO notification_reminders_sent (id, entity_type, entity_id, reminder_type)
	         VALUES (?, ?, ?, ?)`, auth.NewUUIDv7(), entityType, entityID, reminderType)
}

// notifyNow delivers on the calling goroutine, unlike notify(). A sweep has
// nobody waiting on it, and a background send would make the dedupe row and
// the notification race each other.
func notifyNow(event notifications.Event) {
	if pkgNotifier != nil {
		pkgNotifier.Notify(event)
	}
}
