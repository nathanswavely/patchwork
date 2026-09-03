package eventsource

// Decoding the HTML entities a CMS leaves in fields that are not HTML.
//
// A feed's SUMMARY, LOCATION and excerpt are plain text everywhere
// Patchwork renders them, so an entity that survives the reader renders
// literally: "Lanc Workshop &amp; Tool Library" on the events list, the
// map popup, the patch page, the ICS feed and the reminder email at once.
//
// Location is the visible casualty, because it is name-first
// (docs/adr/046): the entity sits in the half that survives truncation on
// a narrow row while the ellipsis eats the harmless postal tail.

import (
	"html"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// plainText decodes entities until the string stops changing, rather than
// once.
//
// Once is not enough, because encoding in the wild gets applied twice.
// Squarespace served PCA&D's address as "PCA&amp;amp;D"; a single pass
// turned that into "PCA&amp;D", which is exactly what the events list
// rendered. The venue's own title in the same document had one layer, so
// titles looked fixed while addresses did not.
//
// Ordinary text is left alone: "5 & Dime" holds no entity, so the first
// pass returns it unchanged and the loop stops. Repeated decoding can
// synthesize markup out of data ("&amp;lt;b&amp;gt;" becomes "<b>"), which
// is safe here only because these fields are rendered as text — never
// through {@html}. Keep it that way.
func plainText(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	for i := 0; i < 8; i++ {
		next := html.UnescapeString(s)
		if next == s {
			break
		}
		s = next
	}
	return s
}

// entityWords are the entity names that survive normalizeKey as words.
// normalizeKey drops punctuation, so "PCA&amp;D" keys as "pca amp d" —
// the ampersand vanishes and the entity's own letters stay behind as a
// token that no reader ever typed.
var entityWords = map[string]bool{
	"amp": true, "quot": true, "apos": true, "nbsp": true,
	"lt": true, "gt": true, "rsquo": true, "lsquo": true,
	"rdquo": true, "ldquo": true, "ndash": true, "mdash": true,
	"hellip": true, "39": true, "8217": true,
}

// healKey removes entity leftovers from an already-normalized key so it
// matches the key the same name normalizes to once decoded. It is the
// crosswalk's half of the fix: a listing cached while encoded was keyed
// "lanc workshop amp tool library", an admin mapped that key to a patch,
// and decoding the reader would leave the entry pointing at a key no
// listing carries again — the patch silently stops receiving events.
//
// Only an interior token is dropped. A venue really called "Amp Room"
// keys as "amp room" and must survive: an entity always has text on both
// sides of it, a name may not.
func healKey(key string) string {
	words := strings.Fields(key)
	if len(words) < 3 {
		return key
	}
	out := make([]string, 0, len(words))
	out = append(out, words[0])
	for _, w := range words[1 : len(words)-1] {
		if entityWords[w] {
			continue
		}
		out = append(out, w)
	}
	out = append(out, words[len(words)-1])
	return strings.Join(out, " ")
}

// HealEncodedEntities decodes entities in text already stored, and is why
// the reader fix alone would not have been enough.
//
// A sync only rewrites an event it still finds in the feed: a listing the
// publisher has since dropped, or one whose date fell out of the window,
// keeps whatever it was imported with forever. Four of Lancaster
// Patchwork's five encoded rows were exactly that — past events of a venue
// that has since renamed itself.
//
// Runs on every start, in the manner of the other startup heals, and is a
// no-op once there is nothing left encoded: each statement is filtered to
// rows that actually hold a "&…;" before anything is read.
func HealEncodedEntities(db *database.DB) (int, error) {
	fixed := 0

	// aggregator_listings is deliberately absent: it is a cache the next
	// successful fetch replaces wholesale, and the reader fix cleans it
	// on the way in.
	type textField struct{ table, id, col string }
	for _, f := range []textField{
		{"events", "id", "title"},
		{"events", "id", "description"},
		{"events", "id", "location"},
		{"aggregator_programs", "id", "display_title"},
	} {
		n, err := healTextColumn(db, f.table, f.id, f.col)
		if err != nil {
			return fixed, err
		}
		fixed += n
	}

	// The keys the crosswalk, the ignore list and the programs are matched
	// on. Rewritten to what the decoded name normalizes to, so a mapping
	// made before the fix keeps working after it.
	for _, k := range []struct{ table, where, col string }{
		{"event_sources", "name_key IS NOT NULL AND name_key != ''", "name_key"},
		{"aggregator_ignored_names", "1=1", "name_key"},
		{"aggregator_programs", "1=1", "name_key"},
		{"aggregator_programs", "1=1", "title_key"},
	} {
		n, err := healKeyColumn(db, k.table, k.where, k.col)
		if err != nil {
			return fixed, err
		}
		fixed += n
	}

	return fixed, nil
}

// healTextColumn decodes one text column in place. The LIKE narrows the
// scan to rows holding something entity-shaped; plainText decides.
func healTextColumn(db *database.DB, table, idCol, col string) (int, error) {
	rows, err := db.Query(
		`SELECT ` + idCol + `, ` + col + ` FROM ` + table +
			` WHERE ` + col + ` LIKE '%&%;%'`)
	if err != nil {
		return 0, err
	}
	type update struct {
		id  any
		val string
	}
	var pending []update
	for rows.Next() {
		var id any
		var val string
		if err := rows.Scan(&id, &val); err != nil {
			rows.Close()
			return 0, err
		}
		if decoded := plainText(val); decoded != val {
			pending = append(pending, update{id, decoded})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	for _, u := range pending {
		if _, err := db.Exec(
			`UPDATE `+table+` SET `+col+` = ? WHERE `+idCol+` = ?`, u.val, u.id,
		); err != nil {
			return 0, err
		}
	}
	return len(pending), nil
}

// healKeyColumn rewrites normalized keys that carry an entity's leftover
// word. Unlike the text columns there is no "&" left to filter on — the
// key is already punctuation-free — so the filter is the entity words
// themselves.
func healKeyColumn(db *database.DB, table, where, col string) (int, error) {
	rows, err := db.Query(
		`SELECT rowid, ` + col + ` FROM ` + table + ` WHERE ` + where)
	if err != nil {
		return 0, err
	}
	type update struct {
		rowid int64
		val   string
	}
	var pending []update
	for rows.Next() {
		var rowid int64
		var val string
		if err := rows.Scan(&rowid, &val); err != nil {
			rows.Close()
			return 0, err
		}
		if healed := healKey(val); healed != val {
			pending = append(pending, update{rowid, healed})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	for _, u := range pending {
		// A heal can collide with a key that already exists — two names
		// that differed only by their encoding. Let the unique index
		// refuse it and keep the row as it was: a duplicate mapping is
		// the admin's to resolve, and a failed heal must not stop the
		// instance from starting.
		if _, err := db.Exec(
			`UPDATE OR IGNORE `+table+` SET `+col+` = ? WHERE rowid = ?`, u.val, u.rowid,
		); err != nil {
			return 0, err
		}
	}
	return len(pending), nil
}
