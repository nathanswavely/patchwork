package ap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// StartDeliveryWorker starts a background goroutine that delivers queued activities.
// It polls the outbox queue and delivers pending activities with exponential backoff.
// Call with a cancellable context to stop the worker.
func StartDeliveryWorker(ctx context.Context, db *database.DB) {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				deliverPending(ctx, db)
			}
		}
	}()
}

func deliverPending(ctx context.Context, db *database.DB) {
	// Fetch pending activities ready for delivery.
	rows, err := db.QueryContext(ctx, `
		SELECT id, activity_json, target_inbox, attempts
		FROM ap_outbox_queue
		WHERE status = 'pending' AND (next_retry_at IS NULL OR next_retry_at <= ?)
		ORDER BY created_at ASC
		LIMIT 10
	`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		log.Printf("delivery worker: query failed: %v", err)
		return
	}
	defer rows.Close()

	type queueItem struct {
		ID           string
		ActivityJSON string
		TargetInbox  string
		Attempts     int
	}

	var items []queueItem
	for rows.Next() {
		var item queueItem
		if err := rows.Scan(&item.ID, &item.ActivityJSON, &item.TargetInbox, &item.Attempts); err != nil {
			log.Printf("delivery worker: scan failed: %v", err)
			continue
		}
		items = append(items, item)
	}

	for _, item := range items {
		if ctx.Err() != nil {
			return
		}

		err := deliverActivity(ctx, db, item.ActivityJSON, item.TargetInbox)
		if err != nil {
			// Failed — increment attempts, set retry time with exponential backoff.
			nextAttempt := item.Attempts + 1
			if nextAttempt >= 5 {
				// Max retries reached — mark as failed.
				db.Exec(`UPDATE ap_outbox_queue SET status = 'failed', last_error = ?, attempts = ? WHERE id = ?`,
					err.Error(), nextAttempt, item.ID)
			} else {
				// Exponential backoff: 30s, 60s, 120s, 240s.
				backoff := time.Duration(30*(1<<uint(nextAttempt-1))) * time.Second
				retryAt := time.Now().Add(backoff).UTC().Format(time.RFC3339)
				db.Exec(`UPDATE ap_outbox_queue SET attempts = ?, last_error = ?, next_retry_at = ? WHERE id = ?`,
					nextAttempt, err.Error(), retryAt, item.ID)
			}
			log.Printf("delivery worker: failed to %s (attempt %d): %v", item.TargetInbox, nextAttempt, err)
		} else {
			// Success — mark as delivered.
			db.Exec(`UPDATE ap_outbox_queue SET status = 'delivered', attempts = ? WHERE id = ?`,
				item.Attempts+1, item.ID)
		}
	}
}

func deliverActivity(ctx context.Context, db *database.DB, activityJSON, targetInbox string) error {
	body := []byte(activityJSON)

	req, err := http.NewRequestWithContext(ctx, "POST", targetInbox, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/activity+json")
	req.Header.Set("Accept", "application/activity+json")
	req.Header.Set("User-Agent", "Patchwork/1.0")

	// Set Digest header (required for HTTP Signatures on POST).
	digest := sha256.Sum256(body)
	req.Header.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(digest[:]))

	// Sign the request with the sending actor's private key so the receiving
	// instance can verify it. Remote inboxes reject unsigned POSTs.
	if err := signOutbound(db, req, body); err != nil {
		return fmt.Errorf("sign: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("http %d from %s", resp.StatusCode, targetInbox)
}

// signOutbound looks up the activity's actor, loads its private key, and signs
// the outgoing request in place.
func signOutbound(db *database.DB, req *http.Request, body []byte) error {
	var activity struct {
		Actor string `json:"actor"`
	}
	if err := json.Unmarshal(body, &activity); err != nil {
		return fmt.Errorf("parse activity: %w", err)
	}
	if activity.Actor == "" {
		return fmt.Errorf("activity has no actor")
	}

	keyID, privPEM, err := PrivateKeyForActor(db, activity.Actor)
	if err != nil {
		return err
	}
	return SignRequest(req, keyID, privPEM)
}
