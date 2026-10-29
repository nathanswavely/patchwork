package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/copystats"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// The AI page (docs/adr/2026-10-20-the-site-says-where-ai-is.md): where AI
// is and is not in this software, in two parts owned by two parties. The
// project part is ai_defaults.go and nobody here edits it; the stewards'
// part is one stored setting. Public, like the legal documents: its most
// important reader has no account yet.

// renderAIProject fills the project part's placeholders from the code they
// describe.
func renderAIProject(db *database.DB, cfg *config.Config) string {
	var outbound strings.Builder
	for _, o := range aiPageOutbound {
		outbound.WriteString("- " + o.Says + "\n")
	}
	crawlers := strings.Join(middleware.AICrawlerNames(), ", ")

	return strings.NewReplacer(
		"{quilt_name}", settings.EffectiveName(db, cfg),
		"{outbound}", strings.TrimRight(outbound.String(), "\n"),
		"{ai_crawlers}", crawlers+".",
	).Replace(aiPageProject)
}

// AIPage handles GET /api/v1/ai: public, no auth.
func AIPage(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		figures, err := copystats.Current()
		if err != nil {
			http.Error(w, `{"error":"failed to read the copy ledger"}`, http.StatusInternalServerError)
			return
		}
		stewards, updatedAt, _ := settings.GetDetailed(db, settings.KeyAIPageStewards)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"project_markdown":    renderAIProject(db, cfg),
			"copy_markdown":       aiPageCopyNote,
			"copy":                figures,
			"stewards_markdown":   strings.TrimSpace(stewards),
			"stewards_updated_at": updatedAt,
		})
	}
}

// AdminUpdateAIStewards handles PUT /api/v1/admin/ai-page: set the
// stewards' part. Clearing it is the DELETE route, as with the legal
// documents, so an empty body is a mistake rather than a choice.
func AdminUpdateAIStewards(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())

		var req struct {
			Markdown string `json:"markdown"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		md := strings.TrimSpace(req.Markdown)
		if md == "" {
			http.Error(w, `{"error":"text cannot be empty, use remove to clear it"}`, http.StatusBadRequest)
			return
		}
		if len(md) > legalMaxBytes {
			http.Error(w, `{"error":"text is too large"}`, http.StatusBadRequest)
			return
		}
		if err := settings.Set(db, settings.KeyAIPageStewards, md); err != nil {
			http.Error(w, `{"error":"failed to save"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, adminUser.ID, "admin.ai_page_update", "instance", "ai", "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// AdminResetAIStewards handles DELETE /api/v1/admin/ai-page: remove the
// stewards' part. The project part is untouched; it was never theirs.
func AdminResetAIStewards(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())

		if err := settings.Unset(db, settings.KeyAIPageStewards); err != nil {
			http.Error(w, `{"error":"failed to remove"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, adminUser.ID, "admin.ai_page_reset", "instance", "ai", "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
