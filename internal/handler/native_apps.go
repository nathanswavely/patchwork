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
// publishes no handle_all_urls relation. Opening this quilt's URLs in an app
// was decided for Apple only
// (docs/adr/2026-10-18-a-listed-app-may-open-the-quilts-links.md); the
// Android half is a later decision on the same column.
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

// AdminSetNativeAppOpenLinks handles PATCH /api/v1/admin/native-apps/{id},
// whose one field is {"open_links": bool}: whether iPhones with this app
// installed open this quilt's links in it rather than in the browser
// (docs/adr/2026-10-18-a-listed-app-may-open-the-quilts-links.md).
//
// Admin, and deliberately no step-up. The grant that needed proof of presence
// was listing the app, which already let it hold this domain's passkeys; this
// switch names no new party and hands over no credential, it only says where
// a tapped link lands among apps the quilt already vouches for. Turning it
// off is always the safe direction. Both directions are audited.
func AdminSetNativeAppOpenLinks(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminUser := middleware.UserFromContext(r.Context())
		id := r.PathValue("id")

		var req struct {
			// A pointer, so a body that forgot the field is a 400 rather
			// than a quiet "turn it off".
			OpenLinks *bool `json:"open_links"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.OpenLinks == nil {
			writeJSONError(w, http.StatusBadRequest, "open_links is required")
			return
		}

		var platform, identifier string
		var current bool
		err := db.QueryRow(`SELECT platform, identifier, open_links FROM native_apps WHERE id = ?`, id).
			Scan(&platform, &identifier, &current)
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "no such app")
			return
		} else if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load the app")
			return
		}
		if platform != "apple" && *req.OpenLinks {
			// Android's handle_all_urls is a separate decision, not yet
			// made. Turning it off is allowed everywhere, since off is the
			// only state an Android row can be in.
			writeJSONError(w, http.StatusBadRequest, "only an Apple app can open this quilt's links for now")
			return
		}

		if current != *req.OpenLinks {
			if _, err := db.Exec(`UPDATE native_apps SET open_links = ? WHERE id = ?`, *req.OpenLinks, id); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "failed to update the app")
				return
			}
			// Only a change is an act. Setting the switch to where it
			// already is writes nothing and says nothing in the log.
			auth.LogAuditEventJSON(db, adminUser.ID, "admin.native_app_open_links", "native_app", id,
				map[string]interface{}{"platform": platform, "identifier": identifier, "open_links": *req.OpenLinks},
				clientIP(r))
		}

		apps, err := loadNativeApps(db, id)
		if err != nil || len(apps) == 0 {
			writeJSONError(w, http.StatusInternalServerError, "the app was updated but could not be read back")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(apps[0]); err != nil {
			log.Printf("native apps: write the updated row: %v", err)
		}
	}
}

// appleLinkComponent is one entry in an applinks "components" array, in the
// format Apple has read since iOS 13. "/" is matched against the URL's path:
// "*" is zero or more characters, "?" exactly one, and "?*" one or more.
// Both wildcards cross "/", so "/patches/?*" alone would claim every path
// under a patch. The system walks the array in order and the first match
// decides, which is what lets an exclusion stand in front of a wider claim.
type appleLinkComponent struct {
	Path    string `json:"/"`
	Exclude bool   `json:"exclude,omitempty"`
}

// appleLinkComponents is the path claim an opted-in Apple app makes on this
// quilt's links (docs/adr/2026-10-18-a-listed-app-may-open-the-quilts-links.md).
//
// It mirrors the native clients' route coverage: every path here is one the
// iOS client renders natively, and a web route the client does not handle is
// left to the browser. When a client learns a route, or the web grows one
// under a claimed prefix, this list changes with it. Matching against the
// SPA's own routes is in web/src/App.svelte.
//
// Order matters. First come the paths that stay the browser's whatever
// follows (writes, settings, sign-in, the admin panel, the API). Then the
// claim, in which each include ending in a "?*" parameter is preceded by an
// exclusion of anything deeper, because a wildcard crosses "/" and would
// otherwise claim sub-pages the client has never seen.
var appleLinkComponents = []appleLinkComponent{
	// Never the app's. Sign-in links (/login/link/{token}) are here on
	// purpose: routing them into an app is a later decision.
	{Path: "/api/*", Exclude: true},
	{Path: "/.well-known/*", Exclude: true},
	{Path: "/admin*", Exclude: true},
	{Path: "/settings*", Exclude: true},
	{Path: "/login*", Exclude: true},
	{Path: "/signup*", Exclude: true},
	{Path: "/welcome", Exclude: true},
	{Path: "/submit", Exclude: true},
	{Path: "/activity", Exclude: true},
	{Path: "/claims/*", Exclude: true},
	{Path: "/events/new", Exclude: true},
	{Path: "/events/?*/edit", Exclude: true},
	{Path: "/patches/new", Exclude: true},
	{Path: "/patches/?*/settings*", Exclude: true},
	{Path: "/patches/?*/manage*", Exclude: true},
	{Path: "/patches/?*/governance/new", Exclude: true},
	{Path: "/patches/?*/governance/rules*", Exclude: true},
	{Path: "/patches/?*/governance/record", Exclude: true},
	{Path: "/patches/?*/governance/docs/new", Exclude: true},
	{Path: "/patches/?*/governance/docs/?*/*", Exclude: true}, // propose, history
	{Path: "/patches/?*/noticeboard/new", Exclude: true},

	// The quilt's own pages, matched exactly.
	{Path: "/"},
	{Path: "/my"},
	{Path: "/map"},
	{Path: "/map/my"},
	{Path: "/events"},
	{Path: "/events/my"},
	{Path: "/discover"},
	{Path: "/dashboard"},
	{Path: "/notifications"},
	{Path: "/about"},
	{Path: "/label"},
	{Path: "/lining"},
	{Path: "/governance"},
	{Path: "/privacy"},
	{Path: "/terms"},

	// One event, one person, one invite, one patch on another quilt.
	{Path: "/events/?*/*", Exclude: true},
	{Path: "/events/?*"},
	{Path: "/users/?*/*", Exclude: true},
	{Path: "/users/?*"},
	{Path: "/invite/?*/*", Exclude: true},
	{Path: "/invite/?*"},
	{Path: "/quilts/?*/patches/?*/*", Exclude: true},
	{Path: "/quilts/?*/patches/?*"},

	// A patch and the pages under it the client renders.
	{Path: "/patches/?*/events"},
	{Path: "/patches/?*/members"},
	{Path: "/patches/?*/governance"},
	{Path: "/patches/?*/noticeboard"},
	{Path: "/patches/?*/claim"},
	{Path: "/patches/?*/setup"},
	// The governance lists are web pages; one proposal or one document is
	// the client's.
	{Path: "/patches/?*/governance/docs", Exclude: true},
	{Path: "/patches/?*/governance/proposals", Exclude: true},
	{Path: "/patches/?*/governance/proposals/?*/*", Exclude: true},
	{Path: "/patches/?*/governance/proposals/?*"},
	{Path: "/patches/?*/governance/docs/?*"},
	{Path: "/patches/?*/governance/?*/*", Exclude: true},
	{Path: "/patches/?*/governance/?*"},
	{Path: "/patches/?*/noticeboard/?*/*", Exclude: true},
	{Path: "/patches/?*/noticeboard/?*"},
	// Anything else under a patch (the legacy redirects among them) stays
	// the browser's, and then the patch page itself.
	{Path: "/patches/?*/*", Exclude: true},
	{Path: "/patches/?*"},
}

// AppleAppSiteAssociation handles GET /.well-known/apple-app-site-association.
//
// Public and unauthenticated, because the fetcher is an app store's, which
// has no account here. Content-Type is application/json and the path carries
// no extension, which is exactly what Apple requires.
//
// The document always carries webcredentials for every listed Apple app. It
// carries an applinks block only when an admin has turned on "open this
// quilt's links" for at least one of them, and that block names only those
// apps (docs/adr/2026-10-18-a-listed-app-may-open-the-quilts-links.md):
// holding the passkeys and claiming the URLs are two grants.
func AppleAppSiteAssociation(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`SELECT identifier, open_links FROM native_apps WHERE platform = 'apple' ORDER BY created_at`)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load native apps")
			return
		}
		defer rows.Close()

		apps := []string{}
		linkApps := []string{}
		for rows.Next() {
			var identifier string
			var openLinks bool
			if rows.Scan(&identifier, &openLinks) == nil {
				apps = append(apps, identifier)
				if openLinks {
					linkApps = append(linkApps, identifier)
				}
			}
		}
		if len(apps) == 0 {
			// Not an empty document: a quilt that vouches for no Apple app
			// publishes no Apple file at all.
			http.NotFound(w, r)
			return
		}

		doc := map[string]interface{}{
			"webcredentials": map[string]interface{}{"apps": apps},
		}
		if len(linkApps) > 0 {
			doc["applinks"] = map[string]interface{}{
				"details": []map[string]interface{}{{
					"appIDs":     linkApps,
					"components": appleLinkComponents,
				}},
			}
		}

		w.Header().Set("Cache-Control", associationCacheControl)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(doc); err != nil {
			log.Printf("native apps: write %s: %v", AppleAssociationPath, err)
		}
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
	query := `SELECT id, platform, identifier, fingerprints, label, open_links, created_at
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
		if err := rows.Scan(&app.ID, &app.Platform, &app.Identifier, &encoded, &app.Label, &app.OpenLinks, &app.CreatedAt); err != nil {
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
