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

// streetTypes are the words that every street shares. They are the tail of a
// street name rather than the name itself: "East King Street" and "North
// Prince Street" have `street` in common and nothing else.
//
// These are the long forms only, because Tokenize has already folded the
// abbreviations into them by the time anything reaches here.
var streetTypes = map[string]bool{
	"street": true, "avenue": true, "road": true, "drive": true,
	"boulevard": true, "lane": true, "court": true, "place": true,
	"square": true, "terrace": true, "parkway": true, "highway": true,
	"circle": true, "alley": true, "way": true, "pike": true,
	"trail": true, "path": true, "run": true, "loop": true, "row": true,
}

// directions are the prefixes people drop. "King St" is what somebody types
// for West King Street far more often than they invent a direction that isn't
// there, so a direction cannot be what decides whether two street names are
// the same one.
var directions = map[string]bool{
	"north": true, "south": true, "east": true, "west": true,
	"northeast": true, "northwest": true, "southeast": true, "southwest": true,
}

// streetCore reduces a street name to the words that tell it apart from every
// other street — "East King Street" to `king`, "Old Philadelphia Pike" to
// `old philadelphia`. It is how scoring asks whether a query named *this*
// street rather than merely the word "street".
//
// A street whose whole name is a direction ("North Street") keeps the
// direction, because dropping it would leave nothing at all; a street whose
// whole name is a type word has no core and cannot be confirmed by name.
func streetCore(street string) []string {
	tokens := Tokenize(street)
	core := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if streetTypes[t] || directions[t] {
			continue
		}
		core = append(core, t)
	}
	if len(core) > 0 {
		return core
	}
	for _, t := range tokens {
		if directions[t] {
			core = append(core, t)
		}
	}
	return core
}
