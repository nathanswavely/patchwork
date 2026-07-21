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
			Title string `json:"title"`
			Body  string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Title == "" {
			http.Error(w, `{"error":"title is required"}`, http.StatusBadRequest)
			return
		}

		id := auth.NewUUIDv7()
		_, err := db.Exec(
			`INSERT INTO governance_docs (id, node_id, title, body, created_by) VALUES (?, ?, ?, ?, ?)`,
			id, nodeID, req.Title, req.Body, user.ID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to create governance doc"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "governance.create", "governance_doc", id, "{}", clientIP(r))

		var doc model.GovernanceDoc
		db.QueryRow(
			`SELECT id, node_id, title, body, version, created_by, created_at, updated_at FROM governance_docs WHERE id = ?`, id,
		).Scan(&doc.ID, &doc.NodeID, &doc.Title, &doc.Body, &doc.Version, &doc.CreatedBy, &doc.CreatedAt, &doc.UpdatedAt)

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
func ListGovernanceDocs(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		rows, err := db.Query(
			`SELECT id, node_id, title, body, version, created_by, created_at, updated_at FROM governance_docs WHERE node_id = ? ORDER BY created_at ASC`,
			nodeID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to list governance docs"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var docs []model.GovernanceDoc
		for rows.Next() {
			var d model.GovernanceDoc
			if err := rows.Scan(&d.ID, &d.NodeID, &d.Title, &d.Body, &d.Version, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt); err != nil {
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
		err := db.QueryRow(
			`SELECT id, node_id, title, body, version, created_by, created_at, updated_at FROM governance_docs WHERE id = ?`, docID,
		).Scan(&doc.ID, &doc.NodeID, &doc.Title, &doc.Body, &doc.Version, &doc.CreatedBy, &doc.CreatedAt, &doc.UpdatedAt)
		if err != nil {
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
		var nodeID string
		var currentVersion int
		err := db.QueryRow("SELECT node_id, version FROM governance_docs WHERE id = ?", docID).Scan(&nodeID, &currentVersion)
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
			Title *string `json:"title"`
			Body  *string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Title == nil && req.Body == nil {
			http.Error(w, `{"error":"title or body is required"}`, http.StatusBadRequest)
			return
		}
		if req.Title != nil && *req.Title == "" {
			http.Error(w, `{"error":"title cannot be empty"}`, http.StatusBadRequest)
			return
		}

		var curTitle, curBody string
		db.QueryRow(`SELECT title, body FROM governance_docs WHERE id = ?`, docID).Scan(&curTitle, &curBody)
		newTitle, newBody := curTitle, curBody
		if req.Title != nil {
			newTitle = *req.Title
		}
		if req.Body != nil {
			newBody = *req.Body
		}

		newVersion := currentVersion + 1
		_, err = db.Exec(
			`UPDATE governance_docs SET title = ?, body = ?, version = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
			newTitle, newBody, newVersion, docID,
		)
		if err != nil {
			http.Error(w, `{"error":"failed to update governance doc"}`, http.StatusInternalServerError)
			return
		}

		// Mirror the edit into the node's git-backed lining repo so version
		// history and diffs reflect edits made through this endpoint. Best
		// effort: repos may not exist (tests, fresh instances).
		if dataDir := governance.GetDataDir(); dataDir != "" {
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

		auth.LogAuditEvent(db, user.ID, "governance.update", "governance_doc", docID, "{}", clientIP(r))

		var doc model.GovernanceDoc
		db.QueryRow(
			`SELECT id, node_id, title, body, version, created_by, created_at, updated_at FROM governance_docs WHERE id = ?`, docID,
		).Scan(&doc.ID, &doc.NodeID, &doc.Title, &doc.Body, &doc.Version, &doc.CreatedBy, &doc.CreatedAt, &doc.UpdatedAt)

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

		// Broadcast governance doc update
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
		var nodeID, title string
		err := db.QueryRow("SELECT node_id, title FROM governance_docs WHERE id = ?", docID).Scan(&nodeID, &title)
		if err != nil {
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

		var nodeID, title string
		err := db.QueryRow("SELECT node_id, title FROM governance_docs WHERE id = ?", docID).Scan(&nodeID, &title)
		if err != nil {
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
