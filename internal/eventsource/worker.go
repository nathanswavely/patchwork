package eventsource

import (
	"context"
	"log"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// StartWorker runs the background goroutine that re-syncs every event
// source hourly. Same pattern as the reminder and AP delivery workers —
// ticker + context cancellation, no queues. Sources sync sequentially,
// which staggers the outbound fetches by itself.
func StartWorker(ctx context.Context, db *database.DB, notifier *notifications.Notifier) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		// Run once at startup after a short delay.
		select {
		case <-ctx.Done():
			return
		case <-time.After(45 * time.Second):
		}
		syncAll(ctx, db, notifier)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				syncAll(ctx, db, notifier)
			}
		}
	}()
}

func syncAll(ctx context.Context, db *database.DB, notifier *notifications.Notifier) {
	// Sources on archived or removed patches lie dormant: no fetch, no
	// imports. The row survives, so a patch restored to 'active' resumes
	// syncing on the next tick with no extra bookkeeping.
	rows, err := db.Query(
		`SELECT es.id FROM event_sources es
		 JOIN nodes n ON n.id = es.node_id
			AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL`)
	if err != nil {
		log.Printf("eventsource: list sources: %v", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			log.Printf("eventsource: scan source: %v", err)
			return
		}
		ids = append(ids, id)
	}
	rows.Close()

	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		if err := Sync(ctx, db, notifier, id); err != nil {
			// Already recorded on the source row; the log line is for
			// the operator tailing the server.
			log.Printf("eventsource: sync %s: %v", id, err)
		}
	}
}
