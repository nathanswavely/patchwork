package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/gazetteer"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

// SuggestPlace handles GET /api/v1/gazetteer/suggest?q=...
//
// It answers with one suggested placement, or with a body saying there is
// none. Not-found is 200 with `found: false`, not 404: a miss is the ordinary
// answer for a valid address (docs/adr/082), and a status code that reads as
// an error would put a broken-looking console message under a form that is
// working exactly as designed.
//
// The same shape answers an instance with no gazetteer installed, so the
// frontend has one code path rather than two.
func SuggestPlace(g *gazetteer.Gazetteer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Authenticated callers only, and throttled. There is nothing
		// confidential in the index — it is built from ODbL data anybody can
		// download — so the limit is about load, not secrecy.
		if !middleware.GazetteerRateLimit(r) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		query := strings.TrimSpace(r.URL.Query().Get("q"))
		// A long string is not an address, and tokenizing one costs a query
		// per token. Addresses are short.
		if len(query) > 300 {
			query = query[:300]
		}

		resp := map[string]any{"found": false, "available": g != nil}
		if place, ok := g.Suggest(query); ok {
			resp["found"] = true
			resp["place"] = place
			resp["label"] = place.Label()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
