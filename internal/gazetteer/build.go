package gazetteer

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"
)

// Builder writes a gazetteer file. It lives beside the reader so the two
// share one schema and one tokenizer; a builder that indexed text differently
// from the way the query folds it would produce a file that answers nothing,
// and no test of either half alone would notice.
type Builder struct {
	db      *sql.DB
	tx      *sql.Tx
	place   *sql.Stmt
	token   *sql.Stmt
	nextID  int64
	written int
	skipped int
}

// NewBuilder creates a fresh file, replacing any file already at the path.
func NewBuilder(path string) (*Builder, error) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	db, err := sql.Open("sqlite3", "file:"+path+"?_journal_mode=OFF&_synchronous=OFF")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(Schema); err != nil {
		db.Close()
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close()
		return nil, err
	}
	b := &Builder{db: db, tx: tx, nextID: 1}
	if b.place, err = tx.Prepare(
		`INSERT INTO places (id, name, housenumber, street, city, postcode, lat, lon)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`); err != nil {
		db.Close()
		return nil, err
	}
	if b.token, err = tx.Prepare(`INSERT INTO place_tokens (token, place_id) VALUES (?, ?)`); err != nil {
		db.Close()
		return nil, err
	}
	return b, nil
}

// Add indexes one place. A place with nothing to match on is skipped rather
// than stored: a row with coordinates and no text can never be selected, and
// it would only inflate the file.
func (b *Builder) Add(p Place) error {
	tokens := PlaceTokens(p)
	if len(tokens) == 0 {
		b.skipped++
		return nil
	}
	id := b.nextID
	b.nextID++
	if _, err := b.place.Exec(id, p.Name, p.HouseNumber, p.Street, p.City, p.Postcode, p.Latitude, p.Longitude); err != nil {
		return err
	}
	for _, t := range tokens {
		if _, err := b.token.Exec(t, id); err != nil {
			return err
		}
	}
	b.written++
	return nil
}

// PlaceTokens is every token a place can be found by. Exported because the
// parity test asserts the builder indexes exactly what Tokenize would produce
// for the same text.
func PlaceTokens(p Place) []string {
	return Tokenize(strings.Join([]string{
		p.Name, p.HouseNumber, p.Street, p.City, p.Postcode,
	}, " "))
}

// Written and Skipped report what Add did, for the CLI's summary.
func (b *Builder) Written() int { return b.written }
func (b *Builder) Skipped() int { return b.skipped }

// Finish commits, indexes and closes. Indexes are built after the inserts:
// maintaining them across a county's worth of rows costs far more than one
// pass at the end.
func (b *Builder) Finish(source string, lat, lon, radiusKM float64) error {
	meta := map[string]string{
		"schema_version": fmt.Sprint(SchemaVersion),
		"built_at":       time.Now().UTC().Format(time.RFC3339),
		"source":         source,
		"center_lat":     fmt.Sprintf("%.6f", lat),
		"center_lon":     fmt.Sprintf("%.6f", lon),
		"radius_km":      fmt.Sprintf("%g", radiusKM),
		"places":         fmt.Sprint(b.written),
	}
	for k, v := range meta {
		if _, err := b.tx.Exec(`INSERT INTO meta (key, value) VALUES (?, ?)`, k, v); err != nil {
			return err
		}
	}
	if err := b.tx.Commit(); err != nil {
		return err
	}
	if _, err := b.db.Exec(SchemaIndexes); err != nil {
		return err
	}
	if _, err := b.db.Exec(`ANALYZE`); err != nil {
		return err
	}
	if _, err := b.db.Exec(`VACUUM`); err != nil {
		return err
	}
	return b.db.Close()
}

// Abort discards a partial build.
func (b *Builder) Abort() {
	if b.tx != nil {
		b.tx.Rollback()
	}
	if b.db != nil {
		b.db.Close()
	}
}
