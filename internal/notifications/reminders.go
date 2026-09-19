package notifications

import (
	"context"
	"log"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/clock"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// StartReminderWorker runs a background goroutine for the sweeps that are
// time-based rather than act-based: expiring claim setups, the bulletin,
// inactive seats, and hygiene. It no longer reminds anyone about an
// event (docs/adr/093). Same pattern as ap/delivery.go — ticker + context
// cancellation.
func StartReminderWorker(ctx context.Context, notifier *Notifier) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		// Run once at startup after a short delay.
		time.Sleep(30 * time.Second)
		RunReminders(notifier)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				RunReminders(notifier)
			}
		}
	}()
}

// RunReminders is one pass of the hourly worker. Exported so cmd/sim can run
// the same pass the server runs after it moves the world (docs/adr/096).
func RunReminders(n *Notifier) {
	// The proposal-deadline notice used to be sent from here, to every member
	// of the patch, once per proposal, in the last 24 hours. It now belongs
	// to handler.SweepVoteNotices, which addresses it to the people who are
	// still carrying the obligation and says where the vote stands
	// (docs/adr/093). It lives there because the electorate is one set
	// expressed once (docs/adr/044), and that one expression is in
	// internal/handler — which this package cannot import.
	checkClaimSetupExpiring(n)
	sendBulletin(n)
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
	cutoff := clock.Format(time.Now().Add(-30 * 24 * time.Hour))
	now := clock.Now()
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

// checkClaimSetupExpiring reminds a claimant once when their approved
// claim's setup window closes within 3 days (docs/adr/039). The claim is
// single-use and there is no permanent reminder-sent table entry for it
// (that table is keyed by entity_type/entity_id, built for proposals and
// events); instead this dedupes the same way instance_actor.go's inbound-AP
// notifications do — checking the notifications table itself for a prior
// send of this type to this user at this link, since the notifications
// table carries no entity_id column of its own.
func checkClaimSetupExpiring(n *Notifier) {
	now := clock.Now()
	soon := clock.Format(time.Now().Add(3 * 24 * time.Hour))

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
	t, err := clock.Parse(iso)
	if err != nil {
		return iso
	}
	return t.Format("January 2, 2006")
}

// cleanupOldNotifications deletes notifications older than 90 days to prevent unbounded growth.
func cleanupOldNotifications(n *Notifier) {
	cutoff := clock.Format(time.Now().Add(-90 * 24 * time.Hour))
	result, err := n.DB.Exec(`DELETE FROM notifications WHERE created_at < ?`, cutoff)
	if err != nil {
		log.Printf("reminders: cleanup: %v", err)
		return
	}
	if rows, _ := result.RowsAffected(); rows > 0 {
		log.Printf("reminders: cleaned up %d old notifications", rows)
	}
}
