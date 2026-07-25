package handler_test

import (
	"testing"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Migration 042 rewrites notification links that were built to shapes the SPA
// never routed (issue #56). setupTestDB starts from an empty table, so there
// is nothing for the migration to have repaired — these tests seed the legacy
// rows and then run the migration's own SQL against them.
func replayLinkRepair(t *testing.T, db *database.DB) {
	t.Helper()
	sql, err := patchwork.MigrationsFS.ReadFile("migrations/042_fix_notification_links.sql")
	if err != nil {
		t.Fatalf("read migration 042: %v", err)
	}
	if _, err := db.Exec(string(sql)); err != nil {
		t.Fatalf("run migration 042: %v", err)
	}
}

func seedNotification(t *testing.T, db *database.DB, userID, notifType, link string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO notifications (id, user_id, type, title, body, link) VALUES (?, ?, ?, '', '', ?)`,
		id, userID, notifType, link,
	)
	if err != nil {
		t.Fatalf("seed notification: %v", err)
	}
	return id
}

func linkOf(t *testing.T, db *database.DB, id string) string {
	t.Helper()
	var link string
	if err := db.QueryRow("SELECT link FROM notifications WHERE id = ?", id).Scan(&link); err != nil {
		t.Fatalf("read link: %v", err)
	}
	return link
}

func seedGovernanceDoc(t *testing.T, db *database.DB, nodeID, createdBy, title string) string {
	t.Helper()
	id := auth.NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO governance_docs (id, node_id, title, body, visibility, created_by) VALUES (?, ?, ?, '', 'members', ?)`,
		id, nodeID, title, createdBy,
	)
	if err != nil {
		t.Fatalf("seed governance doc: %v", err)
	}
	return id
}

func TestMigration041_EventLinksLoseThePatchScope(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "linkuser", "member")
	createTestNode(t, db, user.ID, "Gallery Row", "gallery-row", "open")

	eventID := auth.NewUUIDv7()
	broken := seedNotification(t, db, user.ID, "event.created", "/patches/gallery-row/events/"+eventID)
	// The list link is a real route and must survive untouched.
	list := seedNotification(t, db, user.ID, "event.suggested", "/patches/gallery-row/events")

	replayLinkRepair(t, db)

	if got, want := linkOf(t, db, broken), "/events/"+eventID; got != want {
		t.Errorf("event link: got %q, want %q", got, want)
	}
	if got, want := linkOf(t, db, list), "/patches/gallery-row/events"; got != want {
		t.Errorf("events list link should be untouched: got %q, want %q", got, want)
	}
}

func TestMigration041_CharterLinksGainTheDocsSegment(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "charteruser", "member")
	nodeID := createTestNode(t, db, user.ID, "Gallery Row", "gallery-row", "open")
	docID := seedGovernanceDoc(t, db, nodeID, user.ID, "House Rules")

	broken := seedNotification(t, db, user.ID, "governance.doc_updated", "/patches/gallery-row/governance/"+docID)
	// A proposal id in the same slot is already correct — it must not gain
	// 'docs/', or a working link becomes a broken one.
	proposalID := auth.NewUUIDv7()
	proposal := seedNotification(t, db, user.ID, "proposal.new", "/patches/gallery-row/governance/"+proposalID)
	// An already-correct charter link must be left alone rather than
	// accumulating a second 'docs/'.
	fine := seedNotification(t, db, user.ID, "governance.doc_updated", "/patches/gallery-row/governance/docs/"+docID)

	replayLinkRepair(t, db)

	if got, want := linkOf(t, db, broken), "/patches/gallery-row/governance/docs/"+docID; got != want {
		t.Errorf("charter link: got %q, want %q", got, want)
	}
	if got, want := linkOf(t, db, proposal), "/patches/gallery-row/governance/"+proposalID; got != want {
		t.Errorf("proposal link should be untouched: got %q, want %q", got, want)
	}
	if got, want := linkOf(t, db, fine), "/patches/gallery-row/governance/docs/"+docID; got != want {
		t.Errorf("correct charter link should be untouched: got %q, want %q", got, want)
	}
}

// Running the repair twice must not double-splice. Deployments re-run
// migrations only once, but the statements should be safe regardless.
func TestMigration041_IsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "idemuser", "member")
	nodeID := createTestNode(t, db, user.ID, "Gallery Row", "gallery-row", "open")
	docID := seedGovernanceDoc(t, db, nodeID, user.ID, "House Rules")

	eventID := auth.NewUUIDv7()
	ev := seedNotification(t, db, user.ID, "event.created", "/patches/gallery-row/events/"+eventID)
	doc := seedNotification(t, db, user.ID, "governance.doc_updated", "/patches/gallery-row/governance/"+docID)

	replayLinkRepair(t, db)
	replayLinkRepair(t, db)

	if got, want := linkOf(t, db, ev), "/events/"+eventID; got != want {
		t.Errorf("event link after two runs: got %q, want %q", got, want)
	}
	if got, want := linkOf(t, db, doc), "/patches/gallery-row/governance/docs/"+docID; got != want {
		t.Errorf("charter link after two runs: got %q, want %q", got, want)
	}
}

// A patch slugged "events" or "governance" puts the marker segment's own text
// in the slug position. Splitting on the first occurrence in the whole link
// would splice at the wrong offset, so the migration searches past the fixed
// '/patches/' prefix.
func TestMigration041_SlugNamedAfterTheMarkerSegment(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "sluguser", "member")
	createTestNode(t, db, user.ID, "Events", "events", "open")
	govNodeID := createTestNode(t, db, user.ID, "Governance", "governance", "open")
	docID := seedGovernanceDoc(t, db, govNodeID, user.ID, "House Rules")

	eventID := auth.NewUUIDv7()
	ev := seedNotification(t, db, user.ID, "event.created", "/patches/events/events/"+eventID)
	doc := seedNotification(t, db, user.ID, "governance.doc_updated", "/patches/governance/governance/"+docID)

	replayLinkRepair(t, db)

	if got, want := linkOf(t, db, ev), "/events/"+eventID; got != want {
		t.Errorf("event link under slug 'events': got %q, want %q", got, want)
	}
	if got, want := linkOf(t, db, doc), "/patches/governance/governance/docs/"+docID; got != want {
		t.Errorf("charter link under slug 'governance': got %q, want %q", got, want)
	}
}
