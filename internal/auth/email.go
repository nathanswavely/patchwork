package auth

import (
	"net/mail"
	"strings"
)

// maxEmailLength is the RFC 5321 cap on a forward path. `users.email` is
// TEXT and would happily store a megabyte, so the bound is enforced here or
// nowhere.
const maxEmailLength = 254

// NormalizeEmail canonicalizes an email address for storage and lookup:
// surrounding whitespace trimmed, the whole address lowercased.
//
// The local part of an address is case-sensitive per RFC 5321, but no mail
// provider in practice treats it that way, and honoring the letter of the
// spec here would mean "Bob@Example.com" and "bob@example.com" are two
// accounts nobody can tell apart. `users.email` is UNIQUE under SQLite's
// BINARY collation, so without this every address is as many distinct
// accounts as it has capitalizations — and a returning person who typed
// their address differently the second time falls through to the signup
// branch rather than signing in.
//
// Every path that writes or reads an address must go through here, and
// migration 058 canonicalized the rows that predate it. The two halves are
// one change: normalizing the lookup alone would strand every mixed-case
// row that already existed.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidEmail reports whether an address is well-formed enough to store and
// to send a sign-in link to. Pass it a NormalizeEmail'd address.
//
// The parse is net/mail's, so the grammar is RFC 5322's rather than a regex
// that will be wrong in a way nobody notices for a year. Two rules are laid
// on top of it:
//
// The address must be exactly what net/mail parsed back out. ParseAddress
// also accepts the forms an email *header* may carry — "Bob
// <bob@example.com>", "bob@example.com (Bob)", `"bob"@example.com` — and
// returns the bare address inside. Those are legal headers and illegal
// identities: stored verbatim they are strings that no longer equal the
// address they mean, so the next lookup misses. Demanding equality keeps
// exactly the bare form.
//
// And the domain is NOT required to contain a dot. It is the obvious extra
// check and it would break local development: the seeded dev admin is
// `admin@localhost` (cmd/seed), which is also the marker cmd/seed uses to
// tell a demo database from a real one. A dotless domain is valid, rare,
// and load-bearing here.
//
// This is deliberately shallow. Whether an address *receives mail* is not
// knowable from its syntax, and the magic link is what proves that.
func ValidEmail(email string) bool {
	if email == "" || len(email) > maxEmailLength {
		return false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}
	return addr.Address == email
}
