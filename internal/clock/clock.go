// Package clock is the one place that names the text layout Patchwork
// stores timestamps in.
//
// Every timestamp column is ISO 8601 TEXT, but two call sites wrote two
// different shapes of it: the millisecond literal
// "2006-01-02T15:04:05.000Z" and time.RFC3339 ("2006-01-02T15:04:05Z",
// no fraction). Both are valid ISO 8601, but string comparison — which is
// how every "_at < ?" cutoff and every cursor works, since the column is
// TEXT — sorts '.' before 'Z'. So a row written in RFC3339 the same second
// as a row written with milliseconds can land on the wrong side of a
// cutoff or a page boundary. See docs/adr and issue #311.
//
// Now and Format write the millisecond layout, so every new row is
// uniform going forward. Parse reads either shape (plus RFC3339Nano),
// so rows already on disk in the old format keep parsing; there is no
// migration rewriting existing rows.
package clock

import "time"

// Layout is the one text shape Patchwork stores a timestamp in.
const Layout = "2006-01-02T15:04:05.000Z"

// Now returns the current instant, in UTC, formatted as Layout.
func Now() string {
	return Format(time.Now())
}

// Format renders t, converted to UTC, as Layout.
func Format(t time.Time) string {
	return t.UTC().Format(Layout)
}

// Parse reads a stored timestamp back into a time.Time. It accepts the
// millisecond Layout this package writes, plain RFC3339 (what older rows
// and a couple of legacy write paths used), and RFC3339Nano, tried in
// that order.
func Parse(s string) (time.Time, error) {
	if t, err := time.Parse(Layout, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}
