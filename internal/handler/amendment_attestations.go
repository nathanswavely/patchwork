package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/notifications"
	"github.com/patchwork-toolkit/patchwork/internal/weblink"
)

// Amendments adopted elsewhere (docs/adr/053).
//
// The leadership half of docs/adr/052 records who a meeting put in place.
// This half records what a meeting adopted: a charter, bylaws, an operating
// agreement — a *text*. Three rules from 053 carry it:
//
//   - It is gated on `proposal_venue: elsewhere`, and declaring that venue
//     removes the ballot. Without the second half the gate means nothing: a
//     patch would have both a vote and an attestation, and an admin who
//     disliked where a tally was heading could record a meeting result
//     instead.
//   - The rules file is not attestable, and neither is the lining. The rules
//     file is machine configuration, not a text anyone adopts, and excluding
//     it closes a two-step route around the leadership gate. The lining is
//     docs/adr/037's hard rule, surviving every configuration.
//   - An attestation asserts the whole current text and checks no base.
//     Patchwork's copy is a possibly-stale cache being corrected, not a base
//     to build on, so there is no conflict check to fail — only an open
//     amendment proposal that gets told the ground moved.

// proposalVenue reports where a patch decides the things proposals are about.
// Empty or absent reads as "patchwork", so every existing patch keeps voting.
func proposalVenue(db *database.DB, nodeID string) string {
	var gcJSON string
	if err := db.QueryRow("SELECT COALESCE(governance_config,'{}') FROM nodes WHERE id = ?", nodeID).Scan(&gcJSON); err != nil {
		return "patchwork"
	}
	var gc model.GovernanceConfig
	if err := json.Unmarshal([]byte(gcJSON), &gc); err != nil || gc.ProposalVenue == "" {
		return "patchwork"
	}
	return gc.ProposalVenue
}

// proposalsDecidedElsewhere is the gate every amendment-attestation path
// checks, and the same condition that removes the ballot in CreateProposal.
// One function, so the gate and the thing it is protecting can never disagree.
func proposalsDecidedElsewhere(db *database.DB, nodeID string) bool {
	return proposalVenue(db, nodeID) == "elsewhere"
}

// rulesFilename is the one document a proposal may target that is not prose.
const rulesFilename = "governance-rules.json"

type amendmentAttestationView struct {
	ID         string `json:"id"`
	NodeID     string `json:"node_id"`
	DocID      string `json:"doc_id,omitempty"`
	TargetDoc  string `json:"target_doc"`
	DocTitle   string `json:"doc_title"`
	DecidedAt  string `json:"decided_at"`
	Summary    string `json:"summary"`
	GitSHA     string `json:"git_sha,omitempty"`
	RecordedBy string `json:"recorded_by"`
	// RecorderName is who typed it in, which is not who decided it. Shown
	// because an attestation Patchwork cannot check is worth exactly the
	// public knowledge of who asserted it.
	RecorderName string `json:"recorder_name"`
	CreatedAt    string `json:"created_at"`
	// AdoptedBody is the text as adopted, withheld from a viewer who cannot
	// read the charter it belongs to (docs/adr/036). The record that a
	// meeting adopted something stays public; the text follows the charter.
	AdoptedBody string `json:"adopted_body,omitempty"`
	TextHidden  bool   `json:"text_hidden"`
}

// ListAmendmentAttestations handles
// GET /api/v1/nodes/{slug}/amendment-attestations[?doc={filename}].
//
// Public, for the same reason the leadership records are: an attestation's
// whole value is that the people who were in the room can check it.
func ListAmendmentAttestations(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := NodeIDFromSlug(db, r.PathValue("slug"))
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}

		query := `SELECT a.id, a.node_id, COALESCE(a.doc_id,''), a.target_doc, a.doc_title,
		                 a.decided_at, a.summary, a.adopted_body, a.git_sha, a.recorded_by,
		                 COALESCE(u.display_name, u.username, ''), a.created_at
		          FROM amendment_attestations a
		          LEFT JOIN users u ON u.id = a.recorded_by
		          WHERE a.node_id = ?`
		args := []interface{}{nodeID}
		if doc := r.URL.Query().Get("doc"); doc != "" {
			query += ` AND a.target_doc = ?`
			args = append(args, doc)
		}
		query += ` ORDER BY a.decided_at DESC, a.created_at DESC`

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"error":"failed to list records"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		hidden := hiddenDocRedactor(db, r, nodeID)
		items := []amendmentAttestationView{}
		for rows.Next() {
			var a amendmentAttestationView
			if rows.Scan(&a.ID, &a.NodeID, &a.DocID, &a.TargetDoc, &a.DocTitle, &a.DecidedAt,
				&a.Summary, &a.AdoptedBody, &a.GitSHA, &a.RecordedBy, &a.RecorderName, &a.CreatedAt) != nil {
				continue
			}
			if hidden(a.TargetDoc) {
				a.AdoptedBody, a.TextHidden = "", true
			}
			items = append(items, a)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"items": items})
	}
}

// CreateAmendmentAttestation handles
// POST /api/v1/nodes/{slug}/amendment-attestations.
func CreateAmendmentAttestation(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())
		slug := r.PathValue("slug")
		nodeID := NodeIDFromSlug(db, slug)
		if nodeID == "" {
			http.Error(w, `{"error":"node not found"}`, http.StatusNotFound)
			return
		}
		if !userHasNodeRole(db, user.ID, nodeID, "admin") {
			http.Error(w, `{"error":"only an admin of this patch can record what was adopted"}`, http.StatusForbidden)
			return
		}
		if !proposalsDecidedElsewhere(db, nodeID) {
			http.Error(w, `{"error":"this patch decides its proposals in Patchwork; amend the charter with a proposal"}`, http.StatusConflict)
			return
		}

		var req struct {
			DocID       string `json:"doc_id"`
			Title       string `json:"title"`
			DecidedAt   string `json:"decided_at"`
			Summary     string `json:"summary"`
			AdoptedBody string `json:"adopted_body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.DecidedAt) == "" {
			http.Error(w, `{"error":"decided_at is required: a record of what happened has to say when"}`, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.AdoptedBody) == "" {
			http.Error(w, `{"error":"the adopted text is required: a record of an adopted text has to carry the text"}`, http.StatusBadRequest)
			return
		}

		docID, title, msg := resolveAttestedDoc(db, nodeID, req.DocID, req.Title)
		if msg != "" {
			http.Error(w, `{"error":"`+msg+`"}`, http.StatusConflict)
			return
		}
		filename := governanceFilename(title)

		// Write the adopted text. This is the correction of a stale cache, not
		// a merge, so it goes in whole with no base check (docs/adr/053) —
		// DirectEdit is the same call an admin's own edit makes, and the
		// commit message says where the text came from.
		version := 1
		if docID != "" {
			db.QueryRow("SELECT version FROM governance_docs WHERE id = ?", docID).Scan(&version)
			version++
		}
		author := user.DisplayName
		if author == "" {
			author = user.Username
		}
		gitSHA := ""
		if dataDir := governance.GetDataDir(); dataDir != "" {
			sha, gitErr := governance.DirectEdit(dataDir, nodeID, filename, req.AdoptedBody,
				author, user.Username+"@patchwork.local",
				"Adopted "+strings.TrimSpace(req.DecidedAt)+": "+title)
			if gitErr != nil {
				log.Printf("attestation: git write of %s for node %s failed: %v", filename, nodeID, gitErr)
			} else {
				gitSHA = sha
			}
		}

		if docID == "" {
			// A meeting can adopt a charter this instance was never templated
			// with, and refusing it would mean a community may only record
			// amendments to documents Patchwork happened to guess at
			// (docs/adr/053). Members-only like any new charter — publishing
			// stays a deliberate act (docs/adr/036).
			docID = auth.NewUUIDv7()
			if _, err := db.Exec(
				`INSERT INTO governance_docs (id, node_id, title, body, visibility, version, created_by)
				 VALUES (?, ?, ?, ?, 'members', 1, ?)`,
				docID, nodeID, title, req.AdoptedBody, user.ID,
			); err != nil {
				http.Error(w, `{"error":"failed to create the document"}`, http.StatusInternalServerError)
				return
			}
			version = 1
		} else if _, err := db.Exec(
			`UPDATE governance_docs SET body = ?, version = ?,
			 updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
			req.AdoptedBody, version, docID,
		); err != nil {
			http.Error(w, `{"error":"failed to update the document"}`, http.StatusInternalServerError)
			return
		}

		id := auth.NewUUIDv7()
		if _, err := db.Exec(
			`INSERT INTO amendment_attestations
			 (id, node_id, doc_id, target_doc, doc_title, decided_at, summary, adopted_body, git_sha, recorded_by)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, nodeID, docID, filename, title, strings.TrimSpace(req.DecidedAt),
			strings.TrimSpace(req.Summary), req.AdoptedBody, gitSHA, user.ID,
		); err != nil {
			http.Error(w, `{"error":"failed to record what was adopted"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, user.ID, "attestation.amendment", "governance_doc", docID,
			`{"attestation_id":"`+id+`","target_doc":"`+filename+`"}`, clientIP(r))

		var nodeName string
		db.QueryRow("SELECT name FROM nodes WHERE id = ?", nodeID).Scan(&nodeName)
		notify(notifications.Event{
			Type:     notifications.GovernanceDocUpdated,
			NodeID:   nodeID,
			NodeSlug: slug,
			NodeName: nodeName,
			ActorID:  user.ID,
			EntityID: docID,
			Title:    title + " was adopted in " + nodeName,
			Body:     "Recorded from a decision made outside Patchwork.",
			Link:     weblink.GovernanceDoc(slug, docID),
		})

		// A public charter that changed has to reach the instances holding a
		// copy, whichever venue changed it — otherwise remote followers keep
		// serving text this community has replaced. This is the existing
		// document-update federation, not the attestation federating as its
		// own activity, which docs/adr/053 leaves open.
		broadcastDocUpdate(db, docID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": id, "doc_id": docID})
	}
}

// resolveAttestedDoc works out which document this record is about, and
// refuses the two that are not attestable. Returns ("", "", message) on
// refusal; an empty docID with a title means "create it".
func resolveAttestedDoc(db *database.DB, nodeID, docID, title string) (string, string, string) {
	if docID != "" {
		var ownerNode, docTitle, kind string
		if err := db.QueryRow(
			"SELECT node_id, title, COALESCE(kind,'') FROM governance_docs WHERE id = ?", docID,
		).Scan(&ownerNode, &docTitle, &kind); err != nil || ownerNode != nodeID {
			return "", "", "that document does not belong to this patch"
		}
		// docs/adr/037, restated by 053: the only thing that changes a
		// lining's body is a passed amendment proposal, and an attestation is
		// not one. "Amended lining" means this community voted to change the
		// baseline; if an assertion could diverge one, the badge would look
		// the same either way and the anti-discrimination baseline is what it
		// guards.
		if kind == "lining" {
			return "", "", "the lining changes only by a passed amendment proposal, wherever else this patch decides (docs/adr/037)"
		}
		return docID, docTitle, ""
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return "", "", "name the document that was adopted"
	}
	filename := governanceFilename(title)
	// Structurally unreachable today — governanceFilename always ends in
	// ".md", and nothing here calls the rules sync — which is exactly how
	// docs/adr/053 wanted the rules excluded: "because the rules file simply
	// is not the kind of thing an attestation is about." Kept as the guard
	// that catches it if slugifying ever changes, not as the thing doing the
	// work.
	if filename == rulesFilename {
		return "", "", "the governance rules are not a text a meeting adopts; an admin applies a rules change directly (docs/adr/053)"
	}
	if filename == governanceFilename(DefaultLiningTitle) {
		return "", "", "that title is reserved for the lining"
	}

	// Match an existing document by the identity that links a row to its git
	// file (docs/adr/011), so recording an adoption by name lands on the
	// charter rather than minting a second one beside it.
	rows, err := db.Query("SELECT id, title, COALESCE(kind,'') FROM governance_docs WHERE node_id = ?", nodeID)
	if err != nil {
		return "", title, ""
	}
	defer rows.Close()
	for rows.Next() {
		var id, existing, kind string
		if rows.Scan(&id, &existing, &kind) != nil {
			continue
		}
		if governanceFilename(existing) == filename {
			if kind == "lining" {
				return "", "", "the lining changes only by a passed amendment proposal, wherever else this patch decides (docs/adr/037)"
			}
			return id, existing, ""
		}
	}
	return "", title, ""
}

// broadcastDocUpdate sends an Update for a public governance doc. Members-only
// charters never go out — federating one would publish the very thing it
// withholds.
func broadcastDocUpdate(db *database.DB, docID string) {
	var doc model.GovernanceDoc
	if db.QueryRow(governanceDocColumns+` WHERE id = ?`, docID).Scan(
		&doc.ID, &doc.NodeID, &doc.Title, &doc.Body, &doc.Kind, &doc.Visibility, &doc.Version,
		&doc.CreatedBy, &doc.CreatedAt, &doc.UpdatedAt) != nil {
		return
	}
	if doc.Visibility != "public" {
		return
	}
	go func() {
		ap.BroadcastToFollowers(db, "node", doc.NodeID, map[string]interface{}{
			"@context": ap.GovernanceContext(),
			"type":     "Update",
			"actor":    ap.NodeAPID(ap.GetDomain(), doc.NodeID),
			"object":   ap.GovernanceDocToObject(doc, ap.GetDomain()),
		})
	}()
}

// amendmentGroundMoved reports whether a document was attested since a
// proposal against it was drafted — the cost docs/adr/053 accepted when it
// decided an attestation checks no base.
//
// `base_sha` exists to catch exactly this, and the trade is deliberate: a
// community's own text should win over a draft. What the draft's readers get
// instead is the news, in the same spirit as docs/adr/047's mid-vote rules
// notice — a diff against something that has moved is worse than no diff when
// nobody is told.
//
// Read at request time rather than stamped on the proposal: no backfill, and
// it cannot go stale.
func amendmentGroundMoved(db *database.DB, nodeID, targetDoc, proposalCreatedAt, status string) (bool, string) {
	if targetDoc == "" || status != "open" {
		return false, ""
	}
	var decidedAt string
	err := db.QueryRow(
		`SELECT decided_at FROM amendment_attestations
		 WHERE node_id = ? AND target_doc = ? AND created_at > ?
		 ORDER BY created_at DESC LIMIT 1`, nodeID, targetDoc, proposalCreatedAt,
	).Scan(&decidedAt)
	if err != nil {
		return false, ""
	}
	return true, decidedAt
}
