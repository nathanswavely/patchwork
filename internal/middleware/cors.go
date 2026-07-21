package middleware

import (
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/config"
)

// CORS adds cross-origin headers to public GET endpoints when multi_quilt is
// enabled. It never applies CORS to authenticated or mutation endpoints.
func CORS(cfg *config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cfg.MultiQuilt {
			next.ServeHTTP(w, r)
			return
		}

		// Only apply CORS to API paths that are public GETs.
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Handle preflight.
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Only allow CORS on GET requests — never on mutations.
		if r.Method == http.MethodGet {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		next.ServeHTTP(w, r)
	})
}
