package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // register decoders for DecodeConfig
	_ "image/png"
	"io"
	"log"
	"mime"
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

// Icon upload constraints. Stated verbatim in the admin UI; enforced here.
const (
	iconMaxBytes = 512 * 1024
	iconMinPx    = 64
	iconMaxPx    = 1024
)

// iconState describes the effective quilt icon for API responses.
type iconState struct {
	Kind       string `json:"kind"`        // "upload" | "default"
	DefaultKey string `json:"default_key"` // effective block key when kind=default
	Chosen     bool   `json:"chosen"`      // default_key was picked (vs hash-assigned)
	Mime       string `json:"mime,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

func currentIconState(db *database.DB, cfg *config.Config) iconState {
	if mimeType, _, updatedAt, ok := settings.Icon(db); ok {
		return iconState{Kind: "upload", Mime: mimeType, UpdatedAt: updatedAt}
	}
	if key, ok := settings.Get(db, settings.KeyIconDefault); ok && key != "" {
		if _, valid := iconBlocks[key]; valid {
			return iconState{Kind: "default", DefaultKey: key, Chosen: true}
		}
	}
	return iconState{Kind: "default", DefaultKey: assignedIconBlock(settings.EffectiveName(db, cfg)), Chosen: false}
}

// AdminGetSettings handles GET /api/v1/admin/settings.
func AdminGetSettings(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, nameOverridden := settings.Get(db, settings.KeyName)
		_, descOverridden := settings.Get(db, settings.KeyDescription)
		hideAmended, _ := settings.Get(db, settings.KeyHideAmendedLinings)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":                   settings.EffectiveName(db, cfg),
			"description":            settings.EffectiveDescription(db, cfg),
			"domain":                 cfg.Instance.Domain,
			"name_overridden":        nameOverridden,
			"description_overridden": descOverridden,
			"hide_amended_linings":   hideAmended == "true",
			"icon":                   currentIconState(db, cfg),
			"default_icons":          iconBlockKeys(),
			"icon_constraints": map[string]interface{}{
				"formats":   []string{"image/png", "image/jpeg"},
				"max_bytes": iconMaxBytes,
				"min_px":    iconMinPx,
				"max_px":    iconMaxPx,
				"square":    true,
			},
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
			IconDefault *string `json:"icon_default"`
			// Quilt policy: hide amended-lining patches from discovery for
			// everyone (docs/adr/037).
			HideAmendedLinings *bool `json:"hide_amended_linings"`
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

		if req.IconDefault != nil {
			key := *req.IconDefault
			if key == "" {
				if err := settings.Unset(db, settings.KeyIconDefault); err != nil {
					http.Error(w, `{"error":"failed to clear icon choice"}`, http.StatusInternalServerError)
					return
				}
			} else {
				if _, ok := iconBlocks[key]; !ok {
					http.Error(w, `{"error":"unknown default icon"}`, http.StatusBadRequest)
					return
				}
				if err := settings.Set(db, settings.KeyIconDefault, key); err != nil {
					http.Error(w, `{"error":"failed to save icon choice"}`, http.StatusInternalServerError)
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

// AdminUploadIcon handles PUT /api/v1/admin/settings/icon.
// Raw image body: PNG or JPEG, square, 64-1024px, <=512KB. Dimensions are
// checked with image.DecodeConfig (header parse only — the server never
// decodes or resizes pixels; docs/adr/014).
func AdminUploadIcon(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())

		ct := r.Header.Get("Content-Type")
		if mt, _, err := mime.ParseMediaType(ct); err == nil {
			ct = mt
		}
		if ct != "image/png" && ct != "image/jpeg" {
			http.Error(w, `{"error":"icon must be a PNG or JPEG image"}`, http.StatusBadRequest)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, iconMaxBytes))
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"icon must be %dKB or smaller"}`, iconMaxBytes/1024), http.StatusRequestEntityTooLarge)
			return
		}

		cfgImg, format, err := image.DecodeConfig(bytes.NewReader(body))
		if err != nil {
			http.Error(w, `{"error":"could not read image — is it a valid PNG or JPEG?"}`, http.StatusBadRequest)
			return
		}
		wantFormat := map[string]string{"image/png": "png", "image/jpeg": "jpeg"}[ct]
		if format != wantFormat {
			http.Error(w, `{"error":"image data does not match its declared format"}`, http.StatusBadRequest)
			return
		}
		if cfgImg.Width != cfgImg.Height {
			http.Error(w, `{"error":"icon must be square"}`, http.StatusBadRequest)
			return
		}
		if cfgImg.Width < iconMinPx || cfgImg.Width > iconMaxPx {
			http.Error(w, fmt.Sprintf(`{"error":"icon must be between %d and %d pixels"}`, iconMinPx, iconMaxPx), http.StatusBadRequest)
			return
		}

		if err := settings.SetIcon(db, ct, body); err != nil {
			http.Error(w, `{"error":"failed to save icon"}`, http.StatusInternalServerError)
			return
		}

		auth.LogAuditEvent(db, adminUser.ID, "admin.instance_icon_upload", "instance", "", "{}", clientIP(r))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// AdminDeleteIcon handles DELETE /api/v1/admin/settings/icon.
func AdminDeleteIcon(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())
		if err := settings.DeleteIcon(db); err != nil {
			http.Error(w, `{"error":"failed to remove icon"}`, http.StatusInternalServerError)
			return
		}
		auth.LogAuditEvent(db, adminUser.ID, "admin.instance_icon_delete", "instance", "", "{}", clientIP(r))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// InstanceIcon handles GET /api/v1/instance/icon — the public quilt icon.
// Serves the uploaded image when one exists, otherwise a server-generated
// default SVG block. ?block=<key> previews a specific default (used by the
// admin picker). Cross-quilt <img> loads need no CORS; the multi_quilt
// CORS middleware additionally covers fetch() consumers.
func InstanceIcon(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serveSVG := func(key string) {
			svg, ok := renderIconBlock(key, cfg.Branding.Color)
			if !ok {
				http.Error(w, `{"error":"unknown icon block"}`, http.StatusNotFound)
				return
			}
			etag := fmt.Sprintf(`"block-%s-%s"`, key, cfg.Branding.Color)
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

		if block := r.URL.Query().Get("block"); block != "" {
			serveSVG(block)
			return
		}

		if mimeType, data, updatedAt, ok := settings.Icon(db); ok {
			etag := fmt.Sprintf(`"upload-%s"`, updatedAt)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("Content-Type", mimeType)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "public, max-age=300")
			w.Header().Set("ETag", etag)
			w.Write(data)
			return
		}

		state := currentIconState(db, cfg)
		serveSVG(state.DefaultKey)
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
