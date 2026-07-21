package auth

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// RecoveryCodeCount is how many codes one batch contains.
const RecoveryCodeCount = 10

// recoveryAlphabet omits characters that are ambiguous when a code is read
// back from paper: 0/o, 1/l/i. Twelve characters from this 31-letter set is
// ~59 bits of entropy — far beyond the online-guessing budget the rate
// limits allow, though below the 256-bit bar of the URL-carried tokens,
// which is why redemption gets its own tighter limiter.
const recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

const recoveryCodeLen = 12

// generateRecoveryCode returns a code formatted xxxx-xxxx-xxxx.
func generateRecoveryCode() (string, error) {
	max := big.NewInt(int64(len(recoveryAlphabet)))
	var b strings.Builder
	for i := 0; i < recoveryCodeLen; i++ {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate recovery code: %w", err)
		}
		b.WriteByte(recoveryAlphabet[n.Int64()])
	}
	return b.String(), nil
}

// NormalizeRecoveryCode lowercases and strips separators, so a code survives
// being written down and typed back with different hyphenation or case.
func NormalizeRecoveryCode(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

// GenerateRecoveryCodes replaces the user's recovery codes with a fresh batch
// and returns the raw codes — the only time they exist in plain form.
func GenerateRecoveryCodes(db *database.DB, userID string) ([]string, error) {
	codes := make([]string, 0, RecoveryCodeCount)
	for i := 0; i < RecoveryCodeCount; i++ {
		code, err := generateRecoveryCode()
		if err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// A batch replaces any earlier batch, used and unused alike: after
	// regeneration there is exactly one set of codes that works, the one
	// the person is holding.
	if _, err := tx.Exec(`DELETE FROM recovery_codes WHERE user_id = ?`, userID); err != nil {
		return nil, fmt.Errorf("clear old recovery codes: %w", err)
	}

	for _, code := range codes {
		_, err := tx.Exec(
			`INSERT INTO recovery_codes (id, user_id, code) VALUES (?, ?, ?)`,
			NewUUIDv7(), userID, HashToken(NormalizeRecoveryCode(code)),
		)
		if err != nil {
			return nil, fmt.Errorf("insert recovery code: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return codes, nil
}

// CountRecoveryCodes reports how many codes the user has and how many are
// still unused. total == 0 means no batch was ever generated.
func CountRecoveryCodes(db *database.DB, userID string) (total, remaining int, err error) {
	err = db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(used = 0), 0) FROM recovery_codes WHERE user_id = ?`,
		userID,
	).Scan(&total, &remaining)
	if err != nil {
		return 0, 0, fmt.Errorf("count recovery codes: %w", err)
	}
	return total, remaining, nil
}

// errInvalidRecovery is deliberately identical for a wrong username and a
// wrong code, so redemption attempts can't be used to probe which usernames
// exist.
var errInvalidRecovery = fmt.Errorf("invalid username or recovery code")

// RedeemRecoveryCode validates a username + code pair, burns the code, and
// returns the user. Each code works exactly once.
func RedeemRecoveryCode(db *database.DB, username, rawCode string) (*model.User, error) {
	codeHash := HashToken(NormalizeRecoveryCode(rawCode))

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var user model.User
	err = tx.QueryRow(
		`SELECT id, COALESCE(email,''), username, display_name, bio, avatar_url, role, created_at, updated_at
		 FROM users WHERE username = ?`,
		strings.ToLower(strings.TrimSpace(username)),
	).Scan(&user.ID, &user.Email, &user.Username, &user.DisplayName, &user.Bio, &user.AvatarURL, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, errInvalidRecovery
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}

	var codeID string
	err = tx.QueryRow(
		`SELECT id FROM recovery_codes WHERE user_id = ? AND code = ? AND used = 0`,
		user.ID, codeHash,
	).Scan(&codeID)
	if err == sql.ErrNoRows {
		return nil, errInvalidRecovery
	}
	if err != nil {
		return nil, fmt.Errorf("query recovery code: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`UPDATE recovery_codes SET used = 1, used_at = ? WHERE id = ?`, now, codeID); err != nil {
		return nil, fmt.Errorf("mark recovery code used: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return &user, nil
}
