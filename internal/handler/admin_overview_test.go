package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// The Overview (CONTEXT.md) shows what waits on the instance admin and what
// is unattended, and nothing else. These tests pin the two rules that keep
// it from drifting back into a stats page: an inbox line counts only what
// its tab lists, and a care line appears only for what the instance admin
// holds (docs/adr/115).

type overviewPayload struct {
	Inbox []struct {
		Queue    string `json:"queue"`
		Count    int    `json:"count"`
		OldestAt string `json:"oldest_at"`
	} `json:"inbox"`
	UnroutedNames struct {
		Count       int `json:"count"`
		Aggregators int `json:"aggregators"`
	} `json:"unrouted_names"`
	Care struct {
		AdminlessPatches []struct {
			Slug        string `json:"slug"`
			MemberCount int    `json:"member_count"`
		} `json:"adminless_patches"`
		FailingAggregators []struct {
			Name      string `json:"name"`
			LastError string `json:"last_error"`
		} `json:"failing_aggregators"`
		FailingUnclaimedSources []struct {
			NodeSlug string `json:"node_slug"`
		} `json:"failing_unclaimed_sources"`
		FailedDeliveries int `json:"failed_deliveries"`
	} `json:"care"`
	SMTPConfigured    bool `json:"smtp_configured"`
	FederationEnabled bool `json:"federation_enabled"`
	HasPasskey        bool `json:"has_passkey"`
}

func getOverview(t *testing.T, db *database.DB, cfg *config.Config, token string) (int, overviewPayload) {
	t.Helper()
	r := authedRequest("GET", "/api/v1/admin/overview", nil, token)
	w := serveAdmin(db, "GET", "/api/v1/admin/overview", handler.AdminOverview(db, cfg), r)
	var p overviewPayload
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatalf("decode overview: %v: %s", err, w.Body.String())
		}
	}
	return w.Code, p
}

func queue(t *testing.T, p overviewPayload, name string) (int, string) {
	t.Helper()
	for _, q := range p.Inbox {
		if q.Queue == name {
			return q.Count, q.OldestAt
		}
	}
	t.Fatalf("overview has no %q queue: %+v", name, p.Inbox)
	return 0, ""
}

func TestOverviewIsAdminOnly(t *testing.T) {
	db := setupTestDB(t)
	_, memberToken := createTestUser(t, db, "overview-member", "member")
	code, _ := getOverview(t, db, testConfig(), memberToken)
	if code != http.StatusForbidden {
		t.Fatalf("member reading overview got %d, want 403", code)
	}
}

// A fresh quilt is a quiet page: every queue at zero, no oldest date, nothing
// in care, and the standing conditions stated. The page must be able to say
// "nothing is waiting" from this payload alone.
func TestOverviewQuietOnFreshQuilt(t *testing.T) {
	db := setupTestDB(t)
	_, token := createTestUser(t, db, "overview-admin", "admin")

	code, p := getOverview(t, db, testConfig(), token)
	if code != http.StatusOK {
		t.Fatalf("overview: %d", code)
	}
	if len(p.Inbox) != 5 {
		t.Fatalf("inbox has %d queues, want 5 (reports, submissions, event_submissions, claims, tag_suggestions)", len(p.Inbox))
	}
	for _, q := range p.Inbox {
		if q.Count != 0 || q.OldestAt != "" {
			t.Fatalf("queue %s on a fresh quilt: count=%d oldest=%q", q.Queue, q.Count, q.OldestAt)
		}
	}
	if p.UnroutedNames.Count != 0 || len(p.Care.AdminlessPatches) != 0 ||
		len(p.Care.FailingAggregators) != 0 || len(p.Care.FailingUnclaimedSources) != 0 {
		t.Fatalf("fresh quilt has care items: %+v", p.Care)
	}
	if p.SMTPConfigured {
		t.Fatalf("test config has no SMTP, yet smtp_configured is true")
	}
	if p.HasPasskey {
		t.Fatalf("admin has enrolled no passkey, yet has_passkey is true")
	}
}

// Each inbox line counts exactly what its tab lists, and carries the oldest
// arrival. A report routed to a patch (docs/adr/081) and an approved claim
// waiting on its claimant are both somebody else's move, so neither counts.
func TestOverviewInboxCountsWhatTheTabsList(t *testing.T) {
	db := setupTestDB(t)
	admin, token := createTestUser(t, db, "overview-admin2", "admin")
	reporter, _ := createTestUser(t, db, "reporter", "member")
	claimant, _ := createTestUser(t, db, "claimant", "member")

	active := createTestNode(t, db, admin.ID, "Gallery Row", "gallery-row", "open")
	createTestMembership(t, db, admin.ID, active, "admin", "active")
	unclaimed := seedUnclaimedNode(t, db, "The Selvage", "the-selvage")

	// Two instance-routed reports, one older; one routed to a patch's own queue.
	for i, at := range []string{"2026-09-01T10:00:00.000Z", "2026-09-10T10:00:00.000Z"} {
		if _, err := db.Exec(
			`INSERT INTO content_reports (id, reporter_id, entity_type, entity_id, reason, details, created_at)
			 VALUES (?, ?, 'node', ?, 'spam', '', ?)`, auth.NewUUIDv7(), reporter.ID, unclaimed, at); err != nil {
			t.Fatalf("insert report %d: %v", i, err)
		}
	}
	if _, err := db.Exec(
		`INSERT INTO content_reports (id, reporter_id, entity_type, entity_id, reason, details, node_id)
		 VALUES (?, ?, 'notice', ?, 'spam', '', ?)`, auth.NewUUIDv7(), reporter.ID, auth.NewUUIDv7(), active); err != nil {
		t.Fatalf("insert patch-routed report: %v", err)
	}

	// One pending claim, one approved and awaiting setup.
	if _, err := db.Exec(
		`INSERT INTO claim_requests (id, node_id, user_id, method, status, created_at)
		 VALUES (?, ?, ?, 'admin', 'pending', '2026-08-20T10:00:00.000Z')`, auth.NewUUIDv7(), unclaimed, claimant.ID); err != nil {
		t.Fatalf("insert pending claim: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO claim_requests (id, node_id, user_id, method, status, setup_expires_at)
		 VALUES (?, ?, ?, 'admin', 'approved', '2999-01-01T00:00:00.000Z')`, auth.NewUUIDv7(), unclaimed, reporter.ID); err != nil {
		t.Fatalf("insert approved claim: %v", err)
	}

	// A pending event on the unclaimed patch counts; one on the active patch
	// is that patch's admins' to review (docs/adr/026).
	for _, nodeID := range []string{unclaimed, active} {
		if _, err := db.Exec(
			`INSERT INTO events (id, node_id, created_by, title, starts_at, status)
			 VALUES (?, ?, ?, 'Opening', '2026-10-01T19:00:00.000Z', 'pending_review')`,
			auth.NewUUIDv7(), nodeID, reporter.ID); err != nil {
			t.Fatalf("insert pending event: %v", err)
		}
	}

	// One patch submission, one suggested tag.
	if _, err := db.Exec(
		`INSERT INTO nodes (id, owner_id, name, slug, description, node_type, visibility, membership_policy, status, submitted_by)
		 VALUES (?, ?, 'New Space', 'new-space', '', 'leaf', 'public', 'open', 'pending_review', ?)`,
		auth.NewUUIDv7(), reporter.ID, reporter.ID); err != nil {
		t.Fatalf("insert submission: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO tags (id, name, status, suggested_by) VALUES (?, 'letterpress', 'pending', ?)`,
		auth.NewUUIDv7(), reporter.ID); err != nil {
		t.Fatalf("insert suggested tag: %v", err)
	}

	code, p := getOverview(t, db, testConfig(), token)
	if code != http.StatusOK {
		t.Fatalf("overview: %d", code)
	}

	want := map[string]int{
		"reports": 2, "submissions": 1, "event_submissions": 1, "claims": 1, "tag_suggestions": 1,
	}
	for name, n := range want {
		if got, _ := queue(t, p, name); got != n {
			t.Errorf("%s: count %d, want %d", name, got, n)
		}
	}
	if _, oldest := queue(t, p, "reports"); oldest != "2026-09-01T10:00:00.000Z" {
		t.Errorf("reports oldest_at = %q, want the September 1 report", oldest)
	}
	if _, oldest := queue(t, p, "claims"); oldest != "2026-08-20T10:00:00.000Z" {
		t.Errorf("claims oldest_at = %q, want the pending claim, not the approved one", oldest)
	}
}

// Care follows custody (docs/adr/115): a claimed patch with no admin leads
// it, a failing source is listed only on an unclaimed patch, and a paused
// aggregator's old error is history rather than a fault.
func TestOverviewCareFollowsCustody(t *testing.T) {
	db := setupTestDB(t)
	admin, token := createTestUser(t, db, "overview-admin3", "admin")
	someone, _ := createTestUser(t, db, "someone", "member")

	// An active patch whose only admin membership has lapsed, with one
	// member left to hand it to.
	vacated := createTestNode(t, db, someone.ID, "Vacated Collective", "vacated", "open")
	createTestMembership(t, db, someone.ID, vacated, "admin", "left")
	createTestMembership(t, db, admin.ID, vacated, "member", "active")
	// A followed patch with a real admin is held by nobody else.
	held := createTestNode(t, db, someone.ID, "Run Fine", "run-fine", "open")
	createTestMembership(t, db, someone.ID, held, "admin", "active")

	unclaimed := seedUnclaimedNode(t, db, "The Selvage", "the-selvage")
	for _, nodeID := range []string{unclaimed, held} {
		if _, err := db.Exec(
			`INSERT INTO event_sources (id, node_id, type, url, added_by, status, last_error)
			 VALUES (?, ?, 'ics', 'https://127.0.0.1:9/cal.ics', ?, 'error', 'connection refused')`,
			auth.NewUUIDv7(), nodeID, admin.ID); err != nil {
			t.Fatalf("insert failing source: %v", err)
		}
	}

	for _, a := range []struct{ name string; paused int }{{"City Calendar", 0}, {"Old Weekly", 1}} {
		if _, err := db.Exec(
			`INSERT INTO aggregators (id, name, type, url, added_by, status, paused, last_error)
			 VALUES (?, ?, 'ics', ?, ?, 'error', ?, 'timeout')`,
			auth.NewUUIDv7(), a.name, "https://127.0.0.1:9/"+a.name, admin.ID, a.paused); err != nil {
			t.Fatalf("insert aggregator: %v", err)
		}
	}

	code, p := getOverview(t, db, testConfig(), token)
	if code != http.StatusOK {
		t.Fatalf("overview: %d", code)
	}
	if len(p.Care.AdminlessPatches) != 1 || p.Care.AdminlessPatches[0].Slug != "vacated" {
		t.Fatalf("adminless patches = %+v, want only the vacated one", p.Care.AdminlessPatches)
	}
	if p.Care.AdminlessPatches[0].MemberCount != 1 {
		t.Fatalf("vacated patch member_count = %d, want 1", p.Care.AdminlessPatches[0].MemberCount)
	}
	if len(p.Care.FailingUnclaimedSources) != 1 || p.Care.FailingUnclaimedSources[0].NodeSlug != "the-selvage" {
		t.Fatalf("failing sources = %+v, want only the unclaimed patch's", p.Care.FailingUnclaimedSources)
	}
	if len(p.Care.FailingAggregators) != 1 || p.Care.FailingAggregators[0].Name != "City Calendar" {
		t.Fatalf("failing aggregators = %+v, want only the unpaused one", p.Care.FailingAggregators)
	}
	if p.Care.FailedDeliveries != 0 || p.FederationEnabled {
		t.Fatalf("federation is off in the test config; got enabled=%v failed=%d", p.FederationEnabled, p.Care.FailedDeliveries)
	}
}

// The passkey line is about the caller, not the quilt: it is true for the
// admin who holds none even when another admin does.
func TestOverviewPasskeyIsTheCallers(t *testing.T) {
	db := setupTestDB(t)
	with, withToken := createTestUser(t, db, "has-key", "admin")
	_, withoutToken := createTestUser(t, db, "no-key", "admin")
	if _, err := db.Exec(
		`INSERT INTO credentials (id, user_id, credential_id, public_key, attestation_type, aaguid, sign_count) VALUES (?, ?, ?, ?, 'none', ?, 0)`,
		auth.NewUUIDv7(), with.ID, []byte("cred"), []byte("key"), make([]byte, 16)); err != nil {
		t.Fatalf("insert credential: %v", err)
	}

	if _, p := getOverview(t, db, testConfig(), withToken); !p.HasPasskey {
		t.Fatalf("admin with a passkey reads has_passkey=false")
	}
	if _, p := getOverview(t, db, testConfig(), withoutToken); p.HasPasskey {
		t.Fatalf("admin without a passkey reads has_passkey=true")
	}
}
