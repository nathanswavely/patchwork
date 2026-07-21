package eventsource

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// Source is one event_sources row, loaded fresh at sync time.
type Source struct {
	ID            string
	NodeID        string
	Type          string
	URL           string
	AddedBy       string
	Etag          sql.NullString
	LastModified  sql.NullString
	LastSuccessAt sql.NullString
}

// sourceLocks serializes syncs per source: the hourly worker and a
// manual "sync now" must not reconcile the same feed concurrently.
// Single-process by design, so an in-process mutex is the whole story.
var sourceLocks sync.Map

// Sync fetches one source and reconciles its events. Every mutation
// path honors docs/adr/031: a failed fetch records itself on the source
// and touches no events; only a successful parse may insert, update, or
// delete. The first successful sync is silent; later syncs announce new
// events exactly like a directly posted one.
func Sync(ctx context.Context, db *database.DB, notifier *notifications.Notifier, sourceID string) error {
	mu, _ := sourceLocks.LoadOrStore(sourceID, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	var src Source
	err := db.QueryRow(
		`SELECT id, node_id, type, url, added_by, etag, last_modified, last_success_at
		 FROM event_sources WHERE id = ?`, sourceID,
	).Scan(&src.ID, &src.NodeID, &src.Type, &src.URL, &src.AddedBy,
		&src.Etag, &src.LastModified, &src.LastSuccessAt)
	if err != nil {
		return fmt.Errorf("load source: %w", err)
	}

	result, err := fetchFeed(ctx, src.URL, src.Etag.String, src.LastModified.String)
	if err != nil {
		recordFailure(db, src.ID, err)
		return err
	}
	if result.NotModified {
		recordSuccess(db, src.ID, src.Etag.String, src.LastModified.String)
		return nil
	}

	items, err := ParseICS(result.Body, time.Now().UTC())
	if err != nil {
		recordFailure(db, src.ID, err)
		return err
	}

	if err := reconcile(db, notifier, &src, items); err != nil {
		recordFailure(db, src.ID, err)
		return err
	}
	recordSuccess(db, src.ID, result.Etag, result.LastModified)
	return nil
}

func nowStamp() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

func recordFailure(db *database.DB, sourceID string, cause error) {
	msg := cause.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, err := db.Exec(
		`UPDATE event_sources SET status = 'error', last_fetch_at = ?, last_error = ?, updated_at = ? WHERE id = ?`,
		nowStamp(), msg, nowStamp(), sourceID,
	)
	if err != nil {
		log.Printf("eventsource: record failure for %s: %v", sourceID, err)
	}
}

func recordSuccess(db *database.DB, sourceID, etag, lastModified string) {
	now := nowStamp()
	_, err := db.Exec(
		`UPDATE event_sources SET status = 'ok', last_fetch_at = ?, last_success_at = ?,
		 last_error = NULL, etag = ?, last_modified = ?, updated_at = ? WHERE id = ?`,
		now, now, nullable(etag), nullable(lastModified), now, sourceID,
	)
	if err != nil {
		log.Printf("eventsource: record success for %s: %v", sourceID, err)
	}
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type existingEvent struct {
	ID          string
	Title       string
	Description string
	Location    string
	Latitude    *float64
	Longitude   *float64
	StartsAt    string
	EndsAt      *string
}

// reconcile makes the source's local events match the desired set.
func reconcile(db *database.DB, notifier *notifications.Notifier, src *Source, items []Item) error {
	skipped := map[string]bool{}
	rows, err := db.Query(`SELECT uid, occurrence FROM event_source_skips WHERE source_id = ?`, src.ID)
	if err != nil {
		return fmt.Errorf("load skips: %w", err)
	}
	for rows.Next() {
		var uid, occ string
		if err := rows.Scan(&uid, &occ); err != nil {
			rows.Close()
			return err
		}
		skipped[Key(uid, occ)] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	existing := map[string]existingEvent{}
	rows, err = db.Query(
		`SELECT source_uid, source_occurrence, id, title, description, location,
		 latitude, longitude, starts_at, ends_at
		 FROM events WHERE source_id = ? AND removed_at IS NULL`, src.ID)
	if err != nil {
		return fmt.Errorf("load existing: %w", err)
	}
	for rows.Next() {
		var uid, occ string
		var e existingEvent
		if err := rows.Scan(&uid, &occ, &e.ID, &e.Title, &e.Description, &e.Location,
			&e.Latitude, &e.Longitude, &e.StartsAt, &e.EndsAt); err != nil {
			rows.Close()
			return err
		}
		existing[Key(uid, occ)] = e
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	desired := map[string]Item{}
	for _, it := range items {
		k := Key(it.UID, it.Occurrence)
		if skipped[k] {
			continue
		}
		desired[k] = it
	}

	var nodeSlug, nodeName string
	if err := db.QueryRow(`SELECT slug, name FROM nodes WHERE id = ?`, src.NodeID).Scan(&nodeSlug, &nodeName); err != nil {
		return fmt.Errorf("load node: %w", err)
	}

	// The first successful sync adopts the whole calendar quietly;
	// announcing forty backfilled events would bury every follower's
	// bell. From then on, new events are news.
	announce := src.LastSuccessAt.Valid
	now := time.Now().UTC().Format(time.RFC3339)

	for k, it := range desired {
		if prev, ok := existing[k]; ok {
			if !changed(prev, it) {
				continue
			}
			_, err := db.Exec(
				`UPDATE events SET title = ?, description = ?, location = ?, latitude = ?,
				 longitude = ?, starts_at = ?, ends_at = ?, updated_at = ? WHERE id = ?`,
				it.Title, it.Description, it.Location, it.Latitude, it.Longitude,
				it.StartsAt, it.EndsAt, nowStamp(), prev.ID,
			)
			if err != nil {
				return fmt.Errorf("update event %s: %w", prev.ID, err)
			}
			continue
		}

		id := auth.NewUUIDv7()
		apID := ap.EventAPID(ap.GetDomain(), id)
		_, err := db.Exec(
			`INSERT INTO events (id, node_id, created_by, title, description, location,
			 latitude, longitude, starts_at, ends_at, recurrence, visibility, status,
			 ap_id, source_id, source_uid, source_occurrence)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 'public', 'active', ?, ?, ?, ?)`,
			id, src.NodeID, src.AddedBy, it.Title, it.Description, it.Location,
			it.Latitude, it.Longitude, it.StartsAt, it.EndsAt, apID,
			src.ID, it.UID, it.Occurrence,
		)
		if err != nil {
			return fmt.Errorf("insert event: %w", err)
		}

		if announce {
			if notifier != nil {
				go notifier.Notify(notifications.Event{
					Type:     notifications.EventCreated,
					NodeID:   src.NodeID,
					NodeSlug: nodeSlug,
					NodeName: nodeName,
					ActorID:  src.AddedBy,
					EntityID: id,
					Title:    "New event: " + it.Title,
					Link:     "/patches/" + nodeSlug + "/events/" + id,
				})
			}
			broadcastCreate(db, model.Event{
				ID: id, NodeID: src.NodeID, CreatedBy: src.AddedBy,
				Title: it.Title, Description: it.Description, Location: it.Location,
				Latitude: it.Latitude, Longitude: it.Longitude,
				StartsAt: it.StartsAt, EndsAt: it.EndsAt, Visibility: "public",
			}, src.NodeID)
		}
	}

	// Future events the feed no longer carries are promises withdrawn;
	// the past belongs to the patch and stays (docs/adr/031).
	for k, prev := range existing {
		if _, ok := desired[k]; ok {
			continue
		}
		if prev.StartsAt <= now {
			continue
		}
		if _, err := db.Exec(`DELETE FROM events WHERE id = ?`, prev.ID); err != nil {
			return fmt.Errorf("delete event %s: %w", prev.ID, err)
		}
	}
	return nil
}

func changed(prev existingEvent, it Item) bool {
	return prev.Title != it.Title ||
		prev.Description != it.Description ||
		prev.Location != it.Location ||
		!floatEq(prev.Latitude, it.Latitude) ||
		!floatEq(prev.Longitude, it.Longitude) ||
		prev.StartsAt != it.StartsAt ||
		!strEq(prev.EndsAt, it.EndsAt)
}

func floatEq(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func strEq(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// broadcastCreate mirrors the handler's broadcastEventCreate: a Create
// activity to the patch's AP followers. A no-op without followers.
func broadcastCreate(db *database.DB, e model.Event, nodeID string) {
	go func() {
		obj := ap.EventToObject(e, ap.GetDomain())
		activity := map[string]interface{}{
			"@context": ap.Context,
			"type":     "Create",
			"id":       obj.ID + "/activity",
			"actor":    ap.NodeAPID(ap.GetDomain(), nodeID),
			"object":   obj,
		}
		ap.BroadcastToFollowers(db, "node", nodeID, activity)
	}()
}
