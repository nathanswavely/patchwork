package auth

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"sync"

	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// rowQuerier is satisfied by both *sql.Tx and *database.DB.
type rowQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// countRealUsers counts accounts, excluding the sentinel system user that
// migration 015 seeds to own unclaimed patches.
func countRealUsers(q rowQuerier) (int, error) {
	var n int
	err := q.QueryRow(`SELECT COUNT(*) FROM users WHERE id != ?`, model.SystemUserID).Scan(&n)
	return n, err
}

// roleForNewUser returns the role a newly created account should get. The
// first account on a fresh instance becomes the instance admin — this is the
// bootstrap path for self-hosted deploys, where no admin exists yet to
// generate invite links or promote anyone.
func roleForNewUser(q rowQuerier) string {
	n, err := countRealUsers(q)
	if err != nil || n > 0 {
		return "member"
	}
	return "admin"
}

// NoUsersExist reports whether the instance has no accounts yet (fresh deploy).
func NoUsersExist(q rowQuerier) bool {
	n, err := countRealUsers(q)
	return err == nil && n == 0
}

// The bootstrap token gates the first account (docs/adr/070). Two rules
// that are each reasonable combine badly without it: the first account
// becomes instance admin, and magic-link signup is open registration. An
// instance reachable before its operator signs up is otherwise claimable
// by whoever finds it first, and the prize is instance admin. Self-hosters
// mostly dodge this by accident, because without SMTP the magic link
// prints to a log only they can read — but an accident is not a lock, and
// a provisioning layer whose instances come up with SMTP working on
// guessable addresses does not even get the accident.
//
// It gates bootstrap, never registration: once one account exists the
// token is dead, and the ordinary signup path is untouched.
var (
	bootstrapMu    sync.RWMutex
	bootstrapToken string
)

// ErrBootstrapToken is returned when a fresh instance is handed the wrong
// token, or none. The message names the log because that is where an
// unconfigured instance prints the one it generated.
var ErrBootstrapToken = errors.New(
	"this quilt has not been claimed yet — completing the first account needs its bootstrap token, " +
		"which the server printed in its first-run log notice")

// SetBootstrapToken installs the token the first account must present.
// Called once at startup; the empty string leaves the gate shut, which is
// the safe direction for any entry point that forgets to call it.
func SetBootstrapToken(token string) {
	bootstrapMu.Lock()
	defer bootstrapMu.Unlock()
	bootstrapToken = token
}

// BootstrapTokenSet reports whether a token has been installed. Startup
// uses it to decide whether to say anything in the first-run notice.
func BootstrapTokenSet() bool {
	bootstrapMu.RLock()
	defer bootstrapMu.RUnlock()
	return bootstrapToken != ""
}

// checkBootstrapToken guards account creation on a fresh instance. It is a
// no-op the moment a real account exists, so it can sit in the signup path
// unconditionally: this is a bootstrap gate, not a registration gate.
//
// The comparison is constant-time. The window is small and the token is
// high-entropy, but an attacker racing an operator for instance admin is
// exactly the person who would sit and measure.
func checkBootstrapToken(q rowQuerier, presented string) error {
	if !NoUsersExist(q) {
		return nil
	}
	bootstrapMu.RLock()
	want := bootstrapToken
	bootstrapMu.RUnlock()
	if want == "" || subtle.ConstantTimeCompare([]byte(presented), []byte(want)) != 1 {
		return ErrBootstrapToken
	}
	return nil
}

// GenerateBootstrapToken mints a token for an instance that configured
// none. Not persisted: it lives for the life of the process, so a restart
// before anyone claims the instance prints a fresh one.
func GenerateBootstrapToken() (string, error) {
	return generateToken()
}
