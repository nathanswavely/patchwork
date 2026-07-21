package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// Legal documents (docs/adr/028): the privacy policy and user agreement.
// Defaults ship in the binary so no deployment is ever bare; an instance
// admin can replace either wholesale. Public reads — like the Label,
// their most important reader has no account yet.

// legalMaxBytes caps a custom document. Generous: real policies run a few
// dozen KB at most; the cap only exists to keep the settings table sane.
const legalMaxBytes = 200 * 1024

type legalDocDef struct {
	Key      string
	Title    string
	Template string
}

// legalDocByPath maps the {doc} path segment to its definition. The path
// words are the user-facing URL nouns (/privacy, /terms); everything
// else calls the documents by their titles.
var legalDocByPath = map[string]legalDocDef{
	"privacy": {settings.KeyLegalPrivacy, "Privacy Policy", defaultPrivacyPolicy},
	"terms":   {settings.KeyLegalTerms, "User Agreement", defaultUserAgreement},
}

// renderLegalDefault substitutes deployment identity into a shipped
// template so a rename never strands a stale name in a legal document.
func renderLegalDefault(db *database.DB, cfg *config.Config, def legalDocDef) string {
	return strings.NewReplacer(
		"{quilt_name}", settings.EffectiveName(db, cfg),
		"{domain}", cfg.Instance.Domain,
	).Replace(def.Template)
}

// effectiveLegal returns the document as the public page should render
// it: the stored override when one exists, the rendered default
// otherwise. updatedAt is empty for defaults — the default has no local
// edit history to claim.
func effectiveLegal(db *database.DB, cfg *config.Config, def legalDocDef) (markdown string, customized bool, updatedAt string) {
	if v, at, ok := settings.GetDetailed(db, def.Key); ok && strings.TrimSpace(v) != "" {
		return v, true, at
	}
	return renderLegalDefault(db, cfg, def), false, ""
}

// LegalDoc handles GET /api/v1/legal/{doc} — public, no auth.
func LegalDoc(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		def, ok := legalDocByPath[r.PathValue("doc")]
		if !ok {
			http.Error(w, `{"error":"unknown document"}`, http.StatusNotFound)
			return
		}
		markdown, customized, updatedAt := effectiveLegal(db, cfg, def)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"doc":        r.PathValue("doc"),
			"title":      def.Title,
			"markdown":   markdown,
			"customized": customized,
			"updated_at": updatedAt,
		})
	}
}

// AdminGetLegal handles GET /api/v1/admin/legal — both documents plus
// their rendered defaults, so the editor can show what "restore default"
// would restore.
func AdminGetLegal(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docs := []map[string]interface{}{}
		for _, path := range []string{"privacy", "terms"} {
			def := legalDocByPath[path]
			markdown, customized, updatedAt := effectiveLegal(db, cfg, def)
			docs = append(docs, map[string]interface{}{
				"doc":              path,
				"title":            def.Title,
				"markdown":         markdown,
				"customized":       customized,
				"updated_at":       updatedAt,
				"default_markdown": renderLegalDefault(db, cfg, def),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"docs": docs})
	}
}

// AdminUpdateLegal handles PUT /api/v1/admin/legal/{doc} — replace a
// document with custom markdown.
func AdminUpdateLegal(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())

		def, ok := legalDocByPath[r.PathValue("doc")]
		if !ok {
			http.Error(w, `{"error":"unknown document"}`, http.StatusNotFound)
			return
		}

		var req struct {
			Markdown string `json:"markdown"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		md := strings.TrimSpace(req.Markdown)
		if md == "" {
			// An empty legal document is a hole, not a choice — resetting
			// to the shipped default is the explicit DELETE route.
			http.Error(w, `{"error":"document cannot be empty — use reset to restore the default"}`, http.StatusBadRequest)
			return
		}
		if len(md) > legalMaxBytes {
			http.Error(w, `{"error":"document is too large"}`, http.StatusBadRequest)
			return
		}

		if err := settings.Set(db, def.Key, md); err != nil {
			http.Error(w, `{"error":"failed to save document"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, adminUser.ID, "admin.legal_update", "instance", r.PathValue("doc"), "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// AdminResetLegal handles DELETE /api/v1/admin/legal/{doc} — drop the
// custom document and return to the shipped default.
func AdminResetLegal(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())

		def, ok := legalDocByPath[r.PathValue("doc")]
		if !ok {
			http.Error(w, `{"error":"unknown document"}`, http.StatusNotFound)
			return
		}
		if err := settings.Unset(db, def.Key); err != nil {
			http.Error(w, `{"error":"failed to reset document"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, adminUser.ID, "admin.legal_reset", "instance", r.PathValue("doc"), "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
