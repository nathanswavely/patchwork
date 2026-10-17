package eventsource

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/safehttp"
)

// Classify picks the calendar paragraph of an intro
// (docs/adr/2026-10-16-a-page-made-for-someone-tells-them-so.md, decision
// 4). The first two situations are read off event_sources; the rest are one
// fetch of the site, decided by the same probes the attach form runs.

func classifyDB(t *testing.T) *database.DB {
	t.Helper()
	f, err := os.CreateTemp("", "classify-*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	t.Cleanup(func() { _ = os.Remove(f.Name()) })
	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(f.Name(), migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO users (id, username, display_name, role) VALUES ('u1','u1','u1','admin')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO nodes (id, owner_id, name, slug, description, node_type, visibility, membership_policy, status, follower_permissions)
		VALUES ('n1','u1','Venue','venue','','leaf','public','open','unclaimed','{}')`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestClassify_ReadsSourcesBeforeFetchingAnything(t *testing.T) {
	db := classifyDB(t)
	ctx := context.Background()

	// No website, nothing attached: nothing to read.
	c := Classify(ctx, db, "n1", "")
	if c.Situation != SituationNone {
		t.Fatalf("no website: got %s (%s)", c.Situation, c.Reason)
	}

	// An aggregator crosswalk, and nothing of the patch's own.
	if _, err := db.Exec(`INSERT INTO aggregators (id, name, type, url, added_by) VALUES ('a1','Visit Town','ics','https://example.org/a.ics','u1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO event_sources (id, node_id, type, url, added_by, status, aggregator_id, name_key)
		VALUES ('s1','n1','aggregator','https://example.org/a.ics','u1','ok','a1','venue')`); err != nil {
		t.Fatal(err)
	}
	c = Classify(ctx, db, "n1", "")
	if c.Situation != SituationAggregator || c.AggregatorName != "Visit Town" {
		t.Fatalf("aggregator only: got %s / %q (%s)", c.Situation, c.AggregatorName, c.Reason)
	}

	// A source of its own, syncing. Wins over the crosswalk, and no fetch
	// happens: the website is a URL nothing could reach.
	if _, err := db.Exec(`INSERT INTO event_sources (id, node_id, type, url, added_by, status)
		VALUES ('s2','n1','ics','https://venue.example/cal.ics','u1','ok')`); err != nil {
		t.Fatal(err)
	}
	c = Classify(ctx, db, "n1", "http://127.0.0.1:1/unreachable")
	if c.Situation != SituationOwn {
		t.Fatalf("own source: got %s (%s)", c.Situation, c.Reason)
	}

	// A source that is failing is not a source that is syncing.
	if _, err := db.Exec(`UPDATE event_sources SET status = 'error' WHERE id = 's2'`); err != nil {
		t.Fatal(err)
	}
	c = Classify(ctx, db, "n1", "")
	if c.Situation != SituationAggregator {
		t.Fatalf("own source in error falls through to the crosswalk: got %s", c.Situation)
	}
}

func TestClassifyBody_PicksTheParagraphByWhatThePageIs(t *testing.T) {
	ctx := context.Background()

	// Dated inside the import horizon, or the parser would drop it the way
	// the attach form would.
	soon := time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339)
	jsonld := `<html><head><script type="application/ld+json">
	{"@context":"https://schema.org","@type":"Event","name":"Show","startDate":"` + soon + `","location":{"@type":"Place","name":"Venue"}}
	</script></head><body></body></html>`
	c := classifyBody(ctx, "https://venue.example/", []byte(jsonld))
	if c.Situation != SituationAttachable || c.AttachURL != "https://venue.example/" {
		t.Fatalf("event markup: got %s %q (%s)", c.Situation, c.AttachURL, c.Reason)
	}

	c = classifyBody(ctx, "https://venue.example/", []byte(`<html><body><link href="/wp-content/themes/x.css"></body></html>`))
	if c.Situation != SituationWordPress {
		t.Fatalf("wordpress: got %s (%s)", c.Situation, c.Reason)
	}

	c = classifyBody(ctx, "https://venue.example/", []byte(`<html><body><p>Hand-built. Shows every Friday.</p></body></html>`))
	if c.Situation != SituationOther {
		t.Fatalf("plain site: got %s (%s)", c.Situation, c.Reason)
	}
}

// A Squarespace page is only Attachable when its JSON view is an Events
// collection; a plain page on Squarespace is the page-type situation.
func TestClassifyBody_SquarespacePlainPageIsNotACollection(t *testing.T) {
	prev := safehttp.SetAllowPrivateAddresses(true)
	t.Cleanup(func() { safehttp.SetAllowPrivateAddresses(prev) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") == "json" {
			// A regular page's JSON view: no items.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"collection":{"typeName":"page"},"mainContent":"<p>Shows every Friday.</p>"}`))
			return
		}
		_, _ = w.Write([]byte(`<html><head><script src="https://static1.squarespace.com/x.js"></script></head><body>Shows every Friday.</body></html>`))
	}))
	defer srv.Close()

	c := classifyBody(context.Background(), srv.URL+"/events", []byte(`<html>static1.squarespace.com</html>`))
	if c.Situation != SituationSquarespace {
		t.Fatalf("squarespace plain page: got %s (%s)", c.Situation, c.Reason)
	}
	if !strings.Contains(c.Reason, "Events collection") {
		t.Fatalf("reason should say what to change: %q", c.Reason)
	}
}

func TestValidSituation_AttachableIsAdviceNotAParagraph(t *testing.T) {
	for _, s := range Situations {
		if !ValidSituation(string(s)) {
			t.Errorf("%s should be valid", s)
		}
	}
	if ValidSituation(string(SituationAttachable)) {
		t.Error("attachable has no paragraph and must not validate as one")
	}
	if ValidSituation("") || ValidSituation("maybe") {
		t.Error("unknown situations must not validate")
	}
}
