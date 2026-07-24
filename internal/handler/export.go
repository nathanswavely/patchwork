package handler

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/seamrip"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// AdminExport handles GET /api/v1/admin/export.
// Returns a zip of the instance's portable data (see internal/seamrip for
// what travels and what deliberately does not).
func AdminExport(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=patchwork-export.zip")

		zw := zip.NewWriter(w)
		defer zw.Close()

		writeJSONToZip(zw, "instance.json", map[string]interface{}{
			"name":        settings.EffectiveName(db, cfg),
			"description": settings.EffectiveDescription(db, cfg),
			"domain":      cfg.Instance.Domain,
			"version":     Version,
		})

		if err := seamrip.Export(db, func(t seamrip.Table, items []map[string]any) error {
			writeJSONToZip(zw, t.File, items)
			return nil
		}); err != nil {
			// Headers are already sent; the zip is truncated. Log via the
			// archive itself so the failure is visible to the downloader.
			f, cerr := zw.Create("EXPORT_FAILED.txt")
			if cerr == nil {
				fmt.Fprintf(f, "export aborted: %v\n", err)
			}
			return
		}

		readme, err := zw.Create("README.txt")
		if err == nil {
			fmt.Fprint(readme, seamrip.ReadmeText)
		}
	}
}

// writeJSONToZip writes a JSON-encoded value to a named file in the zip archive.
func writeJSONToZip(zw *zip.Writer, name string, data interface{}) {
	f, err := zw.Create(name)
	if err != nil {
		return
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.Encode(data)
}
