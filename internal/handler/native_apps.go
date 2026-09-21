package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// The apps this quilt vouches for
// (docs/adr/2026-09-20-an-instance-vouches-for-an-app.md).
//
// A native app may hold a domain's passkeys only if the domain says so in a
// file it publishes itself. This file is both halves of that: the admin list
// where an instance decides, and the two well-known files rendered from what
// it decided. Nothing is listed unless an admin lists it, and an instance
// that has listed nothing serves neither file — a 404, which is what the
// platforms read as "this domain vouches for nobody", and what keeps an
// instance that never asked for any of this free of it.

// AppleAssociationPath and AndroidAssociationPath are the two addresses the
// platforms fetch. They are fixed by Apple and Google, not by us.
const (
	AppleAssociationPath   = "/.well-known/apple-app-site-association"
	AndroidAssociationPath = "/.well-known/assetlinks.json"
)

// androidLoginCredsRelation is the one permission an assetlinks entry
// delegates here: may this app use the domain's login credentials. Patchwork
// publishes no app-links relation — opening a Patchwork URL in an app is a
// separate decision nobody has made.
const androidLoginCredsRelation = "delegate_permission/common.get_login_creds"

// associationCacheControl is an hour. Apple's own fetcher caches the file for
// up to a day regardless; this bounds everyone else's.
const associationCacheControl = "public, max-age=3600"

// AdminListNativeApps handles GET /api/v1/admin/native-apps.
//
// It answers the two published addresses alongside the rows, because the
// question an admin has after listing an app is "where do I point the app at
// this", and the answer is derivable from the configured domain and should
// not have to be assembled by hand in the page.
func AdminListNativeApps(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apps, err := loadNativeApps(db, "")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load native apps")
			return
		}
		base := "https://" + cfg.Instance.Domain
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"native_apps": apps,
			"urls": map[string]string{
				"apple":   base + AppleAssociationPath,
				"android": base + AndroidAssociationPath,
			},
		})
	}
}

// AdminAddNativeApp handles POST /api/v1/admin/native-apps.
//
// Mounted behind AdminRequired and SudoRequired. Step-up (docs/adr/017)
// because listing an app is a grant outward: from the moment the identifier
// is published, that app may ask a phone for this domain's passkeys, which is
// the same class of act as pointing an account at a mailbox (docs/adr/072).
// A valid cookie proves identity; what a grant like this needs is proof of
// presence.
func AdminAddNativeApp(db *database.DB, wa *auth.WebAuthnService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())

		var req struct {
			Platform     string   `json:"platform"`
			Identifier   string   `json:"identifier"`
			Fingerprints []string `json:"fingerprints"`
			Label        string   `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		platform := strings.ToLower(strings.TrimSpace(req.Platform))
		var identifier string
		fingerprints := []string{}
		switch platform {
		case "apple":
			id, err := auth.ValidateAppleAppID(req.Identifier)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			identifier = id
			// An Apple row carries no fingerprints even if some were sent:
			// the file Apple reads has nowhere to put them, so storing them
			// would be a fact nothing ever uses.
		case "android":
			id, err := auth.ValidateAndroidPackage(req.Identifier)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			identifier = id
			seen := map[string]bool{}
			for _, raw := range req.Fingerprints {
				if strings.TrimSpace(raw) == "" {
					continue
				}
				fp, err := auth.NormalizeFingerprint(raw)
				if err != nil {
					writeJSONError(w, http.StatusBadRequest, err.Error())
					return
				}
				if seen[fp] {
					continue
				}
				seen[fp] = true
				fingerprints = append(fingerprints, fp)
			}
			if len(fingerprints) == 0 {
				// Without one, the file publishes a package name bound to no
				// key, and Android will not use it. An empty list here is a
				// listing that silently does nothing.
				writeJSONError(w, http.StatusBadRequest, "an Android app needs at least one signing-certificate fingerprint")
				return
			}
		default:
			writeJSONError(w, http.StatusBadRequest, "platform must be apple or android")
			return
		}

		var existing string
		err := db.QueryRow(`SELECT id FROM native_apps WHERE platform = ? AND identifier = ?`,
			platform, identifier).Scan(&existing)
		if err == nil {
			writeJSONError(w, http.StatusConflict, "this quilt already vouches for that app")
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusInternalServerError, "failed to check for an existing listing")
			return
		}

		encoded, err := json.Marshal(fingerprints)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to encode fingerprints")
			return
		}
		label := auth.SanitizeNativeAppLabel(req.Label)

		id := auth.NewUUIDv7()
		_, err = db.Exec(
			`INSERT INTO native_apps (id, platform, identifier, fingerprints, label, added_by)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			id, platform, identifier, string(encoded), label, adminUser.ID,
		)
		if err != nil {
			// The unique index is the last word, in case two admins listed
			// the same app at once.
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				writeJSONError(w, http.StatusConflict, "this quilt already vouches for that app")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "failed to list the app")
			return
		}

		auth.LogAuditEventJSON(db, adminUser.ID, "admin.native_app_add", "native_app", id,
			map[string]string{"platform": platform, "identifier": identifier}, clientIP(r))

		// A listed Android app signs in from an apk-key-hash origin, so the
		// relying party has to learn it before the app is any use. Failing to
		// reload is logged and not fatal: the row is written and published,
		// and the next boot picks the origin up.
		reloadNativeAppOrigins(db, wa)

		apps, err := loadNativeApps(db, id)
		if err != nil || len(apps) == 0 {
			writeJSONError(w, http.StatusInternalServerError, "the app was listed but could not be read back")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(apps[0])
	}
}

// AdminDeleteNativeApp handles DELETE /api/v1/admin/native-apps/{id}.
//
// Admin, and deliberately no step-up: removing trust is the safe direction,
// and a gate on it is a gate between an admin and undoing a mistake.
func AdminDeleteNativeApp(db *database.DB, wa *auth.WebAuthnService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())
		id := r.PathValue("id")

		var platform, identifier string
		err := db.QueryRow(`SELECT platform, identifier FROM native_apps WHERE id = ?`, id).
			Scan(&platform, &identifier)
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "no such app")
			return
		} else if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load the app")
			return
		}

		if _, err := db.Exec(`DELETE FROM native_apps WHERE id = ?`, id); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to remove the app")
			return
		}

		auth.LogAuditEventJSON(db, adminUser.ID, "admin.native_app_remove", "native_app", id,
			map[string]string{"platform": platform, "identifier": identifier}, clientIP(r))

		reloadNativeAppOrigins(db, wa)

		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"status": "ok"})
	}
}

// AppleAppSiteAssociation handles GET /.well-known/apple-app-site-association.
//
// Public and unauthenticated, because the fetcher is an app store's, which
// has no account here. Content-Type is application/json and the path carries
// no extension, which is exactly what Apple requires.
//
// The document carries webcredentials and nothing else. An applinks block
// would tell every iPhone to open this quilt's URLs in an app, which is a
// different decision from "this app may hold our passkeys" and has not been
// made.
func AppleAppSiteAssociation(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`SELECT identifier FROM native_apps WHERE platform = 'apple' ORDER BY created_at`)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load native apps")
			return
		}
		defer rows.Close()

		apps := []string{}
		for rows.Next() {
			var identifier string
			if rows.Scan(&identifier) == nil {
				apps = append(apps, identifier)
			}
		}
		if len(apps) == 0 {
			// Not an empty document: a quilt that vouches for no Apple app
			// publishes no Apple file at all.
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Cache-Control", associationCacheControl)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"webcredentials": map[string]interface{}{"apps": apps},
		})
	}
}

// AssetLinks handles GET /.well-known/assetlinks.json.
//
// The Android half: a JSON array, one statement per listed app, each
// delegating get_login_creds to that package signed by those certificates.
// Same public mount and the same 404-when-empty as the Apple file.
func AssetLinks(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apps, err := loadNativeApps(db, "")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load native apps")
			return
		}

		statements := []map[string]interface{}{}
		for _, app := range apps {
			if app.Platform != "android" {
				continue
			}
			statements = append(statements, map[string]interface{}{
				"relation": []string{androidLoginCredsRelation},
				"target": map[string]interface{}{
					"namespace":                "android_app",
					"package_name":             app.Identifier,
					"sha256_cert_fingerprints": app.Fingerprints,
				},
			})
		}
		if len(statements) == 0 {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Cache-Control", associationCacheControl)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statements)
	}
}

// loadNativeApps reads the listed apps, oldest first, or the single row whose
// id is given. Fingerprints come back as a decoded array so nothing above
// this line has to know they are stored as JSON.
func loadNativeApps(db *database.DB, id string) ([]model.NativeApp, error) {
	query := `SELECT id, platform, identifier, fingerprints, label, created_at
	          FROM native_apps`
	args := []interface{}{}
	if id != "" {
		query += ` WHERE id = ?`
		args = append(args, id)
	}
	query += ` ORDER BY platform, created_at`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	apps := []model.NativeApp{}
	for rows.Next() {
		var app model.NativeApp
		var encoded string
		if err := rows.Scan(&app.ID, &app.Platform, &app.Identifier, &encoded, &app.Label, &app.CreatedAt); err != nil {
			continue
		}
		app.Fingerprints = []string{}
		if encoded != "" {
			_ = json.Unmarshal([]byte(encoded), &app.Fingerprints)
			if app.Fingerprints == nil {
				app.Fingerprints = []string{}
			}
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

// reloadNativeAppOrigins re-derives the extra WebAuthn origins and hands them
// to the relying party. Called after every add and remove; the same call
// happens once at startup in cmd/patchwork.
func reloadNativeAppOrigins(db *database.DB, wa *auth.WebAuthnService) {
	if wa == nil {
		return
	}
	origins, err := auth.NativeAppOrigins(db)
	if err != nil {
		log.Printf("native apps: could not derive sign-in origins: %v", err)
		return
	}
	if err := wa.Reconfigure(origins); err != nil {
		log.Printf("native apps: could not reload sign-in origins: %v", err)
	}
}
