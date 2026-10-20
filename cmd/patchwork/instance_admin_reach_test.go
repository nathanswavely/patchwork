package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
)

// Every route, asked by an instance admin who holds no role in the patch.
//
// docs/adr/115 decided that an instance admin's reach into a patch is
// custody, not rank, and deferred the implementation on purpose (issue #286).
// internal/handler's TestEveryInstanceAdminReachIsDeclared pins the reach at
// the source: every comparison of a user's instance role against "admin". That
// sees the checks, and only the checks in that package. It cannot see
// middleware.RequireNodeRole, which lets an instance admin past six routes
// before any handler runs, and two of those (the patch's notification
// switches) have no check of their own at all. Nor can it see which routes a
// shared helper opens: canReadGovernanceRecord is one line in the inventory
// and five routes here.
//
// So this is the same characterization from the other side: the routes, as
// the caller experiences them. A route reaches when it refuses a signed-in
// stranger and answers the instance admin, or, for a read about a patch,
// answers both with different bodies. Every route that reaches has to be
// named below with what docs/adr/115 says it should end up as. A new one fails
// the build; one that stops reaching is a stale entry and fails it too.
//
// The fixture's patch has an admin of its own, so it is *held*: under ADR 115
// every reach below that is not an instance surface, speech, or the patch's
// own stated consent is one #286 removes. When #286 lands those entries are
// deleted, and this test is what proves they were.

// reachEnd is what docs/adr/115 says an instance admin holding no role in a
// held patch should end up with at this route.
type reachEnd string

const (
	// endNone: nothing. The patch has an admin of its own.
	endNone reachEnd = "none"
	// endUnclaimed: kept only while the patch is unclaimed, where the
	// instance admin holds its calendar in trust (docs/adr/026, 031, 056,
	// 057). On this held patch it is a reach #286 removes.
	endUnclaimed reachEnd = "custody of an unclaimed patch"
	// endVacated: kept only while a claimed patch has no active admin, and
	// only to put one back. On this held patch it is a reach #286 removes.
	endVacated reachEnd = "custody of a vacated patch"
	// endInstance: not a patch's to hold. Kept.
	endInstance reachEnd = "instance surface"
	// endSpeech: commenting. Kept (docs/adr/115's named exception).
	endSpeech reachEnd = "speech"
	// endConsent: the patch said yes to this itself. Kept.
	endConsent reachEnd = "the patch's own consent"
)

// kept reports whether a reach with this end survives on a held patch.
func (e reachEnd) kept() bool {
	return e == endInstance || e == endSpeech || e == endConsent
}

// instanceAdminRoutes is every route an instance admin holding no role in a
// held patch reaches today. The helper named in each reason is the one
// declared in internal/handler/instance_admin_reach.go, where the decision for
// that site is argued; this list says where it surfaces.
var instanceAdminRoutes = map[string]struct {
	end reachEnd
	why string
}{
	// Instance surfaces outside /api/v1/admin. Kept.
	"POST /api/v1/auth/invite-link":                  {endInstance, "AdminRequired: an invitation to the instance, not to a patch"},
	"GET /api/v1/nodes/{slug}/ap-preview":            {endInstance, "AdminRequired: how the instance federates this patch"},
	"DELETE /api/v1/nodes/{slug}":                    {endInstance, "archive, ADR 115's named exception (DeleteNode)"},
	"POST /api/v1/proposals/{id}/comments":           {endSpeech, "commenting (CreateComment)"},
	"POST /api/v1/comments/{id}/reactions":           {endSpeech, "reacting (AddReaction)"},
	"DELETE /api/v1/comments/{id}/reactions/{emoji}": {endSpeech, "un-reacting (RemoveReaction)"},
	"POST /api/v1/nodes/{slug}/crosswalk": {endConsent,
		"crosswalkAccess's suggest outcome: this patch has accept_event_suggestions on, entries go to its own review queue"},

	// The patch itself.
	"PATCH /api/v1/nodes/{slug}": {endNone, "UpdateNode, through RequireNodeRole and its own check"},
	"GET /api/v1/nodes/{slug}":   {endNone, "GetNode's is_admin flag, which every patch-admin surface in the web UI reads"},
	"GET /api/v1/nodes/{slug}/notification-config": {endNone,
		"RequireNodeRole's bypass alone; the handler checks nothing, and internal/handler's inventory cannot see it"},
	"PUT /api/v1/nodes/{slug}/notification-config": {endNone,
		"same: an instance admin can switch off what a patch tells its own people, and no source check says so"},
	"PATCH /api/v1/nodes/{slug}/members/{userId}": {endVacated, "UpdateMember, the vacated-patch recovery path"},

	// Charters and the governance record.
	"GET /api/v1/nodes/{slug}/governance":             {endNone, "members-only charters in the list (canReadPatchDocs)"},
	"GET /api/v1/governance/{id}":                     {endNone, "a members-only charter (canReadPatchDocs)"},
	"GET /api/v1/governance/{id}/versions":            {endNone, "its revisions and editors (canReadPatchDocs)"},
	"POST /api/v1/nodes/{slug}/governance":            {endNone, "CreateGovernanceDoc (also issue #408 item 3)"},
	"PUT /api/v1/governance/{id}":                     {endNone, "UpdateGovernanceDoc, the same reach #408 item 3 names for create"},
	"GET /api/v1/nodes/{slug}/proposals":              {endNone, "a closed governance record (canReadGovernanceRecord → viewerIsInPatchRoom)"},
	"GET /api/v1/proposals/{id}":                      {endNone, "same"},
	"GET /api/v1/proposals/{id}/comments":             {endNone, "same"},
	"GET /api/v1/nodes/{slug}/attestations":           {endNone, "same"},
	"GET /api/v1/nodes/{slug}/amendment-attestations": {endNone, "same"},
	"GET /api/v1/nodes/{slug}/governance/overview":    {endNone, "same"},
	"GET /api/v1/nodes/{slug}/governance/record":      {endNone, "same"},
	"PATCH /api/v1/proposals/{id}":                    {endNone, "UpdateProposal"},
	"DELETE /api/v1/proposals/{id}":                   {endNone, "WithdrawProposal"},
	"DELETE /api/v1/comments/{id}":                    {endNone, "DeleteComment: moderation goes through the report queue"},

	// The calendar. Kept on an unclaimed patch only.
	"GET /api/v1/events/{id}":                           {endUnclaimed, "the edit affordances on an event (GetEvent and its neighbours)"},
	"PATCH /api/v1/events/{id}":                         {endUnclaimed, "UpdateEvent"},
	"DELETE /api/v1/events/{id}":                        {endUnclaimed, "DeleteEvent"},
	"PATCH /api/v1/events/{id}/review":                  {endUnclaimed, "ReviewEventSubmission's active branch"},
	"GET /api/v1/nodes/{slug}/event-submissions":        {endUnclaimed, "ListNodeEventSubmissions"},
	"POST /api/v1/nodes/{slug}/events/bulk":             {endUnclaimed, "BulkCreateEvents"},
	"POST /api/v1/events/{id}/links":                    {endUnclaimed, "userSpeaksForNode"},
	"POST /api/v1/events/{id}/links/{nodeId}/confirm":   {endUnclaimed, "userSpeaksForNode"},
	"DELETE /api/v1/events/{id}/links/{nodeId}":         {endUnclaimed, "userSpeaksForNode"},
	"DELETE /api/v1/events/{id}/mentions/{mentionId}":   {endUnclaimed, "userSpeaksForNode"},
	"GET /api/v1/nodes/{slug}/event-sources":            {endUnclaimed, "sourceNodeAccess"},
	"POST /api/v1/nodes/{slug}/event-sources":           {endUnclaimed, "sourceNodeAccess"},
	"PATCH /api/v1/nodes/{slug}/event-sources/{id}":     {endUnclaimed, "sourceNodeAccess"},
	"DELETE /api/v1/nodes/{slug}/event-sources/{id}":    {endUnclaimed, "sourceNodeAccess"},
	"POST /api/v1/nodes/{slug}/event-sources/{id}/sync": {endUnclaimed, "sourceNodeAccess"},
	"POST /api/v1/events/{id}/detach":                   {endNone, "DetachEvent"},
	"GET /api/v1/nodes/{slug}/aggregator-names":         {endUnclaimed, "crosswalkNodeAccess"},
	"GET /api/v1/nodes/{slug}/crosswalk":                {endUnclaimed, "crosswalkNodeAccess"},
	"DELETE /api/v1/nodes/{slug}/crosswalk/{id}":        {endUnclaimed, "crosswalkNodeAccess"},
	"GET /api/v1/nodes/{slug}/aggregator-holds":         {endUnclaimed, "crosswalkNodeAccess"},
	"POST /api/v1/aggregator-holds/{id}/decide":         {endNone, "DecideAggregatorHold"},
	"GET /api/v1/nodes/{slug}/programs":                 {endUnclaimed, "programNodeAccess → userSpeaksForNode"},
	"POST /api/v1/nodes/{slug}/programs":                {endUnclaimed, "programNodeAccess → userSpeaksForNode"},
	"DELETE /api/v1/nodes/{slug}/programs/{id}":         {endUnclaimed, "programNodeAccess → userSpeaksForNode"},
	"POST /api/v1/nodes/{slug}/offers/dismiss":          {endUnclaimed, "programNodeAccess → userSpeaksForNode"},
}

// unwalkedReads are GET routes whose path parameter this fixture has no row
// for. Each is named so that a new read about a patch cannot slip past the
// walk by taking a parameter nobody seeded; add a row to newRouteFixture
// rather than a line here when the read is about a patch.
var unwalkedReads = map[string]string{
	"GET /api/v1/templates/{id}": "a governance template shipped in the binary, about no patch",
	"GET /pins/{id}":             "the retired pin permalink, a redirect (docs/adr/027)",
}

func TestEveryRouteAnInstanceAdminReachesIsDeclared(t *testing.T) {
	f := newRouteFixture(t)
	steward := stewardSession(t, f)

	reached := map[string]string{}
	var unmarked []string
	var walked int
	for _, rt := range f.routes {
		switch rt.Method {
		case "", http.MethodHead, http.MethodOptions:
			continue
		}
		key := rt.Method + " " + rt.Pattern
		// AdminRequired, every one: instance-level objects that were never
		// a patch's to hold (docs/adr/115's third named exception).
		if strings.HasPrefix(rt.Pattern, "/api/v1/admin/") {
			continue
		}
		if rt.Method != http.MethodGet {
			// Open to any signed-in caller by design, so not an instance
			// admin's reach. CreateEvent's direct publish lives here and is
			// pinned at the source instead (events.go:CreateEvent).
			if _, ok := openToAnyone[key]; ok {
				continue
			}
			if _, ok := openToAnySignedInUser[key]; ok {
				continue
			}
		}
		if !fixtureCanFill(f, rt.Pattern) {
			if _, ok := unwalkedReads[key]; !ok {
				t.Errorf("%s takes a path parameter this fixture has no row for, so the walk cannot ask it anything. "+
					"Add a row to newRouteFixture, or name it in unwalkedReads with a reason.", key)
			}
			continue
		}
		walked++

		outsiderCode, outsiderBody := f.call(t, rt, f.outsiderToken)
		before := lastAuditRow(t, f)
		stewardCode, stewardBody := f.call(t, rt, steward)
		if rt.Method != http.MethodGet {
			unmarked = append(unmarked, unmarkedAuditEntries(t, f, before, key)...)
		}

		switch {
		case refused(outsiderCode) && !refused(stewardCode):
			reached[key] = "outsider " + http.StatusText(outsiderCode) + ", instance admin " + http.StatusText(stewardCode)
		case rt.Method == http.MethodGet && aboutAPatch(rt.Pattern) &&
			!refused(outsiderCode) && !refused(stewardCode) && outsiderBody != stewardBody:
			reached[key] = "both answered, the instance admin was told more"
		}

		// A call that got through may have changed the patch, and the next
		// route must ask about the patch as it was. A refused one changed
		// nothing, which is what lets the walk share one fixture at all.
		if rt.Method != http.MethodGet && (!refused(outsiderCode) || !refused(stewardCode)) {
			f = newRouteFixture(t)
			steward = stewardSession(t, f)
		}
	}

	if walked < 120 {
		t.Fatalf("only %d routes were walked, so the route table did not come through", walked)
	}

	var undeclared, stale []string
	var removable int
	for key, how := range reached {
		rule, ok := instanceAdminRoutes[key]
		if !ok {
			undeclared = append(undeclared, key+" ("+how+")")
			continue
		}
		if !rule.end.kept() {
			removable++
		}
	}
	for key := range instanceAdminRoutes {
		if _, ok := reached[key]; !ok {
			stale = append(stale, key)
		}
	}
	sort.Strings(undeclared)
	sort.Strings(stale)

	if len(undeclared) > 0 {
		t.Errorf("routes an instance admin holding no role in the patch reaches, with no entry in instanceAdminRoutes:\n  %s\n\n"+
			"A new one is almost always docs/adr/115's reflex: a handler copying `user.Role == \"admin\"` from its "+
			"neighbour, or a route mounted behind middleware.RequireNodeRole, which waves every instance admin "+
			"through. Read docs/adr/115, decide what this route should end up as, and declare it. If the answer "+
			"is none, prefer not granting it.",
			strings.Join(undeclared, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("instanceAdminRoutes entries an instance admin no longer reaches:\n  %s\n\n"+
			"If issue #286 removed the reach, delete the entry. If the route was renamed, rename the key. "+
			"If the fixture stopped reaching the handler, fix the fixture: a 404 on a missing row reads as a refusal.",
			strings.Join(stale, "\n  "))
	}

	sort.Strings(unmarked)
	if len(unmarked) > 0 {
		t.Errorf("audit entries an instance admin holding no role wrote without acting_without_role:\n  %s\n\n"+
			"Every act on a patch that the instance role let through says so in its audit entry "+
			"(docs/adr/2026-10-20-an-act-the-instance-role-let-through-says-so.md). Wrap the entry's payload "+
			"in reachDetail with actingWithoutRole, or, if the entry is about no patch at all, name the "+
			"action in auditAboutNoPatch.",
			strings.Join(unmarked, "\n  "))
	}

	t.Logf("docs/adr/115: walked %d routes; an instance admin holding no role reaches %d, of which %d are reaches issue #286 removes from a held patch",
		walked, len(reached), removable)
}

// auditAboutNoPatch are audit actions an instance admin may write from a
// route this walk reaches that are about the instance, not a patch, so there
// is no patch role they were acting without.
var auditAboutNoPatch = map[string]string{
	"invite.generate": "an invitation to the instance (POST /api/v1/auth/invite-link)",
}

func lastAuditRow(t *testing.T, f *routeFixture) int64 {
	t.Helper()
	var rowid int64
	if err := f.db.QueryRow(`SELECT COALESCE(MAX(rowid), 0) FROM audit_log`).Scan(&rowid); err != nil {
		t.Fatalf("reading audit_log: %v", err)
	}
	return rowid
}

// unmarkedAuditEntries returns the entries the steward wrote after rowid that
// do not carry the acting_without_role mark. The steward holds no role
// anywhere, so every act of theirs on a patch went through on the instance
// role and every entry about one should say so.
func unmarkedAuditEntries(t *testing.T, f *routeFixture, after int64, route string) []string {
	t.Helper()
	rows, err := f.db.Query(
		`SELECT a.action, a.metadata FROM audit_log a JOIN users u ON u.id = a.user_id
		 WHERE a.rowid > ? AND u.username = 'steward'`, after)
	if err != nil {
		t.Fatalf("reading audit_log: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var action, metadata string
		if err := rows.Scan(&action, &metadata); err != nil {
			t.Fatalf("scanning audit_log: %v", err)
		}
		if _, ok := auditAboutNoPatch[action]; ok {
			continue
		}
		var detail map[string]any
		if json.Unmarshal([]byte(metadata), &detail) != nil || detail["acting_without_role"] != true {
			out = append(out, route+": "+action+" "+metadata)
		}
	}
	return out
}

// stewardSession adds an instance admin who holds no role anywhere and
// returns their session token. The walk's outsider is the control: same
// standing in the patch, minus the instance role.
func stewardSession(t *testing.T, f *routeFixture) string {
	t.Helper()
	id := auth.NewUUIDv7()
	f.exec(t, `INSERT INTO users (id, username, display_name, role, email) VALUES (?, 'steward', 'Steward', 'admin', 'steward@example.com')`, id)
	token, err := auth.CreateSession(f.db, id, "127.0.0.1", "route-walk")
	if err != nil {
		t.Fatalf("create steward session: %v", err)
	}
	return token
}

// aboutAPatch reports whether a read is about a patch or something inside
// one, which is where two roleless callers should be told the same thing.
// Reads about the caller (their own account, sessions and exports) differ by
// identity and are not compared.
func aboutAPatch(pattern string) bool {
	if strings.Contains(pattern, "{slug}") {
		return true
	}
	for _, prefix := range []string{
		"/api/v1/proposals/", "/api/v1/governance/", "/api/v1/events/",
		"/api/v1/comments/", "/api/v1/notices/", "/api/v1/replies/",
	} {
		if strings.HasPrefix(pattern, prefix) {
			return true
		}
	}
	return false
}

// fixtureCanFill is routeFixture.value without the t.Fatalf, so the walk can
// name an unfillable read instead of stopping on it.
func fixtureCanFill(f *routeFixture, pattern string) bool {
	segs := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	for i, seg := range segs {
		if !strings.HasPrefix(seg, "{") {
			continue
		}
		name := strings.Trim(seg, "{}")
		if _, ok := f.ids[pattern+" {"+name+"}"]; ok {
			continue
		}
		key := name
		if name == "id" && i > 0 {
			key = segs[i-1]
		}
		if _, ok := f.ids[key]; !ok {
			return false
		}
	}
	return true
}
