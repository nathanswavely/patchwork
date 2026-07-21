package auth

import (
	"fmt"
	"regexp"
	"strings"
)

// Username rules (docs/adr/013): chosen by the person at account creation,
// never derived from email, immutable afterwards. Usernames share one
// WebFinger acct: namespace with node slugs, so the charset matches slugs
// exactly: lowercase letters, digits, hyphens. 3-30 chars, alphanumeric at
// both ends. The `_system` sentinel is unspellable under these rules.
var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,28}[a-z0-9]$`)

// reservedUsernames blocks instance-authority words (impersonation surface
// on acct: handles) and API/SPA namespace words.
var reservedUsernames = map[string]bool{
	// Authority / impersonation.
	"admin": true, "administrator": true, "moderator": true, "mod": true,
	"staff": true, "system": true, "root": true, "official": true,
	"patchwork": true, "support": true, "help": true, "security": true,
	"abuse": true, "postmaster": true, "webmaster": true, "owner": true,
	"instance": true,
	// API and SPA namespaces.
	"api": true, "ap": true, "auth": true, "users": true, "user": true,
	"me": true, "nodes": true, "events": true, "patches": true,
	"settings": true, "login": true, "logout": true, "signup": true,
	"invite": true, "welcome": true, "dashboard": true,
	"notifications": true, "activity": true, "about": true,
	// Confusing as actors.
	"everyone": true, "all": true, "anonymous": true, "unknown": true,
	"null": true, "undefined": true,
}

// NormalizeUsername trims whitespace and lowercases. Applied before
// validation so mixed-case input is corrected rather than rejected.
func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// ValidateUsername checks a normalized username against the format rules
// and the reserved list. Returns a user-facing error message.
func ValidateUsername(username string) error {
	if len(username) < 3 || len(username) > 30 {
		return fmt.Errorf("username must be 3-30 characters")
	}
	if !usernameRe.MatchString(username) {
		return fmt.Errorf("username may only contain lowercase letters, numbers, and hyphens, and must start and end with a letter or number")
	}
	if reservedUsernames[username] {
		return fmt.Errorf("that username is reserved")
	}
	return nil
}

// CheckUsernameAvailable reports whether a normalized username is free.
// Case-insensitive against existing usernames (grandfathered mixed-case
// names can't be twinned), and rejects existing node slugs — usernames and
// slugs share the WebFinger acct: namespace, and users win on collision,
// so an unchecked username could shadow a patch's federated actor.
func CheckUsernameAvailable(q rowQuerier, username string) error {
	var n int
	if err := q.QueryRow(`SELECT COUNT(*) FROM users WHERE lower(username) = ?`, username).Scan(&n); err != nil {
		return fmt.Errorf("check username: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("that username is taken")
	}
	if err := q.QueryRow(`SELECT COUNT(*) FROM nodes WHERE slug = ? AND removed_at IS NULL`, username).Scan(&n); err != nil {
		return fmt.Errorf("check username: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("that username is taken")
	}
	return nil
}

// PrepareUsername normalizes, validates, and checks availability in one
// step. Returns the normalized username on success.
func PrepareUsername(q rowQuerier, raw string) (string, error) {
	username := NormalizeUsername(raw)
	if err := ValidateUsername(username); err != nil {
		return "", err
	}
	if err := CheckUsernameAvailable(q, username); err != nil {
		return "", err
	}
	return username, nil
}
