package notifications

import (
	"context"
	"log"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
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
	checkClaimSetupExpiring(n)
	cleanupOldNotifications(n)
	ExpireStaleClaims(n.DB)
	// Seats that went quiet, and the succession that catches a patch they
	// leave empty (docs/adr/051).
	SweepInactiveAdmins(n)
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
			Link:     weblink.Proposal(slug, id),
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
			Link:     weblink.Event(id),
		})

		remID := auth.NewUUIDv7()
		n.DB.Exec(
			`INSERT OR IGNORE INTO notification_reminders_sent (id, entity_type, entity_id, reminder_type) VALUES (?, ?, ?, ?)`,
			remID, "event", id, "reminder",
		)
	}
}

// checkClaimSetupExpiring reminds a claimant once when their approved
// claim's setup window closes within 3 days (docs/adr/039). The claim is
// single-use and there is no permanent reminder-sent table entry for it
// (that table is keyed by entity_type/entity_id, built for proposals and
// events); instead this dedupes the same way instance_actor.go's inbound-AP
// notifications do — checking the notifications table itself for a prior
// send of this type to this user at this link, since the notifications
// table carries no entity_id column of its own.
func checkClaimSetupExpiring(n *Notifier) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	soon := time.Now().Add(3 * 24 * time.Hour).UTC().Format("2006-01-02T15:04:05.000Z")

	rows, err := n.DB.Query(
		`SELECT cr.id, cr.user_id, cr.setup_expires_at, n.id, n.slug, n.name
		 FROM claim_requests cr
		 JOIN nodes n ON n.id = cr.node_id AND n.status = 'unclaimed' AND n.removed_at IS NULL
		 WHERE cr.status = 'approved'
		   AND cr.setup_expires_at IS NOT NULL
		   AND cr.setup_expires_at > ?
		   AND cr.setup_expires_at <= ?`,
		now, soon,
	)
	if err != nil {
		log.Printf("reminders: claim setup expiry query: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var claimID, userID, expiresAt, nodeID, slug, name string
		if err := rows.Scan(&claimID, &userID, &expiresAt, &nodeID, &slug, &name); err != nil {
			continue
		}

		link := weblink.PatchSetup(slug)
		var existing int
		n.DB.QueryRow(
			`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND type = ? AND link = ?`,
			userID, string(ClaimSetupExpiring), link,
		).Scan(&existing)
		if existing > 0 {
			continue
		}

		n.Notify(Event{
			Type:     ClaimSetupExpiring,
			NodeID:   nodeID,
			NodeSlug: slug,
			NodeName: name,
			TargetID: userID,
			EntityID: claimID,
			Title:    "Your approved claim on " + name + " expires " + formatClaimDate(expiresAt),
			Link:     link,
		})
	}
}

// formatClaimDate renders an ISO timestamp for claimant-facing copy
// ("expires August 7, 2026"). Falls back to the raw string if parsing ever
// fails — never worth failing a notification over.
func formatClaimDate(iso string) string {
	t, err := time.Parse("2006-01-02T15:04:05.000Z", iso)
	if err != nil {
		return iso
	}
	return t.Format("January 2, 2006")
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
