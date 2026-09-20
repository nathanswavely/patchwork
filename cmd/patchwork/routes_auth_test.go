package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	patchwork "github.com/patchwork-toolkit/patchwork"
	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// Every mutating route, asked twice: once by nobody, once by somebody who
// holds nothing.
//
// Patchwork decides authorization inside its handlers, deliberately. The
// per-template and per-venue rules (seatRoom, maintainerDecidable,
// electedHere, docs/adr/092's who-decides matrix) are not a role gate and
// would lose their meaning as one. The cost of that choice is that a route
// added without its own negative test gets no coverage at all: the checks are
// invisible from the router, and reading 150 handlers is the only audit there
// is. This test is the audit, run by the machine.
//
// The rule: a route whose method is not GET/HEAD/OPTIONS must answer 401, 403
// or 404 to both callers, or be named in one of the two lists below with a
// reason. 404 counts as a refusal because some rooms hide their existence from
// outsiders rather than admitting to it, and the noticeboard is the worked
// example (docs/adr/081).
//
// The lists are the point. They are what makes a new route that quietly
// accepts a stranger fail the build instead of shipping, and they read like
// the stays-behind list in internal/seamrip: a line each, saying why.

// openToAnyone: routes an unauthenticated caller may reach by design. Each of
// these either is the front door itself or carries a credential of its own in
// the request.
var openToAnyone = map[string]string{
	"POST /api/v1/auth/invite":                                 "redeeming an invite link is how an account is made; the token in the body is the credential",
	"POST /api/v1/auth/magic-link":                             "asking for a sign-in link; the caller has no session yet, by definition",
	"POST /api/v1/auth/signup":                                 "completing signup; the signup token is the credential",
	"POST /api/v1/auth/recovery":                               "redeeming a recovery code, which is the way back in when the passkey is gone",
	"POST /api/v1/auth/webauthn/login/begin":                   "the passkey sign-in ceremony starts before there is a session",
	"POST /api/v1/auth/webauthn/login/finish":                  "same ceremony, second leg: the assertion is the credential",
	"POST /api/v1/claims/verify-email":                         "the email-claim landing (docs/adr/030): possessing the token is the proof",
	"POST /ap/users/{id}/inbox":                                "ActivityPub inbox: remote callers have no local session, and the HTTP signature is the check",
	"POST /ap/nodes/{id}/inbox":                                "ActivityPub inbox, same as above",
	"POST /ap/instance/inbox":                                  "the instance service actor's inbox (docs/adr/024), same as above",
	"POST /api/v1/nodes/{slug}/governance.git/git-upload-pack": "git smart-HTTP fetch: a POST that reads, gated on canReadPatchDocs inside the transport (docs/adr/110)",
}

// openToAnySignedInUser: routes that refuse anonymous callers but accept a
// signed-in person holding no role on the patch. Every one of these is either
// the person acting on their own account, or a rung of the contributor ladder
// that has to be reachable from outside a patch for the ladder to exist.
var openToAnySignedInUser = map[string]string{
	// The person's own account and its credentials.
	"PATCH /api/v1/auth/me":                             "editing your own profile",
	"POST /api/v1/auth/logout":                          "ending your own session",
	"POST /api/v1/auth/recovery-codes":                  "minting your own recovery codes",
	"PATCH /api/v1/auth/credentials/{id}":               "renaming your own passkey; the handler scopes the row to the caller",
	"DELETE /api/v1/auth/credentials/{id}":              "removing your own passkey, same scoping",
	"POST /api/v1/auth/sessions/revoke-others":          "signing your other devices out",
	"DELETE /api/v1/auth/sessions/{id}":                 "revoking one of your own sessions",
	"POST /api/v1/auth/step-up/begin":                   "proving presence for your own account (docs/adr/017)",
	"POST /api/v1/auth/step-up/finish":                  "same ceremony, second leg",
	"POST /api/v1/auth/step-up/recovery":                "the same proof for a person whose device makes no passkey (docs/adr/099)",
	"POST /api/v1/auth/webauthn/register/begin":         "enrolling a passkey on your own account",
	"POST /api/v1/auth/webauthn/register/finish":        "same ceremony, second leg",
	"POST /api/v1/users/me/feed-secret":                 "your own My Quilt calendar URL",
	"DELETE /api/v1/users/me/feed-secret":               "revoking it",
	"POST /api/v1/users/me/contact-items":               "your own contact card (docs/adr/083)",
	"PATCH /api/v1/users/me/contact-items/{id}":         "same card, scoped to the caller's own items",
	"DELETE /api/v1/users/me/contact-items/{id}":        "same",
	"DELETE /api/v1/users/me/contact-items/{id}/shares": "unshare-everywhere, the one item-first write; it can only reduce exposure",
	"POST /api/v1/users/me/quilts":                      "your own connected quilts (docs/adr/024)",
	"DELETE /api/v1/users/me/quilts/{id}":               "same",
	"POST /api/v1/users/me/remote-follows":              "following a patch on another quilt is a fact about you, kept here",
	"PATCH /api/v1/users/me/remote-follows/{id}":        "same",
	"DELETE /api/v1/users/me/remote-follows/{id}":       "same",
	"PATCH /api/v1/users/me/steward":                    "the Label steward's own self-listing switch (docs/adr/023)",
	"DELETE /api/v1/users/me/steward":                   "same",
	"POST /api/v1/users/me/trust-request":               "asking for trust is exactly the act of someone who holds nothing yet",
	"PATCH /api/v1/users/me/memberships/{nodeId}":       "the one switch a member owns on their own membership (docs/adr/006); a non-member has no row to hit",
	"PUT /api/v1/notifications/preferences":             "your own notification settings",
	"POST /api/v1/notifications/read-all":               "your own notifications",
	"PATCH /api/v1/notifications/{id}/read":             "same, scoped to the caller's rows",
	"DELETE /api/v1/notifications/{id}":                 "same",
	"DELETE /api/v1/notifications":                      "same",

	// The contributor ladder: the rungs a stranger has to be able to reach.
	"POST /api/v1/nodes":                            "anyone signed in may start a patch",
	"POST /api/v1/nodes/{slug}/join":                "joining or following is frictionless by design; approval_required patches queue the row",
	"POST /api/v1/nodes/{slug}/leave":               "leaving your own membership; a non-member's call finds no row",
	"POST /api/v1/nodes/{slug}/withdraw":            "withdrawing your own pending request (the verb that is not leave)",
	"POST /api/v1/nodes/{slug}/invitations/accept":  "answering an invitation addressed to you (docs/adr/098); the invitee is by definition not a member yet",
	"POST /api/v1/nodes/{slug}/invitations/decline": "the other half of the same answer",
	"POST /api/v1/submissions":                      "suggesting a patch the quilt does not have; the whole point is that outsiders may",
	"POST /api/v1/events":                           "posting an event; a caller with no standing gets status pending_review (docs/adr/026)",
	"POST /api/v1/reports":                          "reporting content, and a reader who holds nothing is the most likely reporter",
	"POST /api/v1/nodes/{slug}/claim":               "claiming an unclaimed patch (docs/adr/030); the claimant holds nothing yet, that is the point",
	"POST /api/v1/claims/{id}/verify":               "your own claim; the handler scopes it to the caller",
	"POST /api/v1/claims/{id}/withdraw":             "same",
	"POST /api/v1/claims/{id}/resend-email":         "same",
	"POST /api/v1/claims/{id}/setup":                "same",
	"DELETE /api/v1/proposals/{id}/candidates/me":   "withdrawing your own candidacy (docs/adr/107); scoped to the caller's row",
}

// ---------------------------------------------------------------------------
// The walk.

func TestEveryMutatingRouteRefusesAStranger(t *testing.T) {
	f := newRouteFixture(t)

	var walked int
	answers := map[int]int{}
	for _, rt := range f.routes {
		switch rt.Method {
		case "", http.MethodGet, http.MethodHead, http.MethodOptions:
			continue
		}
		key := rt.Method + " " + rt.Pattern
		if _, ok := openToAnyone[key]; ok {
			continue
		}
		walked++

		t.Run("anonymous "+key, func(t *testing.T) {
			code, body := f.call(t, rt, "")
			if !refused(code) {
				t.Errorf("%s answered an anonymous caller with %d, not a refusal: %s\n"+
					"Either the route checks nothing, or it belongs in openToAnyone with a reason.",
					key, code, snippet(body))
			}
		})

		if _, ok := openToAnySignedInUser[key]; ok {
			continue
		}
		t.Run("roleless "+key, func(t *testing.T) {
			code, body := f.call(t, rt, f.outsiderToken)
			answers[code]++
			if !refused(code) {
				t.Errorf("%s answered a signed-in caller who holds no role on the patch with %d, not a refusal: %s\n"+
					"Either the route's check is missing, or it is open to any signed-in user and belongs in openToAnySignedInUser with a reason.",
					key, code, snippet(body))
			}
		})
	}

	if walked < 100 {
		t.Fatalf("only %d mutating routes were walked, so the route table did not come through", walked)
	}

	// The failure this guards against is silent: if the fixture rows stop
	// matching what the handlers look up, every call 404s on a missing row,
	// every assertion above still passes, and the walk quietly stops testing
	// anything. A 403 is the shape of a check that ran. Most of these routes
	// give one; the handful that answer 404 do it on purpose, because the room
	// does not admit to outsiders that it is there (docs/adr/081). So: most,
	// not all, and if this trips, look at the fixture before the allowlists.
	if refusals := answers[http.StatusForbidden]; refusals*2 < walked-len(openToAnySignedInUser) {
		t.Errorf("only %d of the roleless calls were refused with 403 (answers: %v), so the fixture is probably not reaching the handlers any more", refusals, answers)
	}

	t.Logf("walked %d mutating routes; %d open to anyone, %d open to any signed-in user; roleless answers %v",
		walked, len(openToAnyone), len(openToAnySignedInUser), answers)
}

// A route that is named in a list but no longer registered is a stale excuse,
// and a stale excuse is how a list like this stops being read.
func TestNoStaleAllowlistEntries(t *testing.T) {
	f := newRouteFixture(t)

	registered := map[string]bool{}
	for _, rt := range f.routes {
		registered[rt.Method+" "+rt.Pattern] = true
	}
	var stale []string
	for _, list := range []map[string]string{openToAnyone, openToAnySignedInUser} {
		for key := range list {
			if !registered[key] {
				stale = append(stale, key)
			}
		}
	}
	sort.Strings(stale)
	for _, key := range stale {
		t.Errorf("allowlisted route %s is not registered any more, so drop the entry", key)
	}
}

func refused(code int) bool {
	return code == http.StatusUnauthorized || code == http.StatusForbidden || code == http.StatusNotFound
}

func snippet(body string) string {
	body = strings.TrimSpace(body)
	if len(body) > 180 {
		body = body[:180] + "…"
	}
	return body
}

// ---------------------------------------------------------------------------
// The fixture: one public patch with an admin, and enough rows behind every
// path parameter that a request reaches the authorization check instead of
// dying on a missing row first.

type routeFixture struct {
	db            *database.DB
	mux           *http.ServeMux
	routes        []route
	ids           map[string]string
	outsiderToken string
}

func (f *routeFixture) call(t *testing.T, rt route, token string) (int, string) {
	t.Helper()
	path := f.fill(t, rt.Pattern)
	var body []byte
	if rt.Method != http.MethodDelete {
		body = routeBody(rt.Method + " " + rt.Pattern)
	}
	r := httptest.NewRequest(rt.Method, path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	// The header the CSRF middleware wants. The mux under test is the bare
	// route table, so this only keeps the request shaped like a real one.
	r.Header.Set("X-Patchwork-Request", "true")
	if token != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

// bodyFields: the extra fields a particular route's handler validates before
// it gets to the question of who is asking. Without them that route answers
// 400 and the check is never reached, which is a test that proves nothing.
// Nothing here grants anything: if a route accepts one of these bodies from a
// stranger, that is the finding.
var bodyFields = map[string]map[string]any{
	"POST /api/v1/proposals/{id}/decide":        {"decision": "approve"},
	"POST /api/v1/aggregator-holds/{id}/decide": {"decision": "same"},
	"POST /api/v1/comments/{id}/reactions":      {"emoji": "👍"},
	"PATCH /api/v1/events/{id}/review":          {"action": "approve"},
	"POST /api/v1/events/{id}/links":            {"target": "the-selvage"},
}

// routeBody is a body plausible enough to get past a handler's own decoder.
func routeBody(key string) []byte {
	payload := map[string]any{
		"name":        "A Patch",
		"title":       "A Title",
		"body":        "Some words.",
		"reason":      "spam",
		"entity_type": "notice",
		"role":        "follower",
		"value":       "approve",
		"method":      "dns",
		"url":         "https://elsewhere.example/feed.ics",
		"scope":       "all",
		"action":      "dismiss",
		"starts_at":   time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
	}
	for k, v := range bodyFields[key] {
		payload[k] = v
	}
	b, _ := json.Marshal(payload)
	return b
}

// fill substitutes real ids for a pattern's wildcards. An unknown wildcard is
// a hard failure rather than a fabricated id: a request that 404s on a missing
// row would pass this test without ever reaching the check it is here to
// exercise.
func (f *routeFixture) fill(t *testing.T, pattern string) string {
	t.Helper()
	segs := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	for i, seg := range segs {
		if !strings.HasPrefix(seg, "{") {
			continue
		}
		name := strings.Trim(seg, "{}")
		prev := ""
		if i > 0 {
			prev = segs[i-1]
		}
		segs[i] = f.value(t, pattern, name, prev)
	}
	return "/" + strings.Join(segs, "/")
}

func (f *routeFixture) value(t *testing.T, pattern, name, prev string) string {
	t.Helper()
	// One route may need a row of its own. Detach wants an event that is
	// actually attached to a feed, or it 400s before it looks at the caller.
	if v, ok := f.ids[pattern+" {"+name+"}"]; ok {
		return v
	}
	key := name
	if name == "id" {
		// An {id} means whatever the collection before it means.
		key = prev
	}
	if v, ok := f.ids[key]; ok {
		return v
	}
	t.Fatalf("route %s has a {%s} this fixture has no row for (collection %q); add one to newRouteFixture", pattern, name, prev)
	return ""
}

func newRouteFixture(t *testing.T) *routeFixture {
	t.Helper()

	db := routesTestDB(t)
	cfg := &config.Config{}
	cfg.Instance.Name = "Test Quilt"
	cfg.Instance.Domain = "quilt.example"
	// Federation on, so the AP and git-transport mounts are walked too. They
	// are part of the surface on a real instance and they take POSTs.
	cfg.Federation.Enabled = true

	ap.SetDomain(cfg.Instance.Domain)
	governance.SetDataDir(t.TempDir())
	if err := governance.InitInstanceRepo(t.TempDir()); err != nil {
		t.Fatalf("governance init: %v", err)
	}
	handler.SetNotifier(notifications.NewNotifier(db))

	wa, err := auth.NewWebAuthnService(db, cfg)
	if err != nil {
		t.Fatalf("webauthn: %v", err)
	}

	mux, routes := buildRoutes(serverDeps{
		db:       db,
		cfg:      cfg,
		wa:       wa,
		notifier: notifications.NewNotifier(db),
		usage:    middleware.NewUsageCounter(db, func() bool { return false }),
		spa: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "spa", http.StatusNotFound)
		}),
	})

	f := &routeFixture{db: db, mux: mux, routes: routes, ids: map[string]string{}}
	f.seed(t)
	return f
}

func routesTestDB(t *testing.T) *database.DB {
	t.Helper()
	tmp, err := os.CreateTemp("", "patchwork-routes-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	migrations, err := fs.Sub(patchwork.MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("migrations fs: %v", err)
	}
	db, err := database.Open(tmp.Name(), migrations)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func (f *routeFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.db.Exec(query, args...); err != nil {
		t.Fatalf("fixture %s: %v", firstLine(query), err)
	}
}

func firstLine(q string) string {
	q = strings.TrimSpace(q)
	if i := strings.IndexByte(q, '\n'); i >= 0 {
		q = q[:i]
	}
	return q
}

func (f *routeFixture) seed(t *testing.T) {
	t.Helper()
	id := func(key string) string {
		v := auth.NewUUIDv7()
		f.ids[key] = v
		return v
	}
	// Patchwork stores timestamps in the shape SQLite's strftime writes, and
	// some readers parse exactly that layout, so the fixture writes it too.
	const stamp = "2006-01-02T15:04:05.000Z"
	soon := time.Now().Add(72 * time.Hour).UTC().Format(stamp)

	// The patch's admin holds everything; the outsider holds nothing. Both are
	// ordinary accounts, and neither is an instance admin, because an instance
	// admin would sail through the admin routes and prove nothing.
	owner := id("users")
	f.exec(t, `INSERT INTO users (id, username, display_name, role, email) VALUES (?, 'weaver', 'Weaver', 'member', 'weaver@example.com')`, owner)
	outsider := auth.NewUUIDv7()
	f.exec(t, `INSERT INTO users (id, username, display_name, role, email) VALUES (?, 'stranger', 'Stranger', 'member', 'stranger@example.com')`, outsider)
	f.ids["username"] = "weaver"
	f.ids["userId"] = owner

	token, err := auth.CreateSession(f.db, outsider, "127.0.0.1", "route-walk")
	if err != nil {
		t.Fatalf("create outsider session: %v", err)
	}
	f.outsiderToken = token

	// The owner's session, so {id} under /auth/sessions is a real row that is
	// not the caller's.
	ownerSession := auth.NewUUIDv7()
	f.exec(t, `INSERT INTO sessions (id, user_id, token, expires_at) VALUES (?, ?, ?, ?)`,
		ownerSession, owner, auth.NewUUIDv7(), soon)
	f.ids["sessions"] = ownerSession

	// One public, open patch, run by the owner.
	node := id("nodes")
	f.ids["slug"] = "gallery-row"
	f.ids["nodeId"] = node
	f.exec(t, `INSERT INTO nodes (id, owner_id, name, slug, node_type, visibility, membership_policy, status, follower_permissions)
		VALUES (?, ?, 'Gallery Row', 'gallery-row', 'leaf', 'public', 'open', 'active', '{}')`, node, owner)
	f.exec(t, `INSERT INTO memberships (id, user_id, node_id, role, status) VALUES (?, ?, ?, 'admin', 'active')`,
		auth.NewUUIDv7(), owner, node)

	// An unclaimed patch, for the routes whose subject is a suggestion.
	unclaimed := id("submissions")
	f.exec(t, `INSERT INTO nodes (id, owner_id, name, slug, node_type, visibility, membership_policy, status, follower_permissions, submission_source)
		VALUES (?, ?, 'The Selvage', 'the-selvage', 'leaf', 'public', 'open', 'unclaimed', '{}', 'submission')`, unclaimed, owner)

	proposal := id("proposals")
	f.exec(t, `INSERT INTO proposals (id, node_id, author_id, title, body) VALUES (?, ?, ?, 'A proposal', 'Body')`,
		proposal, node, owner)

	// Two elections, because nominating and voting are different moments of
	// one proposal and each route refuses the wrong moment before it looks at
	// the caller (docs/adr/051).
	past := time.Now().Add(-24 * time.Hour).UTC().Format(stamp)
	nominating := auth.NewUUIDv7()
	f.exec(t, `INSERT INTO proposals (id, node_id, author_id, title, seats_contested, nominations_close_at) VALUES (?, ?, ?, 'An election', 1, ?)`,
		nominating, node, owner, soon)
	f.ids["/api/v1/proposals/{id}/candidates {id}"] = nominating
	voting := auth.NewUUIDv7()
	f.exec(t, `INSERT INTO proposals (id, node_id, author_id, title, seats_contested, nominations_close_at, voting_ends_at) VALUES (?, ?, ?, 'An election, voting', 1, ?, ?)`,
		voting, node, owner, past, soon)
	f.ids["/api/v1/proposals/{id}/ballot {id}"] = voting

	comment := id("comments")
	f.exec(t, `INSERT INTO proposal_comments (id, proposal_id, author_id, body) VALUES (?, ?, ?, 'A comment')`,
		comment, proposal, owner)
	f.exec(t, `INSERT INTO comment_reactions (id, comment_id, user_id, emoji) VALUES (?, ?, ?, '🧵')`,
		auth.NewUUIDv7(), comment, owner)
	f.ids["emoji"] = url.PathEscape("🧵")

	event := id("events")
	f.exec(t, `INSERT INTO events (id, node_id, created_by, title, starts_at) VALUES (?, ?, ?, 'An event', ?)`,
		event, node, owner, soon)
	mention := auth.NewUUIDv7()
	f.exec(t, `INSERT INTO event_mentions (id, event_id, host, slug, name) VALUES (?, ?, 'other.example', 'their-patch', 'Their Patch')`,
		mention, event)
	f.ids["mentionId"] = mention

	// A pending link between the patch's event and the other patch, so the two
	// link routes find a row and get as far as asking who is speaking.
	f.exec(t, `INSERT INTO event_links (id, event_id, node_id, status, initiated_by, requested_by) VALUES (?, ?, ?, 'pending', 'owner', ?)`,
		auth.NewUUIDv7(), event, unclaimed, owner)
	f.ids["/api/v1/events/{id}/links/{nodeId}/confirm {nodeId}"] = unclaimed
	f.ids["/api/v1/events/{id}/links/{nodeId} {nodeId}"] = unclaimed

	notice := id("notices")
	f.exec(t, `INSERT INTO notices (id, node_id, author_id, title, body) VALUES (?, ?, ?, 'A notice', 'Body')`,
		notice, node, owner)
	reply := id("replies")
	f.exec(t, `INSERT INTO notice_replies (id, notice_id, author_id, body) VALUES (?, ?, ?, 'A reply')`,
		reply, notice, owner)

	doc := id("governance")
	f.exec(t, `INSERT INTO governance_docs (id, node_id, title, body, created_by) VALUES (?, ?, 'Charter', 'Body', ?)`,
		doc, node, owner)

	seat := id("seats")
	f.exec(t, `INSERT INTO seats (id, node_id, term_ends_at) VALUES (?, ?, ?)`, seat, node, soon)

	attestation := auth.NewUUIDv7()
	f.exec(t, `INSERT INTO attestations (id, node_id, kind, decided_at, recorded_by) VALUES (?, ?, 'leadership', ?, ?)`,
		attestation, node, soon, owner)
	attName := id("attestation-names")
	f.exec(t, `INSERT INTO attestation_names (id, attestation_id, display_name) VALUES (?, ?, 'Someone')`,
		attName, attestation)

	aggregator := id("aggregators")
	f.exec(t, `INSERT INTO aggregators (id, name, url, added_by) VALUES (?, 'City Calendar', 'https://city.example/all.ics', ?)`,
		aggregator, owner)

	source := id("event-sources")
	f.exec(t, `INSERT INTO event_sources (id, node_id, url, added_by) VALUES (?, ?, 'https://gallery.example/events.ics', ?)`,
		source, node, owner)
	// A crosswalk entry is an aggregator-attached source on the patch.
	crosswalk := id("crosswalk")
	f.exec(t, `INSERT INTO event_sources (id, node_id, url, added_by, aggregator_id, name_key) VALUES (?, ?, 'https://city.example/all.ics#gallery-row', ?, ?, 'gallery row')`,
		crosswalk, node, owner, aggregator)
	// Review refuses an event that is not awaiting review before it looks at
	// the caller, so that route gets a submission of its own.
	pending := auth.NewUUIDv7()
	f.exec(t, `INSERT INTO events (id, node_id, created_by, title, starts_at, status) VALUES (?, ?, ?, 'A submitted event', ?, 'pending_review')`,
		pending, node, owner, soon)
	f.ids["/api/v1/events/{id}/review {id}"] = pending

	// Detach refuses an event that came from nobody's feed before it ever
	// looks at the caller, so that one route gets an imported event.
	imported := auth.NewUUIDv7()
	f.exec(t, `INSERT INTO events (id, node_id, created_by, title, starts_at, source_id, source_uid) VALUES (?, ?, ?, 'An imported event', ?, ?, 'uid-imported')`,
		imported, node, owner, soon, source)
	f.ids["/api/v1/events/{id}/detach {id}"] = imported

	hold := id("aggregator-holds")
	f.exec(t, `INSERT INTO aggregator_holds (id, source_id, node_id, uid, rival_event_id, title, starts_at) VALUES (?, ?, ?, 'uid-1', ?, 'A clash', ?)`,
		hold, crosswalk, node, event, soon)
	program := id("programs")
	f.exec(t, `INSERT INTO aggregator_programs (id, aggregator_id, name_key, title_key, display_title, node_id, credited_by)
		VALUES (?, ?, 'gallery row', 'life drawing', 'Life Drawing', ?, ?)`,
		program, aggregator, node, owner)

	claim := id("claims")
	f.exec(t, `INSERT INTO claim_requests (id, node_id, user_id, method) VALUES (?, ?, ?, 'dns')`,
		claim, unclaimed, owner)

	report := id("reports")
	f.exec(t, `INSERT INTO content_reports (id, reporter_id, entity_type, entity_id, reason, node_id) VALUES (?, ?, 'notice', ?, 'spam', ?)`,
		report, owner, notice, node)

	notification := id("notifications")
	f.exec(t, `INSERT INTO notifications (id, user_id, type, title) VALUES (?, ?, 'proposal.created', 'Something happened')`,
		notification, owner)

	credential := id("credentials")
	f.exec(t, `INSERT INTO credentials (id, user_id, credential_id, public_key) VALUES (?, ?, ?, ?)`,
		credential, owner, []byte("credential-bytes"), []byte("public-key-bytes"))

	item := id("contact-items")
	f.exec(t, `INSERT INTO contact_items (id, user_id, kind, value) VALUES (?, ?, 'email', 'weaver@example.com')`,
		item, owner)

	quilt := id("quilts")
	f.exec(t, `INSERT INTO user_quilts (id, user_id, url, name) VALUES (?, ?, 'https://other.example', 'Other Quilt')`,
		quilt, owner)

	follow := id("remote-follows")
	f.exec(t, `INSERT INTO remote_follows (id, user_id, quilt_url, node_ap_id, node_slug, node_name)
		VALUES (?, ?, 'https://other.example', 'https://other.example/ap/nodes/1', 'their-patch', 'Their Patch')`,
		follow, owner)

	// One listed native app, so DELETE /admin/native-apps/{id} finds a row
	// and gets as far as asking who is calling
	// (docs/adr/2026-09-20-an-instance-vouches-for-an-app.md).
	nativeApp := id("native-apps")
	f.exec(t, `INSERT INTO native_apps (id, platform, identifier, label) VALUES (?, 'apple', 'ABCDE12345.org.example.app', 'Example')`, nativeApp)

	neighbor := id("neighbor-quilts")
	f.exec(t, `INSERT INTO neighbor_quilts (id, url, name) VALUES (?, 'https://neighbor.example', 'Neighbor')`, neighbor)

	tag := id("tags")
	f.exec(t, `INSERT INTO tags (id, name) VALUES (?, 'textiles')`, tag)
	suggestion := id("tag-suggestions")
	f.exec(t, `INSERT INTO tags (id, name, status, suggested_by) VALUES (?, 'letterpress', 'pending', ?)`, suggestion, owner)
	f.exec(t, `INSERT INTO node_tags (node_id, tag_id) VALUES (?, ?)`, node, suggestion)
	f.ids["name"] = "letterpress"

	trust := id("trust-requests")
	f.exec(t, `INSERT INTO trust_requests (id, user_id, scope, created_at) VALUES (?, ?, 'all', ?)`,
		trust, owner, time.Now().UTC().Format(stamp))

	steward := id("stewards")
	f.exec(t, `INSERT INTO label_stewards (id, user_id, blurb) VALUES (?, ?, 'Keeps the label')`, steward, owner)

	// Path values that name a thing rather than a row.
	f.ids["legal"] = "privacy"
	f.ids["doc"] = "privacy"
	f.ids["token"] = "not-a-real-token"
	f.ids["secret"] = "not-a-real-secret"
}
