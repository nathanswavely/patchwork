package middleware

import (
	"net/http"
	"strings"
)

// CSRF checks that mutation requests (POST, PUT, PATCH, DELETE) include
// the X-Patchwork-Request: true header. GET, HEAD, and OPTIONS are exempt.
//
// Federation endpoints are also exempt: they are reached by remote servers
// (which cannot send a browser-only custom header) and are authenticated by
// HTTP Signatures or git transport, not by the session cookie that CSRF
// protects. Specifically: ActivityPub paths (/ap/), WebFinger
// (/.well-known/), and the governance git smart-HTTP transport
// (.../governance.git/).
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			// Safe methods are exempt.
		default:
			if !isFederationPath(r.URL.Path) && r.Header.Get("X-Patchwork-Request") != "true" {
				http.Error(w, `{"error":"missing X-Patchwork-Request header"}`, http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// isFederationPath reports whether a request path is a server-to-server
// federation endpoint that must bypass cookie-based CSRF protection.
func isFederationPath(p string) bool {
	if strings.HasPrefix(p, "/ap/") || strings.HasPrefix(p, "/.well-known/") {
		return true
	}
	// Governance git smart-HTTP transport, e.g.
	// /api/v1/nodes/{slug}/governance.git/git-upload-pack
	if strings.Contains(p, "/governance.git/") {
		return true
	}
	return false
}
