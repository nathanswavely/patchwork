package gazetteer

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// SchemaVersion is bumped when the file layout changes. The server refuses a
// file it does not understand rather than answering from half of one; the
// remedy is to rebuild with a matching cmd/gazetteer.
const SchemaVersion = 1

// Schema is the whole file. It lives here rather than in cmd/gazetteer so the
// reader and the builder cannot drift apart.
//
// There is no FTS5 table here on purpose: mattn/go-sqlite3 compiles FTS5 only
// under the sqlite_fts5 build tag, and this project sets no tags — `CREATE
// VIRTUAL TABLE ... USING fts5` fails at runtime with "no such module: fts5".
// place_tokens is a plain inverted index doing the same job within reach of
// the driver as it is actually built.
const Schema = `
CREATE TABLE meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE places (
  id          INTEGER PRIMARY KEY,
  name        TEXT NOT NULL DEFAULT '',
  housenumber TEXT NOT NULL DEFAULT '',
  street      TEXT NOT NULL DEFAULT '',
  city        TEXT NOT NULL DEFAULT '',
  postcode    TEXT NOT NULL DEFAULT '',
  lat         REAL NOT NULL,
  lon         REAL NOT NULL
);
CREATE TABLE place_tokens (
  token    TEXT    NOT NULL,
  place_id INTEGER NOT NULL
);
`

// Indexes are created after the bulk insert, which is far faster than
// maintaining them per row across a county's worth of addresses.
const SchemaIndexes = `
CREATE INDEX idx_place_tokens_token ON place_tokens(token);
`

// Place is one entry in the index.
type Place struct {
	Name        string  `json:"name,omitempty"`
	HouseNumber string  `json:"housenumber,omitempty"`
	Street      string  `json:"street,omitempty"`
	City        string  `json:"city,omitempty"`
	Postcode    string  `json:"postcode,omitempty"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

// Label renders a place name-first, the contract docs/adr/046 set for every
// location string this project assembles.
func (p Place) Label() string {
	parts := make([]string, 0, 4)
	if p.Name != "" {
		parts = append(parts, p.Name)
	}
	street := strings.TrimSpace(p.HouseNumber + " " + p.Street)
	if street != "" {
		parts = append(parts, street)
	}
	if p.City != "" {
		parts = append(parts, p.City)
	}
	return strings.Join(parts, ", ")
}

// Gazetteer is a read-only handle on the index file. A nil *Gazetteer is
// valid and answers nothing, which is what an instance without the file has.
type Gazetteer struct {
	db    *sql.DB
	path  string
	count int
}

// ErrNotConfigured means no path was given. It is not a failure: the feature
// is optional and the caller carries on without suggestions.
var ErrNotConfigured = errors.New("gazetteer: no path configured")

// Open attaches to a gazetteer file read-only. A configured path that cannot
// be opened is an error worth surfacing — the admin meant to install one and
// something is wrong with it — but the caller is expected to log and continue
// rather than refuse to boot over an optional convenience.
func Open(path string) (*Gazetteer, error) {
	if strings.TrimSpace(path) == "" {
		return nil, ErrNotConfigured
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("gazetteer: %w", err)
	}
	// Read-only, and deliberately not immutable=1: an admin refreshes the
	// index by replacing the file, and immutable promises SQLite the bytes
	// never change.
	db, err := sql.Open("sqlite3", "file:"+path+"?mode=ro&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("gazetteer: %w", err)
	}
	g := &Gazetteer{db: db, path: path}
	var version string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil {
		db.Close()
		return nil, fmt.Errorf("gazetteer: %s is not a gazetteer file: %w", path, err)
	}
	if version != fmt.Sprint(SchemaVersion) {
		db.Close()
		return nil, fmt.Errorf("gazetteer: %s is schema %s, this build reads %d — rebuild it with cmd/gazetteer", path, version, SchemaVersion)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM places`).Scan(&g.count); err != nil {
		db.Close()
		return nil, fmt.Errorf("gazetteer: %w", err)
	}
	return g, nil
}

// Close releases the handle. Safe on nil.
func (g *Gazetteer) Close() error {
	if g == nil || g.db == nil {
		return nil
	}
	return g.db.Close()
}

// Count is how many places the index holds, for the admin-facing readout.
func (g *Gazetteer) Count() int {
	if g == nil {
		return 0
	}
	return g.count
}

// Scoring weights. A housenumber that matches is worth more than any other
// single token because every building on a street shares everything else.
const (
	weightHouseNumber = 3
	weightWholeName   = 2

	// A named city is worth more than the bare token it contributes, because
	// the token often contributes nothing at all: "Millersville Road,
	// Millersville" tokenizes to one `millersville`, so the same housenumber
	// on the same street in the next town over scored identically and the
	// pair cancelled each other out as ambiguous. Naming the city has to
	// break that tie, which is the whole reason somebody typed it.
	weightCity = 2

	minScore = 2

	// Two candidates tied on score are only ambiguous if they are far
	// enough apart for the difference to matter. Within this distance
	// either answer puts the marker on the right block, and the person is
	// about to look at it anyway.
	ambiguousMetres = 250
)

type candidate struct {
	place Place
	score int
}

// Suggest returns the one place the text most likely names, or false.
//
// False is an ordinary answer, not an error. CONTEXT.md blesses "above the
// record shop on Prince St" as a valid address, and no index resolves it; a
// miss must leave the caller exactly where it would have been with no
// gazetteer installed.
func (g *Gazetteer) Suggest(text string) (Place, bool) {
	if g == nil || g.db == nil {
		return Place{}, false
	}
	tokens := Tokenize(text)
	if len(tokens) == 0 {
		return Place{}, false
	}
	// A single token is a city or a street name shared by hundreds of rows.
	// Nothing it selects is a placement anybody meant.
	if len(tokens) < 2 {
		return Place{}, false
	}

	args := make([]any, len(tokens))
	holes := make([]string, len(tokens))
	for i, t := range tokens {
		args[i] = t
		holes[i] = "?"
	}
	rows, err := g.db.Query(`
		SELECT p.id, p.name, p.housenumber, p.street, p.city, p.postcode, p.lat, p.lon,
		       COUNT(DISTINCT t.token) AS hits
		FROM place_tokens t
		JOIN places p ON p.id = t.place_id
		WHERE t.token IN (`+strings.Join(holes, ",")+`)
		GROUP BY p.id
		ORDER BY hits DESC
		LIMIT 50`, args...)
	if err != nil {
		return Place{}, false
	}
	defer rows.Close()

	query := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		query[t] = true
	}

	var best, runner candidate
	for rows.Next() {
		var id, hits int
		var p Place
		if err := rows.Scan(&id, &p.Name, &p.HouseNumber, &p.Street, &p.City, &p.Postcode, &p.Latitude, &p.Longitude, &hits); err != nil {
			return Place{}, false
		}
		score := hits
		// The housenumber only counts when the query actually carried it.
		if p.HouseNumber != "" {
			for _, ht := range Tokenize(p.HouseNumber) {
				if query[ht] {
					score += weightHouseNumber
					break
				}
			}
		}
		// The city the query named beats the same street and number in a
		// different town.
		if p.City != "" && containsAll(query, Tokenize(p.City)) {
			score += weightCity
		}
		// A named venue whose whole name was typed beats an address that
		// happens to share a word with it.
		if p.Name != "" && containsAll(query, Tokenize(p.Name)) {
			score += weightWholeName
		}
		c := candidate{place: p, score: score}
		switch {
		case score > best.score:
			runner = best
			best = c
		case score > runner.score:
			runner = c
		}
	}
	if err := rows.Err(); err != nil {
		return Place{}, false
	}
	if best.score < minScore {
		return Place{}, false
	}
	// A tie between two places far apart is the "Main Street in three
	// townships" case. Guessing one would put a marker in the wrong town and
	// dress it as an answer, so say nothing.
	if runner.score == best.score &&
		DistanceMetres(best.place.Latitude, best.place.Longitude, runner.place.Latitude, runner.place.Longitude) > ambiguousMetres {
		return Place{}, false
	}
	return best.place, true
}

// containsAll reports whether every one of these tokens was in the query.
// Empty tokens are not "all of them" — a place with no city has not matched
// the city somebody typed.
func containsAll(query map[string]bool, tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	for _, t := range tokens {
		if !query[t] {
			return false
		}
	}
	return true
}

// DistanceMetres is the great-circle distance between two points. Used to
// crop the index to an instance's radius at build time and to judge whether
// two tied candidates are the same place at query time.
func DistanceMetres(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadius * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
