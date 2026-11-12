package handler

import (
	"log"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
)

// CreateTemplateDocs gives a new patch the documents its template promised,
// as governance_docs rows (docs/adr/2026-11-11-a-template-gives-the-patch-what-its-preview-lists.md).
//
// Until this, ForkForNode wrote them into the git repo alone. The repo is a
// mirror and the rows are canonical (docs/adr/084), so every read path that
// lists a patch's documents saw the lining and nothing else: the Formal
// preview listed six and the patch had one.
//
// Members-only, the default every new charter gets (docs/adr/036): a founder
// publishes a document deliberately, at Governance → Documents, and a
// template's text is a starting point the patch has not yet said it means.
// The lining is not touched here; CreateDefaultLining makes it, public.
//
// The git side needs nothing: ForkForNode writes each body at
// Filename(title), byte for byte what these rows hold. Where the fork failed,
// the rows stand anyway, and the startup repair builds the repo from them.
func CreateTemplateDocs(db *database.DB, nodeID, template, userID string) {
	for _, d := range governance.TemplateDocuments(template) {
		if _, err := db.Exec(
			`INSERT INTO governance_docs (id, node_id, title, body, visibility, created_by) VALUES (?, ?, ?, ?, 'members', ?)`,
			auth.NewUUIDv7(), nodeID, d.Title, d.Body, userID,
		); err != nil {
			log.Printf("template docs: %s for node %s: %v", d.Title, nodeID, err)
		}
	}
}
