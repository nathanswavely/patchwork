package eventsource

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The generic markup door docs/adr/031 anticipated: any page that embeds
// schema.org Event objects as JSON-LD (Humanitix host pages, SEO-minded
// venue sites, many event platforms) can be an event source. One fetch,
// no per-platform adapter — the page's own search-engine markup is the
// feed.

var ldScriptRe = regexp.MustCompile(`(?is)<script[^>]*type="application/ld\+json"[^>]*>(.*?)</script>`)

// ParseJSONLD extracts schema.org Events from an HTML document's JSON-LD
// blocks, applying the same window as the other parsers. It errors when
// the page carries no Event markup at all, so the auto-detect chain can
// tell "not this kind of page" from "empty calendar".
func ParseJSONLD(data []byte, now time.Time, zone *time.Location) ([]Item, error) {
	var events []map[string]any
	for _, m := range ldScriptRe.FindAllSubmatch(data, -1) {
		var doc any
		if err := json.Unmarshal(m[1], &doc); err != nil {
			continue // one malformed block shouldn't kill the page
		}
		collectEvents(doc, &events, 0)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no schema.org Event markup found")
	}

	windowStart := now.Add(-pastGrace)
	windowEnd := now.Add(Horizon)

	// schema.org has no calendar-wide zone to consult the way ICS does,
	// and a great many CMS plugins emit startDate as the site's local
	// wall clock with no offset at all. Those read in the patch's zone
	// rather than as UTC (docs/adr/045).
	if zone == nil {
		zone = time.UTC
	}

	var items []Item
	seen := map[string]bool{}
	for _, ev := range events {
		if status, _ := ev["eventStatus"].(string); strings.Contains(status, "Cancelled") {
			continue
		}
		start, err := parseJSONLDTime(str(ev["startDate"]), zone)
		if err != nil {
			continue
		}
		if start.Before(windowStart) || start.After(windowEnd) {
			continue
		}

		uid := str(ev["url"])
		if uid == "" {
			uid = str(ev["@id"])
		}
		if uid == "" {
			// No stable identity in the markup; derive one from the
			// fields that make the event itself.
			sum := sha256.Sum256([]byte(str(ev["name"]) + "|" + str(ev["startDate"])))
			uid = "jsonld-" + hex.EncodeToString(sum[:8])
		}
		if seen[uid] {
			continue
		}
		seen[uid] = true

		title := strings.TrimSpace(html.UnescapeString(str(ev["name"])))
		if title == "" {
			title = "(untitled)"
		}
		it := Item{
			UID:         uid,
			Occurrence:  "",
			Title:       title,
			Description: stripHTML(str(ev["description"])),
			StartsAt:    start.UTC().Format(time.RFC3339),
		}
		if end, err := parseJSONLDTime(str(ev["endDate"]), zone); err == nil && end.After(start) {
			s := end.UTC().Format(time.RFC3339)
			it.EndsAt = &s
		}
		it.URL = jsonLDEventURL(ev)
		fillJSONLDLocation(ev["location"], &it)
		items = append(items, it)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].StartsAt != items[j].StartsAt {
			return items[i].StartsAt < items[j].StartsAt
		}
		return items[i].UID < items[j].UID
	})
	if len(items) > MaxItems {
		items = items[:MaxItems]
	}
	return items, nil
}

// collectEvents walks a decoded JSON-LD value gathering every node whose
// @type is Event or a subtype (MusicEvent, TheaterEvent, …). Covers
// bare objects, arrays, @graph, and ItemList/itemListElement shapes.
func collectEvents(v any, out *[]map[string]any, depth int) {
	if depth > 8 {
		return
	}
	switch node := v.(type) {
	case map[string]any:
		if isEventType(node["@type"]) {
			*out = append(*out, node)
		}
		for _, child := range node {
			collectEvents(child, out, depth+1)
		}
	case []any:
		for _, child := range node {
			collectEvents(child, out, depth+1)
		}
	}
}

func isEventType(t any) bool {
	switch tt := t.(type) {
	case string:
		return strings.HasSuffix(tt, "Event")
	case []any:
		for _, e := range tt {
			if s, ok := e.(string); ok && strings.HasSuffix(s, "Event") {
				return true
			}
		}
	}
	return false
}

// parseJSONLDTime accepts the ISO 8601 shapes seen in the wild: RFC
// 3339, offsets without a colon (Humanitix), floating date-times, and
// bare dates. The first two carry their own offset and ignore zone; the
// last two carry none and are read in it.
func parseJSONLDTime(s string, zone *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
	} {
		// ParseInLocation for all of them: the layouts that carry an
		// offset use it, and only the ones that don't fall back to zone.
		if t, err := time.ParseInLocation(layout, s, zone); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date %q", s)
}

// fillJSONLDLocation maps schema.org's location shapes (a plain string,
// or a Place with a string-or-PostalAddress address and optional geo).
func fillJSONLDLocation(loc any, it *Item) {
	switch l := loc.(type) {
	case string:
		it.Location = strings.TrimSpace(html.UnescapeString(l))
	case map[string]any:
		parts := nonEmpty(str(l["name"]))
		switch addr := l["address"].(type) {
		case string:
			parts = append(parts, strings.TrimSpace(addr))
		case map[string]any:
			if s := str(addr["streetAddress"]); s != "" {
				parts = append(parts, s)
			}
		}
		it.Location = strings.Join(nonEmpty(parts...), ", ")
		if geo, ok := l["geo"].(map[string]any); ok {
			lat, latOK := toFloat(geo["latitude"])
			lng, lngOK := toFloat(geo["longitude"])
			if latOK && lngOK {
				it.Latitude, it.Longitude = &lat, &lng
			}
		}
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

// jsonLDEventURL finds the page a listing wants a reader sent to
// (docs/adr/079). `url` on the Event is the canonical answer; when the
// markup omits it — common on ticketing platforms, where the Event node
// describes the show and the offer holds the buy link — the first offer
// with a URL is the next best thing, and is usually what the visitor
// actually wanted.
//
// Only http(s) survives. Some CMS plugins emit a bare path or a mailto:,
// and a relative URL has no base here worth guessing at.
func jsonLDEventURL(ev map[string]any) string {
	if u := absoluteHTTPURL(str(ev["url"])); u != "" {
		return u
	}
	return offerURL(ev["offers"], 0)
}

// offerURL walks schema.org offers, which appear as a bare object, an
// array, or (rarely) nested one level.
func offerURL(v any, depth int) string {
	if depth > 2 {
		return ""
	}
	switch node := v.(type) {
	case map[string]any:
		if u := absoluteHTTPURL(str(node["url"])); u != "" {
			return u
		}
		return offerURL(node["offers"], depth+1)
	case []any:
		for _, item := range node {
			if u := offerURL(item, depth+1); u != "" {
				return u
			}
		}
	}
	return ""
}

func absoluteHTTPURL(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") {
		return s
	}
	return ""
}
