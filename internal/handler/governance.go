package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"log"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
)

// governanceDocColumns is the one column list every governance_docs read uses,
// so adding a column can't leave one read path scanning a stale order.
const governanceDocColumns = `SELECT id, node_id, title, body, kind, visibility, version, created_by, created_at, updated_at FROM governance_docs`

// validDocVisibility reports whether v is a governance doc visibility the API
// accepts. Mirrors the CHECK constraint in migration 036.
func validDocVisibility(v string) bool {
	return v == "public" || v == "members"
}

// canReadPatchDocs reports whether the request's viewer may read this patch's
// members-only charters (docs/adr/036). Instance admins and the patch's
// admins/members always may; a follower may when the patch's follower
// permissions grant charters — the same knob the workspace UI reads, so the
// two never disagree. Signed-out visitors never may.
func canReadPatchDocs(db *database.DB, r *http.Request, nodeID string) bool {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		return false
	}
	if user.Role == "admin" {
		return true
	}
	var role string
	if err := db.QueryRow(
		"SELECT role FROM memberships WHERE user_id = ? AND node_id = ? AND status = 'active'",
		user.ID, nodeID,
	).Scan(&role); err != nil {
		return false
	}
	if role == "admin" || role == "member" {
		return true
	}
	var fpJSON string
	db.QueryRow("SELECT COALESCE(follower_permissions,'{}') FROM nodes WHERE id = ?", nodeID).Scan(&fpJSON)
	var fp model.FollowerPermissions
	json.Unmarshal([]byte(fpJSON), &fp)
	return fp.Charters
}

// membersOnlyDocFilenames returns the git filenames of a node's members-only
// charters. An amendment proposal carries the full proposed text of the doc it
// targets, and proposals are a public read — without this, a members-only
// charter is one amendment away from being world-readable anyway.
func membersOnlyDocFilenames(db *database.DB, nodeID string) map[string]bool {
	rows, err := db.Query("SELECT title FROM governance_docs WHERE node_id = ? AND visibility = 'members'", nodeID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	hidden := map[string]bool{}
	for rows.Next() {
		var title string
		if rows.Scan(&title) != nil {
			continue
		}
		hidden[governanceFilename(title)] = true
	}
	if len(hidden) == 0 {
		return nil
	}
	return hidden
}

// hiddenDocRedactor returns a predicate reporting whether a proposal's
// target_doc names a charter this viewer can't read. Nil-safe and lazy: for the
// common case (viewer can read, or the patch has no members-only charters) it
// costs one membership lookup and no per-row work.
//
// Only the mirrored document text is withheld. The proposal itself — title,
// the proposer's own rationale, votes — stays public, because that is what the
// author posted knowing proposals are public deliberation.
func hiddenDocRedactor(db *database.DB, r *http.Request, nodeID string) func(targetDoc string) bool {
	if canReadPatchDocs(db, r, nodeID) {
		return func(string) bool { return false }
	}
	hidden := membersOnlyDocFilenames(db, nodeID)
	if hidden == nil {
		return func(string) bool { return false }
	}
	return func(targetDoc string) bool { return targetDoc != "" && hidden[targetDoc] }
}

// CreateGovernanceDoc handles POST /api/v1/nodes/{slug}/governance.
func CreateGovernanceDoc(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")

		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// Require admin role on the node or global admin.
		if user.Role != "admin" && !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
			return
		}

		var req struct {
			Title      string `json:"title"`
			Body       string `json:"body"`
			Visibility string `json:"visibility"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Title == "" {
			http.Error(w, `{"error":"title is required"}`, http.StatusBadRequest)
			return
		}
		// Members-only unless the admin asks otherwise, here and in the DB
		// default: a charter is published deliberately (docs/adr/036).
		if req.Visibility == "" {
			req.Visibility = "members"
		}
		if !validDocVisibility(req.Visibility) {
			http.Error(w, `{"error":"visibility must be public or members"}`, http.StatusBadRequest)
			return
		}
		// A new charter can't take the lining's identity: title→filename is
		// how DB rows and git files link (docs/adr/011), so a charter that
		// slugifies to the lining's filename would collide with it in the
		// repo and in the amendment flow (docs/adr/037).
		if governanceFilename(req.Title) == governanceFilename(DefaultLiningTitle) {
			http.Error(w, `{"error":"that title is reserved for the lining"}`, http.StatusBadRequest)
			return
		}

		id := auth.NewUUIDv7()
		_, err := db.Exec(
			`INSERT INTO governance_docs (id, node_id, title, body, visibility, created_by) VALUES (?, ?, ?, ?, ?, ?)`,
			id, nodeID, req.Title, req.Body, req.Visibility, user.ID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to create governance doc"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "governance.create", "governance_doc", id, "{}", clientIP(r))

		var doc model.GovernanceDoc
		db.QueryRow(governanceDocColumns+` WHERE id = ?`, id).Scan(
			&doc.ID, &doc.NodeID, &doc.Title, &doc.Body, &doc.Kind, &doc.Visibility, &doc.Version,
			&doc.CreatedBy, &doc.CreatedAt, &doc.UpdatedAt)

		// Notify members about the new governance doc.
		var nodeNameN string
		db.QueryRow("SELECT name FROM nodes WHERE id = ?", nodeID).Scan(&nodeNameN)
		notify(notifications.Event{
			Type:     notifications.GovernanceDocUpdated,
			NodeID:   nodeID,
			NodeSlug: slug,
			NodeName: nodeNameN,
			ActorID:  user.ID,
			EntityID: id,
			Title:    "New governance document: " + req.Title,
			Link:     "/patches/" + slug + "/governance/" + id,
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(doc)
	}
}

// ListGovernanceDocs handles GET /api/v1/nodes/{slug}/governance.
// Members-only docs are omitted for viewers who can't read them (docs/adr/036),
// so the route is mounted with AuthOptional rather than left anonymous.
func ListGovernanceDocs(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		query := governanceDocColumns + ` WHERE node_id = ?`
		if !canReadPatchDocs(db, r, nodeID) {
			query += ` AND visibility = 'public'`
		}
		query += ` ORDER BY created_at ASC`

		rows, err := db.Query(query, nodeID)
		if err != nil {
			http.Error(w, `{"error":"failed to list governance docs"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var docs []model.GovernanceDoc
		for rows.Next() {
			var d model.GovernanceDoc
			if err := rows.Scan(&d.ID, &d.NodeID, &d.Title, &d.Body, &d.Kind, &d.Visibility, &d.Version, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt); err != nil {
				continue
			}
			docs = append(docs, d)
		}
		if docs == nil {
			docs = []model.GovernanceDoc{}
		}

		type docWithFilename struct {
			model.GovernanceDoc
			Filename string `json:"filename"`
		}
		var items []docWithFilename
		for _, d := range docs {
			items = append(items, docWithFilename{d, governanceFilename(d.Title)})
		}
		if items == nil {
			items = []docWithFilename{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": items,
		})
	}
}

// GetGovernanceDoc handles GET /api/v1/governance/{id}.
func GetGovernanceDoc(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID := r.PathValue("id")

		var doc model.GovernanceDoc
		err := db.QueryRow(governanceDocColumns+` WHERE id = ?`, docID).Scan(
			&doc.ID, &doc.NodeID, &doc.Title, &doc.Body, &doc.Kind, &doc.Visibility, &doc.Version,
			&doc.CreatedBy, &doc.CreatedAt, &doc.UpdatedAt)
		if err != nil {
			http.Error(w, `{"error":"governance doc not found"}`, http.StatusNotFound)
			return
		}
		// Not-found rather than forbidden: a members-only charter's existence
		// is itself part of what the patch hasn't published.
		if doc.Visibility != "public" && !canReadPatchDocs(db, r, doc.NodeID) {
			http.Error(w, `{"error":"governance doc not found"}`, http.StatusNotFound)
			return
		}

		filename := governanceFilename(doc.Title)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         doc.ID,
			"node_id":    doc.NodeID,
			"title":      doc.Title,
			"body":       doc.Body,
			"visibility": doc.Visibility,
			"version":    doc.Version,
			"created_by": doc.CreatedBy,
			"created_at": doc.CreatedAt,
			"updated_at": doc.UpdatedAt,
			"filename":   filename,
		})
	}
}

// UpdateGovernanceDoc handles PUT /api/v1/governance/{id}.
func UpdateGovernanceDoc(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		docID := r.PathValue("id")

		// Get the doc to check node ownership.
		var nodeID, docKind string
		var currentVersion int
		err := db.QueryRow("SELECT node_id, kind, version FROM governance_docs WHERE id = ?", docID).Scan(&nodeID, &docKind, &currentVersion)
		if err != nil {
			http.Error(w, `{"error":"governance doc not found"}`, http.StatusNotFound)
			return
		}

		// Require admin role on the node or global admin.
		if user.Role != "admin" && !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
			return
		}

		var req struct {
			Title      *string `json:"title"`
			Body       *string `json:"body"`
			Visibility *string `json:"visibility"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Title == nil && req.Body == nil && req.Visibility == nil {
			http.Error(w, `{"error":"title, body, or visibility is required"}`, http.StatusBadRequest)
			return
		}
		if req.Title != nil && *req.Title == "" {
			http.Error(w, `{"error":"title cannot be empty"}`, http.StatusBadRequest)
			return
		}
		if req.Visibility != nil && !validDocVisibility(*req.Visibility) {
			http.Error(w, `{"error":"visibility must be public or members"}`, http.StatusBadRequest)
			return
		}

		// The lining is bible (docs/adr/037): its title is its identity, its
		// visibility is pinned public, and its body changes only through a
		// passed amendment — a voted, recorded act — never a direct edit.
		if docKind == "lining" {
			if req.Title != nil {
				http.Error(w, `{"error":"the lining's title cannot be changed"}`, http.StatusBadRequest)
				return
			}
			if req.Visibility != nil && *req.Visibility != "public" {
				http.Error(w, `{"error":"the lining is always public"}`, http.StatusBadRequest)
				return
			}
			if req.Body != nil {
				http.Error(w, `{"error":"the lining can only be changed by amendment proposal"}`, http.StatusBadRequest)
				return
			}
		}
		// And no charter may be retitled into the lining's identity (same
		// filename-collision rule as CreateGovernanceDoc).
		if docKind != "lining" && req.Title != nil &&
			governanceFilename(*req.Title) == governanceFilename(DefaultLiningTitle) {
			http.Error(w, `{"error":"that title is reserved for the lining"}`, http.StatusBadRequest)
			return
		}

		var curTitle, curBody, curVisibility string
		db.QueryRow(`SELECT title, body, visibility FROM governance_docs WHERE id = ?`, docID).Scan(&curTitle, &curBody, &curVisibility)
		newTitle, newBody, newVisibility := curTitle, curBody, curVisibility
		if req.Title != nil {
			newTitle = *req.Title
		}
		if req.Body != nil {
			newBody = *req.Body
		}
		if req.Visibility != nil {
			newVisibility = *req.Visibility
		}

		// A visibility flip is not an amendment: the text is unchanged, so it
		// doesn't earn a version bump, a git commit, or a notification. Only a
		// title/body edit does.
		contentChanged := newTitle != curTitle || newBody != curBody

		newVersion := currentVersion
		if contentChanged {
			newVersion = currentVersion + 1
		}
		_, err = db.Exec(
			`UPDATE governance_docs SET title = ?, body = ?, visibility = ?, version = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
			newTitle, newBody, newVisibility, newVersion, docID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to update governance doc"}`, http.StatusInternalServerError)
			return
		}

		// Mirror the edit into the node's git-backed lining repo so version
		// history and diffs reflect edits made through this endpoint. Best
		// effort: repos may not exist (tests, fresh instances).
		if dataDir := governance.GetDataDir(); dataDir != "" && contentChanged {
			author := user.DisplayName
			if author == "" {
				author = user.Username
			}
			if _, gitErr := governance.DirectEdit(dataDir, nodeID, governanceFilename(newTitle),
				newBody, author, user.Username+"@patchwork.local",
				"Update "+newTitle+" (v"+strconv.Itoa(newVersion)+")"); gitErr != nil {
				log.Printf("governance: git mirror of doc %s failed: %v", docID, gitErr)
			}
		}

		action := "governance.update"
		if !contentChanged {
			action = "governance.visibility"
		}
		auth.LogAuditEvent(db, user.ID, action, "governance_doc", docID, `{"visibility":"`+newVisibility+`"}`, clientIP(r))

		var doc model.GovernanceDoc
		db.QueryRow(governanceDocColumns+` WHERE id = ?`, docID).Scan(
			&doc.ID, &doc.NodeID, &doc.Title, &doc.Body, &doc.Kind, &doc.Visibility, &doc.Version,
			&doc.CreatedBy, &doc.CreatedAt, &doc.UpdatedAt)

		if contentChanged {
			// Notify members about the governance doc update.
			var nodeSlugN, nodeNameN string
			db.QueryRow("SELECT slug, name FROM nodes WHERE id = ?", nodeID).Scan(&nodeSlugN, &nodeNameN)
			notify(notifications.Event{
				Type:     notifications.GovernanceDocUpdated,
				NodeID:   nodeID,
				NodeSlug: nodeSlugN,
				NodeName: nodeNameN,
				ActorID:  user.ID,
				EntityID: docID,
				Title:    "Governance document updated: " + newTitle,
				Link:     "/patches/" + nodeSlugN + "/governance/" + docID,
			})
		}

		// Broadcast governance doc update. Followers here are remote actors on
		// other instances, so only a public doc may go out — federating a
		// members-only charter would publish the very thing it withholds.
		if doc.Visibility == "public" && contentChanged {
			go func() {
				docObj := ap.GovernanceDocToObject(doc, ap.GetDomain())
				activity := map[string]interface{}{
					"@context": ap.GovernanceContext(),
					"type":     "Update",
					"actor":    ap.NodeAPID(ap.GetDomain(), doc.NodeID),
					"object":   docObj,
				}
				ap.BroadcastToFollowers(db, "node", doc.NodeID, activity)
			}()
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(doc)
	}
}

// GetGovernanceVersions handles GET /api/v1/governance/{id}/versions.
// Returns the git commit history for the governance document's file.
func GetGovernanceVersions(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID := r.PathValue("id")

		// Get the doc to find the node and derive the filename
		var nodeID, title, visibility string
		err := db.QueryRow("SELECT node_id, title, visibility FROM governance_docs WHERE id = ?", docID).Scan(&nodeID, &title, &visibility)
		if err != nil {
			http.Error(w, `{"error":"governance doc not found"}`, http.StatusNotFound)
			return
		}
		// History carries the same text as the doc, so it wears the same gate.
		if visibility != "public" && !canReadPatchDocs(db, r, nodeID) {
			http.Error(w, `{"error":"governance doc not found"}`, http.StatusNotFound)
			return
		}

		// Derive filename from title (same logic as seed: slugify + .md)
		filename := governanceFilename(title)

		// Get git history
		dataDir := governance.GetDataDir()
		if dataDir == "" {
			// No governance repos configured — return empty
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"items": []interface{}{}})
			return
		}

		history, err := governance.GetHistory(dataDir, nodeID, filename)
		if err != nil {
			// Repo might not exist — return empty
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"items": []interface{}{}})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items":    history,
			"filename": filename,
		})
	}
}

// GetGovernanceDiff handles GET /api/v1/governance/{id}/diff?from={sha}&to={sha}.
// Returns the word-level diff between two git versions of a governance doc,
// for the version-compare UI.
func GetGovernanceDiff(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docID := r.PathValue("id")
		fromSHA := r.URL.Query().Get("from")
		toSHA := r.URL.Query().Get("to")
		if fromSHA == "" || toSHA == "" {
			http.Error(w, `{"error":"from and to query params are required"}`, http.StatusBadRequest)
			return
		}

		var nodeID, title, visibility string
		err := db.QueryRow("SELECT node_id, title, visibility FROM governance_docs WHERE id = ?", docID).Scan(&nodeID, &title, &visibility)
		if err != nil {
			http.Error(w, `{"error":"governance doc not found"}`, http.StatusNotFound)
			return
		}
		if visibility != "public" && !canReadPatchDocs(db, r, nodeID) {
			http.Error(w, `{"error":"governance doc not found"}`, http.StatusNotFound)
			return
		}

		dataDir := governance.GetDataDir()
		if dataDir == "" {
			http.Error(w, `{"error":"governance repos not configured"}`, http.StatusNotFound)
			return
		}

		diff, err := governance.GetDiff(dataDir, nodeID, governanceFilename(title), fromSHA, toSHA)
		if err != nil {
			http.Error(w, `{"error":"version not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(diff)
	}
}

// GetGovernanceRules handles GET /api/v1/nodes/{slug}/governance/rules.
// Reads governance config from the DB cache and returns the combined rules.
func GetGovernanceRules(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")

		var gcJSON, membershipPolicy, fpJSON string
		err := db.QueryRow(
			`SELECT COALESCE(governance_config,'{}'), membership_policy, COALESCE(follower_permissions,'{}')
			 FROM nodes WHERE slug = ? AND status IN ('active','unclaimed') AND removed_at IS NULL`, slug,
		).Scan(&gcJSON, &membershipPolicy, &fpJSON)
		if err != nil {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		// Parse the DB-cached values into the combined rules response.
		var gc model.GovernanceConfig
		json.Unmarshal([]byte(gcJSON), &gc)

		var fp model.FollowerPermissions
		json.Unmarshal([]byte(fpJSON), &fp)

		rules := governance.GovernanceRules{
			DecisionMethod:      gc.DecisionMethod,
			QuorumPercent:       gc.QuorumPercent,
			DefaultVoteDuration: gc.DefaultVoteDuration,
			AmendmentThreshold:  gc.AmendmentThreshold,
			AmendmentAutoApply:  gc.AmendmentAutoApply,
			SuccessionPolicy:    gc.SuccessionPolicy,
			MinVotingTenureDays: gc.MinVotingTenureDays,
			MembershipPolicy:    membershipPolicy,
			FollowerPermissions: fp,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rules)
	}
}

// governanceFilename converts a governance doc title to a kebab-case .md filename.
func governanceFilename(title string) string {
	name := strings.ToLower(title)
	name = strings.ReplaceAll(name, " ", "-")
	// Remove non-alphanumeric except hyphens
	var clean []byte
	for _, c := range []byte(name) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			clean = append(clean, c)
		}
	}
	return string(clean) + ".md"
}

// syncLiningToDB mirrors a merged amendment's file back into the
// governance_docs table — the canonical store for linings (docs/adr/011).
// Git keeps history and diffs; the DB row is what the governance hub,
// seamrip, and every other read path serve, so a merged amendment that
// stays only in git is invisible to the community that just voted it in.
// This is the symmetric inverse of UpdateGovernanceDoc's DB→git mirror,
// and like it, best effort: it reads the merged file from git HEAD (the
// merge is truth, not the proposal's proposed_body) and logs on failure.
//
// Rules files have their own sync (governance.SyncRulesToDB); only
// markdown docs come through here.
func syncLiningToDB(db *database.DB, nodeID, targetDoc, proposedTitle, editorID string) {
	if targetDoc == "" || !strings.HasSuffix(targetDoc, ".md") {
		return
	}
	dataDir := governance.GetDataDir()
	if dataDir == "" {
		return
	}
	content, err := governance.GetDocument(dataDir, nodeID, targetDoc)
	if err != nil {
		log.Printf("governance: DB sync of %s for node %s: read merged file: %v", targetDoc, nodeID, err)
		return
	}

	// Find the row this file mirrors, using the same identity rule as the
	// DB→git direction: filename = governanceFilename(title).
	rows, err := db.Query("SELECT id, title, version FROM governance_docs WHERE node_id = ?", nodeID)
	if err != nil {
		return
	}
	var docID string
	var version int
	for rows.Next() {
		var id, title string
		var v int
		if rows.Scan(&id, &title, &v) != nil {
			continue
		}
		if governanceFilename(title) == targetDoc {
			docID, version = id, v
			break
		}
	}
	rows.Close()

	if docID != "" {
		// Body only — the title stays, because the title IS the filename
		// identity linking this row to targetDoc; renaming here would orphan
		// the git file for every future mirror write.
		db.Exec(
			`UPDATE governance_docs SET body = ?, version = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
			content, version+1, docID,
		)
		return
	}

	// No row yet — a doc that until now lived only in git (pre-ADR-011
	// forks, template docs). Create it so the canonical store has it.
	title := proposedTitle
	if title == "" {
		title = titleFromGovernanceFilename(targetDoc)
	}
	db.Exec(
		`INSERT INTO governance_docs (id, node_id, title, body, created_by) VALUES (?, ?, ?, ?, ?)`,
		auth.NewUUIDv7(), nodeID, title, content, editorID,
	)
}

// titleFromGovernanceFilename inverts governanceFilename well enough for a
// display title: "community-standards.md" -> "Community Standards".
func titleFromGovernanceFilename(filename string) string {
	name := strings.TrimSuffix(filename, ".md")
	words := strings.Split(name, "-")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
