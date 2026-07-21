package auth

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// Signup tokens bridge the two-phase magic-link flow (docs/adr/013):
// clicking the emailed link proves control of the address; the account is
// created only when the person chooses their permanent username. Longer
// lived than the magic link itself (60m vs 15m) so the username step is
// not a trap — and if it expires, the recovery is simply requesting a new
// sign-in link.
const signupTokenExpiry = 60 * time.Minute

// createSignupToken stores a hashed single-use signup token for an email
// inside an existing transaction and returns the raw token.
func createSignupToken(tx *sql.Tx, email string) (string, error) {
	rawToken, err := generateToken()
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	id := NewUUIDv7()
	expiresAt := time.Now().Add(signupTokenExpiry).UTC().Format(time.RFC3339)

	_, err = tx.Exec(
		`INSERT INTO signup_tokens (id, email, token, expires_at) VALUES (?, ?, ?, ?)`,
		id, email, tokenHash, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("insert signup token: %w", err)
	}

	return rawToken, nil
}

// ValidateSignupToken checks a raw signup token without consuming it and
// returns the email it certifies. Lets the signup-completion page show
// which address is being signed up before the form is submitted.
func ValidateSignupToken(db *database.DB, rawToken string) (string, error) {
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	var email, expiresAt string
	var used int
	err := db.QueryRow(
		`SELECT email, expires_at, used FROM signup_tokens WHERE token = ?`,
		tokenHash,
	).Scan(&email, &expiresAt, &used)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("invalid signup token")
	}
	if err != nil {
		return "", fmt.Errorf("query signup token: %w", err)
	}
	if used != 0 {
		return "", fmt.Errorf("signup already completed")
	}
	exp, err := time.Parse(time.RFC3339, expiresAt)
	if err == nil && time.Now().After(exp) {
		return "", fmt.Errorf("signup link has expired — request a new sign-in link")
	}
	return email, nil
}

// CompleteSignup consumes a signup token and creates the account with the
// chosen username. Same guarantees as the invite path: first-account admin
// bootstrap, ap_id, AP keypair, all inside one transaction.
func CompleteSignup(db *database.DB, rawToken, rawUsername, displayName string) (*model.User, error) {
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var id, email, expiresAt string
	var used int
	err = tx.QueryRow(
		`SELECT id, email, expires_at, used FROM signup_tokens WHERE token = ?`,
		tokenHash,
	).Scan(&id, &email, &expiresAt, &used)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid signup token")
	}
	if err != nil {
		return nil, fmt.Errorf("query signup token: %w", err)
	}
	if used != 0 {
		return nil, fmt.Errorf("signup already completed")
	}
	exp, err := time.Parse(time.RFC3339, expiresAt)
	if err == nil && time.Now().After(exp) {
		return nil, fmt.Errorf("signup link has expired — request a new sign-in link")
	}

	// Two concurrent signup tokens for the same address: first one wins.
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM users WHERE email = ?`, email).Scan(&n); err != nil {
		return nil, fmt.Errorf("check email: %w", err)
	}
	if n > 0 {
		return nil, fmt.Errorf("an account with this email already exists — sign in instead")
	}

	username, err := PrepareUsername(tx, rawUsername)
	if err != nil {
		return nil, err
	}

	if displayName = strings.TrimSpace(displayName); displayName == "" {
		displayName = username
	}

	userID := NewUUIDv7()
	now := time.Now().UTC().Format(time.RFC3339)
	role := roleForNewUser(tx)

	apID := ap.UserAPID(ap.GetDomain(), userID)
	_, err = tx.Exec(
		`INSERT INTO users (id, email, username, display_name, role, created_at, updated_at, ap_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, email, username, displayName, role, now, now, apID,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	if role == "admin" {
		log.Printf("auth: first account %q created — bootstrapped as instance admin", username)
	}

	if _, err := tx.Exec(`UPDATE signup_tokens SET used = 1 WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("mark signup token used: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Generate keypair for ActivityPub federation (after commit, since
	// EnsureUserKeypair uses db directly).
	ap.EnsureUserKeypair(db, userID)

	return &model.User{
		ID:          userID,
		Email:       email,
		Username:    username,
		DisplayName: displayName,
		Role:        role,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}
