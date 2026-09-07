package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// maxMovedTo matches maxEventURL. A new home is an address somebody pasted,
// and the ones that come out of a browser bar carry query strings.
const maxMovedTo = 2048

// validateMovedTo checks a "we've moved" pointer (docs/adr/090).
//
// It is validateEventURL's rule with one clause added, and it is a separate
// function rather than a flag on that one because the clause is about this
// quilt rather than about the link: a pointer at this instance's own domain
// is a patch or a person saying they moved to where they already are, which
// is either a typo or a loop for anything that follows the field.
//
// http as well as https, for the same reason an event's link keeps it: a
// community that has just stood a server up somewhere cheap should not be
// told its new home is not a link. Empty is always fine and means no move.
func validateMovedTo(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) > maxMovedTo {
		return "that link is too long"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "that doesn't look like a link — it should start with https://"
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "that doesn't look like a link — it should start with https://"
	}
	if sameHost(u.Host, ap.GetDomain()) {
		return "a new home has to be somewhere else, and this link points back here"
	}
	return ""
}

// sameHost compares a pasted URL's host with this instance's domain. The
// port is part of it (a dev quilt on :8080 is a different host from one on
// :8081) and the case is not, because host names are case-insensitive and a
// capital letter is not a different quilt.
func sameHost(host, domain string) bool {
	return strings.EqualFold(strings.TrimSuffix(host, "."), strings.TrimSuffix(domain, "."))
}

// nodeMovedTo returns where a patch says it has gone, or "".
func nodeMovedTo(db *database.DB, nodeID string) string {
	var moved string
	db.QueryRow("SELECT COALESCE(moved_to,'') FROM nodes WHERE id = ?", nodeID).Scan(&moved)
	return moved
}

// movedAwayBody is the refusal a moved patch answers a join, a follow or an
// outsider's event with.
//
// It is written as a whole `{"error":"..."}` literal and then unwrapped,
// rather than typed as a bare message, because that shape is the one the
// copy ledger's extractor recognises inside a handler. A sentence hidden in
// a two-key literal would be a string a visitor reads with no review
// decision recorded against it, which is the exact gap the ledger exists to
// close.
const movedAwayBody = `{"error":"this patch has moved. Take part at its new home instead."}`

// writeMovedAway answers with the refusal and the address to go to. The
// pointer rides as its own field rather than only inside the sentence, so a
// client can render it as a link without parsing prose.
func writeMovedAway(w http.ResponseWriter, movedTo string) {
	msg := strings.TrimSuffix(strings.TrimPrefix(movedAwayBody, `{"error":"`), `"}`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(map[string]string{"error": msg, "moved_to": movedTo})
}
