package auth

import (
	"database/sql"

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
