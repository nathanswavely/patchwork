package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/clock"
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/mail"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

const magicLinkExpiry = 15 * time.Minute

// maxCodeAttempts is how many wrong codes one magic link row answers before
// it stops answering at all. A code is six digits — a millionth of the
// keyspace the link's token carries — so the row, not the keyspace, is what
// has to bound a guesser. Five is generous for somebody reading digits off a
// screen and useless to anybody else.
const maxCodeAttempts = 5

// ErrInvalidMagicCode is the single answer every code failure gives: wrong
// code, spent row, expired row, a row that never had a code, an address
// nobody ever asked for. One error, because the differences are exactly what
// an endpoint like this must not teach — "has this address been asked for
// here" is the question the blanket 200 on the request endpoint already
// refuses to answer, and it would be no better answered on the way back.
var ErrInvalidMagicCode = fmt.Errorf("invalid or expired code")

// generateMagicCode returns a uniform six-digit numeric code: crypto/rand
// over exactly 10^6 values, so no modulo folds the top of the range onto its
// bottom and no code is likelier than another. Zero-padded, because "004271"
// is a six-digit code and "4271" is not the same string to read back.
func generateMagicCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", fmt.Errorf("generate sign-in code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// hashMagicCode is a code's stored form: the same sha256 hex the link's own
// token is stored as (migration 022).
func hashMagicCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// formatMagicCode groups a code for reading — "123456" → "123 456". Only the
// email and the no-SMTP log say it that way; verification normalizes back, so
// the grouping is a reading aid and never part of the secret.
func formatMagicCode(code string) string {
	if len(code) != 6 {
		return code
	}
	return code[:3] + " " + code[3:]
}

// normalizeMagicCode removes the separators a person may carry over from the
// email or add by habit, so "123 456" and "123456" are the same code.
// Nothing else is stripped: a code is digits, and any other character in the
// string is a mistyped code that should fail as one.
func normalizeMagicCode(raw string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '-':
			return -1
		}
		return r
	}, strings.TrimSpace(raw))
}

// GenerateMagicLink creates a magic link, stores its hash, and sends it via
// SMTP. linkFor turns the raw token into the URL that goes in the email — the
// caller owns URL shape so the emailed link and the one printed to the log
// (no-SMTP dev) can never drift apart again.
//
// The email carries a sign-in code beside the link. Both are minted onto one
// row and prove the same thing — control of this mailbox — so a client that
// cannot be handed the emailed URL in its own session finishes by typing six
// digits instead.
func GenerateMagicLink(db *database.DB, email string, smtpCfg config.SMTP, linkFor func(token string) string) error {
	email, err := NormalizeEmail(email)
	if err != nil {
		return err
	}

	rawToken, code, err := insertMagicLinkRow(db, email)
	if err != nil {
		return err
	}

	// Send the email.
	link := linkFor(rawToken)
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Sign in to Patchwork\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nClick to sign in:\n\n%s\n\nOr, if you asked for this from an app or another device, enter this code there:\n\n%s\n\nThe link and the code both expire in 15 minutes.\n", smtpCfg.From, email, link, formatMagicCode(code))

	if err := mail.Send(smtpCfg, []string{email}, []byte(body)); err != nil {
		return fmt.Errorf("send magic link email: %w", err)
	}

	return nil
}

// GenerateMagicLinkLocal creates a magic link and stores it, but returns the
// raw token and the sign-in code instead of emailing them. Used when SMTP is
// not configured (local dev), where the server log is the delivery channel
// and so has to carry both.
func GenerateMagicLinkLocal(db *database.DB, email string) (string, string, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return "", "", err
	}
	return insertMagicLinkRow(db, email)
}

// insertMagicLinkRow mints one magic link — token and code together, hashed
// — and returns both raws. One row, one expiry, one `used` flag: the two
// credentials are two ways to answer the same request, so spending either
// spends the row.
func insertMagicLinkRow(db *database.DB, email string) (rawToken, code string, err error) {
	rawToken, err = generateToken()
	if err != nil {
		return "", "", err
	}

	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	code, err = generateMagicCode()
	if err != nil {
		return "", "", err
	}

	id := NewUUIDv7()
	expiresAt := clock.Format(time.Now().Add(magicLinkExpiry))

	_, err = db.Exec(
		`INSERT INTO magic_links (id, email, token, code_hash, expires_at) VALUES (?, ?, ?, ?, ?)`,
		id, email, tokenHash, hashMagicCode(code), expiresAt,
	)
	if err != nil {
		return "", "", fmt.Errorf("insert magic link: %w", err)
	}

	return rawToken, code, nil
}

// VerifyMagicLink validates a raw magic link token and marks it used.
// For an email with an existing account it returns that user. For an
// unknown email it creates NO user — usernames are chosen, never derived
// (docs/adr/013) — and instead returns a raw signup token the caller
// exchanges for an account once a username is picked.
func VerifyMagicLink(db *database.DB, rawToken string) (*model.User, string, error) {
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	tx, err := db.Begin()
	if err != nil {
		return nil, "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var id, email, expiresAt string
	var used int

	err = tx.QueryRow(
		`SELECT id, email, expires_at, used FROM magic_links WHERE token = ?`,
		tokenHash,
	).Scan(&id, &email, &expiresAt, &used)
	if err == sql.ErrNoRows {
		return nil, "", fmt.Errorf("invalid magic link token")
	}
	if err != nil {
		return nil, "", fmt.Errorf("query magic link: %w", err)
	}

	if used != 0 {
		return nil, "", fmt.Errorf("magic link already used")
	}

	exp, err := clock.Parse(expiresAt)
	if err == nil && time.Now().After(exp) {
		return nil, "", fmt.Errorf("magic link has expired")
	}

	// Mark used.
	_, err = tx.Exec(`UPDATE magic_links SET used = 1 WHERE id = ?`, id)
	if err != nil {
		return nil, "", fmt.Errorf("mark magic link used: %w", err)
	}

	// Links minted before migration 058 can still hold the address exactly
	// as it was typed, so canonicalize on the way out too — otherwise a
	// pending link would miss the row the migration just lowercased and
	// mint a second account for an address that already has one. A stored
	// address that no longer parses is a dead link rather than the start of
	// an account: better to refuse it than to sign up whatever it holds.
	email, err = NormalizeEmail(email)
	if err != nil {
		return nil, "", err
	}

	// Find user by email.
	var user model.User
	err = tx.QueryRow(
		`SELECT id, COALESCE(email,''), username, display_name, bio, avatar_url, role, created_at, updated_at FROM users WHERE email = ?`,
		email,
	).Scan(&user.ID, &user.Email, &user.Username, &user.DisplayName, &user.Bio, &user.AvatarURL, &user.Role, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		// New email: issue a signup token instead of creating a user.
		// The account is created in CompleteSignup once the person has
		// chosen their permanent username.
		rawSignup, err := createSignupToken(tx, email)
		if err != nil {
			return nil, "", err
		}
		if err := tx.Commit(); err != nil {
			return nil, "", fmt.Errorf("commit: %w", err)
		}
		return nil, rawSignup, nil
	} else if err != nil {
		return nil, "", fmt.Errorf("query user: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, "", fmt.Errorf("commit: %w", err)
	}

	return &user, "", nil
}

// VerifyMagicCode validates the six-digit code from the sign-in email and
// marks its row used, mirroring VerifyMagicLink: an address with an account
// returns that user, an unknown address returns a signup token and creates
// nobody (docs/adr/013). It exists for a client that cannot receive the
// emailed link in its own session — the proof walks back by hand instead of
// by redirect — and it proves exactly what the link proves.
//
// Every failure is ErrInvalidMagicCode and nothing else. Wrong digits, a
// spent row, an expired one, a row minted before codes existed, and an
// address nobody ever asked about are one answer, so the endpoint cannot be
// asked whether an address has ever been used here.
func VerifyMagicCode(db *database.DB, email, code string) (*model.User, string, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		// A malformed address here is not the request endpoint's
		// correctable typo: nothing is being sent, so the generic refusal
		// is the whole answer.
		return nil, "", ErrInvalidMagicCode
	}

	code = normalizeMagicCode(code)
	if code == "" {
		// Nothing was typed, so there is nothing to compare — refused
		// without spending one of the row's five attempts, which belong to
		// guesses rather than to empty submissions.
		return nil, "", ErrInvalidMagicCode
	}
	codeHash := hashMagicCode(code)

	tx, err := db.Begin()
	if err != nil {
		return nil, "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// The newest unused row for this address that actually carries a code.
	// `code_hash IS NOT NULL` is what keeps a row minted before the codes
	// migration permanently un-code-verifiable: it never had one, so no
	// string can be the code it was never sent.
	//
	// Expiry is checked below rather than in the WHERE clause. Every row has
	// the same 15-minute life, so the newest is also the last to expire and
	// no live row hides behind an expired one; and expires_at is text written
	// by more than one layout, which a SQL string comparison would accept or
	// refuse on formatting rather than on time.
	var id, expiresAt, storedHash string
	var attempts int
	err = tx.QueryRow(
		`SELECT id, expires_at, code_hash, attempts FROM magic_links
		 WHERE email = ? AND used = 0 AND code_hash IS NOT NULL
		 ORDER BY created_at DESC LIMIT 1`,
		email,
	).Scan(&id, &expiresAt, &storedHash, &attempts)
	if err == sql.ErrNoRows {
		return nil, "", ErrInvalidMagicCode
	}
	if err != nil {
		return nil, "", fmt.Errorf("query magic link: %w", err)
	}

	if exp, perr := clock.Parse(expiresAt); perr == nil && time.Now().After(exp) {
		return nil, "", ErrInvalidMagicCode
	}

	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(codeHash)) != 1 {
		// Count the guess, and at five spend the row. A code is six digits,
		// so the row has to stop answering long before a guesser gets
		// anywhere — the attempt counter, not the keyspace, is the guard.
		// The emailed link shares the row's fate because both credentials
		// prove the same one thing, and a link left live after its code was
		// attacked would leave the attacked request still open.
		attempts++
		used := 0
		if attempts >= maxCodeAttempts {
			used = 1
		}
		if _, err := tx.Exec(`UPDATE magic_links SET attempts = ?, used = ? WHERE id = ?`, attempts, used, id); err != nil {
			return nil, "", fmt.Errorf("record code attempt: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, "", fmt.Errorf("commit: %w", err)
		}
		return nil, "", ErrInvalidMagicCode
	}

	if _, err := tx.Exec(`UPDATE magic_links SET used = 1 WHERE id = ?`, id); err != nil {
		return nil, "", fmt.Errorf("mark magic link used: %w", err)
	}

	var user model.User
	err = tx.QueryRow(
		`SELECT id, COALESCE(email,''), username, display_name, bio, avatar_url, role, created_at, updated_at FROM users WHERE email = ?`,
		email,
	).Scan(&user.ID, &user.Email, &user.Username, &user.DisplayName, &user.Bio, &user.AvatarURL, &user.Role, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		// New address: a signup token, not an account — the username is
		// chosen, never derived (docs/adr/013), exactly as the link does it.
		rawSignup, err := createSignupToken(tx, email)
		if err != nil {
			return nil, "", err
		}
		if err := tx.Commit(); err != nil {
			return nil, "", fmt.Errorf("commit: %w", err)
		}
		return nil, rawSignup, nil
	} else if err != nil {
		return nil, "", fmt.Errorf("query user: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, "", fmt.Errorf("commit: %w", err)
	}

	return &user, "", nil
}
