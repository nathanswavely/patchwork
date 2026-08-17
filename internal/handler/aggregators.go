package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/eventsource"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// maxCrosswalkEntriesPerNode bounds how many aggregator names one patch
// may answer to. Deliberately roomier than maxSourcesPerNode: a venue
// legitimately arrives under several spellings (Binns Park reaches
// Lancaster's city calendar four ways), and each entry costs no fetch.
const maxCrosswalkEntriesPerNode = 10

// crosswalkAccess answers what this user may do with a patch's crosswalk
// (docs/adr/056). Three outcomes, because mapping is two different acts:
//
//   - manage: the patch's own admins, and the instance admin on an
//     unclaimed patch they hold in trust. Entries publish directly —
//     mapping your own patch is the standing consent.
//   - suggest: the instance admin on a claimed patch whose
//     accept_event_suggestions switch is on. That switch is the patch
//     saying "suggest to me", so entries route into its review queue and
//     nothing publishes without its own admins. It is not permission to
//     adopt a feed — ADR 031 keeps those apart — which is why the patch
//     can see every entry pointing at it and stop any of them.
//   - neither: instance role alone never writes onto an autonomous
//     patch's calendar.
func crosswalkAccess(db *database.DB, user *model.User, slug string) (nodeID string, manage, suggest bool) {
	var status string
	var acceptsSuggestions bool
	err := db.QueryRow(
		`SELECT id, status, accept_event_suggestions FROM nodes
		 WHERE slug = ? AND status IN ('active','unclaimed') AND removed_at IS NULL`,
		slug,
	).Scan(&nodeID, &status, &acceptsSuggestions)
	if err != nil {
		return "", false, false
	}
	if status == "unclaimed" {
		return nodeID, user.Role == "admin", false
	}
	if userHasNodeRole(db, user.ID, nodeID, "admin") {
		return nodeID, true, false
	}
	return nodeID, false, user.Role == "admin" && acceptsSuggestions
}

// crosswalkNodeAccess is the read/manage gate: listing entries and holds,
// and unmapping. It follows sourceNodeAccess rather than the creation
// gate above — an instance admin already manages any patch's event
// sources, and one who could set an entry up but never see or undo it
// would be a support problem, not a safeguard. The constraint ADR 056
// actually draws is on *publishing* onto an autonomous calendar, and
// that is enforced where entries are created.
func crosswalkNodeAccess(db *database.DB, user *model.User, slug string) (nodeID string, ok bool) {
	nodeID, manage, _ := crosswalkAccess(db, user, slug)
	if manage {
		return nodeID, true
	}
	return nodeID, nodeID != "" && user.Role == "admin"
}

func scanAggregators(db *database.DB) ([]model.Aggregator, error) {
	rows, err := db.Query(
		`SELECT a.id, a.name, a.type, a.url, a.added_by, a.paused, a.status,
		 a.last_fetch_at, a.last_success_at, a.last_error,
		 (SELECT COUNT(*) FROM aggregator_listings l WHERE l.aggregator_id = a.id),
		 (SELECT COUNT(*) FROM aggregator_listings l
		    JOIN event_sources es ON es.aggregator_id = a.id AND es.name_key = l.name_key
		    WHERE l.aggregator_id = a.id),
		 a.created_at, a.updated_at
		 FROM aggregators a ORDER BY a.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []model.Aggregator{}
	for rows.Next() {
		var a model.Aggregator
		if err := rows.Scan(&a.ID, &a.Name, &a.Type, &a.URL, &a.AddedBy, &a.Paused,
			&a.Status, &a.LastFetchAt, &a.LastSuccessAt, &a.LastError,
			&a.ListingCount, &a.MappedCount, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.UnroutedCount = a.ListingCount - a.MappedCount
		items = append(items, a)
	}
	return items, rows.Err()
}

// AdminListAggregators handles GET /api/v1/admin/aggregators.
func AdminListAggregators(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := scanAggregators(db)
		if err != nil {
			http.Error(w, `{"error":"failed to list aggregators"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	}
}

// AdminCreateAggregator handles POST /api/v1/admin/aggregators. Attaching
// one creates no event: nothing lands anywhere until a crosswalk entry
// addresses a name (docs/adr/056).
func AdminCreateAggregator(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"name and url are required"}`, http.StatusBadRequest)
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.URL = strings.TrimSpace(req.URL)
		if req.Name == "" || req.URL == "" {
			http.Error(w, `{"error":"name and url are required"}`, http.StatusBadRequest)
			return
		}
		u, err := url.Parse(req.URL)
		if err != nil || u.Host == "" {
			http.Error(w, `{"error":"url must be http(s)"}`, http.StatusBadRequest)
			return
		}
		if u.Scheme == "webcal" {
			// What calendar apps hand people; accept the intent.
			u.Scheme = "https"
			req.URL = u.String()
		} else if u.Scheme != "http" && u.Scheme != "https" {
			http.Error(w, `{"error":"url must be http(s)"}`, http.StatusBadRequest)
			return
		}

		id := auth.NewUUIDv7()
		if _, err := db.Exec(
			`INSERT INTO aggregators (id, name, type, url, added_by) VALUES (?, ?, 'ics', ?, ?)`,
			id, req.Name, req.URL, user.ID,
		); err != nil {
			http.Error(w, `{"error":"this feed is already attached"}`, http.StatusConflict)
			return
		}
		auth.LogAuditEvent(db, user.ID, "aggregator.create", "aggregator", id, `{"url":"`+req.URL+`"}`, clientIP(r))

		// First fetch in the background; the UI polls the list. Not the
		// request context — the fetch must outlive this response.
		go eventsource.SyncAggregator(context.Background(), db, pkgNotifier, id)

		respondWithAggregator(w, db, id, http.StatusCreated)
	}
}

// AdminUpdateAggregator handles PATCH /api/v1/admin/aggregators/{id} —
// pause and resume. Pausing stops the fetch, not the patches that
// already consented: routing continues off cached listings.
func AdminUpdateAggregator(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		id := r.PathValue("id")

		var req struct {
			Paused *bool   `json:"paused"`
			Name   *string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
			return
		}
		var exists int
		db.QueryRow(`SELECT COUNT(*) FROM aggregators WHERE id = ?`, id).Scan(&exists)
		if exists == 0 {
			http.Error(w, `{"error":"aggregator not found"}`, http.StatusNotFound)
			return
		}
		if req.Paused != nil {
			if _, err := db.Exec(
				`UPDATE aggregators SET paused = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?`,
				*req.Paused, id,
			); err != nil {
				http.Error(w, `{"error":"failed to update aggregator"}`, http.StatusInternalServerError)
				return
			}
			auth.LogAuditEvent(db, user.ID, "aggregator.pause", "aggregator", id, "{}", clientIP(r))
		}
		if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
			db.Exec(`UPDATE aggregators SET name = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?`,
				strings.TrimSpace(*req.Name), id)
		}
		respondWithAggregator(w, db, id, http.StatusOK)
	}
}

// AdminDeleteAggregator handles DELETE /api/v1/admin/aggregators/{id}.
// Every crosswalk entry is unmapped first, so no patch loses a calendar
// because the instance admin unplugged a feed (docs/adr/056).
func AdminDeleteAggregator(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		id := r.PathValue("id")

		var exists int
		db.QueryRow(`SELECT COUNT(*) FROM aggregators WHERE id = ?`, id).Scan(&exists)
		if exists == 0 {
			http.Error(w, `{"error":"aggregator not found"}`, http.StatusNotFound)
			return
		}
		if err := eventsource.RemoveAggregator(db, id); err != nil {
			http.Error(w, `{"error":"failed to remove aggregator"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "aggregator.delete", "aggregator", id, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// AdminSyncAggregator handles POST /api/v1/admin/aggregators/{id}/sync.
func AdminSyncAggregator(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		var lastFetch *string
		if err := db.QueryRow(`SELECT last_fetch_at FROM aggregators WHERE id = ?`, id).Scan(&lastFetch); err != nil {
			http.Error(w, `{"error":"aggregator not found"}`, http.StatusNotFound)
			return
		}
		if lastFetch != nil {
			if t, err := time.Parse("2006-01-02T15:04:05.000Z", *lastFetch); err == nil && time.Since(t) < time.Minute {
				http.Error(w, `{"error":"this aggregator just synced. Try again in a minute."}`, http.StatusTooManyRequests)
				return
			}
		}
		eventsource.SyncAggregator(r.Context(), db, pkgNotifier, id)
		respondWithAggregator(w, db, id, http.StatusOK)
	}
}

func respondWithAggregator(w http.ResponseWriter, db *database.DB, id string, code int) {
	items, err := scanAggregators(db)
	if err != nil {
		http.Error(w, `{"error":"failed to load aggregator"}`, http.StatusInternalServerError)
		return
	}
	for _, a := range items {
		if a.ID == id {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			json.NewEncoder(w).Encode(a)
			return
		}
	}
	http.Error(w, `{"error":"aggregator not found"}`, http.StatusNotFound)
}

// unroutedNames lists every name any aggregator carries that no
// crosswalk entry addresses, commonest first — the order that makes the
// work finite, since a handful of names covers most of a city calendar.
// Names are merged across aggregators rather than nested under them: the
// mapping is the work, and which feed a name arrived on is a detail of
// the row. Blank names (a listing with no location at all) are excluded —
// there is nothing there to map.
// ignoreFilter picks which side of the instance admin's ignore list to
// return: "unignored" is their working list, "ignored" the set-aside
// view, and "all" disregards the list entirely — which is what the patch
// picker wants, since ignoring is the instance admin's view of their own
// screen and must not reach a patch's own judgement (docs/adr/056).
func unroutedNames(db *database.DB, ignoreFilter string) ([]model.UnroutedName, error) {
	test := "AND ig.name_key IS NULL"
	switch ignoreFilter {
	case "ignored":
		test = "AND ig.name_key IS NOT NULL"
	case "all":
		test = ""
	}
	rows, err := db.Query(
		`SELECT l.aggregator_id, MIN(a.name), l.name_key, MIN(l.display_name), COUNT(*),
		 MIN(l.starts_at), GROUP_CONCAT(l.title, char(31))
		 FROM aggregator_listings l
		 JOIN aggregators a ON a.id = l.aggregator_id
		 LEFT JOIN event_sources es
		   ON es.aggregator_id = l.aggregator_id AND es.name_key = l.name_key
		 LEFT JOIN aggregator_ignored_names ig
		   ON ig.aggregator_id = l.aggregator_id AND ig.name_key = l.name_key
		 WHERE l.name_key != '' AND es.id IS NULL `+test+`
		 GROUP BY l.aggregator_id, l.name_key
		 ORDER BY COUNT(*) DESC, MIN(l.starts_at)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []model.UnroutedName{}
	for rows.Next() {
		var n model.UnroutedName
		var titles sql.NullString
		if err := rows.Scan(&n.AggregatorID, &n.AggregatorName, &n.NameKey, &n.DisplayName,
			&n.Count, &n.NextStartsAt, &titles); err != nil {
			return nil, err
		}
		n.SampleTitles = []string{}
		for i, t := range strings.Split(titles.String, "\x1f") {
			if i == 3 {
				break
			}
			n.SampleTitles = append(n.SampleTitles, t)
		}
		items = append(items, n)
	}
	return items, rows.Err()
}

// AdminListUnroutedNames handles GET /api/v1/admin/aggregator-names —
// the instance admin's working list, and the substance of that screen.
// ?ignored=true returns the set-aside names instead, so a judgement can
// be revisited without being remembered.
func AdminListUnroutedNames(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ignored := r.URL.Query().Get("ignored") == "true"
		filter := "unignored"
		if ignored {
			filter = "ignored"
		}
		items, err := unroutedNames(db, filter)
		if err != nil {
			http.Error(w, `{"error":"failed to list unrouted names"}`, http.StatusInternalServerError)
			return
		}
		for i := range items {
			items[i].Ignored = ignored
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	}
}

// AdminIgnoreName handles POST /api/v1/admin/aggregator-names/ignore and
// .../unignore (body: aggregator_id, name_key). A name is set aside, not
// deleted: the listings stay, and the judgement is reversible.
//
// The key travels in the body rather than the path because a name is
// free text — "3rd floor atrium", "Binns Park & Ewell Plaza" — and
// path-encoding it buys nothing.
func AdminIgnoreName(db *database.DB, ignore bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			AggregatorID string `json:"aggregator_id"`
			NameKey      string `json:"name_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
			req.AggregatorID == "" || req.NameKey == "" {
			http.Error(w, `{"error":"aggregator_id and name_key are required"}`, http.StatusBadRequest)
			return
		}

		var err error
		if ignore {
			_, err = db.Exec(
				`INSERT OR IGNORE INTO aggregator_ignored_names (aggregator_id, name_key, ignored_by)
				 VALUES (?, ?, ?)`, req.AggregatorID, req.NameKey, user.ID)
		} else {
			_, err = db.Exec(
				`DELETE FROM aggregator_ignored_names WHERE aggregator_id = ? AND name_key = ?`,
				req.AggregatorID, req.NameKey)
		}
		if err != nil {
			http.Error(w, `{"error":"failed to record that"}`, http.StatusInternalServerError)
			return
		}
		action := "aggregator_name.ignore"
		if !ignore {
			action = "aggregator_name.unignore"
		}
		auth.LogAuditEvent(db, user.ID, action, "aggregator", req.AggregatorID,
			`{"name_key":"`+req.NameKey+`"}`, clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// AdminListNameListings handles GET /api/v1/admin/aggregator-listings
// ?aggregator_id=&name_key= — the listings filed under one name, as the
// feed published them. Deciding whether "West Art" is an organization or
// a room means reading what it actually carries (docs/adr/056).
func AdminListNameListings(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		aggregatorID := r.URL.Query().Get("aggregator_id")
		nameKey := r.URL.Query().Get("name_key")
		if aggregatorID == "" || nameKey == "" {
			http.Error(w, `{"error":"aggregator_id and name_key are required"}`, http.StatusBadRequest)
			return
		}
		rows, err := db.Query(
			`SELECT uid, occurrence, title, description, location, starts_at, ends_at, url
			 FROM aggregator_listings
			 WHERE aggregator_id = ? AND name_key = ?
			 ORDER BY starts_at`, aggregatorID, nameKey)
		if err != nil {
			http.Error(w, `{"error":"failed to list listings"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		items := []model.AggregatorListing{}
		for rows.Next() {
			var l model.AggregatorListing
			if err := rows.Scan(&l.UID, &l.Occurrence, &l.Title, &l.Description,
				&l.Location, &l.StartsAt, &l.EndsAt, &l.URL); err != nil {
				http.Error(w, `{"error":"failed to list listings"}`, http.StatusInternalServerError)
				return
			}
			items = append(items, l)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	}
}

// ListAggregatorNames handles GET /api/v1/nodes/{slug}/aggregator-names:
// every unrouted name across every aggregator, so a patch's admins can
// find the ones that mean them. A name they cannot see is a name they
// cannot map, which is why this is the door rather than a global queue.
func ListAggregatorNames(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := crosswalkNodeAccess(db, user, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !ok {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}
		// Deliberately not filtered by the instance admin's ignore list:
		// whether this patch answers to a name is this patch's call
		// (docs/adr/056).
		items, err := unroutedNames(db, "all")
		if err != nil {
			http.Error(w, `{"error":"failed to list aggregator names"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	}
}

func scanCrosswalk(db *database.DB, nodeID string) ([]model.EventSource, error) {
	rows, err := db.Query(
		`SELECT es.id, es.node_id, es.type, es.url, es.added_by, es.status,
		 es.last_fetch_at, es.last_success_at, es.last_error,
		 (SELECT COUNT(*) FROM events e WHERE e.source_id = es.id
		    AND e.removed_at IS NULL AND e.status = 'active'),
		 es.created_at, es.updated_at, es.aggregator_id, es.name_key,
		 a.name, COALESCE((SELECT MIN(l.display_name) FROM aggregator_listings l
		   WHERE l.aggregator_id = es.aggregator_id AND l.name_key = es.name_key), es.name_key),
		 es.suggests, COALESCE(u.display_name, u.username, ''),
		 (SELECT COUNT(*) FROM events e WHERE e.source_id = es.id
		    AND e.removed_at IS NULL AND e.status = 'pending_review')
		 FROM event_sources es
		 JOIN aggregators a ON a.id = es.aggregator_id
		 LEFT JOIN users u ON u.id = es.added_by
		 WHERE es.node_id = ? ORDER BY es.created_at`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []model.EventSource{}
	for rows.Next() {
		var s model.EventSource
		if err := rows.Scan(&s.ID, &s.NodeID, &s.Type, &s.URL, &s.AddedBy, &s.Status,
			&s.LastFetchAt, &s.LastSuccessAt, &s.LastError, &s.EventCount,
			&s.CreatedAt, &s.UpdatedAt, &s.AggregatorID, &s.NameKey,
			&s.AggregatorName, &s.DisplayName, &s.Suggests, &s.AddedByName,
			&s.PendingCount); err != nil {
			return nil, err
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

// ListCrosswalk handles GET /api/v1/nodes/{slug}/crosswalk.
func ListCrosswalk(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := crosswalkNodeAccess(db, user, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !ok {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}
		items, err := scanCrosswalk(db, nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to list crosswalk entries"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	}
}

// CreateCrosswalkEntry handles POST /api/v1/nodes/{slug}/crosswalk.
// Mapping a name is a standing consent covering every event that name
// will ever carry, which is why it is the patch's own act on an active
// patch (docs/adr/056). The routing pass runs immediately and is silent:
// the entry has no last_success_at yet, so the back-fill announces
// nothing.
func CreateCrosswalkEntry(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, manage, suggest := crosswalkAccess(db, user, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !manage && !suggest {
			http.Error(w, `{"error":"this patch isn't accepting event suggestions. Only its own admins can map it."}`, http.StatusForbidden)
			return
		}

		var req struct {
			AggregatorID string `json:"aggregator_id"`
			NameKey      string `json:"name_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AggregatorID == "" || req.NameKey == "" {
			http.Error(w, `{"error":"aggregator_id and name_key are required"}`, http.StatusBadRequest)
			return
		}

		var aggURL string
		if err := db.QueryRow(`SELECT url FROM aggregators WHERE id = ?`, req.AggregatorID).Scan(&aggURL); err != nil {
			http.Error(w, `{"error":"aggregator not found"}`, http.StatusNotFound)
			return
		}

		var listings int
		db.QueryRow(`SELECT COUNT(*) FROM aggregator_listings WHERE aggregator_id = ? AND name_key = ?`,
			req.AggregatorID, req.NameKey).Scan(&listings)
		if listings == 0 {
			http.Error(w, `{"error":"this aggregator is not carrying that name"}`, http.StatusBadRequest)
			return
		}

		var count int
		db.QueryRow(`SELECT COUNT(*) FROM event_sources WHERE node_id = ? AND aggregator_id IS NOT NULL`, nodeID).Scan(&count)
		if count >= maxCrosswalkEntriesPerNode {
			http.Error(w, `{"error":"this patch already answers to the maximum number of aggregator names"}`, http.StatusConflict)
			return
		}

		id := auth.NewUUIDv7()
		// The aggregator's address, fragment-scoped to this name — see
		// migration 053: event_sources is unique on (node_id, url), and a
		// patch legitimately answers to several names on one aggregator.
		entryURL := aggURL + "#" + url.QueryEscape(req.NameKey)
		if _, err := db.Exec(
			`INSERT INTO event_sources (id, node_id, type, url, added_by, aggregator_id, name_key, suggests)
			 VALUES (?, ?, 'aggregator', ?, ?, ?, ?, ?)`,
			id, nodeID, entryURL, user.ID, req.AggregatorID, req.NameKey, suggest,
		); err != nil {
			http.Error(w, `{"error":"that name is already mapped"}`, http.StatusConflict)
			return
		}
		auth.LogAuditEvent(db, user.ID, "crosswalk.create", "event_source", id,
			`{"name_key":"`+req.NameKey+`"}`, clientIP(r))

		// Synchronous: the admin who just mapped a name wants to see the
		// events arrive, and they are already in the listings cache.
		eventsource.Sync(r.Context(), db, pkgNotifier, id)

		items, err := scanCrosswalk(db, nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to load crosswalk entry"}`, http.StatusInternalServerError)
			return
		}
		for _, s := range items {
			if s.ID == id {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(s)
				return
			}
		}
		http.Error(w, `{"error":"failed to create crosswalk entry"}`, http.StatusInternalServerError)
	}
}

// DeleteCrosswalkEntry handles DELETE /api/v1/nodes/{slug}/crosswalk/{id}.
// Unmapping detaches what it routed rather than deleting it: the patch
// consented to this name individually, so stopping the feed must not
// empty its calendar (docs/adr/056).
func DeleteCrosswalkEntry(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := crosswalkNodeAccess(db, user, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !ok {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}
		entryID := r.PathValue("id")

		var exists int
		db.QueryRow(`SELECT COUNT(*) FROM event_sources WHERE id = ? AND node_id = ? AND aggregator_id IS NOT NULL`,
			entryID, nodeID).Scan(&exists)
		if exists == 0 {
			http.Error(w, `{"error":"crosswalk entry not found"}`, http.StatusNotFound)
			return
		}
		if err := eventsource.Unmap(db, entryID); err != nil {
			http.Error(w, `{"error":"failed to unmap"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "crosswalk.delete", "event_source", entryID, "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// ListAggregatorHolds handles GET /api/v1/nodes/{slug}/aggregator-holds:
// listings withheld because the patch already has an event at that
// instant (docs/adr/056).
func ListAggregatorHolds(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := crosswalkNodeAccess(db, user, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !ok {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		rows, err := db.Query(
			`SELECT h.id, h.source_id, h.node_id, h.uid, h.occurrence, h.rival_event_id,
			 e.title, h.title, h.location, h.starts_at, a.name, h.created_at
			 FROM aggregator_holds h
			 JOIN events e ON e.id = h.rival_event_id
			 JOIN event_sources es ON es.id = h.source_id
			 JOIN aggregators a ON a.id = es.aggregator_id
			 WHERE h.node_id = ? ORDER BY h.starts_at`, nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to list holds"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		items := []model.AggregatorHold{}
		for rows.Next() {
			var h model.AggregatorHold
			if err := rows.Scan(&h.ID, &h.SourceID, &h.NodeID, &h.UID, &h.Occurrence,
				&h.RivalEventID, &h.RivalTitle, &h.Title, &h.Location, &h.StartsAt,
				&h.AggregatorName, &h.CreatedAt); err != nil {
				http.Error(w, `{"error":"failed to list holds"}`, http.StatusInternalServerError)
				return
			}
			items = append(items, h)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	}
}

// DecideAggregatorHold handles POST /api/v1/aggregator-holds/{id}/decide.
// "same" skip-lists the listing permanently, the way absorbing a
// duplicate does (docs/adr/032). "different" creates the event now, with
// its source identity, so the next routing pass matches it instead of
// asking again.
func DecideAggregatorHold(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		holdID := r.PathValue("id")

		var req struct {
			Decision string `json:"decision"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
			(req.Decision != "same" && req.Decision != "different") {
			http.Error(w, `{"error":"decision must be same or different"}`, http.StatusBadRequest)
			return
		}

		var sourceID, nodeID, uid, occurrence, aggregatorID string
		err := db.QueryRow(
			`SELECT h.source_id, h.node_id, h.uid, h.occurrence, es.aggregator_id
			 FROM aggregator_holds h JOIN event_sources es ON es.id = h.source_id
			 WHERE h.id = ?`, holdID,
		).Scan(&sourceID, &nodeID, &uid, &occurrence, &aggregatorID)
		if err != nil {
			http.Error(w, `{"error":"hold not found"}`, http.StatusNotFound)
			return
		}
		if user.Role != "admin" && !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}

		if req.Decision == "same" {
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO event_source_skips (source_id, uid, occurrence) VALUES (?, ?, ?)`,
				sourceID, uid, occurrence,
			); err != nil {
				http.Error(w, `{"error":"failed to record decision"}`, http.StatusInternalServerError)
				return
			}
		} else {
			var title, description, location, startsAt string
			var endsAt *string
			var lat, lon *float64
			err := db.QueryRow(
				`SELECT title, description, location, latitude, longitude, starts_at, ends_at
				 FROM aggregator_listings WHERE aggregator_id = ? AND uid = ? AND occurrence = ?`,
				aggregatorID, uid, occurrence,
			).Scan(&title, &description, &location, &lat, &lon, &startsAt, &endsAt)
			if err != nil {
				http.Error(w, `{"error":"this listing is no longer in the feed"}`, http.StatusConflict)
				return
			}
			id := auth.NewUUIDv7()
			apID := ap.EventAPID(ap.GetDomain(), id)
			if _, err := db.Exec(
				`INSERT INTO events (id, node_id, created_by, title, description, location,
				 latitude, longitude, starts_at, ends_at, recurrence, visibility, status,
				 ap_id, source_id, source_uid, source_occurrence)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 'public', 'active', ?, ?, ?, ?)`,
				id, nodeID, user.ID, title, description, location, lat, lon,
				startsAt, endsAt, apID, sourceID, uid, occurrence,
			); err != nil {
				http.Error(w, `{"error":"failed to create event"}`, http.StatusInternalServerError)
				return
			}
		}

		if _, err := db.Exec(`DELETE FROM aggregator_holds WHERE id = ?`, holdID); err != nil {
			http.Error(w, `{"error":"failed to clear hold"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, user.ID, "aggregator_hold.decide", "event_source", sourceID,
			`{"decision":"`+req.Decision+`"}`, clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
