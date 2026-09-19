package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// The admin panel's Overview (CONTEXT.md, "Overview"): what is waiting on
// the instance admin and what is unattended, and nothing else. It replaced
// a page of size and growth counts, which told a steward nothing they could
// act on and taught them to stop opening the page.
//
// Something is here only when the next act is the instance admin's. Each
// inbox queue counts exactly what its own tab lists, so the number on
// Overview is never one the tab cannot show; the predicates are repeated
// here rather than shared because the tabs page and enrich theirs and this
// page wants a count and an oldest date. Keep them in step.
//
// What is unattended follows custody (docs/adr/115): the instance's own
// machinery, and patches held in custody. A fault on a patch that has admins
// of its own is theirs, and is deliberately not surfaced here.

// overviewQueue is one inbox line: a decision queue, how many are waiting,
// and when the oldest arrived. The age is the number that matters on an
// inbox — three claims from yesterday is a normal week, one from three
// weeks ago is a person who thinks the quilt is dead.
type overviewQueue struct {
	Queue    string `json:"queue"`
	Count    int    `json:"count"`
	OldestAt string `json:"oldest_at,omitempty"`
}

// overviewUnrouted is the lower line under the inbox: routing work nobody is
// waiting on. A freshly attached aggregator can publish forty names in a
// day, most of them rooms and street corners, so this is a count and not a
// queue of people.
type overviewUnrouted struct {
	Count       int `json:"count"`
	Aggregators int `json:"aggregators"`
}

// overviewAdminless is a claimed patch with zero active admins: held in
// custody only to hand back (docs/adr/115), and the one care item that is a
// decision rather than a repair. MemberCount says whether there is anyone to
// hand it to.
type overviewAdminless struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
}

type overviewFailingAggregator struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	LastError     string `json:"last_error"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
}

// overviewFailingSource is an event source on an unclaimed patch whose last
// fetch failed. Unclaimed calendars are the instance admin's to keep alive
// (docs/adr/026, 057); the same failure on an active patch is its admins'.
type overviewFailingSource struct {
	ID            string `json:"id"`
	NodeSlug      string `json:"node_slug"`
	NodeName      string `json:"node_name"`
	URL           string `json:"url"`
	LastError     string `json:"last_error"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
}

type overviewCare struct {
	AdminlessPatches       []overviewAdminless         `json:"adminless_patches"`
	FailingAggregators     []overviewFailingAggregator `json:"failing_aggregators"`
	FailingUnclaimedSources []overviewFailingSource    `json:"failing_unclaimed_sources"`
	// Outbound activities the delivery worker has given up on. Only
	// meaningful when federation is on; zero and unremarkable otherwise.
	FailedDeliveries int `json:"failed_deliveries"`
}

type overviewResponse struct {
	Inbox         []overviewQueue  `json:"inbox"`
	UnroutedNames overviewUnrouted `json:"unrouted_names"`
	Care          overviewCare     `json:"care"`
	// Standing conditions, stated plainly and never dismissed: whether
	// magic links can be sent at all, whether federation is on (so the page
	// knows whether a delivery count means anything), and whether the
	// caller holds a passkey, without which every step-up-gated verb in the
	// panel answers 403 (docs/adr/017).
	SMTPConfigured    bool `json:"smtp_configured"`
	FederationEnabled bool `json:"federation_enabled"`
	HasPasskey        bool `json:"has_passkey"`
}

// countAndOldest runs a query expected to return COUNT(*) and MIN(created_at)
// over one queue's pending rows. MIN over zero rows is NULL, so the date
// scans through a NullString.
func countAndOldest(db *database.DB, query string) (int, string) {
	var count int
	var oldest sql.NullString
	if err := db.QueryRow(query).Scan(&count, &oldest); err != nil {
		return 0, ""
	}
	return count, oldest.String
}

// AdminOverview handles GET /api/v1/admin/overview.
func AdminOverview(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		resp := overviewResponse{
			SMTPConfigured:    cfg != nil && cfg.SMTP.Configured(),
			FederationEnabled: cfg != nil && cfg.Federation.Enabled,
			HasPasskey:        auth.HasCredential(db, user.ID),
		}

		// The five decision queues, in the order the tabs run. Each predicate
		// mirrors its tab's listing: reports routed to a patch are that
		// patch's (docs/adr/081); event submissions are counted only on
		// unclaimed patches (docs/adr/026); an approved claim awaiting its
		// claimant's setup is not counted, because it waits on them.
		queues := []struct {
			name  string
			query string
		}{
			{"reports", `SELECT COUNT(*), MIN(created_at) FROM content_reports
				WHERE status = 'pending' AND node_id IS NULL`},
			{"submissions", `SELECT COUNT(*), MIN(created_at) FROM nodes
				WHERE status = 'pending_review' AND removed_at IS NULL`},
			{"event_submissions", `SELECT COUNT(*), MIN(e.created_at) FROM events e
				JOIN nodes n ON n.id = e.node_id
				WHERE e.status = 'pending_review' AND e.removed_at IS NULL AND n.status = 'unclaimed'`},
			{"claims", `SELECT COUNT(*), MIN(created_at) FROM claim_requests
				WHERE status = 'pending'`},
			{"tag_suggestions", `SELECT COUNT(*), MIN(created_at) FROM tags
				WHERE status = 'pending'`},
		}
		resp.Inbox = make([]overviewQueue, 0, len(queues))
		for _, q := range queues {
			count, oldest := countAndOldest(db, q.query)
			resp.Inbox = append(resp.Inbox, overviewQueue{Queue: q.name, Count: count, OldestAt: oldest})
		}

		// Unrouted names, through the same query the Aggregators tab lists
		// them with, so "ignored" means the same thing on both pages.
		if names, err := unroutedNames(db, "unignored"); err == nil {
			seen := map[string]bool{}
			for _, n := range names {
				seen[n.AggregatorID] = true
			}
			resp.UnroutedNames = overviewUnrouted{Count: len(names), Aggregators: len(seen)}
		}

		resp.Care = overviewCare{
			AdminlessPatches:        []overviewAdminless{},
			FailingAggregators:      []overviewFailingAggregator{},
			FailingUnclaimedSources: []overviewFailingSource{},
		}

		// A claimed patch with zero active admins. The membership test is the
		// one memberships.go runs before letting an instance admin promote
		// there, so what this lists is exactly what that verb will accept.
		rows, err := db.Query(`
			SELECT n.slug, n.name,
			       (SELECT COUNT(*) FROM memberships m
			        WHERE m.node_id = n.id AND m.status = 'active' AND m.role IN ('member', 'admin'))
			FROM nodes n
			WHERE n.status = 'active' AND n.removed_at IS NULL
			  AND NOT EXISTS (SELECT 1 FROM memberships m
			                  WHERE m.node_id = n.id AND m.role = 'admin' AND m.status = 'active')
			ORDER BY n.name`)
		if err == nil {
			for rows.Next() {
				var p overviewAdminless
				if rows.Scan(&p.Slug, &p.Name, &p.MemberCount) == nil {
					resp.Care.AdminlessPatches = append(resp.Care.AdminlessPatches, p)
				}
			}
			rows.Close()
		}

		// A paused aggregator never fetches, so its last error is history
		// rather than a fault.
		rows, err = db.Query(`
			SELECT id, name, COALESCE(last_error, ''), COALESCE(last_success_at, '')
			FROM aggregators WHERE status = 'error' AND paused = 0 ORDER BY name`)
		if err == nil {
			for rows.Next() {
				var a overviewFailingAggregator
				if rows.Scan(&a.ID, &a.Name, &a.LastError, &a.LastSuccessAt) == nil {
					resp.Care.FailingAggregators = append(resp.Care.FailingAggregators, a)
				}
			}
			rows.Close()
		}

		rows, err = db.Query(`
			SELECT es.id, n.slug, n.name, es.url, COALESCE(es.last_error, ''), COALESCE(es.last_success_at, '')
			FROM event_sources es JOIN nodes n ON n.id = es.node_id
			WHERE es.status = 'error' AND n.status = 'unclaimed' AND n.removed_at IS NULL
			ORDER BY n.name, es.url`)
		if err == nil {
			for rows.Next() {
				var s overviewFailingSource
				if rows.Scan(&s.ID, &s.NodeSlug, &s.NodeName, &s.URL, &s.LastError, &s.LastSuccessAt) == nil {
					resp.Care.FailingUnclaimedSources = append(resp.Care.FailingUnclaimedSources, s)
				}
			}
			rows.Close()
		}

		if resp.FederationEnabled {
			db.QueryRow(`SELECT COUNT(*) FROM ap_outbox_queue WHERE status = 'failed'`).
				Scan(&resp.Care.FailedDeliveries)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
