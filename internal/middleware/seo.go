package middleware

import (
	"fmt"
	"html"
	"net/http"
	"path"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// SEO wraps a SPA handler to inject Open Graph meta tags into index.html
// for crawler-friendly rendering. It reads the SPA HTML once at startup
// and performs string replacement per request. Instance name/description
// are read per request so admin-UI renames (docs/adr/014) apply without a
// restart.
func SEO(db *database.DB, cfg *config.Config, spaHTML []byte) func(http.Handler) http.Handler {
	template := string(spaHTML)
	domain := cfg.Instance.Domain

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := r.URL.Path

			// Pass through API routes and static assets unchanged.
			if strings.HasPrefix(p, "/api/") {
				next.ServeHTTP(w, r)
				return
			}

			ext := path.Ext(p)
			if ext != "" && ext != ".html" {
				next.ServeHTTP(w, r)
				return
			}

			instanceName := settings.EffectiveName(db, cfg)
			instanceDesc := settings.EffectiveDescription(db, cfg)

			// Determine OG tags based on route.
			ogTitle := instanceName
			ogDesc := instanceDesc
			ogURL := "https://" + domain + p
			ogType := "website"

			segments := strings.Split(strings.Trim(p, "/"), "/")

			if len(segments) >= 2 && segments[0] == "patches" {
				slug := segments[1]
				var name, desc string
				err := db.QueryRow(
					"SELECT name, COALESCE(description, '') FROM nodes WHERE slug = ? AND status IN ('active','unclaimed') AND removed_at IS NULL",
					slug,
				).Scan(&name, &desc)
				if err == nil && name != "" {
					ogTitle = name + " — " + instanceName
					if desc != "" {
						ogDesc = desc
					}
				}
			} else if len(segments) >= 2 && segments[0] == "events" {
				id := segments[1]
				var title, desc string
				err := db.QueryRow(
					"SELECT title, COALESCE(description, '') FROM events WHERE id = ?",
					id,
				).Scan(&title, &desc)
				if err == nil && title != "" {
					ogTitle = title + " — " + instanceName
					if desc != "" {
						ogDesc = desc
					}
				}
			}

			// Truncate description for OG tags.
			if len(ogDesc) > 200 {
				ogDesc = ogDesc[:197] + "..."
			}

			// Build the OG meta tags.
			ogTags := fmt.Sprintf(
				`<meta property="og:title" content="%s" />`+"\n"+
					`    <meta property="og:description" content="%s" />`+"\n"+
					`    <meta property="og:url" content="%s" />`+"\n"+
					`    <meta property="og:type" content="%s" />`+"\n"+
					`    <meta property="og:site_name" content="%s" />`,
				html.EscapeString(ogTitle),
				html.EscapeString(ogDesc),
				html.EscapeString(ogURL),
				ogType,
				html.EscapeString(instanceName),
			)

			// Inject OG tags before </head>.
			modified := strings.Replace(template, "</head>", ogTags+"\n  </head>", 1)

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(modified))
		})
	}
}
