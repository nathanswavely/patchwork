package eventsource

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Squarespace events collections have no whole-calendar ICS, but every
// collection page serves structured JSON at ?format=json — one fetch,
// stable item ids, epoch timestamps. This is the second source type the
// event_sources.type column anticipated (docs/adr/031); it exists
// because small venues live on Squarespace.

// squarespaceJSONURL rewrites a collection page URL to its JSON view.
func squarespaceJSONURL(pageURL string) (string, error) {
	u, err := url.Parse(pageURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("format", "json")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

type squarespaceItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	StartDate int64  `json:"startDate"` // epoch milliseconds
	EndDate   int64  `json:"endDate"`
	Excerpt   string `json:"excerpt"`
	Location  struct {
		AddressTitle string  `json:"addressTitle"`
		AddressLine1 string  `json:"addressLine1"`
		AddressLine2 string  `json:"addressLine2"`
		MarkerLat    float64 `json:"markerLat"`
		MarkerLng    float64 `json:"markerLng"`
	} `json:"location"`
}

type squarespaceCollection struct {
	Collection struct {
		TypeName string `json:"typeName"`
	} `json:"collection"`
	Upcoming []squarespaceItem `json:"upcoming"`
	Past     []squarespaceItem `json:"past"`
	Items    []squarespaceItem `json:"items"`
}

// ParseSquarespace extracts the desired item set from a Squarespace
// collection JSON document, applying the same window as ParseICS.
// It errors when the document isn't an events collection, so the
// auto-detect path can tell "not Squarespace" from "empty calendar".
func ParseSquarespace(data []byte, now time.Time) ([]Item, error) {
	var doc squarespaceCollection
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse squarespace json: %w", err)
	}
	if !strings.Contains(doc.Collection.TypeName, "events") {
		return nil, fmt.Errorf("not a squarespace events collection (type %q)", doc.Collection.TypeName)
	}

	windowStart := now.Add(-pastGrace)
	windowEnd := now.Add(Horizon)

	var items []Item
	seen := map[string]bool{}
	for _, group := range [][]squarespaceItem{doc.Upcoming, doc.Items, doc.Past} {
		for _, si := range group {
			if si.ID == "" || si.StartDate == 0 || seen[si.ID] {
				continue
			}
			start := time.UnixMilli(si.StartDate).UTC()
			if start.Before(windowStart) || start.After(windowEnd) {
				continue
			}
			seen[si.ID] = true

			title := strings.TrimSpace(si.Title)
			if title == "" {
				title = "(untitled)"
			}
			it := Item{
				UID:         si.ID,
				Occurrence:  "",
				Title:       title,
				Description: stripHTML(si.Excerpt),
				StartsAt:    start.Format(time.RFC3339),
			}
			if si.EndDate > si.StartDate {
				end := time.UnixMilli(si.EndDate).UTC().Format(time.RFC3339)
				it.EndsAt = &end
			}

			// Squarespace ships a DEFAULT map position (lower Manhattan)
			// even when no address was entered — location only counts
			// when a human actually typed one.
			addr := strings.TrimSpace(strings.Join(nonEmpty(
				si.Location.AddressTitle, si.Location.AddressLine1, si.Location.AddressLine2), ", "))
			if addr != "" {
				it.Location = addr
				if si.Location.MarkerLat != 0 || si.Location.MarkerLng != 0 {
					lat, lng := si.Location.MarkerLat, si.Location.MarkerLng
					it.Latitude, it.Longitude = &lat, &lng
				}
			}
			items = append(items, it)
		}
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

func nonEmpty(parts ...string) []string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return out
}

// stripHTML flattens an HTML fragment to plain text: tags dropped,
// entities unescaped, whitespace collapsed. Good enough for excerpts.
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
			b.WriteRune(' ')
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(html.UnescapeString(b.String())), " ")
}
