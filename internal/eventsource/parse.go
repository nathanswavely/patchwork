// Package eventsource pulls events into patches from standing feeds
// (docs/adr/031). An event source is attached by whoever owns the
// calendar; attaching is vouching for the feed once, so imported events
// publish without per-event review. The source stays authoritative:
// imported events follow the feed until detached, and only a successful
// fetch ever changes them.
package eventsource

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	// The distroless image has no tzdata; feeds carry TZIDs that must
	// resolve wherever the binary runs.
	_ "time/tzdata"

	"github.com/emersion/go-ical"
)

const (
	// Horizon is how far ahead recurring feed events are materialized
	// into occurrence rows (docs/adr/031).
	Horizon = 90 * 24 * time.Hour
	// pastGrace keeps items that started within the last day in the
	// desired set, so an ongoing event still receives updates and
	// cancellations instead of flapping out of the window mid-show.
	pastGrace = 24 * time.Hour
	// MaxItems caps how many items one feed may produce per sync.
	MaxItems = 500
)

// Item is one concrete event a feed wants to exist: a single event
// (Occurrence "") or one materialized occurrence of a recurring event
// (Occurrence = the original occurrence start, UTC RFC3339 — stable even
// when an override moves the actual start time).
type Item struct {
	UID         string
	Occurrence  string
	Title       string
	Description string
	Location    string
	Latitude    *float64
	Longitude   *float64
	StartsAt    string
	EndsAt      *string
}

// Key identifies an item within one source. NUL can't appear in either
// part (UIDs are single ICS text values, occurrences are RFC3339).
func Key(uid, occurrence string) string { return uid + "\x00" + occurrence }

// ParseICS extracts the desired set of items from an ICS document:
// every event whose start falls inside [now-1d, now+Horizon], with
// recurring events expanded to occurrences. Cancelled items are simply
// absent — the reconciler treats absence as removal. PRIVATE and
// CONFIDENTIAL items never leave the parser.
func ParseICS(data []byte, now time.Time) ([]Item, error) {
	cal, err := ical.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		return nil, fmt.Errorf("parse ics: %w", err)
	}

	windowStart := now.Add(-pastGrace)
	windowEnd := now.Add(Horizon)

	type group struct {
		master    *ical.Event
		overrides map[string]*ical.Event // original occurrence (UTC RFC3339) → override
	}
	groups := map[string]*group{}
	order := []string{} // deterministic output for tests and diffs

	events := cal.Events()
	for i := range events {
		e := &events[i]
		uid, _ := e.Props.Text(ical.PropUID)
		if uid == "" {
			continue // a feed item we can't track has no place in a sync
		}
		if class, _ := e.Props.Text(ical.PropClass); class == "PRIVATE" || class == "CONFIDENTIAL" {
			continue
		}
		g := groups[uid]
		if g == nil {
			g = &group{overrides: map[string]*ical.Event{}}
			groups[uid] = g
			order = append(order, uid)
		}
		if rid := e.Props.Get(ical.PropRecurrenceID); rid != nil {
			t, err := rid.DateTime(time.UTC)
			if err != nil {
				continue
			}
			g.overrides[t.UTC().Format(time.RFC3339)] = e
		} else {
			g.master = e
		}
	}

	var items []Item
	add := func(it *Item, start time.Time) {
		if start.Before(windowStart) || start.After(windowEnd) {
			return
		}
		items = append(items, *it)
	}

	for _, uid := range order {
		g := groups[uid]

		// Overrides without a master happen in sliced feeds; each stands
		// alone. Emitted after the master's expansion consumes its share.
		if g.master == nil {
			for occ, ov := range g.overrides {
				if it, start, ok := itemFromEvent(ov, uid, occ); ok {
					add(it, start)
				}
			}
			continue
		}

		if status, _ := g.master.Status(); status == ical.EventCancelled {
			continue // whole series cancelled: nothing desired
		}

		set, err := g.master.RecurrenceSet(time.UTC)
		if err != nil || set == nil {
			// No RRULE (or one we can't parse — treat as non-recurring
			// rather than dropping the event entirely).
			if it, start, ok := itemFromEvent(g.master, uid, ""); ok {
				add(it, start)
			}
			continue
		}

		masterStart, err := g.master.DateTimeStart(time.UTC)
		if err != nil {
			continue
		}
		var duration time.Duration
		if masterEnd, err := g.master.DateTimeEnd(time.UTC); err == nil && masterEnd.After(masterStart) {
			duration = masterEnd.Sub(masterStart)
		}

		consumed := map[string]bool{}
		for _, occ := range set.Between(windowStart, windowEnd, true) {
			occKey := occ.UTC().Format(time.RFC3339)
			if ov, ok := g.overrides[occKey]; ok {
				consumed[occKey] = true
				if it, start, ok := itemFromEvent(ov, uid, occKey); ok {
					add(it, start)
				}
				continue
			}
			it, _, ok := itemFromEvent(g.master, uid, occKey)
			if !ok {
				continue
			}
			it.StartsAt = occ.UTC().Format(time.RFC3339)
			it.EndsAt = nil
			if duration > 0 {
				e := occ.Add(duration).UTC().Format(time.RFC3339)
				it.EndsAt = &e
			}
			add(it, occ)
		}

		// Overrides moved to a time whose original occurrence sits
		// outside the window still deserve to exist if their new start
		// is inside it.
		for occKey, ov := range g.overrides {
			if consumed[occKey] {
				continue
			}
			if it, start, ok := itemFromEvent(ov, uid, occKey); ok {
				add(it, start)
			}
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].StartsAt != items[j].StartsAt {
			return items[i].StartsAt < items[j].StartsAt
		}
		return Key(items[i].UID, items[i].Occurrence) < Key(items[j].UID, items[j].Occurrence)
	})
	if len(items) > MaxItems {
		items = items[:MaxItems]
	}
	return items, nil
}

// itemFromEvent builds an Item from one VEVENT. ok is false when the
// event is unusable (no valid start) or cancelled.
func itemFromEvent(e *ical.Event, uid, occurrence string) (*Item, time.Time, bool) {
	if status, _ := e.Status(); status == ical.EventCancelled {
		return nil, time.Time{}, false
	}
	start, err := e.DateTimeStart(time.UTC)
	if err != nil || start.IsZero() {
		return nil, time.Time{}, false
	}

	title, _ := e.Props.Text(ical.PropSummary)
	if strings.TrimSpace(title) == "" {
		title = "(untitled)"
	}
	description, _ := e.Props.Text(ical.PropDescription)
	location, _ := e.Props.Text(ical.PropLocation)

	it := &Item{
		UID:         uid,
		Occurrence:  occurrence,
		Title:       title,
		Description: description,
		Location:    location,
		StartsAt:    start.UTC().Format(time.RFC3339),
	}
	if end, err := e.DateTimeEnd(time.UTC); err == nil && end.After(start) {
		s := end.UTC().Format(time.RFC3339)
		it.EndsAt = &s
	}
	if geo := e.Props.Get(ical.PropGeo); geo != nil {
		if parts := strings.SplitN(geo.Value, ";", 2); len(parts) == 2 {
			lat, errLat := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			lng, errLng := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if errLat == nil && errLng == nil {
				it.Latitude, it.Longitude = &lat, &lng
			}
		}
	}
	return it, start, true
}
