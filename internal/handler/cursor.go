package handler

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// Keyset pagination is only correct when the cursor predicate compares the same
// tuple the query orders by. When a list sorts on a non-unique column (starts_at,
// created_at) the tuple has to be (sort key, id) — the id breaks ties so no row
// is served twice and none is skipped. These helpers pack that pair into one
// opaque cursor string and build the matching SQL predicate.

const cursorSep = "\x1f"

// encodeCursor packs a sort-key value and its row id into an opaque cursor.
func encodeCursor(sortKey, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(sortKey + cursorSep + id))
}

// decodeCursor unpacks a cursor produced by encodeCursor. ok is false for
// malformed input, in which case callers should ignore the cursor and serve the
// first page rather than emitting a predicate with garbage bounds.
func decodeCursor(cursor string) (sortKey, id string, ok bool) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(string(raw), cursorSep, 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// keysetCondition returns the SQL predicate for a composite (sortCol, idCol)
// cursor, matching an "ORDER BY sortCol, idCol" in the same direction. Bind
// sortKey, sortKey, id — in that order — to its three placeholders.
func keysetCondition(sortCol, idCol string, desc bool) string {
	cmp := ">"
	if desc {
		cmp = "<"
	}
	return fmt.Sprintf("(%s %s ? OR (%s = ? AND %s %s ?))", sortCol, cmp, sortCol, idCol, cmp)
}
