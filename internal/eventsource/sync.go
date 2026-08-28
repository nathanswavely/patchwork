package eventsource

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/atproto"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// Source is one event_sources row, loaded fresh at sync time. It also
// carries an aggregator row where the two overlap, so the fetch-and-
// detect path can serve both (docs/adr/056).
type Source struct {
	ID            string
	NodeID        string
	Type          string
	URL           string
	AddedBy       string
	Etag          sql.NullString
	LastModified  sql.NullString
	LastSuccessAt sql.NullString
	// AggregatorID and NameKey make this row a crosswalk entry rather
	// than a feed of its own: its items come from the aggregator's
	// cached listings under NameKey, and it never fetches anything.
	AggregatorID sql.NullString
	NameKey      sql.NullString
	// Suggests routes into the patch's review queue rather than
	// publishing (docs/adr/056). The patch opened its door to
	// suggestions; it did not adopt the feed.
	Suggests bool
	// Zone is what a time in this feed means when the feed itself does
	// not say: the patch's zone, else the instance's (docs/adr/045).
	// Resolved once when the source is loaded, so the whole sync reads
	// one answer.
	Zone *time.Location
	// LocalTimeStampedUTC marks a publisher that emits the venue's wall
	// clock as though it were UTC (docs/adr/073). Set by whoever attached
	// the source, because only a person comparing the markup against the
	// page can tell — the feed's own offset is internally consistent and
	// simply wrong.
	LocalTimeStampedUTC bool
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
	var zoneName string
	err := db.QueryRow(
		`SELECT s.id, s.node_id, s.type, s.url, s.added_by, s.etag, s.last_modified,
		 s.last_success_at, s.aggregator_id, s.name_key, s.suggests,
		 s.local_time_stamped_utc, COALESCE(NULLIF(n.timezone,''), ?)
		 FROM event_sources s JOIN nodes n ON n.id = s.node_id WHERE s.id = ?`,
		settings.EffectiveTimezone(db), sourceID,
	).Scan(&src.ID, &src.NodeID, &src.Type, &src.URL, &src.AddedBy,
		&src.Etag, &src.LastModified, &src.LastSuccessAt,
		&src.AggregatorID, &src.NameKey, &src.Suggests,
		&src.LocalTimeStampedUTC, &zoneName)
	if err != nil {
		return fmt.Errorf("load source: %w", err)
	}
	src.Zone = loadZone(zoneName)

	items, result, err := loadItems(ctx, db, &src)
	if err != nil {
		recordFailure(db, src.ID, err)
		return err
	}
	// After every parser, so the correction reads the same whichever
	// format the feed turned out to be — and after a crosswalk entry's
	// cached listings too, since a publisher's defect belongs to the
	// publisher rather than to the shape it publishes in.
	if src.LocalTimeStampedUTC {
		items = correctStampedUTC(items, src.Zone)
	}
	if result.NotModified {
		recordSuccess(db, src.ID, src.Etag.String, src.LastModified.String)
		return nil
	}

	if err := reconcile(db, notifier, &src, items); err != nil {
		recordFailure(db, src.ID, err)
		return err
	}
	recordSuccess(db, src.ID, result.Etag, result.LastModified)
	return nil
}

// loadItems supplies a source's desired items. A crosswalk entry takes
// them from the aggregator's cached listings and fetches nothing — the
// aggregator already fetched, once, for all of its entries
// (docs/adr/056). Everything else fetches its own feed, and a successful
// type detection is persisted so later syncs skip the probes.
func loadItems(ctx context.Context, db *database.DB, src *Source) ([]Item, *fetchResult, error) {
	if src.AggregatorID.Valid {
		items, err := listingsFor(db, src.AggregatorID.String, src.NameKey.String)
		if err != nil {
			return nil, nil, err
		}
		return items, &fetchResult{}, nil
	}

	declared := src.Type
	items, result, err := loadItemsFor(ctx, src)
	if err != nil {
		return nil, nil, err
	}
	if src.Type != declared {
		if _, err := db.Exec(
			`UPDATE event_sources SET type = ?, updated_at = ? WHERE id = ?`,
			src.Type, nowStamp(), src.ID,
		); err != nil {
			return nil, nil, fmt.Errorf("persist detected type: %w", err)
		}
	}
	return items, result, nil
}

// loadItemsFor fetches and parses according to src.Type. An 'ics' source
// whose document doesn't parse gets one shot at being read as schema.org
// markup and then as a Squarespace events page — "paste the calendar's
// address" shouldn't require knowing which kind of address it is. A
// successful detection is written to src.Type; persisting it belongs to
// the caller, because sources and aggregators keep it in different
// tables.
func loadItemsFor(ctx context.Context, src *Source) ([]Item, *fetchResult, error) {
	now := time.Now().UTC()

	if src.Type == "squarespace" {
		jsonURL, err := squarespaceJSONURL(src.URL)
		if err != nil {
			return nil, nil, err
		}
		result, err := fetchFeed(ctx, jsonURL, src.Etag.String, src.LastModified.String)
		if err != nil || result.NotModified {
			return nil, result, err
		}
		items, err := ParseSquarespace(result.Body, now)
		if err != nil {
			return nil, nil, err
		}
		return items, result, nil
	}

	// An atproto source names a repository, not a document: resolve the
	// DID to its PDS and read one collection (docs/adr/064). No
	// conditional GET — listRecords carries no etag — so this refetches
	// in full each cycle, which a venue-sized collection can afford.
	if src.Type == "atproto" {
		did, collection, err := atproto.ParseATURI(src.URL)
		if err != nil {
			return nil, nil, err
		}
		res := atprotoResolver()
		doc, err := res.ResolveDoc(did)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve %s: %w", did, err)
		}
		pds, err := doc.PDSEndpoint()
		if err != nil {
			return nil, nil, err
		}
		records, err := res.ListRecords(pds, did, collection)
		if err != nil {
			return nil, nil, err
		}
		items, err := ParseATProtoEvents(records, now)
		if err != nil {
			return nil, nil, err
		}
		return items, &fetchResult{}, nil
	}

	if src.Type == "jsonld" {
		result, err := fetchFeed(ctx, src.URL, src.Etag.String, src.LastModified.String)
		if err != nil || result.NotModified {
			return nil, result, err
		}
		items, err := ParseJSONLD(result.Body, now, src.Zone)
		if err != nil {
			return nil, nil, err
		}
		return items, result, nil
	}

	result, err := fetchFeed(ctx, src.URL, src.Etag.String, src.LastModified.String)
	if err != nil || result.NotModified {
		return nil, result, err
	}
	items, icsErr := ParseICS(result.Body, now, src.Zone)
	if icsErr == nil {
		return items, result, nil
	}

	// Not ICS. The page is already in hand, so the JSON-LD probe is
	// free: any schema.org Event markup makes this a jsonld source.
	if ldItems, ldErr := ParseJSONLD(result.Body, now, src.Zone); ldErr == nil {
		src.Type = "jsonld"
		return ldItems, result, nil
	}

	// Still unknown — probe the Squarespace JSON view once. Any failure
	// from here reports the ORIGINAL ICS error: the admin pasted
	// something that claimed to be a calendar, and that's the story
	// they need.
	jsonURL, err := squarespaceJSONURL(src.URL)
	if err != nil {
		return nil, nil, icsErr
	}
	ssResult, err := fetchFeed(ctx, jsonURL, "", "")
	if err != nil || ssResult.NotModified {
		return nil, nil, icsErr
	}
	ssItems, err := ParseSquarespace(ssResult.Body, now)
	if err != nil {
		return nil, nil, icsErr
	}
	src.Type = "squarespace"
	return ssItems, ssResult, nil
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
	// Removed marks a soft-removed (moderated) row. It still occupies
	// the source-identity unique index, so the reconciler must see it —
	// and must leave it alone: moderation outranks the feed.
	Removed bool
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

	// Removed rows are included on purpose: they still hold their slot in
	// the source-identity unique index, and a reconciler blind to them
	// would re-INSERT the same key and wedge the source in 'error'.
	existing := map[string]existingEvent{}
	rows, err = db.Query(
		`SELECT source_uid, source_occurrence, id, title, description, location,
		 latitude, longitude, starts_at, ends_at, removed_at IS NOT NULL
		 FROM events WHERE source_id = ?`, src.ID)
	if err != nil {
		return fmt.Errorf("load existing: %w", err)
	}
	for rows.Next() {
		var uid, occ string
		var e existingEvent
		if err := rows.Scan(&uid, &occ, &e.ID, &e.Title, &e.Description, &e.Location,
			&e.Latitude, &e.Longitude, &e.StartsAt, &e.EndsAt, &e.Removed); err != nil {
			rows.Close()
			return err
		}
		existing[Key(uid, occ)] = e
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Listings already held as possible duplicates are decided-pending,
	// not undecided: re-checking them every hour would re-hold what an
	// admin is already looking at (docs/adr/056).
	held := map[string]bool{}
	if src.AggregatorID.Valid {
		rows, err = db.Query(`SELECT uid, occurrence FROM aggregator_holds WHERE source_id = ?`, src.ID)
		if err != nil {
			return fmt.Errorf("load holds: %w", err)
		}
		for rows.Next() {
			var uid, occ string
			if err := rows.Scan(&uid, &occ); err != nil {
				rows.Close()
				return err
			}
			held[Key(uid, occ)] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
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

	// One transaction for the whole reconcile: a mid-sync failure must
	// not leave half a calendar applied, and one fsync beats hundreds on
	// the Pi-class hardware this targets. Announcements wait for commit —
	// nobody gets notified about an event a rollback then erases.
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin reconcile: %w", err)
	}
	defer tx.Rollback()

	var announcements []model.Event

	for k, it := range desired {
		if prev, ok := existing[k]; ok {
			// Moderation outranks the feed: a soft-removed row is
			// matched (so it can't be re-inserted) but never revived
			// or edited by a sync.
			if prev.Removed || !changed(prev, it) {
				continue
			}
			_, err := tx.Exec(
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

		if held[k] {
			continue
		}

		// A suggesting entry needs no duplicate hold: every item it
		// brings already stops at a human, and a reviewer looking at a
		// show they already have will reject it (docs/adr/056).
		if src.Suggests {
			id := auth.NewUUIDv7()
			apID := ap.EventAPID(ap.GetDomain(), id)
			if _, err := tx.Exec(
				`INSERT INTO events (id, node_id, created_by, title, description, location,
				 latitude, longitude, starts_at, ends_at, recurrence, visibility, status,
				 ap_id, source_id, source_uid, source_occurrence)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 'public', 'pending_review', ?, ?, ?, ?)`,
				id, src.NodeID, src.AddedBy, it.Title, it.Description, it.Location,
				it.Latitude, it.Longitude, it.StartsAt, it.EndsAt, apID,
				src.ID, it.UID, it.Occurrence,
			); err != nil {
				return fmt.Errorf("insert suggestion: %w", err)
			}
			// Deliberately not announced and never broadcast: a pending
			// event is not news, and never federates (docs/adr/026).
			continue
		}

		// A listing arriving on a patch that already has an event at that
		// instant is held, never guessed at (docs/adr/056). Titles are
		// not compared — the city writes "Music Friday hosted by Music
		// For Everyone" where the venue writes "Music Friday" — so the
		// collision signal is the start instant alone, and the patch's
		// own event wins until one of its admins says otherwise.
		if src.AggregatorID.Valid {
			var rivalID string
			err := tx.QueryRow(
				`SELECT id FROM events WHERE node_id = ? AND starts_at = ?
				 AND removed_at IS NULL AND (source_id IS NULL OR source_id != ?)
				 ORDER BY created_at LIMIT 1`,
				src.NodeID, it.StartsAt, src.ID,
			).Scan(&rivalID)
			if err == nil {
				if _, err := tx.Exec(
					`INSERT OR IGNORE INTO aggregator_holds
					 (id, source_id, node_id, uid, occurrence, rival_event_id, title, location, starts_at)
					 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					auth.NewUUIDv7(), src.ID, src.NodeID, it.UID, it.Occurrence,
					rivalID, it.Title, it.Location, it.StartsAt,
				); err != nil {
					return fmt.Errorf("hold listing: %w", err)
				}
				continue
			} else if err != sql.ErrNoRows {
				return fmt.Errorf("check for duplicate: %w", err)
			}
		}

		id := auth.NewUUIDv7()
		apID := ap.EventAPID(ap.GetDomain(), id)
		_, err := tx.Exec(
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
			announcements = append(announcements, model.Event{
				ID: id, NodeID: src.NodeID, CreatedBy: src.AddedBy,
				Title: it.Title, Description: it.Description, Location: it.Location,
				Latitude: it.Latitude, Longitude: it.Longitude,
				StartsAt: it.StartsAt, EndsAt: it.EndsAt, Visibility: "public",
			})
		}
	}

	// Future events the feed no longer carries are promises withdrawn;
	// the past belongs to the patch and stays, and moderated rows stay
	// moderated (docs/adr/031).
	for k, prev := range existing {
		if _, ok := desired[k]; ok {
			continue
		}
		if prev.Removed || prev.StartsAt <= now {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM events WHERE id = ?`, prev.ID); err != nil {
			return fmt.Errorf("delete event %s: %w", prev.ID, err)
		}
	}

	// A held listing the feed no longer carries is not a question anymore.
	for k := range held {
		if _, ok := desired[k]; ok {
			continue
		}
		uid, occ, _ := strings.Cut(k, "\x00")
		if _, err := tx.Exec(
			`DELETE FROM aggregator_holds WHERE source_id = ? AND uid = ? AND occurrence = ?`,
			src.ID, uid, occ,
		); err != nil {
			return fmt.Errorf("drop stale hold: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reconcile: %w", err)
	}

	for _, e := range announcements {
		if notifier != nil {
			go notifier.Notify(notifications.Event{
				Type:     notifications.EventCreated,
				NodeID:   src.NodeID,
				NodeSlug: nodeSlug,
				NodeName: nodeName,
				ActorID:  src.AddedBy,
				EntityID: e.ID,
				Title:    "New event: " + e.Title,
				Link:     weblink.Event(e.ID),
			})
		}
		broadcastCreate(db, e, src.NodeID)
	}
	offerAnnouncements(db, notifier, src, announcements)
	return nil
}

// offerAnnouncements tells credited patches that a new listing matched one
// of their programs (docs/adr/063). Only newly created events pass
// through here, which is what makes crediting silent back-fill: everything
// the feed already carried became an offer the instant the program
// landed, and nobody wants six notifications for a decision just made.
//
// A program still holding a NULL backfilled_at has never had a routing
// pass — it arrived in a seamrip. It gets one silent pass and then
// announces, the same courtesy a fresh crosswalk entry gets.
func offerAnnouncements(db *database.DB, notifier *notifications.Notifier, src *Source, created []model.Event) {
	if !src.AggregatorID.Valid || !src.NameKey.Valid || len(created) == 0 {
		return
	}
	rows, err := db.Query(
		`SELECT p.id, p.node_id, n.name, n.slug, p.title_key, p.display_title,
		        p.backfilled_at IS NOT NULL
		   FROM aggregator_programs p
		   JOIN nodes n ON n.id = p.node_id
		    AND n.status IN ('active','unclaimed') AND n.removed_at IS NULL
		  WHERE p.aggregator_id = ? AND p.name_key = ? AND p.node_id != ?`,
		src.AggregatorID.String, src.NameKey.String, src.NodeID)
	if err != nil {
		return
	}
	type program struct {
		id, nodeID, nodeName, nodeSlug, titleKey, displayTitle string
		backfilled                                             bool
	}
	var programs []program
	for rows.Next() {
		var p program
		if err := rows.Scan(&p.id, &p.nodeID, &p.nodeName, &p.nodeSlug,
			&p.titleKey, &p.displayTitle, &p.backfilled); err == nil {
			programs = append(programs, p)
		}
	}
	rows.Close()

	for _, p := range programs {
		matched := 0
		var first *model.Event
		for i := range created {
			if TitleKey(created[i].Title) == p.titleKey {
				matched++
				if first == nil {
					first = &created[i]
				}
			}
		}
		if matched == 0 {
			continue
		}
		if !p.backfilled {
			db.Exec(`UPDATE aggregator_programs
			         SET backfilled_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			         WHERE id = ?`, p.id)
			continue
		}
		title := "A listing matched " + p.displayTitle
		if matched > 1 {
			title = fmt.Sprintf("%d listings matched %s", matched, p.displayTitle)
		}
		if notifier != nil {
			go notifier.Notify(notifications.Event{
				Type:     notifications.ProgramOffer,
				NodeID:   p.nodeID,
				NodeSlug: p.nodeSlug,
				NodeName: p.nodeName,
				EntityID: first.ID,
				Title:    title,
				Body:     "Propose a link if it is yours, or dismiss the offer.",
				Link:     weblink.PatchSources(p.nodeSlug),
			})
		}
	}
}

// Remove deletes a source under the same per-source lock Sync holds, so
// a mid-reconcile insert can't slip between its steps, and in one
// transaction, so it can't half-apply. Past events stay with the patch
// as detached history (moderated rows stay moderated, just detached);
// future imported events were the feed's promises and go with it
// (docs/adr/031).
func Remove(db *database.DB, sourceID string) error {
	mu, _ := sourceLocks.LoadOrStore(sourceID, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin remove: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(
		`DELETE FROM events WHERE source_id = ? AND removed_at IS NULL AND starts_at > ?`,
		sourceID, now,
	); err != nil {
		return fmt.Errorf("remove future events: %w", err)
	}
	if _, err := tx.Exec(
		`UPDATE events SET source_id = NULL, source_uid = NULL, source_occurrence = '' WHERE source_id = ?`,
		sourceID,
	); err != nil {
		return fmt.Errorf("detach past events: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM event_sources WHERE id = ?`, sourceID); err != nil {
		return fmt.Errorf("delete source: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remove: %w", err)
	}
	sourceLocks.Delete(sourceID)
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
