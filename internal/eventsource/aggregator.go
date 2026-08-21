package eventsource

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"unicode"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// An aggregator lists events it does not own (docs/adr/056). Syncing one
// is two steps that must stay separate: fetch the feed once and cache
// what it carried, then route each name a crosswalk entry addresses.
// Splitting them is what keeps forty crosswalk entries from meaning
// forty fetches of the same URL, and what lets an admin see a name
// before deciding whether it means anything.

var aggregatorLocks sync.Map

// NameKey normalizes a location's first field into the string a
// crosswalk entry is keyed on (docs/adr/046, docs/adr/056). Deliberately
// dumb: case-folded, punctuation dropped, whitespace collapsed. It
// closes "West Art" against "west art," and nothing further — matching
// "Scool" to "School" would mean guessing, and a wrong guess puts a
// stranger's event on somebody's calendar.
func NameKey(location string) string {
	first := location
	if i := strings.IndexAny(first, ","); i >= 0 {
		first = first[:i]
	}
	return normalizeKey(first)
}

// TitleKey normalizes a listing's SUMMARY the way NameKey normalizes its
// LOCATION, and is what a program groups on (docs/adr/063). It keeps the
// whole string: a title has no first field, and truncating one would fold
// together programs that merely open alike. Equally dumb on purpose —
// the safety here is not that the matching is careful but that a program
// ends at a link the named patch confirms.
func TitleKey(title string) string { return normalizeKey(title) }

func normalizeKey(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		default:
			space = true
		}
	}
	return b.String()
}

// displayName is the first field as written, trimmed — what the admin
// reads when deciding. Kept alongside the key so "The Conway Room"
// doesn't reach them as "the conway room".
func displayName(location string) string {
	first := location
	if i := strings.IndexAny(first, ","); i >= 0 {
		first = first[:i]
	}
	return strings.TrimSpace(first)
}

// SyncAggregator fetches one aggregator and caches its listings, then
// routes every crosswalk entry hanging off it. A failed fetch records
// itself and routes nothing: the cached listings from the last good
// fetch stay exactly as they were, so an unreachable city calendar can't
// empty forty patches (docs/adr/031's rule, inherited).
func SyncAggregator(ctx context.Context, db *database.DB, notifier *notifications.Notifier, aggregatorID string) error {
	mu, _ := aggregatorLocks.LoadOrStore(aggregatorID, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	var src Source
	var paused bool
	err := db.QueryRow(
		`SELECT id, type, url, added_by, etag, last_modified, last_success_at, paused
		 FROM aggregators WHERE id = ?`, aggregatorID,
	).Scan(&src.ID, &src.Type, &src.URL, &src.AddedBy, &src.Etag,
		&src.LastModified, &src.LastSuccessAt, &paused)
	if err != nil {
		return fmt.Errorf("load aggregator: %w", err)
	}
	if paused {
		// A seamrip import, or an admin who pulled the plug. Routing
		// still runs off cached listings — pausing stops the fetch, not
		// the patches that already consented.
		return routeAll(ctx, db, notifier, aggregatorID)
	}

	items, result, err := loadAggregatorItems(ctx, db, &src)
	if err != nil {
		recordAggregatorFailure(db, aggregatorID, err)
		return err
	}
	if !result.NotModified {
		if err := storeListings(db, aggregatorID, items); err != nil {
			recordAggregatorFailure(db, aggregatorID, err)
			return err
		}
	}
	recordAggregatorSuccess(db, aggregatorID, result.Etag, result.LastModified)

	return routeAll(ctx, db, notifier, aggregatorID)
}

// loadAggregatorItems is loadItems' fetch half, persisting a detected
// type to the aggregator row instead of the source row — an admin pastes
// an address, not a format, and that holds for a city calendar too.
func loadAggregatorItems(ctx context.Context, db *database.DB, src *Source) ([]Item, *fetchResult, error) {
	declared := src.Type
	items, result, err := loadItemsFor(ctx, src)
	if err != nil {
		return nil, nil, err
	}
	if src.Type != declared {
		if _, err := db.Exec(
			`UPDATE aggregators SET type = ?, updated_at = ? WHERE id = ?`,
			src.Type, nowStamp(), src.ID,
		); err != nil {
			return nil, nil, fmt.Errorf("persist detected type: %w", err)
		}
	}
	return items, result, nil
}

// storeListings replaces the cached listings wholesale. The cache is
// what the last successful fetch carried and nothing else — a listing
// the feed dropped must stop appearing as an unrouted name, or admins
// map ghosts.
func storeListings(db *database.DB, aggregatorID string, items []Item) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin listings: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM aggregator_listings WHERE aggregator_id = ?`, aggregatorID); err != nil {
		return fmt.Errorf("clear listings: %w", err)
	}
	for _, it := range items {
		if _, err := tx.Exec(
			`INSERT OR REPLACE INTO aggregator_listings
			 (aggregator_id, uid, occurrence, name_key, display_name, title,
			  title_key, description, location, latitude, longitude, starts_at, ends_at, url)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			aggregatorID, it.UID, it.Occurrence, NameKey(it.Location), displayName(it.Location),
			it.Title, TitleKey(it.Title), it.Description, it.Location, it.Latitude, it.Longitude,
			it.StartsAt, it.EndsAt, it.URL,
		); err != nil {
			return fmt.Errorf("insert listing: %w", err)
		}
	}
	return tx.Commit()
}

// routeAll reconciles every crosswalk entry on this aggregator. Entries
// on archived or removed patches lie dormant, matching how the worker
// treats ordinary sources.
func routeAll(ctx context.Context, db *database.DB, notifier *notifications.Notifier, aggregatorID string) error {
	rows, err := db.Query(
		`SELECT es.id FROM event_sources es
		 JOIN nodes n ON n.id = es.node_id
		    AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL
		 WHERE es.aggregator_id = ?`, aggregatorID)
	if err != nil {
		return fmt.Errorf("list crosswalk entries: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := Sync(ctx, db, notifier, id); err != nil {
			log.Printf("eventsource: route crosswalk entry %s: %v", id, err)
		}
	}
	return nil
}

// listingsFor reads one crosswalk entry's items out of the cache. This
// is the whole of a crosswalk entry's "fetch": the aggregator already
// did it.
func listingsFor(db *database.DB, aggregatorID, nameKey string) ([]Item, error) {
	rows, err := db.Query(
		`SELECT uid, occurrence, title, description, location, latitude, longitude,
		 starts_at, ends_at FROM aggregator_listings
		 WHERE aggregator_id = ? AND name_key = ?`, aggregatorID, nameKey)
	if err != nil {
		return nil, fmt.Errorf("load listings: %w", err)
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.UID, &it.Occurrence, &it.Title, &it.Description,
			&it.Location, &it.Latitude, &it.Longitude, &it.StartsAt, &it.EndsAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func recordAggregatorFailure(db *database.DB, id string, cause error) {
	msg := cause.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	if _, err := db.Exec(
		`UPDATE aggregators SET status = 'error', last_fetch_at = ?, last_error = ?, updated_at = ? WHERE id = ?`,
		nowStamp(), msg, nowStamp(), id,
	); err != nil {
		log.Printf("eventsource: record aggregator failure for %s: %v", id, err)
	}
}

func recordAggregatorSuccess(db *database.DB, id, etag, lastModified string) {
	now := nowStamp()
	if _, err := db.Exec(
		`UPDATE aggregators SET status = 'ok', last_fetch_at = ?, last_success_at = ?,
		 last_error = NULL, etag = ?, last_modified = ?, updated_at = ? WHERE id = ?`,
		now, now, nullable(etag), nullable(lastModified), now, id,
	); err != nil {
		log.Printf("eventsource: record aggregator success for %s: %v", id, err)
	}
}

// Unmap removes one crosswalk entry and detaches everything it routed.
// This is the deliberate departure from Remove (docs/adr/056): an
// ordinary source's future events were the feed's promises and go with
// it, but a routed event landed on a patch that consented to this name
// individually. Unmapping means "stop sending", not "those never
// happened" — so the events stay and become the patch's own.
func Unmap(db *database.DB, sourceID string) error {
	mu, _ := sourceLocks.LoadOrStore(sourceID, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin unmap: %w", err)
	}
	defer tx.Rollback()

	// Suggestions nobody acted on go with the entry: they were never
	// this patch's events, and leaving them would strand a queue full of
	// items whose source the patch just turned off (docs/adr/056).
	if _, err := tx.Exec(
		`DELETE FROM events WHERE source_id = ? AND status = 'pending_review'`, sourceID,
	); err != nil {
		return fmt.Errorf("drop pending suggestions: %w", err)
	}
	// Everything published stays and becomes the patch's own — including
	// suggestions its admins approved, which are simply its events now.
	if _, err := tx.Exec(
		`UPDATE events SET source_id = NULL, source_uid = NULL, source_occurrence = '' WHERE source_id = ?`,
		sourceID,
	); err != nil {
		return fmt.Errorf("detach routed events: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM event_sources WHERE id = ?`, sourceID); err != nil {
		return fmt.Errorf("delete crosswalk entry: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit unmap: %w", err)
	}
	sourceLocks.Delete(sourceID)
	return nil
}

// RemoveAggregator deletes an aggregator, unmapping every crosswalk
// entry on it first so no patch loses a calendar because the instance
// admin unplugged a feed (docs/adr/056).
func RemoveAggregator(db *database.DB, aggregatorID string) error {
	rows, err := db.Query(`SELECT id FROM event_sources WHERE aggregator_id = ?`, aggregatorID)
	if err != nil {
		return fmt.Errorf("list crosswalk entries: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range ids {
		if err := Unmap(db, id); err != nil {
			return err
		}
	}

	mu, _ := aggregatorLocks.LoadOrStore(aggregatorID, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()
	if _, err := db.Exec(`DELETE FROM aggregators WHERE id = ?`, aggregatorID); err != nil {
		return fmt.Errorf("delete aggregator: %w", err)
	}
	aggregatorLocks.Delete(aggregatorID)
	return nil
}
