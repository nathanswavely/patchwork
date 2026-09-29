package eventsource

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/safehttp"
)

func TestTribeListURL(t *testing.T) {
	cases := map[string]string{
		// The addresses the plugin itself hands out, all of which answered
		// with an empty body on the site this was measured against.
		"https://venue.example/events/?ical=1":       "https://venue.example/events/?eventDisplay=list&ical=1",
		"https://venue.example/?ical=1":              "https://venue.example/?eventDisplay=list&ical=1",
		"https://venue.example/events/month/?ical=1": "https://venue.example/events/list/?eventDisplay=list&ical=1",
		// An events page pasted as-is.
		"https://venue.example/events/":      "https://venue.example/events/?eventDisplay=list&ical=1",
		"https://venue.example/events/week/": "https://venue.example/events/list/?eventDisplay=list&ical=1",
		// A category stays a category.
		"https://venue.example/events/category/jazz/": "https://venue.example/events/category/jazz/?eventDisplay=list&ical=1",
	}
	for in, want := range cases {
		got, err := tribeListURL(in)
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%s:\n got %s\nwant %s", in, got, want)
		}
	}
}

// tribeServer behaves like the plugin: an export without the list view
// answers 200 with nothing in it, and the list export is the calendar.
func tribeServer(t *testing.T, ics string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("ical") == "1" && q.Get("eventDisplay") == "list" && strings.HasSuffix(r.URL.Path, "/events/"):
			w.Header().Set("Content-Type", "text/calendar")
			w.Write([]byte(strings.ReplaceAll(ics, "\n", "\r\n")))
		case q.Get("ical") == "1":
			w.Header().Set("Content-Type", "text/html")
		default:
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<!doctype html><html><head>
<link rel="stylesheet" href="/app/plugins/the-events-calendar/build/css/tribe-events-views.css">
</head><body>a month of shows</body></html>`))
		}
	}))
	t.Cleanup(srv.Close)
	prev := safehttp.SetAllowPrivateAddresses(true)
	t.Cleanup(func() { safehttp.SetAllowPrivateAddresses(prev) })
	return srv
}

// The export link a venue's site offers answers with an empty page; the
// source reads the list export instead, remembers that it is a tribe
// source, and stays healthy on the next sync.
func TestSync_EventsCalendarExportLinkReadsTheListExport(t *testing.T) {
	for _, path := range []string{"/events/?ical=1", "/events/"} {
		t.Run(path, func(t *testing.T) {
			db := setupTestDB(t)
			srv := tribeServer(t, wrap(
				vevent("a@venue", "Show A", future(48*time.Hour))+
					vevent("b@venue", "Show B", future(72*time.Hour))))
			sourceID := seedSource(t, db, srv.URL+path)

			for i := 0; i < 2; i++ {
				if err := Sync(context.Background(), db, nil, sourceID); err != nil {
					t.Fatalf("sync %d: %v", i+1, err)
				}
				if n := countEvents(t, db, sourceID); n != 2 {
					t.Errorf("sync %d: expected 2 imported events, got %d", i+1, n)
				}
				status, lastError := sourceState(t, db, sourceID)
				if status != "ok" || lastError != nil {
					t.Errorf("sync %d: source state %s / %v", i+1, status, lastError)
				}
			}
			var typ, stored string
			db.QueryRow(`SELECT type, url FROM event_sources WHERE id = ?`, sourceID).Scan(&typ, &stored)
			if typ != "tribe" {
				t.Errorf("detected type not persisted: %q", typ)
			}
			// The address stays the one the admin pasted.
			if stored != srv.URL+path {
				t.Errorf("stored url rewritten: %q", stored)
			}
		})
	}
}

// An ordinary page that loads no calendar plugin is not probed for one.
func TestSync_PlainPageIsNotProbedAsEventsCalendar(t *testing.T) {
	db := setupTestDB(t)
	var probed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("eventDisplay") != "" {
			probed = true
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<!doctype html><html><body>just a homepage</body></html>"))
	}))
	t.Cleanup(srv.Close)
	prev := safehttp.SetAllowPrivateAddresses(true)
	t.Cleanup(func() { safehttp.SetAllowPrivateAddresses(prev) })

	sourceID := seedSource(t, db, srv.URL+"/")
	if err := Sync(context.Background(), db, nil, sourceID); err == nil {
		t.Fatal("expected error for a non-calendar page")
	}
	if probed {
		t.Error("a page without the plugin was probed for its list export")
	}
	_, lastError := sourceState(t, db, sourceID)
	if lastError == nil || *lastError != "the address returned a web page, not a calendar" {
		t.Errorf("last_error = %v", lastError)
	}
}

// What an admin reads under a failed source names what came back.
func TestParseICS_SaysWhatCameBack(t *testing.T) {
	now := time.Now()
	for body, want := range map[string]string{
		"":                             "the address returned an empty page, not a calendar",
		"  \r\n":                       "the address returned an empty page, not a calendar",
		"<!doctype html><html></html>": "the address returned a web page, not a calendar",
	} {
		_, err := ParseICS([]byte(body), now, time.UTC)
		if err == nil || err.Error() != want {
			t.Errorf("%q: got %v, want %q", body, err, want)
		}
	}
}
