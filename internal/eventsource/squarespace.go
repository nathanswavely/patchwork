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
	ID    string `json:"id"`
	Title string `json:"title"`
	// The item's own page, site-relative ("/events/opening-night").
	FullURL   string `json:"fullUrl"`
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
	// Where the site says it lives. Squarespace serves the same JSON from
	// a custom domain and from the .squarespace.com staging host, and
	// baseUrl is the one that says which the venue publishes.
	Website struct {
		BaseURL string `json:"baseUrl"`
	} `json:"website"`
	Upcoming []squarespaceItem `json:"upcoming"`
	Past     []squarespaceItem `json:"past"`
	Items    []squarespaceItem `json:"items"`
}

// ParseSquarespace extracts the desired item set from a Squarespace
// collection JSON document, applying the same window as ParseICS.
// It errors when the document isn't an events collection, so the
// auto-detect path can tell "not Squarespace" from "empty calendar".
//
// pageURL is the collection address the admin attached; item links are
// site-relative, so resolving them needs a base and the document's own
// website.baseUrl is preferred where it has one (docs/adr/079).
func ParseSquarespace(data []byte, now time.Time, pageURL string) ([]Item, error) {
	var doc squarespaceCollection
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse squarespace json: %w", err)
	}
	if !strings.Contains(doc.Collection.TypeName, "events") {
		return nil, fmt.Errorf("not a squarespace events collection (type %q)", doc.Collection.TypeName)
	}

	windowStart := now.Add(-pastGrace)
	windowEnd := now.Add(Horizon)
	base := squarespaceBase(doc.Website.BaseURL, pageURL)

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

			// Entities, same as the excerpt below: only stripHTML unescaped
			// before, so a title kept its &amp; while its description lost
			// one.
			title := strings.TrimSpace(html.UnescapeString(si.Title))
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
			it.URL = resolveRef(base, si.FullURL)

			// Squarespace ships a DEFAULT map position (lower Manhattan)
			// even when no address was entered — location only counts
			// when a human actually typed one.
			// Entities here too — the address fields come out of the same
			// CMS. Location is where it showed: it is name-first
			// (docs/adr/046), so an entity sits in the half that survives
			// truncation on a narrow row.
			addr := strings.TrimSpace(html.UnescapeString(strings.Join(nonEmpty(
				si.Location.AddressTitle, si.Location.AddressLine1, si.Location.AddressLine2), ", ")))
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

// squarespaceBase picks what item links resolve against: the document's
// declared site, falling back to the origin of the address the admin
// pasted. Returns "" when neither is usable, which just means no links.
func squarespaceBase(declared, pageURL string) string {
	for _, candidate := range []string{strings.TrimSpace(declared), pageURL} {
		u, err := url.Parse(strings.TrimSpace(candidate))
		if err != nil || u.Host == "" {
			continue
		}
		if u.Scheme != "https" && u.Scheme != "http" {
			continue
		}
		return u.Scheme + "://" + u.Host
	}
	return ""
}

// resolveRef turns a site-relative item path into an absolute link, and
// passes an already-absolute http(s) one through. Anything else yields "".
//
// The scheme is checked on the way out, not only on the way in: a ref that
// carries its own scheme is returned by ResolveReference unchanged, so a
// feed answering "javascript:alert(1)" would otherwise resolve to itself.
func resolveRef(base, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "http://") {
		return ref
	}
	if base == "" {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	out := b.ResolveReference(r)
	if out.Scheme != "https" && out.Scheme != "http" {
		return ""
	}
	return out.String()
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
