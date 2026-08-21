package eventsource

// Parsing community.lexicon.calendar.event records into items
// (docs/adr/063).
//
// The lexicon requires only `createdAt` and `name`. Everything this
// project needs to make an event — above all a start time — is optional
// there, so most of the work here is deciding what to do about absence.

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/atproto"
)

// lexEvent is the subset of community.lexicon.calendar.event read here.
type lexEvent struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	StartsAt    string `json:"startsAt"`
	EndsAt      string `json:"endsAt"`
	// locations and uris are open arrays in the lexicon — entries are
	// typed by $type, and unknown shapes are ignored rather than guessed.
	Locations []json.RawMessage `json:"locations"`
	URIs      []json.RawMessage `json:"uris"`
}

type lexLocation struct {
	Type        string `json:"$type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// community.lexicon.location.address
	Street     string `json:"street"`
	Locality   string `json:"locality"`
	Region     string `json:"region"`
	Country    string `json:"country"`
	PostalCode string `json:"postalCode"`
	// community.lexicon.location.geo
	Latitude  string `json:"latitude"`
	Longitude string `json:"longitude"`
}

type lexURI struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// ParseATProtoEvents turns repository records into items, keeping those
// whose start falls inside the same window ICS sources use.
//
// A record with no parsable `startsAt` is skipped rather than defaulted.
// createdAt, today, and import time are all available and all wrong: each
// would put a fictional date on a venue's public calendar, and a record
// that does not say when it happens is not yet an event (docs/adr/063
// decision 3).
func ParseATProtoEvents(records []atproto.Record, now time.Time) ([]Item, error) {
	horizonEnd := now.Add(Horizon)
	windowStart := now.AddDate(0, 0, -1)

	items := []Item{}
	for _, rec := range records {
		var ev lexEvent
		if err := json.Unmarshal(rec.Value, &ev); err != nil {
			continue // one malformed record must not fail the whole sync
		}
		title := strings.TrimSpace(ev.Name)
		if title == "" {
			continue
		}
		start, ok := parseLexTime(ev.StartsAt)
		if !ok {
			continue
		}
		if start.Before(windowStart) || start.After(horizonEnd) {
			continue
		}
		rkey := rec.Rkey()
		if rkey == "" {
			continue
		}

		item := Item{
			// docs/adr/063 decision 4: the rkey is the UID. Occurrence
			// stays empty — the lexicon has no recurrence, so a repeating
			// event is repeated records and there is nothing to expand.
			UID:         rkey,
			Title:       title,
			Description: strings.TrimSpace(ev.Description),
			StartsAt:    start.UTC().Format(time.RFC3339),
			Location:    locationFrom(ev.Locations),
			URL:         uriFrom(ev.URIs),
		}
		if end, ok := parseLexTime(ev.EndsAt); ok && end.After(start) {
			s := end.UTC().Format(time.RFC3339)
			item.EndsAt = &s
		}
		items = append(items, item)
	}
	return items, nil
}

// parseLexTime reads a lexicon datetime. RFC3339 is what the format
// demands; a value that isn't one is treated as absent rather than
// coerced.
func parseLexTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// locationFrom renders the first location entry that carries a usable
// name, following ADR 046: location is one field, written name-first.
func locationFrom(raw []json.RawMessage) string {
	for _, r := range raw {
		var loc lexLocation
		if json.Unmarshal(r, &loc) != nil {
			continue
		}
		if name := strings.TrimSpace(loc.Name); name != "" {
			if street := strings.TrimSpace(loc.Street); street != "" {
				return name + ", " + street
			}
			return name
		}
		// An address with no name still says where: build it name-first
		// from what there is rather than dropping the location entirely.
		parts := []string{}
		for _, p := range []string{loc.Street, loc.Locality, loc.Region} {
			if p = strings.TrimSpace(p); p != "" {
				parts = append(parts, p)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, ", ")
		}
	}
	return ""
}

// uriFrom returns the first http(s) URI on the record — the feed's own
// page for the listing, in the sense Item.URL means.
func uriFrom(raw []json.RawMessage) string {
	for _, r := range raw {
		var u lexURI
		if json.Unmarshal(r, &u) != nil {
			continue
		}
		v := strings.TrimSpace(u.URI)
		if strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "http://") {
			return v
		}
	}
	return ""
}
