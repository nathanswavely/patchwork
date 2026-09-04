// Package gazetteer reads the optional local place index described in
// docs/adr/082. The index is built offline by cmd/gazetteer and copied onto
// the server as a file beside patchwork.db; nothing here fetches anything.
//
// An instance without the file simply has no gazetteer, and every caller must
// treat that as normal rather than as a failure — a suggestion is a
// convenience, and a patch with no suggestion is placed by hand exactly as it
// was before this package existed.
package gazetteer

import (
	"strings"
	"unicode"
)

// canonical folds the abbreviations that make the same address look like two.
// "433 Ice Ave" and "433 Ice Avenue" have to tokenize identically or the
// index answers neither, and OSM writes the long form while people type the
// short one.
//
// Both sides of the index run this: the builder folds what it stores and the
// query folds what it is asked. That is the whole contract, and it is fragile
// in the way a shared constant is not — see TestBuilderAndQueryTokenizeAlike.
var canonical = map[string]string{
	"st": "street", "str": "street",
	"ave": "avenue", "av": "avenue",
	"rd":   "road",
	"dr":   "drive",
	"blvd": "boulevard",
	"ln":   "lane",
	"ct":   "court",
	"pl":   "place",
	"sq":   "square",
	"ter":  "terrace",
	"pkwy": "parkway", "pky": "parkway",
	"hwy": "highway",
	"cir": "circle",
	"aly": "alley",
	"n":   "north", "s": "south", "e": "east", "w": "west",
	"ne": "northeast", "nw": "northwest",
	"se": "southeast", "sw": "southwest",
	"mt": "mount",
	"ft": "fort",
	"co": "county",
}

// noise words carry no locating power and match everything, so they are
// dropped on both sides rather than scored.
var noise = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "and": true,
	"at": true, "in": true, "on": true, "to": true,
}

// Tokenize reduces free text to the comparable units the index is keyed on:
// lowercased, punctuation-stripped, abbreviations expanded, noise dropped.
// Order is not preserved as meaningful — scoring counts which tokens matched,
// never where they sat, because "433 Ice Avenue" and "Ice Avenue 433" name the
// same doorway.
func Tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(fields))
	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		if c, ok := canonical[f]; ok {
			f = c
		}
		if noise[f] || f == "" {
			continue
		}
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// IsNumber reports whether a token looks like a housenumber. A housenumber is
// the strongest signal an address carries — every building on a street shares
// every other token — so scoring weights it, and this is how it is spotted.
func IsNumber(tok string) bool {
	if tok == "" {
		return false
	}
	for _, r := range tok {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
