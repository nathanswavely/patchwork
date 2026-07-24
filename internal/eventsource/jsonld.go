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
func ParseJSONLD(data []byte, now time.Time) ([]Item, error) {
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

	var items []Item
	seen := map[string]bool{}
	for _, ev := range events {
		if status, _ := ev["eventStatus"].(string); strings.Contains(status, "Cancelled") {
			continue
		}
		start, err := parseJSONLDTime(str(ev["startDate"]))
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
		if end, err := parseJSONLDTime(str(ev["endDate"])); err == nil && end.After(start) {
			s := end.UTC().Format(time.RFC3339)
			it.EndsAt = &s
		}
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

// parseJSONLDTime accepts the ISO 8601 shapes seen in the wild:
// RFC 3339, offsets without a colon (Humanitix), floating date-times
// (treated as UTC), and bare dates.
func parseJSONLDTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
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
