package main

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// The world moves; the clock does not (docs/adr/096).
//
// Advancing the simulation by thirty days means every stored instant becomes
// thirty days older. A vote that was to close next week closed three weeks
// ago; a term that ran to spring ran out in winter. The product then reads
// the database with the real clock and finds exactly what it would have found
// had a month passed — which is the point: the product is never told it is
// being simulated, so it can't behave differently for the test than for a
// community.
//
// Columns are not listed. Every TEXT value in every table is inspected, and
// anything shaped like an ISO 8601 instant or date is moved. A new
// `*_at` column is covered the day it is added, and the only way to keep a
// timestamp still is to name its table below and say why.

// excludedTables hold instants measured against the real wall clock: sign-in
// state and anything a person must be able to redeem after the world moves.
// Shifting a session's expiry logs every persona out; shifting a magic link
// makes onboarding untestable across an epoch.
var excludedTables = map[string]string{
	"sessions":          "sign-in state; a shifted expiry logs every persona out",
	"magic_links":       "redeemable by a person against the real clock",
	"invite_links":      "redeemable by a person against the real clock",
	"signup_tokens":     "redeemable by a person against the real clock",
	"credentials":       "WebAuthn material; a passkey's clock is the browser's",
	"recovery_codes":    "redeemable by a person against the real clock",
	"instance_actor":    "keys, not history",
	"ap_outbox_queue":   "delivery retry timing is wall-clock and never reaches a persona",
	"schema_migrations": "bookkeeping about the binary, not the community",
}

var (
	// Whole-value shapes. The zone suffix and the fraction are captured so the
	// value can be written back in the exact layout it was read in — a column
	// that held `.000Z` keeps holding `.000Z`, one that held RFC3339 keeps that.
	reInstant = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(\.\d+)?(Z|[+-]\d{2}:\d{2})$`)
	reNaive   = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})([T ])(\d{2}:\d{2}:\d{2})(\.\d+)?$`)
	reDate    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

	// Instants inside longer text: JSON payloads (an activity, an audit
	// detail, frozen voting terms). Only the zoned shape is touched there —
	// a bare date inside prose is as likely a street number as a clock.
	reEmbedded = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})`)
)

// shiftWhole moves a value that is, in its entirety, an instant or a date.
// The second return is false when the value is not one.
func shiftWhole(s string, by time.Duration) (string, bool) {
	if m := reInstant.FindStringSubmatch(s); m != nil {
		t, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return s, false
		}
		layout := "2006-01-02T15:04:05" + fractionLayout(m[2])
		if m[3] == "Z" {
			layout += "Z07:00"
		} else {
			layout += "-07:00"
		}
		return t.Add(-by).Format(layout), true
	}
	if m := reNaive.FindStringSubmatch(s); m != nil {
		layout := "2006-01-02" + m[2] + "15:04:05" + fractionLayout(m[4])
		t, err := time.Parse(layout, s)
		if err != nil {
			return s, false
		}
		return t.Add(-by).Format(layout), true
	}
	if reDate.MatchString(s) {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return s, false
		}
		return t.Add(-by).Format("2006-01-02"), true
	}
	return s, false
}

func fractionLayout(frac string) string {
	if frac == "" {
		return ""
	}
	return "." + strings.Repeat("0", len(frac)-1)
}

// shiftText moves a stored value: whole if it is a timestamp, otherwise every
// zoned instant embedded in it. The int is how many instants moved.
func shiftText(s string, by time.Duration) (string, int) {
	if out, ok := shiftWhole(s, by); ok {
		return out, 1
	}
	if len(s) <= 10 || !strings.Contains(s, "T") {
		return s, 0
	}
	n := 0
	out := reEmbedded.ReplaceAllStringFunc(s, func(m string) string {
		moved, ok := shiftWhole(m, by)
		if ok {
			n++
		}
		return moved
	})
	return out, n
}

// ShiftReport says what a shift touched, per table, so the person running the
// simulation can see a column they didn't expect to move — or one they did
// expect that stayed still.
type ShiftReport struct {
	By       time.Duration
	Tables   map[string]int // table -> values moved
	Skipped  map[string]string
	Instants int
}

func (r ShiftReport) String() string {
	names := make([]string, 0, len(r.Tables))
	for t := range r.Tables {
		names = append(names, t)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "moved %d instants back by %s across %d tables\n", r.Instants, r.By, len(names))
	for _, t := range names {
		fmt.Fprintf(&b, "  %-32s %d\n", t, r.Tables[t])
	}
	return b.String()
}

// shiftWorld moves every stored instant in the database back by `by`, in one
// transaction. Auth tables stay where they are (excludedTables).
func shiftWorld(db *sql.DB, by time.Duration) (ShiftReport, error) {
	report := ShiftReport{By: by, Tables: map[string]int{}, Skipped: map[string]string{}}
	if by <= 0 {
		return report, fmt.Errorf("advance must be a positive duration")
	}

	tables, err := listTables(db)
	if err != nil {
		return report, err
	}

	tx, err := db.Begin()
	if err != nil {
		return report, err
	}
	defer tx.Rollback()

	for _, table := range tables {
		if why, skip := excludedTables[table]; skip {
			report.Skipped[table] = why
			continue
		}
		cols, err := textColumns(tx, table)
		if err != nil {
			return report, fmt.Errorf("%s: %w", table, err)
		}
		for _, col := range cols {
			n, err := shiftColumn(tx, table, col, by)
			if err != nil {
				return report, fmt.Errorf("%s.%s: %w", table, col, err)
			}
			if n > 0 {
				report.Tables[table] += n
				report.Instants += n
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return report, err
	}
	return report, nil
}

func listTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name, COALESCE(sql,'') FROM sqlite_master
	                       WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name, ddl string
		if err := rows.Scan(&name, &ddl); err != nil {
			return nil, err
		}
		// A table with no rowid can't be addressed the way the update below
		// addresses rows; none exist today, and one arriving should be a
		// loud stop rather than a silent hole in the simulation.
		if strings.Contains(strings.ToUpper(ddl), "WITHOUT ROWID") || strings.HasPrefix(strings.ToUpper(ddl), "CREATE VIRTUAL") {
			return nil, fmt.Errorf("table %s has no rowid; teach cmd/sim how to address it before simulating", name)
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// textColumns returns every column of the table that could hold a stored
// instant. Declared types are advisory in SQLite, so the filter is loose:
// anything not declared as a number is inspected.
func textColumns(tx *sql.Tx, table string) ([]string, error) {
	rows, err := tx.Query(fmt.Sprintf(`PRAGMA table_info("%s")`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		switch strings.ToUpper(ctype) {
		case "INTEGER", "INT", "REAL", "NUMERIC", "BLOB", "BOOLEAN":
			continue
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

func shiftColumn(tx *sql.Tx, table, col string, by time.Duration) (int, error) {
	// A cheap prefilter so a wide table is not read whole: whole-value dates
	// start with a date, embedded instants contain one followed by T.
	q := fmt.Sprintf(`SELECT rowid, "%s" FROM "%s" WHERE "%s" LIKE '____-__-__%%' OR "%s" LIKE '%%____-__-__T__:__:__%%'`,
		col, table, col, col)
	rows, err := tx.Query(q)
	if err != nil {
		return 0, err
	}
	type update struct {
		rowid int64
		value string
	}
	var updates []update
	moved := 0
	for rows.Next() {
		var rowid int64
		var val sql.NullString
		if err := rows.Scan(&rowid, &val); err != nil {
			rows.Close()
			return 0, err
		}
		if !val.Valid {
			continue
		}
		out, n := shiftText(val.String, by)
		if n > 0 && out != val.String {
			updates = append(updates, update{rowid, out})
			moved += n
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	stmt := fmt.Sprintf(`UPDATE "%s" SET "%s" = ? WHERE rowid = ?`, table, col)
	for _, u := range updates {
		if _, err := tx.Exec(stmt, u.value, u.rowid); err != nil {
			return 0, err
		}
	}
	return moved, nil
}
