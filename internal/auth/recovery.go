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

// The three ways a step-up by recovery code can fail. Unlike sign-in
// redemption these are distinguishable, and safely: the caller is already
// authenticated as the account in question, so nothing here tells them
// anything about somebody else's.
var (
	// ErrInvalidRecoveryCode — no unused code of theirs matches.
	ErrInvalidRecoveryCode = fmt.Errorf("that recovery code is not one of yours, or has been used")
	// ErrNoRecoveryCodes — the account holds no unused codes at all.
	ErrNoRecoveryCodes = fmt.Errorf("this account has no unused recovery codes")
	// ErrRecoveryCodesTooNew — they hold codes, but every one was issued
	// during the session asking to use it. See StepUpWithRecoveryCode.
	ErrRecoveryCodesTooNew = fmt.Errorf("these codes were made during this sign-in")
)

// StepUpWithRecoveryCode burns one recovery code as proof of presence, so a
// person with no passkey can still confirm a step-up-gated action
// (docs/adr/099). It reports how many codes remain.
//
// The code must have been issued *before* the session presenting it began.
// Step-up exists to prove a person is at the keyboard, and anything the
// session itself could have produced proves nothing: generating a batch
// needs only a session, so without this rule a stolen cookie could mint its
// own second factor and wipe an instance with it. A batch made beforehand
// and kept on paper is the "something you have" the gate is asking for.
//
// The first time, that costs a sign-out: make codes, sign back in with one
// (which opens the window by itself, see the redemption path), and from
// then on the rest of the batch predates the session and works directly.
func StepUpWithRecoveryCode(db *database.DB, userID, rawSessionToken, rawCode string) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var sessionCreatedAt string
	err = tx.QueryRow(`SELECT created_at FROM sessions WHERE token = ?`, HashToken(rawSessionToken)).Scan(&sessionCreatedAt)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("no such session")
	}
	if err != nil {
		return 0, fmt.Errorf("query session: %w", err)
	}

	var codeID string
	err = tx.QueryRow(
		`SELECT id FROM recovery_codes
		 WHERE user_id = ? AND code = ? AND used = 0 AND created_at < ?`,
		userID, HashToken(NormalizeRecoveryCode(rawCode)), sessionCreatedAt,
	).Scan(&codeID)
	if err == sql.ErrNoRows {
		// Say which of the three it is, so the page can tell someone whose
		// codes are simply too young what to do about it rather than
		// leaving them retyping a code that will never work.
		var usable, unused int
		tx.QueryRow(`SELECT COUNT(*) FROM recovery_codes WHERE user_id = ? AND used = 0`, userID).Scan(&unused)
		tx.QueryRow(`SELECT COUNT(*) FROM recovery_codes WHERE user_id = ? AND used = 0 AND created_at < ?`,
			userID, sessionCreatedAt).Scan(&usable)
		switch {
		case unused == 0:
			return 0, ErrNoRecoveryCodes
		case usable == 0:
			return 0, ErrRecoveryCodesTooNew
		default:
			return 0, ErrInvalidRecoveryCode
		}
	}
	if err != nil {
		return 0, fmt.Errorf("query recovery code: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`UPDATE recovery_codes SET used = 1, used_at = ? WHERE id = ?`, now, codeID); err != nil {
		return 0, fmt.Errorf("mark recovery code used: %w", err)
	}

	var remaining int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM recovery_codes WHERE user_id = ? AND used = 0`, userID).Scan(&remaining); err != nil {
		return 0, fmt.Errorf("count remaining: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return remaining, nil
}

// UsableStepUpCodes counts the codes this session could step up with: unused,
// and issued before it began.
func UsableStepUpCodes(db *database.DB, userID, rawSessionToken string) int {
	var n int
	db.QueryRow(
		`SELECT COUNT(*) FROM recovery_codes rc
		 JOIN sessions s ON s.token = ?
		 WHERE rc.user_id = ? AND rc.used = 0 AND rc.created_at < s.created_at`,
		HashToken(rawSessionToken), userID,
	).Scan(&n)
	return n
}

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
