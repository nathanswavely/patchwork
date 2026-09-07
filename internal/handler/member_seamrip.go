package handler

import (
	"archive/zip"
	"fmt"
	"net/http"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/seamrip"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// Member seamrip (docs/adr/089, docs/adr/012 affordance 2): an export of the
// requesting member's own VIEW of the quilt, in the format cmd/import
// already reads, so any member can seed a fork without asking anybody.
//
// The affordance exists because the only whole-instance export was
// admin-gated, which left the exact scenario seamrip is for — leadership
// gone sideways — protected by nothing but the deployment rule of never
// running a single admin. The admin export stays admin-gated, because it
// carries emails and hidden memberships and is therefore a custody
// transfer. This one carries neither.
//
// Nothing about which rows leave is decided here. The whole rule set lives
// on the boundary in internal/seamrip, one stated member-view rule per
// travelling table, and this handler runs it for the session's user. A
// visibility question answered in this file would be a second copy of a rule
// that has to be right once.
//
// The zip is the admin export's layout, file for file, plus a manifest that
// says which kind of bundle it is and who took it — a fork should be able to
// show where it came from, and the two bundles are not interchangeable.

// MemberSeamrip handles GET /api/v1/users/me/seamrip.
func MemberSeamrip(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.UserFromContext(r.Context())

		if !middleware.MemberSeamripRateLimit(r) {
			w.Header().Set("Retry-After", "3600")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"you have taken two copies of this quilt today, so the next one is tomorrow"}`))
			return
		}

		filename := fmt.Sprintf("patchwork-member-seamrip-%s-%s.zip",
			exportFilenameSafe(user.Username), time.Now().UTC().Format("2006-01-02"))
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

		zw := zip.NewWriter(w)
		defer zw.Close()

		// instance.json is the file cmd/import reads for the "seamripped
		// from" line, so it keeps its name and its shape. The kind and the
		// requester are new keys beside them: an importer that only knows
		// the admin export still finds what it looks for, and one that
		// looks can tell the two apart.
		writeJSONToZip(zw, "instance.json", map[string]interface{}{
			"name":        settings.EffectiveName(db, cfg),
			"description": settings.EffectiveDescription(db, cfg),
			"domain":      cfg.Instance.Domain,
			"version":     Version,
			"kind":        seamrip.KindMember,
		})
		writeJSONToZip(zw, "manifest.json", map[string]interface{}{
			"kind":         seamrip.KindMember,
			"requested_by": user.Username,
			"exported_at":  time.Now().UTC().Format(time.RFC3339),
			"instance": map[string]interface{}{
				"name":   settings.EffectiveName(db, cfg),
				"domain": cfg.Instance.Domain,
			},
			"version": Version,
		})

		if err := seamrip.MemberExport(db, user.ID, func(t seamrip.Table, items []map[string]any) error {
			writeJSONToZip(zw, t.File, items)
			return nil
		}); err != nil {
			// Headers are already sent and the zip is truncated, so the
			// failure is reported inside the archive where the person who
			// downloaded it will meet it.
			f, cerr := zw.Create("EXPORT_FAILED.txt")
			if cerr == nil {
				fmt.Fprintf(f, "member seamrip aborted: %v\n", err)
			}
			return
		}

		readme, err := zw.Create("README.txt")
		if err == nil {
			fmt.Fprint(readme, seamrip.MemberReadmeText)
		}

		auth.LogAuditEvent(db, user.ID, "user.seamrip", "user", user.ID, "{}", clientIP(r))
	}
}
