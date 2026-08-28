package auth

import (
	"fmt"
	"net/mail"
	"strings"
)

// maxEmailLen bounds a stored address. RFC 5321 caps a path at 256 octets;
// anything near that is not a community member's mailbox.
const maxEmailLen = 254

// NormalizeEmail trims and lowercases an address, and checks it parses.
//
// Lowercasing matters more than it looks: every lookup that finds an existing
// account is `WHERE email = ?`, an exact match. Storing one person as
// "Someone@example.com" and letting them later type "someone@example.com"
// misses their row, and the magic-link path reads a miss as a new visitor —
// so instead of signing them in it offers to build them a second account,
// stranding the first. Normalizing at every entry point keeps the stored form
// and the typed form the same string.
func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" {
		return "", fmt.Errorf("email is required")
	}
	if len(email) > maxEmailLen {
		return "", fmt.Errorf("email address is too long")
	}
	// ParseAddress accepts "Name <addr>"; we want the bare address only, so
	// anything that round-trips to something other than what we passed in is
	// rejected rather than silently rewritten.
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", fmt.Errorf("that does not look like an email address")
	}
	return email, nil
}
