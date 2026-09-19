package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
)

// role x tier x the patch's Follower Permissions switch, across every surface
// an event is read through
// (docs/adr/2026-09-19-an-event-says-who-it-is-for-within-what-the-patch-allows.md).
//
// Written as one table because the failure this guards against is a surface
// drifting from the rest. The predicate lives in one place (event_tiers.go)
// but it reaches the read paths in three shapes — a Go call, a WHERE fragment
// over the event's own patch, and a WHERE fragment over the membership a
// scope=my query already matched — and nothing but a walk like this notices
// when one of them stops agreeing.

type tierFixture struct {
	db                       *database.DB
	nodeID, slug             string
	adminTok, memberTok      string
	followerTok, outsiderTok string
	instanceAdminTok         string
	publicID, followersID    string
	membersID                string
	followerID               string
	memberID                 string
}

func setupTierFixture(t *testing.T, key string, followersMayHaveEvents bool) *tierFixture {
	t.Helper()
	db := setupTestDB(t)
	f := &tierFixture{db: db, slug: "tier-" + key}

	admin, adminTok := createTestUser(t, db, "tier"+key+"admin", "member")
	member, memberTok := createTestUser(t, db, "tier"+key+"member", "member")
	follower, followerTok := createTestUser(t, db, "tier"+key+"follower", "member")
	_, outsiderTok := createTestUser(t, db, "tier"+key+"outsider", "member")
	// An instance admin holding no role on the patch is an outsider here.
	// There is no bypass in the predicate, deliberately.
	_, instanceAdminTok := createTestUser(t, db, "tier"+key+"instance", "admin")

	f.adminTok, f.memberTok, f.followerTok = adminTok, memberTok, followerTok
	f.outsiderTok, f.instanceAdminTok = outsiderTok, instanceAdminTok
	f.followerID, f.memberID = follower.ID, member.ID

	f.nodeID = createTestNode(t, db, admin.ID, "Tier "+key, f.slug, "open")
	createTestMembership(t, db, admin.ID, f.nodeID, "admin", "active")
	createTestMembership(t, db, member.ID, f.nodeID, "member", "active")
	createTestMembership(t, db, follower.ID, f.nodeID, "follower", "active")

	fp := `{"events":true,"proposals":true,"charters":true,"members":true}`
	if !followersMayHaveEvents {
		fp = `{"events":false,"proposals":true,"charters":true,"members":true}`
	}
	if _, err := db.Exec(`UPDATE nodes SET follower_permissions = ? WHERE id = ?`, fp, f.nodeID); err != nil {
		t.Fatal(err)
	}

	f.publicID = seedEvent(t, db, f.nodeID, admin.ID, "Open Night", daysOut(2))
	f.followersID = seedEvent(t, db, f.nodeID, admin.ID, "Followers Night", daysOut(3))
	f.membersID = seedEvent(t, db, f.nodeID, admin.ID, "House Meeting", daysOut(4))
	for id, vis := range map[string]string{f.followersID: "followers", f.membersID: "members"} {
		if _, err := db.Exec(`UPDATE events SET visibility = ? WHERE id = ?`, vis, id); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// wants maps a token to the status the detail endpoint should answer, per
// tier, for a patch whose switch is on and one whose switch is off.
type tierWant struct {
	who   string
	token func(*tierFixture) string
	// public, followers, members — with the switch ON
	on [3]bool
	// ... and with it OFF. Only the followers column moves: the switch is a
	// ceiling on that tier and says nothing about the other two.
	off [3]bool
}

func tierWants() []tierWant {
	return []tierWant{
		{"an admin", func(f *tierFixture) string { return f.adminTok },
			[3]bool{true, true, true}, [3]bool{true, true, true}},
		{"a member", func(f *tierFixture) string { return f.memberTok },
			[3]bool{true, true, true}, [3]bool{true, true, true}},
		{"a follower", func(f *tierFixture) string { return f.followerTok },
			[3]bool{true, true, false}, [3]bool{true, false, false}},
		{"an outsider", func(f *tierFixture) string { return f.outsiderTok },
			[3]bool{true, false, false}, [3]bool{true, false, false}},
		{"an instance admin with no role there", func(f *tierFixture) string { return f.instanceAdminTok },
			[3]bool{true, false, false}, [3]bool{true, false, false}},
		{"nobody", func(f *tierFixture) string { return "" },
			[3]bool{true, false, false}, [3]bool{true, false, false}},
	}
}

func (f *tierFixture) events() [3]string {
	return [3]string{f.publicID, f.followersID, f.membersID}
}

var tierNames = [3]string{"public", "followers", "members"}

func TestEventTiers_DetailEndpoint(t *testing.T) {
	for _, sw := range []bool{true, false} {
		f := setupTierFixture(t, boolKey(sw)+"detail", sw)
		for _, w := range tierWants() {
			want := w.on
			if !sw {
				want = w.off
			}
			for i, id := range f.events() {
				r := authedRequest("GET", "/api/v1/events/"+id, nil, w.token(f))
				rec := serveOptionalAuthMux(t, f.db, "GET", "/api/v1/events/{id}", handler.GetEvent(f.db), r)
				code := http.StatusNotFound
				if want[i] {
					code = http.StatusOK
				}
				if rec.Code != code {
					t.Errorf("switch=%v %s reading a %s event: code=%d, want %d",
						sw, w.who, tierNames[i], rec.Code, code)
				}
			}
		}
	}
}

func TestEventTiers_SingleEventICS(t *testing.T) {
	cfg := feedTestConfig()
	for _, sw := range []bool{true, false} {
		f := setupTierFixture(t, boolKey(sw)+"ics", sw)
		for _, w := range tierWants() {
			want := w.on
			if !sw {
				want = w.off
			}
			for i, id := range f.events() {
				r := authedRequest("GET", "/api/v1/events/"+id+"/event.ics", nil, w.token(f))
				rec := serveOptionalAuthMux(t, f.db, "GET", "/api/v1/events/{id}/event.ics", handler.EventICS(f.db, cfg), r)
				code := http.StatusNotFound
				if want[i] {
					code = http.StatusOK
				}
				if rec.Code != code {
					t.Errorf("switch=%v %s downloading a %s event: code=%d, want %d",
						sw, w.who, tierNames[i], rec.Code, code)
				}
			}
		}
	}
}

// The patch's own events tab. Before tiers this listing was public-only for
// everybody, so a members-only event was invisible on the one page it is for.
func TestEventTiers_PatchEventsTab(t *testing.T) {
	for _, sw := range []bool{true, false} {
		f := setupTierFixture(t, boolKey(sw)+"tab", sw)
		for _, w := range tierWants() {
			want := w.on
			if !sw {
				want = w.off
			}
			ids := listScopedEventIDs(t, f.db,
				authedRequest("GET", "/api/v1/events?node_slug="+f.slug+"&limit=100", nil, w.token(f)))
			for i, id := range f.events() {
				if ids[id] != want[i] {
					t.Errorf("switch=%v %s on the patch's events tab, %s event present=%v, want %v",
						sw, w.who, tierNames[i], ids[id], want[i])
				}
			}
		}
	}
}

// scope=my reads the membership row it matched, so it gets its own walk.
func TestEventTiers_MyQuiltScope(t *testing.T) {
	for _, sw := range []bool{true, false} {
		f := setupTierFixture(t, boolKey(sw)+"my", sw)
		for _, w := range tierWants() {
			// Only people with a relationship have a My Quilt at all.
			if w.who != "a member" && w.who != "an admin" && w.who != "a follower" {
				continue
			}
			want := w.on
			if !sw {
				want = w.off
			}
			ids := listScopedEventIDs(t, f.db,
				authedRequest("GET", "/api/v1/events?scope=my&limit=100", nil, w.token(f)))
			for i, id := range f.events() {
				if ids[id] != want[i] {
					t.Errorf("switch=%v %s on My Quilt, %s event present=%v, want %v",
						sw, w.who, tierNames[i], ids[id], want[i])
				}
			}
		}
	}
}

// The personal ICS feed is the subscribable half of My Quilt and must list
// exactly what it lists.
func TestEventTiers_PersonalICSFeed(t *testing.T) {
	cfg := feedTestConfig()
	for _, sw := range []bool{true, false} {
		f := setupTierFixture(t, boolKey(sw)+"feed", sw)
		for _, who := range []struct {
			name   string
			userID string
			want   [3]bool
		}{
			{"a member", f.memberID, [3]bool{true, true, true}},
			{"a follower", f.followerID, [3]bool{true, sw, false}},
		} {
			secret := strings.Repeat("s", 8) + who.userID
			if _, err := f.db.Exec(`UPDATE users SET feed_secret_hash = ? WHERE id = ?`,
				auth.HashToken(secret), who.userID); err != nil {
				t.Fatal(err)
			}
			r := authedRequest("GET", "/api/v1/feeds/"+secret+"/events.ics", nil, "")
			rec := servePublicMux(t, "GET", "/api/v1/feeds/{secret}/events.ics",
				handler.PersonalICSFeed(f.db, cfg), r)
			if rec.Code != http.StatusOK {
				t.Fatalf("personal feed for %s: code=%d", who.name, rec.Code)
			}
			body := rec.Body.String()
			for i, id := range f.events() {
				got := strings.Contains(body, id)
				if got != who.want[i] {
					t.Errorf("switch=%v %s in the personal ICS feed, %s event present=%v, want %v",
						sw, who.name, tierNames[i], got, who.want[i])
				}
			}
		}
	}
}

// The per-patch public feeds are public-only and stay that way: a subscriber
// is a URL, not a person, so there is nobody to ask about.
func TestEventTiers_PatchFeedStaysPublicOnly(t *testing.T) {
	cfg := feedTestConfig()
	f := setupTierFixture(t, "patchfeed", true)
	for _, tok := range []string{"", f.adminTok} {
		r := authedRequest("GET", "/api/v1/nodes/"+f.slug+"/events.ics", nil, tok)
		rec := serveOptionalAuthMux(t, f.db, "GET", "/api/v1/nodes/{slug}/events.ics",
			handler.NodeICSFeed(f.db, cfg), r)
		body := rec.Body.String()
		if !strings.Contains(body, f.publicID) {
			t.Error("the patch feed dropped its public event")
		}
		if strings.Contains(body, f.followersID) || strings.Contains(body, f.membersID) {
			t.Error("the patch feed carried a non-public event")
		}
	}
}

// The upcoming count on the patch page states what the list under it shows.
func TestEventTiers_UpcomingCountMatchesTheList(t *testing.T) {
	for _, sw := range []bool{true, false} {
		f := setupTierFixture(t, boolKey(sw)+"count", sw)
		for _, w := range tierWants() {
			want := w.on
			if !sw {
				want = w.off
			}
			n := 0
			for _, v := range want {
				if v {
					n++
				}
			}
			r := authedRequest("GET", "/api/v1/nodes/"+f.slug, nil, w.token(f))
			rec := serveOptionalAuthMux(t, f.db, "GET", "/api/v1/nodes/{slug}", handler.GetNode(f.db), r)
			var resp struct {
				Node struct {
					UpcomingEventCount int `json:"upcoming_event_count"`
				} `json:"node"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if resp.Node.UpcomingEventCount != n {
				t.Errorf("switch=%v %s: upcoming_event_count=%d, want %d",
					sw, w.who, resp.Node.UpcomingEventCount, n)
			}
		}
	}
}

// A confirmed event link grants a patch presence on somebody else's event.
// It never widens who may read it — that is a statement about the event's own
// patch, and the linked patch's members hold no role there.
func TestEventTiers_ALinkNeverWidens(t *testing.T) {
	f := setupTierFixture(t, "link", true)
	other, otherTok := createTestUser(t, f.db, "tierlinkother", "member")
	linked := createTestNode(t, f.db, other.ID, "Linked", "tier-linked", "open")
	createTestMembership(t, f.db, other.ID, linked, "admin", "active")
	if _, err := f.db.Exec(
		`INSERT INTO event_links (id, event_id, node_id, status, initiated_by, requested_by, confirmed_at)
		 VALUES ('tier-link-1', ?, ?, 'confirmed', 'owner', ?, '2026-01-01T00:00:00Z')`,
		f.followersID, linked, other.ID,
	); err != nil {
		t.Fatal(err)
	}
	ids := listScopedEventIDs(t, f.db,
		authedRequest("GET", "/api/v1/events?scope=my&limit=100", nil, otherTok))
	if ids[f.followersID] {
		t.Error("a confirmed link carried a followers-tier event to the linked patch's admin")
	}
	r := authedRequest("GET", "/api/v1/events/"+f.followersID, nil, otherTok)
	rec := serveOptionalAuthMux(t, f.db, "GET", "/api/v1/events/{id}", handler.GetEvent(f.db), r)
	if rec.Code != http.StatusNotFound {
		t.Errorf("the linked patch's admin read a followers-tier event: code=%d", rec.Code)
	}
}

// The switch reads the row down; it never rewrites it. Turning Follower
// Permissions back on restores the event's own statement of who it is for.
func TestEventTiers_TheSwitchIsReadNotWritten(t *testing.T) {
	f := setupTierFixture(t, "restore", false)
	var stored string
	if err := f.db.QueryRow(`SELECT visibility FROM events WHERE id = ?`, f.followersID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "followers" {
		t.Fatalf("the stored tier is %q; the ceiling rewrote the row", stored)
	}
	if _, err := f.db.Exec(
		`UPDATE nodes SET follower_permissions = '{"events":true,"proposals":true,"charters":true,"members":true}' WHERE id = ?`,
		f.nodeID); err != nil {
		t.Fatal(err)
	}
	r := authedRequest("GET", "/api/v1/events/"+f.followersID, nil, f.followerTok)
	rec := serveOptionalAuthMux(t, f.db, "GET", "/api/v1/events/{id}", handler.GetEvent(f.db), r)
	if rec.Code != http.StatusOK {
		t.Errorf("after the switch went back on, the follower still cannot read it: code=%d", rec.Code)
	}
}

// A patch that has never opened the rules editor stores "{}" and has said
// nothing, which means the box is ticked — the shipped default.
func TestEventTiers_AbsentPermissionsMeanAllowed(t *testing.T) {
	f := setupTierFixture(t, "absent", true)
	for _, blob := range []string{"{}", "", "not json at all"} {
		if _, err := f.db.Exec(`UPDATE nodes SET follower_permissions = ? WHERE id = ?`, blob, f.nodeID); err != nil {
			t.Fatal(err)
		}
		r := authedRequest("GET", "/api/v1/events/"+f.followersID, nil, f.followerTok)
		rec := serveOptionalAuthMux(t, f.db, "GET", "/api/v1/events/{id}", handler.GetEvent(f.db), r)
		if rec.Code != http.StatusOK {
			t.Errorf("follower_permissions = %q: code=%d, want 200", blob, rec.Code)
		}
		ids := listScopedEventIDs(t, f.db,
			authedRequest("GET", "/api/v1/events?node_slug="+f.slug+"&limit=100", nil, f.followerTok))
		if !ids[f.followersID] {
			t.Errorf("follower_permissions = %q: the listing dropped the followers event", blob)
		}
	}
}

func boolKey(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
