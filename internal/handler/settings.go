package handler

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/governance"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// Quilt settings (docs/adr/014): community identity — name, description,
// icon — is database state editable by the instance admin; deployment
// concerns stay in patchwork.yaml.

// iconState describes the effective quilt icon for API responses: the
// design that is being served, and whether an admin drafted it or the
// quilt was assigned one (docs/adr/043).
type iconState struct {
	Chosen bool       `json:"chosen"`
	Design iconDesign `json:"design"`
}

func currentIconState(db *database.DB, cfg *config.Config) iconState {
	if raw, ok := settings.Get(db, settings.KeyIconDesign); ok && raw != "" {
		if d, valid := parseIconDesign(raw); valid {
			return iconState{Chosen: true, Design: d}
		}
	}
	return iconState{Design: iconDesign{
		Block:  startersByKey[assignedStarter(settings.EffectiveName(db, cfg))],
		Bundle: defaultBundle(cfg.Branding.Color),
	}}
}

// AdminGetSettings handles GET /api/v1/admin/settings.
func AdminGetSettings(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, nameOverridden := settings.Get(db, settings.KeyName)
		_, descOverridden := settings.Get(db, settings.KeyDescription)
		hideAmended, _ := settings.Get(db, settings.KeyHideAmendedLinings)
		_, tzOverridden := settings.Get(db, settings.KeyTimezone)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":                   settings.EffectiveName(db, cfg),
			"description":            settings.EffectiveDescription(db, cfg),
			"domain":                 cfg.Instance.Domain,
			"name_overridden":        nameOverridden,
			"description_overridden": descOverridden,
			"hide_amended_linings":   hideAmended == "true",
			// Where this quilt keeps time (docs/adr/045) — the rung an
			// event's zone falls through to when neither it nor its patch
			// names one. Editable here because getting it wrong shows up
			// as every event being hours off, and that should not wait on
			// a redeploy.
			"timezone":            settings.EffectiveTimezone(db),
			"timezone_overridden": tzOverridden,
			"timezone_configured": cfg.Timezone(),
			"icon":                currentIconState(db, cfg),
			"icon_starters":       starters,
		})
	}
}

// AdminUpdateSettings handles PATCH /api/v1/admin/settings.
func AdminUpdateSettings(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())

		var req struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
			// IconDesign is a drafted block plus its fabrics; explicit
			// null clears it and the quilt goes back to an assigned block
			// (docs/adr/043). Raw so absent and null stay distinguishable.
			IconDesign json.RawMessage `json:"icon_design"`
			// Quilt policy: hide amended-lining patches from discovery for
			// everyone (docs/adr/037).
			HideAmendedLinings *bool `json:"hide_amended_linings"`
			// The quilt's zone (docs/adr/045). Empty string clears the
			// override and falls back to geographic.timezone.
			Timezone *string `json:"timezone"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		if req.Name != nil {
			name := strings.TrimSpace(*req.Name)
			if name == "" {
				http.Error(w, `{"error":"name cannot be empty"}`, http.StatusBadRequest)
				return
			}
			if len(name) > 100 {
				http.Error(w, `{"error":"name must be 100 characters or fewer"}`, http.StatusBadRequest)
				return
			}
			if err := settings.Set(db, settings.KeyName, name); err != nil {
				http.Error(w, `{"error":"failed to save name"}`, http.StatusInternalServerError)
				return
			}
		}

		if req.Description != nil {
			desc := strings.TrimSpace(*req.Description)
			if len(desc) > 500 {
				http.Error(w, `{"error":"description must be 500 characters or fewer"}`, http.StatusBadRequest)
				return
			}
			if err := settings.Set(db, settings.KeyDescription, desc); err != nil {
				http.Error(w, `{"error":"failed to save description"}`, http.StatusInternalServerError)
				return
			}
		}

		if req.Timezone != nil {
			tz := strings.TrimSpace(*req.Timezone)
			if tz == "" {
				if err := settings.Unset(db, settings.KeyTimezone); err != nil {
					http.Error(w, `{"error":"failed to clear the timezone"}`, http.StatusInternalServerError)
					return
				}
			} else if !settings.ValidTimezone(tz) {
				http.Error(w, `{"error":"timezone must be an IANA zone name, like America/New_York"}`, http.StatusBadRequest)
				return
			} else if err := settings.Set(db, settings.KeyTimezone, tz); err != nil {
				http.Error(w, `{"error":"failed to save the timezone"}`, http.StatusInternalServerError)
				return
			}
		}

		if len(req.IconDesign) > 0 {
			var decoded interface{}
			if err := json.Unmarshal(req.IconDesign, &decoded); err != nil {
				http.Error(w, `{"error":"invalid icon_design"}`, http.StatusBadRequest)
				return
			}
			if decoded == nil {
				if err := settings.Unset(db, settings.KeyIconDesign); err != nil {
					http.Error(w, `{"error":"failed to clear the icon"}`, http.StatusInternalServerError)
					return
				}
			} else {
				stored, err := normalizeIconDesign(decoded)
				if err != nil {
					http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
					return
				}
				if err := settings.Set(db, settings.KeyIconDesign, stored); err != nil {
					http.Error(w, `{"error":"failed to save the icon"}`, http.StatusInternalServerError)
					return
				}
			}
		}

		if req.HideAmendedLinings != nil {
			v := "false"
			if *req.HideAmendedLinings {
				v = "true"
			}
			if err := settings.Set(db, settings.KeyHideAmendedLinings, v); err != nil {
				http.Error(w, `{"error":"failed to save policy"}`, http.StatusInternalServerError)
				return
			}
		}

		auth.LogAuditEvent(db, adminUser.ID, "admin.instance_settings_update", "instance", "", "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"name":   settings.EffectiveName(db, cfg),
			"icon":   currentIconState(db, cfg),
		})
	}
}

// InstanceIcon handles GET /api/v1/instance/icon — the public quilt icon.
// Renders the drafted design to SVG (docs/adr/043); an instance that has
// not drafted one gets a starter block assigned from its name, so every
// quilt has an icon from first boot. Cross-quilt <img> loads need no
// CORS; the multi_quilt CORS middleware additionally covers fetch()
// consumers.
func InstanceIcon(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svg := renderIconSVG(currentIconState(db, cfg).Design, cfg.Branding.Color)
		etag := fmt.Sprintf(`"quilt-%08x"`, crc32.ChecksumIEEE([]byte(svg)))
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("ETag", etag)
		io.WriteString(w, svg)
	}
}

// AdminWipe handles POST /api/v1/admin/wipe — the danger zone.
// Erases every row of community data (docs/adr/014): patches, people,
// events, proposals, governance records, sessions — returning the
// deployment to first-run. The deployment itself (domain, config,
// container) survives. Requires the exact instance name as confirmation.
func AdminWipe(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())

		var req struct {
			ConfirmName string `json:"confirm_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		effectiveName := settings.EffectiveName(db, cfg)
		if req.ConfirmName != effectiveName {
			http.Error(w, `{"error":"confirmation name does not match the quilt name"}`, http.StatusBadRequest)
			return
		}

		// The audit log is wiped along with everything else, so the record
		// of who did this lives in the server log.
		log.Printf("DANGER: instance wipe requested by %s (id %s, ip %s) — erasing all community data",
			adminUser.Username, adminUser.ID, clientIP(r))

		if err := db.Wipe(r.Context()); err != nil {
			log.Printf("wipe failed: %v", err)
			http.Error(w, `{"error":"wipe failed — no data was deleted"}`, http.StatusInternalServerError)
			return
		}

		// Re-seed the sentinel system user (migration 015 created it and
		// migrations don't re-run): it owns unclaimed patches and the
		// bootstrap "first account becomes admin" rule already excludes it.
		if _, err := db.Exec(`INSERT OR IGNORE INTO users (id, username, display_name, role, bio, avatar_url)
			VALUES (?, '_system', 'Community', 'member', '', '')`, model.SystemUserID); err != nil {
			log.Printf("wipe: failed to re-seed system user: %v", err)
		}

		// Remove governance repos and re-initialize the instance baseline
		// so the running process matches a fresh boot. Best effort: the DB
		// wipe is already committed.
		dataDir := governance.GetDataDir()
		if dataDir != "" {
			if err := os.RemoveAll(filepath.Join(dataDir, "governance")); err != nil {
				log.Printf("wipe: failed to remove governance repos: %v", err)
			}
			if err := governance.InitInstanceRepo(dataDir); err != nil {
				log.Printf("wipe: failed to re-init instance governance repo: %v", err)
			}
		}

		log.Printf("wipe complete: instance is back to first-run — the next account created becomes the instance admin")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
