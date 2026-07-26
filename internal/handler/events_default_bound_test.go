package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// GET /api/v1/events sorts starts_at ascending, so a list with no lower
// bound is a list of a patch's *oldest* events — the inverse of what every
// caller headed "upcoming events" meant (issue #88). Omitting `from` now
// means upcoming; history is asked for by name.

func listTitles(t *testing.T, db *database.DB, query string) []string {
	t.Helper()
	r, _ := http.NewRequest("GET", "/api/v1/events"+query, nil)
	w := servePublicMux(t, "GET", "/api/v1/events", handler.ListEvents(db), r)
	if w.Code != http.StatusOK {
		t.Fatalf("list%s: %d — %s", query, w.Code, w.Body.String())
	}
	var resp struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	out := make([]string, 0, len(resp.Items))
	for _, i := range resp.Items {
		out = append(out, i.Title)
	}
	return out
}

func has(titles []string, want string) bool {
	for _, t := range titles {
		if t == want {
			return true
		}
	}
	return false
}

func seedPastAndFuture(t *testing.T, db *database.DB) string {
	t.Helper()
	owner, _ := createTestUser(t, db, "bound-owner", "member")
	nodeID := createTestNode(t, db, owner.ID, "Bound Venue", "bound-venue", "open")
	now := time.Now().UTC()
	insertEvent(t, db, nodeID, owner.ID, "Long ago", now.Add(-90*24*time.Hour))
	insertEvent(t, db, nodeID, owner.ID, "Yesterday", now.Add(-24*time.Hour))
	insertEvent(t, db, nodeID, owner.ID, "Tonight", now.Add(3*time.Hour))
	insertEvent(t, db, nodeID, owner.ID, "Next month", now.Add(30*24*time.Hour))
	return nodeID
}

func TestListEventsDefaultsToUpcoming(t *testing.T) {
	db := setupTestDB(t)
	seedPastAndFuture(t, db)

	titles := listTitles(t, db, "?node_slug=bound-venue")

	// The bug in one assertion: ascending sort meant an unbounded list
	// opened with the oldest event the patch ever held.
	if has(titles, "Long ago") || has(titles, "Yesterday") {
		t.Errorf("omitting from returned past events: %v", titles)
	}
	if !has(titles, "Tonight") || !has(titles, "Next month") {
		t.Errorf("omitting from dropped upcoming events: %v", titles)
	}
}

func TestListEventsIncludePastOptsOut(t *testing.T) {
	db := setupTestDB(t)
	seedPastAndFuture(t, db)

	titles := listTitles(t, db, "?node_slug=bound-venue&include_past=true")

	// The workspace calendar, the "any events yet" probe, and scoped search
	// all legitimately want the whole calendar.
	for _, want := range []string{"Long ago", "Yesterday", "Tonight", "Next month"} {
		if !has(titles, want) {
			t.Errorf("include_past dropped %q: %v", want, titles)
		}
	}
}

func TestListEventsExplicitFromWinsOverTheDefault(t *testing.T) {
	db := setupTestDB(t)
	seedPastAndFuture(t, db)

	// A caller asking for history by date must still get it — the default
	// applies only when no bound was given at all.
	from := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	titles := listTitles(t, db, "?node_slug=bound-venue&from="+from)

	if !has(titles, "Yesterday") {
		t.Errorf("explicit past from was overridden by the default: %v", titles)
	}
	if has(titles, "Long ago") {
		t.Errorf("explicit from ignored its own lower bound: %v", titles)
	}
}

func TestListEventsIncludePastOnlyOnExactlyTrue(t *testing.T) {
	db := setupTestDB(t)
	seedPastAndFuture(t, db)

	// A opt-out this consequential should not fire on a stray value; anything
	// that is not "true" leaves the safe default in place.
	for _, v := range []string{"", "1", "yes", "false", "TRUE"} {
		titles := listTitles(t, db, "?node_slug=bound-venue&include_past="+v)
		if has(titles, "Yesterday") {
			t.Errorf("include_past=%q opted out of the default: %v", v, titles)
		}
	}
}
