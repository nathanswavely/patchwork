package eventsource

import (
	"bytes"
	"net/url"
	"strings"
)

// The Events Calendar (the WordPress plugin, "tribe" in its own markup)
// is how a great many venue sites publish a calendar, and its export
// links do not all answer. Measured on a live venue site in September
// 2026: `/events/?ical=1`, the month view's "Export Events" link and the
// `/?ical=1` its subscribe button builds all return 200 with an empty
// body, while the same addresses with `eventDisplay=list` return the whole
// upcoming calendar. An admin who pasted what the site offered got
// "parse ics: EOF", which is true and useless.
//
// So a 'tribe' source keeps the address the admin pasted and reads the
// list export derived from it, the way a 'squarespace' source reads the
// JSON view of the page it was given.

// tribeViews are the plugin's view slugs that can close an events path.
// The path wins over the query, so `/events/month/?eventDisplay=list`
// still answers as a month; the slug itself has to become `list`.
var tribeViews = map[string]bool{
	"month": true, "week": true, "day": true, "photo": true,
	"map": true, "summary": true, "list": true,
}

// tribeListURL rewrites an events address to its list-view ICS export.
// The rest of the path is kept, so a category page
// (`/events/category/jazz/`) stays a category feed.
func tribeListURL(pageURL string) (string, error) {
	u, err := url.Parse(pageURL)
	if err != nil {
		return "", err
	}
	segs := strings.Split(strings.TrimSuffix(u.Path, "/"), "/")
	if last := segs[len(segs)-1]; tribeViews[last] {
		segs[len(segs)-1] = "list"
		u.Path = strings.Join(segs, "/") + "/"
	}
	q := u.Query()
	q.Set("ical", "1")
	q.Set("eventDisplay", "list")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// looksLikeTribe says whether an address that did not parse as ICS is
// worth one try at the list export. Either the address is already one of
// the plugin's exports (`ical=1` is its parameter), or the page it served
// loads the plugin. Anything else never costs the extra fetch.
func looksLikeTribe(pageURL string, body []byte) bool {
	if u, err := url.Parse(pageURL); err == nil && u.Query().Has("ical") {
		return true
	}
	return bytes.Contains(body, []byte("the-events-calendar")) ||
		bytes.Contains(body, []byte("tribe-events"))
}
