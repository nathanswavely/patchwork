package notifications

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// The bulletin (docs/adr/076): the one broadcast this system sends.
//
// Every other notification here is a consequence of a relationship the
// recipient already holds — a proposal at their patch, their membership
// changing, an event where they follow. This one is the quilt saying who
// arrived, and it exists because a community organizing platform where
// nobody ever learns that new communities have arrived is failing at its
// literal job. Word of mouth is the thing the platform exists to scale.
//
// Four constraints keep it a bulletin rather than a feed, and each is
// load-bearing:
//
//   - It ships off. Nobody receives this without asking (DefaultEnabled).
//   - It is complete. Every patch that joined in the window, no exceptions.
//   - It is unranked. Arrival order, which is a fact rather than a judgement.
//     The moment something selects among the arrivals, the promise the intro
//     card makes to every anonymous visitor — that no algorithm runs this —
//     is false.
//   - It names arrivals, not rows. `activated_at` is when a community came,
//     not when someone typed a directory listing in.
//
// Deliberately absent: any notion of what the recipient already follows.
// Filtering the list per person would make it a different bulletin for
// everybody, which is a feed with a monthly cadence.

// bulletinLastSentKey stores the end of the last window in instance_settings.
// One instance-wide cursor, not a per-person one: the bulletin is the same
// for everyone who gets it, so there is only one window to track.
const bulletinLastSentKey = "bulletin_last_sent_at"

// bulletinInterval is "monthly" in the only form a ticker can hold it.
const bulletinInterval = 30 * 24 * time.Hour

func bulletinStamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// sendBulletin runs on the hourly reminder pass and does nothing on all but
// one of them.
func sendBulletin(n *Notifier) {
	now := time.Now().UTC()

	last, ok := settings.Get(n.DB, bulletinLastSentKey)
	if !ok || last == "" {
		// First pass on this instance. Start the clock rather than announcing
		// every patch that has ever joined: a bulletin's window is the month
		// behind it, and before today there were no bulletins. This is what
		// keeps an instance that has just upgraded from mailing its whole
		// history to its first subscriber.
		if err := settings.Set(n.DB, bulletinLastSentKey, bulletinStamp(now)); err != nil {
			log.Printf("notifications: bulletin cursor init: %v", err)
		}
		return
	}

	lastAt, err := time.Parse("2006-01-02T15:04:05.000Z", last)
	if err != nil {
		// An unreadable cursor would otherwise wedge the bulletin forever or
		// send it every hour. Reset the window and skip this pass.
		log.Printf("notifications: bulletin cursor unreadable (%q), resetting", last)
		settings.Set(n.DB, bulletinLastSentKey, bulletinStamp(now))
		return
	}
	if now.Sub(lastAt) < bulletinInterval {
		return
	}

	nowStamp := bulletinStamp(now)
	arrivals := patchesJoinedBetween(n, last, nowStamp)

	// The cursor advances whether or not anyone joined. The window is the
	// month behind, not "everything since the last time there was news" —
	// otherwise a quiet quarter would eventually deliver a quarter's worth.
	if err := settings.Set(n.DB, bulletinLastSentKey, nowStamp); err != nil {
		log.Printf("notifications: bulletin cursor advance: %v", err)
	}
	if len(arrivals) == 0 {
		return
	}

	n.Notify(Event{
		Type:  QuiltBulletin,
		Title: bulletinTitle(len(arrivals)),
		Body:  strings.Join(arrivals, ", "),
		// Discovery mode is where a name in this list becomes a patch you can
		// actually follow (docs/adr/075).
		Link: "/discover",
	})
}

func bulletinTitle(n int) string {
	if n == 1 {
		return "1 patch joined this month"
	}
	return fmt.Sprintf("%d patches joined this month", n)
}

// patchesJoinedBetween returns the names of every patch that became active in
// (after, upTo], in arrival order.
//
// Public and active only. A private patch is off the quilt entirely
// (CONTEXT.md), and announcing one would be a disclosure rather than news;
// an archived patch is not an arrival either, even if it arrived inside the
// window and left again.
func patchesJoinedBetween(n *Notifier, after, upTo string) []string {
	rows, err := n.DB.Query(`
		SELECT name FROM nodes
		 WHERE activated_at IS NOT NULL
		   AND activated_at > ? AND activated_at <= ?
		   AND status = 'active'
		   AND visibility = 'public'
		   AND removed_at IS NULL
		 ORDER BY activated_at ASC`, after, upTo)
	if err != nil {
		log.Printf("notifications: bulletin arrivals: %v", err)
		return nil
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		names = append(names, name)
	}
	return names
}
