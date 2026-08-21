package handler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/eventsource"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// seedCrosswalkEntry routes one name to one patch. There is exactly one
// per (aggregator, name) by construction — a name addresses one patch, or
// the same event lands twice (migration 053).
func seedCrosswalkEntry(t *testing.T, db *database.DB, aggID, nameKey, ownerID, addedBy string) string {
	t.Helper()
	sourceID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO event_sources (id, node_id, type, url, added_by, aggregator_id, name_key)
		 VALUES (?, ?, 'aggregator', ?, ?, ?, ?)`,
		sourceID, ownerID, "https://city.example/feed#"+nameKey, addedBy, aggID, nameKey,
	); err != nil {
		t.Fatalf("seed crosswalk entry: %v", err)
	}
	return sourceID
}

// seedRoutedEvent puts an event on the entry's patch as the reconciler
// would, carrying the uid of the listing it came from. Programs are
// derived across the entry, the listing, and the event together.
func seedRoutedEvent(t *testing.T, db *database.DB, sourceID, ownerID, addedBy, uid, title string) (eventID string) {
	t.Helper()
	eventID = auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO events (id, node_id, created_by, title, description, location,
		 starts_at, visibility, status, source_id, source_uid, source_occurrence)
		 VALUES (?, ?, ?, ?, '', '', ?, 'public', 'active', ?, ?, '')`,
		eventID, ownerID, addedBy, title,
		time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339), sourceID, uid,
	); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	return eventID
}

// The load-bearing constraint of docs/adr/063: a program never routes.
// Crediting one changes no event's owner — it only produces offers, which
// a person turns into an ordinary link the owner confirms.
func TestProgram_CreditingNeverMovesTheEvent(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "steward-p1", "admin")
	venueID := seedUnclaimedNode(t, db, "Penn Square Hotel", "penn-square-hotel")
	seedUnclaimedNode(t, db, "Historical Society", "historical-society")
	aggID := seedAggregatorRow(t, db, admin.ID)
	seedListing(t, db, aggID, "tour@city", "penn square hotel", "Penn Square Hotel", "Heritage Walking Tour")
	srcID := seedCrosswalkEntry(t, db, aggID, "penn square hotel", venueID, admin.ID)
	eventID := seedRoutedEvent(t, db, srcID, venueID, admin.ID, "tour@city", "Heritage Walking Tour")

	r := authedRequest("POST", "/api/v1/nodes/historical-society/programs", map[string]string{
		"aggregator_id": aggID,
		"name_key":      "penn square hotel",
		"title_key":     eventsource.TitleKey("Heritage Walking Tour"),
	}, adminToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/programs", handler.CreateProgram(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var owner string
	db.QueryRow(`SELECT node_id FROM events WHERE id = ?`, eventID).Scan(&owner)
	if owner != venueID {
		t.Fatalf("crediting a program moved the event off its venue: owner %s, venue %s", owner, venueID)
	}
	var links int
	db.QueryRow(`SELECT COUNT(*) FROM event_links WHERE event_id = ?`, eventID).Scan(&links)
	if links != 0 {
		t.Errorf("crediting must not create a link; the credited patch proposes and the owner confirms (got %d)", links)
	}
}

// The offer is what remains after subtracting what has already been
// answered. A dismissal has to stick, or the same refusal is owed every
// hour — the defect docs/adr/056 found in its own review path.
func TestProgram_OffersAppearThenYieldToLinksAndDismissals(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "steward-p2", "admin")
	venueID := seedUnclaimedNode(t, db, "Penn Square Hotel", "penn-sq-2")
	seedUnclaimedNode(t, db, "Historical Society", "hist-soc-2")
	aggID := seedAggregatorRow(t, db, admin.ID)
	seedListing(t, db, aggID, "t1@city", "penn sq", "Penn Sq", "Heritage Tour")
	seedListing(t, db, aggID, "t2@city", "penn sq", "Penn Sq", "Heritage Tour")
	// One entry carries every listing under its name, so both events hang
	// off the same source.
	sourceID := seedCrosswalkEntry(t, db, aggID, "penn sq", venueID, admin.ID)
	firstEvent := seedRoutedEvent(t, db, sourceID, venueID, admin.ID, "t1@city", "Heritage Tour")
	seedRoutedEvent(t, db, sourceID, venueID, admin.ID, "t2@city", "Heritage Tour")

	create := authedRequest("POST", "/api/v1/nodes/hist-soc-2/programs", map[string]string{
		"aggregator_id": aggID, "name_key": "penn sq", "title_key": eventsource.TitleKey("Heritage Tour"),
	}, adminToken)
	if w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/programs", handler.CreateProgram(db), create); w.Code != http.StatusCreated {
		t.Fatalf("credit failed: %d %s", w.Code, w.Body.String())
	}

	offers := func() []any {
		r := authedRequest("GET", "/api/v1/nodes/hist-soc-2/programs", nil, adminToken)
		w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/programs", handler.ListPrograms(db), r)
		items, _ := decodeJSON(t, w)["offers"].([]any)
		return items
	}
	if got := len(offers()); got != 2 {
		t.Fatalf("both routed listings should be offered, got %d", got)
	}

	// A link — pending or confirmed — is already somebody's move, so the
	// offer stops being one.
	var programID string
	db.QueryRow(`SELECT id FROM aggregator_programs`).Scan(&programID)
	var orgID string
	db.QueryRow(`SELECT id FROM nodes WHERE slug = 'hist-soc-2'`).Scan(&orgID)
	db.Exec(`INSERT INTO event_links (id, event_id, node_id, status, initiated_by, requested_by)
	         VALUES (?, ?, ?, 'pending', 'linked', ?)`,
		auth.NewUUIDv7(), firstEvent, orgID, admin.ID)
	if got := len(offers()); got != 1 {
		t.Fatalf("a proposed link should retire its offer, got %d", got)
	}

	var remaining string
	for _, o := range offers() {
		remaining, _ = o.(map[string]any)["event_id"].(string)
	}
	dismiss := authedRequest("POST", "/api/v1/nodes/hist-soc-2/offers/dismiss",
		map[string]string{"program_id": programID, "event_id": remaining}, adminToken)
	if w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/offers/dismiss", handler.DismissOffer(db), dismiss); w.Code != http.StatusNoContent {
		t.Fatalf("dismiss failed: %d %s", w.Code, w.Body.String())
	}
	if got := len(offers()); got != 0 {
		t.Errorf("a dismissed offer must not come back, got %d", got)
	}
}

// Standing is over the credited patch and nothing else (docs/adr/063).
// The venue whose event it is may not point standing machinery at a
// stranger who would then owe a refusal every month; ADR 032 already lets
// it tag one event at a time.
func TestProgram_OnlySomeoneSpeakingForTheCreditedPatchMayCredit(t *testing.T) {
	db := setupTestDB(t)
	admin, _ := createTestUser(t, db, "steward-p3", "admin")
	venueOwner, venueToken := createTestUser(t, db, "venue-admin-p3", "member")
	orgOwner, orgToken := createTestUser(t, db, "org-admin-p3", "member")

	venueID := createTestNode(t, db, venueOwner.ID, "Penn Square Hotel", "penn-sq-3", "open")
	createTestMembership(t, db, venueOwner.ID, venueID, "admin", "active")
	orgID := createTestNode(t, db, orgOwner.ID, "Historical Society", "hist-soc-3", "open")
	createTestMembership(t, db, orgOwner.ID, orgID, "admin", "active")

	aggID := seedAggregatorRow(t, db, admin.ID)
	seedListing(t, db, aggID, "t@city", "penn sq 3", "Penn Sq", "Heritage Tour")

	body := map[string]string{
		"aggregator_id": aggID, "name_key": "penn sq 3", "title_key": eventsource.TitleKey("Heritage Tour"),
	}

	// The venue's admin speaks for the venue, not for the society.
	r := authedRequest("POST", "/api/v1/nodes/hist-soc-3/programs", body, venueToken)
	w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/programs", handler.CreateProgram(db), r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("a venue admin must not credit somebody else's patch, got %d: %s", w.Code, w.Body.String())
	}

	// The society's own admin does, and needs nobody's permission: the
	// venue's calendar is untouched until a link is proposed.
	r = authedRequest("POST", "/api/v1/nodes/hist-soc-3/programs", body, orgToken)
	w = serveMux(t, db, "POST", "/api/v1/nodes/{slug}/programs", handler.CreateProgram(db), r)
	if w.Code != http.StatusCreated {
		t.Fatalf("the credited patch's own admin should be able to credit it, got %d: %s", w.Code, w.Body.String())
	}
	_ = venueID
}

// An unrouted name has no events, so its program is inert rather than
// broken — and the row has to say which it is (docs/adr/063).
func TestProgram_IsInertUntilItsNameIsRouted(t *testing.T) {
	db := setupTestDB(t)
	admin, adminToken := createTestUser(t, db, "steward-p4", "admin")
	seedUnclaimedNode(t, db, "Historical Society", "hist-soc-4")
	aggID := seedAggregatorRow(t, db, admin.ID)
	seedListing(t, db, aggID, "t@city", "unmapped venue", "Unmapped Venue", "Heritage Tour")

	r := authedRequest("POST", "/api/v1/nodes/hist-soc-4/programs", map[string]string{
		"aggregator_id": aggID, "name_key": "unmapped venue", "title_key": eventsource.TitleKey("Heritage Tour"),
	}, adminToken)
	if w := serveMux(t, db, "POST", "/api/v1/nodes/{slug}/programs", handler.CreateProgram(db), r); w.Code != http.StatusCreated {
		t.Fatalf("credit failed: %d %s", w.Code, w.Body.String())
	}

	r = authedRequest("GET", "/api/v1/nodes/hist-soc-4/programs", nil, adminToken)
	w := serveMux(t, db, "GET", "/api/v1/nodes/{slug}/programs", handler.ListPrograms(db), r)
	body := decodeJSON(t, w)
	items, _ := body["items"].([]any)
	offers, _ := body["offers"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected the program to exist, got %d", len(items))
	}
	if p, _ := items[0].(map[string]any); p["routed"] != false {
		t.Errorf("an unmapped name must report itself unrouted so the row can say what it waits on: %v", items[0])
	}
	if len(offers) != 0 {
		t.Errorf("an unrouted program has no events and so no offers, got %d", len(offers))
	}
}
