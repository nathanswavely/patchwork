package notifications

import (
	"context"
	"log"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// StartReminderWorker runs a background goroutine that checks for upcoming
// deadlines and events, sending reminder notifications. Same pattern as
// ap/delivery.go — ticker + context cancellation.
func StartReminderWorker(ctx context.Context, notifier *Notifier) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		// Run once at startup after a short delay.
		time.Sleep(30 * time.Second)
		runReminders(notifier)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runReminders(notifier)
			}
		}
	}()
}

func runReminders(n *Notifier) {
	checkProposalDeadlines(n)
	checkEventReminders(n)
	cleanupOldNotifications(n)
	ExpireStaleClaims(n.DB)
}

// ExpireStaleClaims moves pending claim requests older than 30 days to
// 'expired' (docs/adr/030). Hygiene, not security — an open claim blocks
// nobody but its author, and re-opening costs nothing. Exported so the
// claim tests can trigger the sweep directly.
func ExpireStaleClaims(db *database.DB) {
	cutoff := time.Now().Add(-30 * 24 * time.Hour).UTC().Format("2006-01-02T15:04:05.000Z")
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	result, err := db.Exec(
		"UPDATE claim_requests SET status = 'expired', updated_at = ? WHERE status = 'pending' AND created_at < ?",
		now, cutoff,
	)
	if err != nil {
		log.Printf("reminders: claim expiry sweep: %v", err)
		return
	}
	if rows, _ := result.RowsAffected(); rows > 0 {
		log.Printf("reminders: expired %d stale claims", rows)
	}
}

// checkProposalDeadlines finds proposals where voting_ends_at is within 24 hours
// and sends proposal.deadline notifications (deduped).
func checkProposalDeadlines(n *Notifier) {
	now := time.Now().UTC().Format(time.RFC3339)
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)

	rows, err := n.DB.Query(
		`SELECT p.id, p.title, p.node_id, n.slug, n.name
		 FROM proposals p
		 JOIN nodes n ON n.id = p.node_id
			AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL
		 WHERE p.status = 'open'
		   AND p.voting_ends_at > ?
		   AND p.voting_ends_at <= ?
		   AND p.id NOT IN (
		     SELECT entity_id FROM notification_reminders_sent
		     WHERE entity_type = 'proposal' AND reminder_type = 'deadline'
		   )`,
		now, future,
	)
	if err != nil {
		log.Printf("reminders: proposal deadlines query: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, title, nodeID, slug, name string
		if err := rows.Scan(&id, &title, &nodeID, &slug, &name); err != nil {
			continue
		}

		n.Notify(Event{
			Type:     ProposalDeadline,
			NodeID:   nodeID,
			NodeSlug: slug,
			NodeName: name,
			EntityID: id,
			Title:    "Voting ends soon: " + title,
			Body:     "Less than 24 hours to vote on this proposal.",
			Link:     "/patches/" + slug + "/governance/" + id,
		})

		// Mark as sent.
		remID := auth.NewUUIDv7()
		n.DB.Exec(
			`INSERT OR IGNORE INTO notification_reminders_sent (id, entity_type, entity_id, reminder_type) VALUES (?, ?, ?, ?)`,
			remID, "proposal", id, "deadline",
		)
	}
}

// checkEventReminders finds events starting within 24 hours and sends
// event.reminder notifications (deduped).
func checkEventReminders(n *Notifier) {
	now := time.Now().UTC().Format(time.RFC3339)
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)

	rows, err := n.DB.Query(
		`SELECT e.id, e.title, e.node_id, n.slug, n.name
		 FROM events e
		 JOIN nodes n ON n.id = e.node_id
			AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL
		 WHERE e.starts_at > ?
		   AND e.starts_at <= ?
		   AND e.removed_at IS NULL
		   AND e.status = 'active'
		   AND e.id NOT IN (
		     SELECT entity_id FROM notification_reminders_sent
		     WHERE entity_type = 'event' AND reminder_type = 'reminder'
		   )`,
		now, future,
	)
	if err != nil {
		log.Printf("reminders: event reminders query: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, title, nodeID, slug, name string
		if err := rows.Scan(&id, &title, &nodeID, &slug, &name); err != nil {
			continue
		}

		n.Notify(Event{
			Type:     EventReminder,
			NodeID:   nodeID,
			NodeSlug: slug,
			NodeName: name,
			EntityID: id,
			Title:    "Tomorrow: " + title,
			Body:     "This event starts in less than 24 hours.",
			Link:     "/patches/" + slug + "/events/" + id,
		})

		remID := auth.NewUUIDv7()
		n.DB.Exec(
			`INSERT OR IGNORE INTO notification_reminders_sent (id, entity_type, entity_id, reminder_type) VALUES (?, ?, ?, ?)`,
			remID, "event", id, "reminder",
		)
	}
}

// cleanupOldNotifications deletes notifications older than 90 days to prevent unbounded growth.
func cleanupOldNotifications(n *Notifier) {
	cutoff := time.Now().Add(-90 * 24 * time.Hour).UTC().Format(time.RFC3339)
	result, err := n.DB.Exec(`DELETE FROM notifications WHERE created_at < ?`, cutoff)
	if err != nil {
		log.Printf("reminders: cleanup: %v", err)
		return
	}
	if rows, _ := result.RowsAffected(); rows > 0 {
		log.Printf("reminders: cleaned up %d old notifications", rows)
	}
}
