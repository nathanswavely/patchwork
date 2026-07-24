package auth

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/mail"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

const magicLinkExpiry = 15 * time.Minute

// GenerateMagicLink creates a magic link, stores its hash, and sends it via
// SMTP. linkFor turns the raw token into the URL that goes in the email — the
// caller owns URL shape so the emailed link and the one printed to the log
// (no-SMTP dev) can never drift apart again.
func GenerateMagicLink(db *database.DB, email string, smtpCfg config.SMTP, linkFor func(token string) string) error {
	rawToken, err := generateToken()
	if err != nil {
		return err
	}

	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	id := NewUUIDv7()
	expiresAt := time.Now().Add(magicLinkExpiry).UTC().Format(time.RFC3339)

	_, err = db.Exec(
		`INSERT INTO magic_links (id, email, token, expires_at) VALUES (?, ?, ?, ?)`,
		id, email, tokenHash, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("insert magic link: %w", err)
	}

	// Send the email.
	link := linkFor(rawToken)
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Sign in to Patchwork\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nClick to sign in:\n\n%s\n\nThis link expires in 15 minutes.\n", smtpCfg.From, email, link)

	if err := mail.Send(smtpCfg, []string{email}, []byte(body)); err != nil {
		return fmt.Errorf("send magic link email: %w", err)
	}

	return nil
}

// GenerateMagicLinkLocal creates a magic link and stores it, but returns the raw token
// instead of emailing it. Used when SMTP is not configured (local dev).
func GenerateMagicLinkLocal(db *database.DB, email string) (string, error) {
	rawToken, err := generateToken()
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	id := NewUUIDv7()
	expiresAt := time.Now().Add(magicLinkExpiry).UTC().Format(time.RFC3339)

	_, err = db.Exec(
		`INSERT INTO magic_links (id, email, token, expires_at) VALUES (?, ?, ?, ?)`,
		id, email, tokenHash, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("insert magic link: %w", err)
	}

	return rawToken, nil
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

	exp, err := time.Parse(time.RFC3339, expiresAt)
	if err == nil && time.Now().After(exp) {
		return nil, "", fmt.Errorf("magic link has expired")
	}

	// Mark used.
	_, err = tx.Exec(`UPDATE magic_links SET used = 1 WHERE id = ?`, id)
	if err != nil {
		return nil, "", fmt.Errorf("mark magic link used: %w", err)
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
