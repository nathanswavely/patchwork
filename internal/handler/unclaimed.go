package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/clock"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/eventsource"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// suggestedFeedProbeTimeout bounds the fetch a suggestion's feed gets. The
// person is waiting on the form, so the answer has to arrive or stop.
const suggestedFeedProbeTimeout = 10 * time.Second

// normalizeSuggestedFeedURL accepts what a calendar app hands people. Same
// two shapes CreateEventSource takes for an ics source: http(s) as given,
// and webcal rewritten to https, because teaching everyone to rewrite the
// scheme themselves is not a design. An atproto handle is deliberately not
// accepted here — that is a source a patch's keeper attaches, not a URL a
// stranger types into a suggestion.
func normalizeSuggestedFeedURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", false
	}
	switch u.Scheme {
	case "http", "https":
		return raw, true
	case "webcal":
		u.Scheme = "https"
		return u.String(), true
	}
	return "", false
}

// attachSuggestedFeed turns a suggestion's stored feed URL into a real
// event_sources row on the patch that just published
// (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar.md,
// decision 6). Type 'ics' exactly as CreateEventSource sets it: detection at
// sync time turns it into jsonld or squarespace if that is what the address
// turns out to be, and persists the answer.
//
// addedBy is whoever held standing at the moment the listing published — the
// suggester where the grant reached them, the reviewing admin otherwise. The
// vouch is theirs.
func attachSuggestedFeed(db *database.DB, r *http.Request, nodeID, feedURL, addedBy string) bool {
	sourceID := auth.NewUUIDv7()
	if _, err := db.Exec(
		`INSERT INTO event_sources (id, node_id, type, url, added_by) VALUES (?, ?, 'ics', ?, ?)`,
		sourceID, nodeID, feedURL, addedBy,
	); err != nil {
		return false
	}
	auth.LogAuditEvent(db, addedBy, "event_source.create", "event_source", sourceID,
		`{"url":`+jsonString(feedURL)+`}`, clientIP(r))
	// Not the request context: the sync must outlive this response.
	go eventsource.Sync(context.Background(), db, pkgNotifier, sourceID)
	return true
}

// SubmitPatch handles POST /api/v1/submissions.
// Community members submit places/orgs to add to the quilt.
func SubmitPatch(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !cfg.Submissions.Enabled {
			http.Error(w, `{"error":"community submissions are disabled on this instance"}`, http.StatusForbidden)
			return
		}
		user := middleware.UserFromContext(r.Context())

		var req struct {
			Name        string           `json:"name"`
			Description string           `json:"description"`
			Website     string           `json:"website"`
			Links       []model.NodeLink `json:"links"`
			Address     string           `json:"address"`
			Latitude    *float64         `json:"latitude"`
			Longitude   *float64         `json:"longitude"`
			Tags        []string         `json:"tags"`
			// The calendar this place already publishes (docs/adr/2026-09-18-
			// trust-has-a-scope-and-a-suggestion-carries-its-calendar.md,
			// decision 6). Optional, and read here rather than at approval so
			// an address Patchwork cannot make sense of is refused while the
			// person who typed it is still looking at the screen.
			FeedURL string `json:"feed_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
			return
		}

		// Tags are optional, but any offered must come from the curated
		// vocabulary — validated the same way the admin create path does
		// (docs/adr/021); unknown tags are rejected rather than dropped, so
		// a submitter finds out immediately instead of the tag silently
		// vanishing.
		tagIDs, unknownTag := resolveTagIDs(db, req.Tags)
		if unknownTag != "" {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, "unknown tag: "+unknownTag), http.StatusBadRequest)
			return
		}

		// Check for duplicate by slug.
		slug := uniqueSlug(db, generateSlug(req.Name))
		baseSlug := generateSlug(req.Name)
		var existingSlug string
		db.QueryRow("SELECT slug FROM nodes WHERE slug = ? AND status IN ('active','unclaimed','pending_review')", baseSlug).Scan(&existingSlug)
		if existingSlug != "" {
			http.Error(w, fmt.Sprintf(`{"error":"a patch with a similar name already exists","existing_slug":"%s"}`, existingSlug), http.StatusConflict)
			return
		}

		// The feed is fetched last of the validations, because it is the only
		// one that costs a round trip: a name collision or an unknown tag
		// should answer without going out to somebody's calendar first.
		feedURL, feedCount := "", 0
		if strings.TrimSpace(req.FeedURL) != "" {
			normalized, ok := normalizeSuggestedFeedURL(req.FeedURL)
			if !ok {
				http.Error(w, `{"error":"feed url must be http(s)"}`, http.StatusBadRequest)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), suggestedFeedProbeTimeout)
			n, perr := eventsource.Probe(ctx, normalized)
			cancel()
			if perr != nil {
				http.Error(w, `{"error":"could not read a calendar at that address"}`, http.StatusBadRequest)
				return
			}
			feedURL, feedCount = normalized, n
		}

		// A quilt-wide trusted contributor's suggestion lands as a listing at
		// once (decision 4). ADR 026's own reasoning for the grant is that it
		// is the instance admin delegating their own queue, and the listing
		// queue is that same queue: nothing about a listing is owed to patch
		// admins, because there are none yet. The per-patch grant does not
		// reach this — a new suggestion is not the patch it names — which is
		// why this reads the flag and not userTrustedOn.
		trusted := user.TrustedContributor
		status := "pending_review"
		if cfg.Submissions.AutoApprove || trusted {
			status = "unclaimed"
		}

		// A listing that publishes now attaches its feed now; one that waits
		// for review parks the URL and the count on the row, for the admin to
		// read and decide. Nothing syncs a feed onto a patch nobody approved.
		//
		// The condition is "did this publish", not "is this person trusted",
		// because an instance running submissions.auto_approve has already
		// answered the approval question for every submission it takes. Were
		// it the narrower test, a feed suggested on such an instance would
		// park on a published listing and wait for a review that never comes:
		// a column nothing would ever read again.
		attachNow := status == "unclaimed" && feedURL != ""
		storedFeedURL, storedFeedCount := feedURL, feedCount
		if attachNow {
			storedFeedURL, storedFeedCount = "", 0
		}

		id := auth.NewUUIDv7()
		linksStr := "[]"
		if len(req.Links) > 0 {
			lb, _ := json.Marshal(req.Links)
			linksStr = string(lb)
		}

		apID := ap.NodeAPID(ap.GetDomain(), id)
		now := clock.Now()

		// The verification domain is a trust anchor (docs/adr/030): only a
		// trusted contributor's website auto-derives one. Ordinary community
		// submissions get none — the admin sets it at approval time.
		verificationDomain := ""
		if user.TrustedContributor {
			verificationDomain = deriveVerificationDomain(req.Website)
		}

		// The membership policy a listing is born with decides nothing while
		// it is a listing — an unclaimed patch takes followers only
		// (memberships.go) — so nobody is being asked this question here and
		// nobody could answer it: the submitter is not the patch. All this
		// value does is sit in the row until a claim activates it, and setup
		// asks the claimant and overwrites it (claims.go). It is invite_only
		// so that the one thing it can still do — decide the door for a path
		// that forgets to ask — fails closed. It was 'open', and a claimed
		// patch went live admitting anyone, chosen by nobody. Same at the
		// two admin-side listing inserts below.
		_, err := db.Exec(
			`INSERT INTO nodes (id, owner_id, name, slug, description, latitude, longitude, address, website, links, visibility, membership_policy, status, submitted_by, submission_source, verification_domain, ap_id, created_at, updated_at, follower_permissions, suggested_feed_url, suggested_feed_count)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'public', 'invite_only', ?, ?, 'community', ?, ?, ?, ?, '{}', ?, ?)`,
			id, model.SystemUserID, req.Name, slug, req.Description, req.Latitude, req.Longitude, req.Address, req.Website, linksStr, status, user.ID, verificationDomain, apID, now, now, storedFeedURL, storedFeedCount,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to create submission"}`, http.StatusInternalServerError)
			return
		}

		// Array order is the stored (priority) order — same as every other
		// tag-writing path (docs/adr/021).
		if len(tagIDs) > 0 {
			setNodeTags(db, id, tagIDs)
		}

		// The details argument is the metadata one and the last is the IP —
		// this call had them the other way round, so every node.submit entry
		// ever written filed its status as an address and the address as its
		// metadata.
		details := fmt.Sprintf(`{"status":%q}`, status)
		if trusted {
			details = fmt.Sprintf(`{"status":%q,"trusted":true}`, status)
		}
		auth.LogAuditEvent(db, user.ID, "node.submit", "node", id, details, clientIP(r))

		if attachNow {
			attachSuggestedFeed(db, r, id, feedURL, user.ID)
		}

		// Notify site admins about the new submission — unless there is
		// nothing to review, because a trusted contributor's listing is
		// already on the quilt. A queue notification pointing at an empty
		// queue is worse than silence.
		if !trusted {
			notify(notifications.Event{
				Type:     notifications.AdminSubmission,
				ActorID:  user.ID,
				EntityID: id,
				Title:    "New patch submission: " + req.Name,
				Link:     "/admin/submissions",
			})
		}

		resp := map[string]interface{}{
			"status": status,
		}
		if status == "unclaimed" {
			resp["node"] = map[string]string{"id": id, "slug": slug, "name": req.Name}
		} else {
			resp["message"] = "Submission sent for review"
			resp["id"] = id
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}
}

// notifySubmissionApproved tells the suggester their listing is on the quilt
// and what they now hold
// (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar.md,
// decision 8).
//
// It states the standing rather than congratulating, because the standing is
// the actionable part: a person who suggested a venue in order to put its
// events up needs to know whether those events will queue. Approval used to
// notify site admins and nobody else, so the one person waiting on an answer
// was the one person not told.
func notifySubmissionApproved(submittedBy *string, reviewerID, nodeID, slug, name string, holdsTrust, feedAttached bool) {
	if submittedBy == nil {
		return
	}
	body := "Its events go through review."
	if holdsTrust {
		body = "You can add its events without review until its owner claims it."
	}
	if feedAttached {
		body += " Its feed is attached."
	}
	notify(notifications.Event{
		Type:     notifications.SubmissionApproved,
		NodeID:   nodeID,
		NodeSlug: slug,
		NodeName: name,
		ActorID:  reviewerID,
		TargetID: *submittedBy,
		EntityID: nodeID,
		Title:    name + " is on the quilt",
		Body:     body,
		Link:     weblink.Patch(slug),
	})
}

// CreateUnclaimedPatch handles POST /api/v1/admin/unclaimed.
// Admin directly creates an unclaimed patch.
func CreateUnclaimedPatch(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			Name               string           `json:"name"`
			Description        string           `json:"description"`
			Website            string           `json:"website"`
			VerificationDomain string           `json:"verification_domain"`
			Links              []model.NodeLink `json:"links"`
			Address            string           `json:"address"`
			Latitude           *float64         `json:"latitude"`
			Longitude          *float64         `json:"longitude"`
			Tags               []string         `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
			return
		}

		// Explicit domain wins; otherwise derive from the website the admin
		// supplied (shared platforms yield none — docs/adr/030).
		verificationDomain := deriveVerificationDomain(req.Website)
		if req.VerificationDomain != "" {
			d, err := validateExplicitDomain(req.VerificationDomain)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
				return
			}
			verificationDomain = d
		}

		id := auth.NewUUIDv7()
		slug := uniqueSlug(db, generateSlug(req.Name))

		linksStr := "[]"
		if len(req.Links) > 0 {
			lb, _ := json.Marshal(req.Links)
			linksStr = string(lb)
		}

		apID := ap.NodeAPID(ap.GetDomain(), id)
		now := clock.Now()

		_, err := db.Exec(
			`INSERT INTO nodes (id, owner_id, name, slug, description, latitude, longitude, address, website, links, visibility, membership_policy, status, submitted_by, submission_source, verification_domain, ap_id, created_at, updated_at, follower_permissions)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'public', 'invite_only', 'unclaimed', ?, 'admin', ?, ?, ?, ?, '{}')`,
			id, model.SystemUserID, req.Name, slug, req.Description, req.Latitude, req.Longitude, req.Address, req.Website, linksStr, user.ID, verificationDomain, apID, now, now,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to create unclaimed patch"}`, http.StatusInternalServerError)
			return
		}

		// Assign tags if provided.
		for _, tagName := range req.Tags {
			var tagID string
			err := db.QueryRow("SELECT id FROM tags WHERE name = ?", tagName).Scan(&tagID)
			if err != nil {
				continue
			}
			db.Exec("INSERT OR IGNORE INTO node_tags (node_id, tag_id) VALUES (?, ?)", id, tagID)
		}

		auth.LogAuditEvent(db, user.ID, "node.create_unclaimed", "node", id, r.RemoteAddr, "")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": id, "slug": slug, "name": req.Name, "status": "unclaimed"})
	}
}

// BulkCreateUnclaimed handles POST /api/v1/admin/unclaimed/bulk.
func BulkCreateUnclaimed(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		var req struct {
			Nodes []struct {
				Name               string           `json:"name"`
				Description        string           `json:"description"`
				Website            string           `json:"website"`
				VerificationDomain string           `json:"verification_domain"`
				Links              []model.NodeLink `json:"links"`
				Address            string           `json:"address"`
				Latitude           *float64         `json:"latitude"`
				Longitude          *float64         `json:"longitude"`
				Tags               []string         `json:"tags"`
			} `json:"nodes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		created := 0
		var errors []string
		now := clock.Now()

		for i, n := range req.Nodes {
			if n.Name == "" {
				errors = append(errors, fmt.Sprintf("item %d: name is required", i))
				continue
			}

			id := auth.NewUUIDv7()
			slug := uniqueSlug(db, generateSlug(n.Name))

			linksStr := "[]"
			if len(n.Links) > 0 {
				lb, _ := json.Marshal(n.Links)
				linksStr = string(lb)
			}

			verificationDomain := deriveVerificationDomain(n.Website)
			if n.VerificationDomain != "" {
				d, err := validateExplicitDomain(n.VerificationDomain)
				if err != nil {
					errors = append(errors, fmt.Sprintf("item %d (%s): %v", i, n.Name, err))
					continue
				}
				verificationDomain = d
			}

			apID := ap.NodeAPID(ap.GetDomain(), id)
			_, err := db.Exec(
				`INSERT INTO nodes (id, owner_id, name, slug, description, latitude, longitude, address, website, links, visibility, membership_policy, status, submitted_by, submission_source, verification_domain, ap_id, created_at, updated_at, follower_permissions)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'public', 'invite_only', 'unclaimed', ?, 'admin', ?, ?, ?, ?, '{}')`,
				id, model.SystemUserID, n.Name, slug, n.Description, n.Latitude, n.Longitude, n.Address, n.Website, linksStr, user.ID, verificationDomain, apID, now, now,
			)
			if err != nil {
				errors = append(errors, fmt.Sprintf("item %d (%s): %v", i, n.Name, err))
				continue
			}

			for _, tagName := range n.Tags {
				var tagID string
				if db.QueryRow("SELECT id FROM tags WHERE name = ?", tagName).Scan(&tagID) == nil {
					db.Exec("INSERT OR IGNORE INTO node_tags (node_id, tag_id) VALUES (?, ?)", id, tagID)
				}
			}
			created++
		}

		auth.LogAuditEvent(db, user.ID, "node.bulk_create_unclaimed", "", "", r.RemoteAddr, fmt.Sprintf(`{"created":%d}`, created))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"created": created,
			"errors":  errors,
		})
	}
}

// ListSubmissions handles GET /api/v1/admin/submissions.
// Returns nodes with status='pending_review' for admin moderation.
func ListSubmissions(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		after, limit := parsePaginationParams(r)

		query := `SELECT n.id, n.name, n.slug, n.description, n.website, COALESCE(n.links,'[]'), n.address, n.submitted_by, n.created_at,
			` + usernameExpr("u") + `, ` + displayNameExpr("u") + `,
			COALESCE(n.suggested_feed_url,''), COALESCE(n.suggested_feed_count,0)
			FROM nodes n LEFT JOIN users u ON n.submitted_by = u.id
			WHERE n.status = 'pending_review' AND n.removed_at IS NULL`
		args := []interface{}{}

		if sortKey, id, ok := decodeCursor(after); after != "" && ok {
			query += " AND " + keysetCondition("n.created_at", "n.id", true)
			args = append(args, sortKey, sortKey, id)
		}
		query += " ORDER BY n.created_at DESC, n.id DESC LIMIT ?"
		args = append(args, limit+1)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to query submissions"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type submission struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			Slug            string `json:"slug"`
			// What the website would derive as a trust anchor — shown to the
			// reviewing admin as a suggestion, never applied automatically.
			SuggestedVerificationDomain string `json:"suggested_verification_domain"`
			Description     string `json:"description"`
			Website         string `json:"website"`
			Links           json.RawMessage `json:"links"`
			Address         string `json:"address"`
			SubmittedBy     string `json:"submitted_by"`
			CreatedAt       string `json:"created_at"`
			SubmitterName   string `json:"submitter_username"`
			SubmitterDisplay string `json:"submitter_display_name"`
			// Tags the submitter picked, in priority order — shown so the
			// reviewer can see what the community suggested before approving
			// (docs/adr/021). Corrections happen after approval, once the
			// patch exists as a normal node with the usual tag editor.
			Tags []string `json:"tags"`
			// The calendar the suggestion carries, and what one fetch found
			// on it at submission time (docs/adr/2026-09-18-trust-has-a-
			// scope-and-a-suggestion-carries-its-calendar.md, decision 6).
			// The count is a snapshot, never kept current: it is here so the
			// reviewing admin can see whether the address holds anything
			// before deciding to attach it.
			FeedURL           string `json:"feed_url"`
			FeedUpcomingCount int    `json:"feed_upcoming_count"`
		}

		var items []submission
		for rows.Next() {
			var s submission
			var linksStr string
			if err := rows.Scan(&s.ID, &s.Name, &s.Slug, &s.Description, &s.Website, &linksStr, &s.Address, &s.SubmittedBy, &s.CreatedAt, &s.SubmitterName, &s.SubmitterDisplay, &s.FeedURL, &s.FeedUpcomingCount); err != nil {
				continue
			}
			s.Links = json.RawMessage(linksStr)
			s.SuggestedVerificationDomain = deriveVerificationDomain(s.Website)
			s.Tags = nodeTagNames(db, s.ID)
			items = append(items, s)
		}

		hasMore := len(items) > limit
		if hasMore {
			items = items[:limit]
		}
		if items == nil {
			items = []submission{}
		}

		resp := map[string]interface{}{"items": items}
		if hasMore && len(items) > 0 {
			last := items[len(items)-1]
			resp["next_cursor"] = encodeCursor(last.CreatedAt, last.ID)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// ReviewSubmission handles PATCH /api/v1/admin/submissions/{id}.
func ReviewSubmission(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		nodeID := r.PathValue("id")

		var req struct {
			Action string `json:"action"` // "approve" or "reject"
			Note   string `json:"note"`
			// Trust anchor for self-service claims, vetted here by the
			// reviewing admin (docs/adr/030). The submitter's website never
			// becomes one on its own.
			VerificationDomain string `json:"verification_domain"`
			// Reviewer's correction of the submitted tags: when present,
			// replaces them wholesale on approval (empty array clears);
			// absent keeps what the submitter picked. Same curated
			// vocabulary as every other tag-writing path (docs/adr/021).
			Tags []string `json:"tags"`
			// The two things the approval carries besides the listing
			// itself (docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-
			// carries-its-calendar.md, decisions 3 and 6). Both default to
			// on — nil is yes — because the form offers them checked and the
			// admin unchecks what they don't mean. They are pointers so
			// "unchecked" and "this client doesn't know about the field" stay
			// different answers.
			//
			// Approving a listing is a judgement about the place; letting its
			// suggester keep its calendar unreviewed is a second judgement.
			// The checked line is what the admin reads before clicking, which
			// is what keeps ADR 026's "given explicitly" true.
			GrantTrust *bool `json:"grant_trust"`
			AttachFeed *bool `json:"attach_feed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Verify node exists and is pending_review.
		var status, slug, name, suggestedFeed string
		var submittedBy *string
		err := db.QueryRow(
			`SELECT status, slug, name, submitted_by, COALESCE(suggested_feed_url,'')
			 FROM nodes WHERE id = ?`, nodeID,
		).Scan(&status, &slug, &name, &submittedBy, &suggestedFeed)
		if err != nil || status != "pending_review" {
			http.Error(w, `{"error":"submission not found"}`, http.StatusNotFound)
			return
		}

		now := clock.Now()

		switch req.Action {
		case "approve":
			verificationDomain, derr := validateExplicitDomain(req.VerificationDomain)
			if derr != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, derr.Error()), http.StatusBadRequest)
				return
			}
			// Validate the tag override before touching the node, so a typo
			// rejects the request instead of half-approving.
			tagIDs, unknownTag := resolveTagIDs(db, req.Tags)
			if unknownTag != "" {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, "unknown tag: "+unknownTag), http.StatusBadRequest)
				return
			}
			// The review is over either way, so the parked feed goes either
			// way: attached below, or dropped here. Leaving it on the row
			// would make an approved listing look like it still had a
			// pending decision on it.
			db.Exec(
				`UPDATE nodes SET status = 'unclaimed', verification_domain = ?, updated_at = ?,
				 suggested_feed_url = '', suggested_feed_count = 0 WHERE id = ?`,
				verificationDomain, now, nodeID,
			)
			if req.Tags != nil {
				setNodeTags(db, nodeID, tagIDs)
			}
			auth.LogAuditEvent(db, user.ID, "node.submission_approved", "node", nodeID, "", clientIP(r))

			// The per-patch grant (decision 3). Skipped where there is
			// nobody to grant it to, and where the person already holds the
			// quilt-wide grant — a narrower row under a wider one decides
			// nothing and would outlive the wider one's revocation.
			grantTrust := req.GrantTrust == nil || *req.GrantTrust
			granted, quiltWide := false, false
			if submittedBy != nil {
				db.QueryRow(`SELECT trusted_contributor FROM users WHERE id = ?`, *submittedBy).Scan(&quiltWide)
			}
			if grantTrust && submittedBy != nil && !quiltWide {
				if _, gerr := db.Exec(
					`INSERT OR IGNORE INTO node_trusted_contributors (user_id, node_id, granted_by, granted_at, source)
					 VALUES (?, ?, ?, ?, 'suggestion')`,
					*submittedBy, nodeID, user.ID, now,
				); gerr == nil {
					granted = true
					// On the person, not on the patch: what changed is what
					// they may do, and Admin → Users is where it is revoked.
					auth.LogAuditEvent(db, user.ID, "trust.granted", "user", *submittedBy,
						fmt.Sprintf(`{"scope":"patch","node_id":%q,"source":"suggestion"}`, nodeID), clientIP(r))
				}
			}
			holdsTrust := granted || quiltWide

			// The feed (decision 6). added_by is whoever held standing at
			// this moment: the suggester where the grant reached them, the
			// reviewing admin otherwise.
			attachFeed := (req.AttachFeed == nil || *req.AttachFeed) && suggestedFeed != ""
			attached := false
			if attachFeed {
				addedBy := user.ID
				if holdsTrust && submittedBy != nil {
					addedBy = *submittedBy
				}
				attached = attachSuggestedFeed(db, r, nodeID, suggestedFeed, addedBy)
			}

			notifySubmissionApproved(submittedBy, user.ID, nodeID, slug, name, holdsTrust, attached)
		case "reject":
			db.Exec("UPDATE nodes SET archived_from = status, status = 'archived', removed_at = ?, updated_at = ? WHERE id = ?", now, now, nodeID)
			auth.LogAuditEvent(db, user.ID, "node.submission_rejected", "node", nodeID, fmt.Sprintf(`{"note":%q}`, req.Note), clientIP(r))

			// The note the review form already collected, said to the person
			// it is about. Until now it only ever reached the audit log
			// (decision 8).
			if submittedBy != nil {
				notify(notifications.Event{
					Type:     notifications.SubmissionRejected,
					ActorID:  user.ID,
					TargetID: *submittedBy,
					EntityID: nodeID,
					Title:    name + " was not added",
					Body:     req.Note,
					Link:     weblink.SubmitPatch(),
				})
			}
		default:
			http.Error(w, `{"error":"action must be 'approve' or 'reject'"}`, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
