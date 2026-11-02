package handler

import (
	"net/http"
	"strings"
)

// A list a person searches asks the server for the matches, because a page
// of the list filtered in the browser can only ever find the twenty rows it
// happened to load. The workspace finder did exactly that: it fetched one
// page of a patch's events, proposals and members and filtered them in
// memory, so on a venue with 264 events a search inside the patch never
// found anything past the twentieth.
//
// searchParam and containsPattern are the one way a list handler reads that
// question, so every list that takes `search` treats it the same.

// maxSearchRunes bounds the term. A finder sends what someone typed; nothing
// a person types into a search box needs more, and a LIKE over a long
// pattern is work the server does for nobody.
const maxSearchRunes = 100

// searchParam is the `search` query parameter, trimmed and bounded. Empty
// means no search was asked for.
func searchParam(r *http.Request) string {
	s := strings.TrimSpace(r.URL.Query().Get("search"))
	if runes := []rune(s); len(runes) > maxSearchRunes {
		s = string(runes[:maxSearchRunes])
	}
	return s
}

// likeEscape is the ESCAPE clause every containsPattern argument is used
// with. A query writes `col LIKE ?` + likeEscape.
const likeEscape = ` ESCAPE '\'`

// containsPattern turns a typed term into a LIKE pattern matching it
// anywhere, with the term's own % and _ taken literally: someone searching
// for "50%" means the characters, not "50 followed by anything".
//
// SQLite's LIKE folds case for ASCII letters only, so "jazz" finds "Jazz"
// but "tornado" does not find "Tornādo", which the old in-browser filter
// did. That is the price of asking the server, recorded here rather than
// discovered.
func containsPattern(term string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(term) + "%"
}
