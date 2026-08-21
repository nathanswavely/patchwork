package handler

import (
	"encoding/json"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// Programs credit a recurring title in an aggregator's feed to the patch
// that runs it (docs/adr/063). The feed says where an event is; nothing in
// it says who runs it, so a person recognizes that and a program records
// what they recognized.
//
// A program never routes. The owning patch keeps its events; what a
// program produces is offers, which a person turns into ordinary event
// links (docs/adr/032) that the owner confirms. That is the whole reason
// title matching is allowed here and forbidden in the crosswalk: a wrong
// program is declined, a wrong route lands silently.

// programNodeAccess resolves a slug and reports whether this user may
// credit programs to it. Standing over the *credited* patch and nothing
// else — instance admin, its own admins, or a trusted contributor while it
// is unclaimed. A program is a claim about who you are, so the venue
// whose event it is has no say here and needs none: its calendar is
// untouched until a link is proposed and its admins confirm.
func programNodeAccess(db *database.DB, user *model.User, slug string) (nodeID string, ok bool) {
	err := db.QueryRow(
		`SELECT id FROM nodes
		 WHERE slug = ? AND status IN ('active','unclaimed') AND removed_at IS NULL`,
		slug,
	).Scan(&nodeID)
	if err != nil {
		return "", false
	}
	return nodeID, userSpeaksForNode(db, user, nodeID)
}

const maxProgramsPerNode = 100

// programsFor returns every program credited to one patch, with the
// count of listings currently under its title and whether its name is
// routed at all. An unrouted program is inert rather than broken — with
// no events there is nothing to offer — and the row has to say so.
func programsFor(db *database.DB, nodeID string) ([]model.AggregatorProgram, error) {
	rows, err := db.Query(
		`SELECT p.id, p.aggregator_id, a.name, p.name_key, p.title_key,
		        p.display_title, p.node_id, n.name, n.slug, p.credited_by, p.created_at,
		        COALESCE((SELECT MIN(l.display_name) FROM aggregator_listings l
		                   WHERE l.aggregator_id = p.aggregator_id
		                     AND l.name_key = p.name_key), p.name_key),
		        (SELECT COUNT(*) FROM aggregator_listings l
		          WHERE l.aggregator_id = p.aggregator_id
		            AND l.name_key = p.name_key AND l.title_key = p.title_key),
		        EXISTS(SELECT 1 FROM event_sources es
		                WHERE es.aggregator_id = p.aggregator_id
		                  AND es.name_key = p.name_key)
		   FROM aggregator_programs p
		   JOIN aggregators a ON a.id = p.aggregator_id
		   JOIN nodes n ON n.id = p.node_id
		  WHERE p.node_id = ?
		  ORDER BY p.created_at DESC`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []model.AggregatorProgram{}
	for rows.Next() {
		var p model.AggregatorProgram
		if err := rows.Scan(&p.ID, &p.AggregatorID, &p.AggregatorName, &p.NameKey,
			&p.TitleKey, &p.DisplayTitle, &p.NodeID, &p.NodeName, &p.NodeSlug,
			&p.CreditedBy, &p.CreatedAt, &p.DisplayName, &p.ListingCount, &p.Routed); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

// offersFor computes the offers waiting for one patch. Derived, never
// stored (docs/adr/063): an offer is what remains after subtracting the
// links already made — pending ones included, since a proposal is already
// somebody's move — and the offers already declined. Nothing in the
// reconciler can write one, so a program cannot quietly become a route.
func offersFor(db *database.DB, nodeID string) ([]model.AggregatorOffer, error) {
	rows, err := db.Query(
		`SELECT p.id, p.display_title, e.id, e.title, e.starts_at, e.location,
		        e.node_id, n.name, n.slug
		   FROM aggregator_programs p
		   JOIN event_sources es
		     ON es.aggregator_id = p.aggregator_id AND es.name_key = p.name_key
		   JOIN events e
		     ON e.source_id = es.id AND e.removed_at IS NULL AND e.status = 'active'
		   JOIN aggregator_listings l
		     ON l.aggregator_id = p.aggregator_id
		    AND l.uid = e.source_uid
		    AND l.occurrence = e.source_occurrence
		    AND l.title_key = p.title_key
		   JOIN nodes n ON n.id = e.node_id AND n.removed_at IS NULL
		  WHERE p.node_id = ?
		    AND e.node_id != p.node_id
		    AND NOT EXISTS (SELECT 1 FROM event_links el
		                     WHERE el.event_id = e.id AND el.node_id = p.node_id)
		    AND NOT EXISTS (SELECT 1 FROM aggregator_offer_dismissals d
		                     WHERE d.program_id = p.id AND d.event_id = e.id)
		  ORDER BY e.starts_at`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []model.AggregatorOffer{}
	for rows.Next() {
		var o model.AggregatorOffer
		if err := rows.Scan(&o.ProgramID, &o.DisplayTitle, &o.EventID, &o.Title,
			&o.StartsAt, &o.Location, &o.OwnerNodeID, &o.OwnerName, &o.OwnerSlug); err != nil {
			return nil, err
		}
		items = append(items, o)
	}
	return items, rows.Err()
}

// AdminListPrograms handles GET /api/v1/admin/programs: every program
// on every aggregator, so the instance admin's list holds names and
// programs together (docs/adr/063). They are the same kind of object —
// a key grouping listings, awaiting one human judgement — and separating
// them would hide that a name has already been read.
func AdminListPrograms(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(
			`SELECT p.id, p.aggregator_id, a.name, p.name_key, p.title_key,
			        p.display_title, p.node_id, n.name, n.slug, p.credited_by, p.created_at,
			        COALESCE((SELECT MIN(l.display_name) FROM aggregator_listings l
			                   WHERE l.aggregator_id = p.aggregator_id
			                     AND l.name_key = p.name_key), p.name_key),
			        (SELECT COUNT(*) FROM aggregator_listings l
			          WHERE l.aggregator_id = p.aggregator_id
			            AND l.name_key = p.name_key AND l.title_key = p.title_key),
			        EXISTS(SELECT 1 FROM event_sources es
			                WHERE es.aggregator_id = p.aggregator_id
			                  AND es.name_key = p.name_key)
			   FROM aggregator_programs p
			   JOIN aggregators a ON a.id = p.aggregator_id
			   JOIN nodes n ON n.id = p.node_id AND n.removed_at IS NULL
			  ORDER BY p.created_at DESC`)
		if err != nil {
			http.Error(w, `{"error":"failed to list programs"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		items := []model.AggregatorProgram{}
		for rows.Next() {
			var p model.AggregatorProgram
			if err := rows.Scan(&p.ID, &p.AggregatorID, &p.AggregatorName, &p.NameKey,
				&p.TitleKey, &p.DisplayTitle, &p.NodeID, &p.NodeName, &p.NodeSlug,
				&p.CreditedBy, &p.CreatedAt, &p.DisplayName, &p.ListingCount, &p.Routed); err != nil {
				http.Error(w, `{"error":"failed to list programs"}`, http.StatusInternalServerError)
				return
			}
			items = append(items, p)
		}
		counts := offerCountsAll(db)
		for i := range items {
			items[i].OfferCount = counts[items[i].ID]
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	}
}

// offerCountsAll counts the offers outstanding on every program at once,
// so the instance admin's list can say what a credited program is still
// waiting on without asking per row.
func offerCountsAll(db *database.DB) map[string]int {
	counts := map[string]int{}
	rows, err := db.Query(
		`SELECT p.id, COUNT(*)
		   FROM aggregator_programs p
		   JOIN event_sources es
		     ON es.aggregator_id = p.aggregator_id AND es.name_key = p.name_key
		   JOIN events e
		     ON e.source_id = es.id AND e.removed_at IS NULL AND e.status = 'active'
		   JOIN aggregator_listings l
		     ON l.aggregator_id = p.aggregator_id
		    AND l.uid = e.source_uid
		    AND l.occurrence = e.source_occurrence
		    AND l.title_key = p.title_key
		  WHERE e.node_id != p.node_id
		    AND NOT EXISTS (SELECT 1 FROM event_links el
		                     WHERE el.event_id = e.id AND el.node_id = p.node_id)
		    AND NOT EXISTS (SELECT 1 FROM aggregator_offer_dismissals d
		                     WHERE d.program_id = p.id AND d.event_id = e.id)
		  GROUP BY p.id`)
	if err != nil {
		return counts
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err == nil {
			counts[id] = n
		}
	}
	return counts
}

// ListPrograms handles GET /api/v1/nodes/{slug}/programs.
func ListPrograms(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := programNodeAccess(db, user, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !ok {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}
		items, err := programsFor(db, nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to list programs"}`, http.StatusInternalServerError)
			return
		}
		offers, err := offersFor(db, nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to list offers"}`, http.StatusInternalServerError)
			return
		}
		for i := range items {
			for _, o := range offers {
				if o.ProgramID == items[i].ID {
					items[i].OfferCount++
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": items, "offers": offers})
	}
}

// CreateProgram handles POST /api/v1/nodes/{slug}/programs — crediting a
// recurring title to this patch. Body: aggregator_id, name_key, title_key.
//
// backfilled_at is stamped here because offers are derived: everything the
// feed already carries becomes an offer the instant the row lands, and
// announcing that would be six notifications for a decision just made.
// Only listings arriving afterward announce (docs/adr/056's rule for
// crosswalk entries, applied to programs).
func CreateProgram(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := programNodeAccess(db, user, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !ok {
			http.Error(w, `{"error":"only this patch can be credited with a program by you"}`, http.StatusForbidden)
			return
		}

		var req struct {
			AggregatorID string `json:"aggregator_id"`
			NameKey      string `json:"name_key"`
			TitleKey     string `json:"title_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
			req.AggregatorID == "" || req.NameKey == "" || req.TitleKey == "" {
			http.Error(w, `{"error":"aggregator_id, name_key and title_key are required"}`, http.StatusBadRequest)
			return
		}

		// The display title comes from what the feed said, not from
		// anything the client sent: a program names itself.
		var displayTitle string
		err := db.QueryRow(
			`SELECT MIN(title) FROM aggregator_listings
			  WHERE aggregator_id = ? AND name_key = ? AND title_key = ?`,
			req.AggregatorID, req.NameKey, req.TitleKey).Scan(&displayTitle)
		if err != nil || displayTitle == "" {
			http.Error(w, `{"error":"this aggregator is not carrying that title under that name"}`, http.StatusBadRequest)
			return
		}

		var count int
		db.QueryRow(`SELECT COUNT(*) FROM aggregator_programs WHERE node_id = ?`, nodeID).Scan(&count)
		if count >= maxProgramsPerNode {
			http.Error(w, `{"error":"this patch is credited with the maximum number of programs"}`, http.StatusConflict)
			return
		}

		id := auth.NewUUIDv7()
		if _, err := db.Exec(
			`INSERT INTO aggregator_programs
			   (id, aggregator_id, name_key, title_key, display_title, node_id,
			    credited_by, backfilled_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
			id, req.AggregatorID, req.NameKey, req.TitleKey, displayTitle, nodeID, user.ID,
		); err != nil {
			http.Error(w, `{"error":"that program is already credited to this patch"}`, http.StatusConflict)
			return
		}
		auth.LogAuditEvent(db, user.ID, "program.create", "aggregator_program", id,
			`{"title_key":`+jsonString(req.TitleKey)+`}`, clientIP(r))

		items, err := programsFor(db, nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to load program"}`, http.StatusInternalServerError)
			return
		}
		for _, p := range items {
			if p.ID == id {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(p)
				return
			}
		}
		http.Error(w, `{"error":"failed to create program"}`, http.StatusInternalServerError)
	}
}

// DeleteProgram handles DELETE /api/v1/nodes/{slug}/programs/{id}.
//
// Uncrediting stops future offers and keeps every link already confirmed:
// those are the patch's events now, agreed to by both sides, and a
// program was only ever how they were noticed. Same instinct as ADR
// 056's rule that unmapping detaches rather than deletes.
func DeleteProgram(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := programNodeAccess(db, user, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !ok {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}
		res, err := db.Exec(`DELETE FROM aggregator_programs WHERE id = ? AND node_id = ?`,
			r.PathValue("id"), nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to remove program"}`, http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			http.Error(w, `{"error":"program not found"}`, http.StatusNotFound)
			return
		}
		auth.LogAuditEvent(db, user.ID, "program.delete", "aggregator_program",
			r.PathValue("id"), "{}", clientIP(r))
		w.WriteHeader(http.StatusNoContent)
	}
}

// DismissOffer handles POST /api/v1/nodes/{slug}/offers/dismiss.
// Body: program_id, event_id.
//
// The only stored part of an offer. Without it the next sync re-offers the
// same event and the same refusal is owed every hour — the live defect ADR
// 056 found in its own review path.
func DismissOffer(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID, ok := programNodeAccess(db, user, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !ok {
			http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
			return
		}
		var req struct {
			ProgramID string `json:"program_id"`
			EventID   string `json:"event_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
			req.ProgramID == "" || req.EventID == "" {
			http.Error(w, `{"error":"program_id and event_id are required"}`, http.StatusBadRequest)
			return
		}
		// The program must be this patch's, so one patch cannot silence
		// another's offers.
		var owned int
		db.QueryRow(`SELECT COUNT(*) FROM aggregator_programs WHERE id = ? AND node_id = ?`,
			req.ProgramID, nodeID).Scan(&owned)
		if owned == 0 {
			http.Error(w, `{"error":"program not found"}`, http.StatusNotFound)
			return
		}
		if _, err := db.Exec(
			`INSERT OR IGNORE INTO aggregator_offer_dismissals
			   (program_id, event_id, dismissed_by) VALUES (?, ?, ?)`,
			req.ProgramID, req.EventID, user.ID); err != nil {
			http.Error(w, `{"error":"failed to dismiss offer"}`, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// jsonString quotes a value for the small JSON blobs the audit log takes.
func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
