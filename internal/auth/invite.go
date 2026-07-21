package auth

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// GenerateInviteLink creates an invite link and stores its SHA-256 hash.
// Returns the raw token (to be shared).
func GenerateInviteLink(db *database.DB, createdBy string, maxUses int, expiresAt *time.Time) (string, error) {
	rawToken, err := generateToken()
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	id := NewUUIDv7()
	if maxUses <= 0 {
		maxUses = 1
	}

	var expStr *string
	if expiresAt != nil {
		s := expiresAt.UTC().Format(time.RFC3339)
		expStr = &s
	}

	_, err = db.Exec(
		`INSERT INTO invite_links (id, created_by, token, max_uses, expires_at) VALUES (?, ?, ?, ?, ?)`,
		id, createdBy, tokenHash, maxUses, expStr,
	)
	if err != nil {
		return "", fmt.Errorf("insert invite link: %w", err)
	}

	return rawToken, nil
}

// ValidateInviteLink checks whether a raw invite token is redeemable
// (exists, not expired, uses remaining) without consuming a use.
func ValidateInviteLink(db *database.DB, rawToken string) error {
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	var maxUses, useCount int
	var expiresAt sql.NullString
	err := db.QueryRow(
		`SELECT max_uses, use_count, expires_at FROM invite_links WHERE token = ?`,
		tokenHash,
	).Scan(&maxUses, &useCount, &expiresAt)
	if err == sql.ErrNoRows {
		return fmt.Errorf("invalid invite token")
	}
	if err != nil {
		return fmt.Errorf("query invite: %w", err)
	}

	if expiresAt.Valid {
		exp, err := time.Parse(time.RFC3339, expiresAt.String)
		if err == nil && time.Now().After(exp) {
			return fmt.Errorf("invite link has expired")
		}
	}

	if useCount >= maxUses {
		return fmt.Errorf("invite link has reached maximum uses")
	}

	return nil
}

// RedeemInviteLink validates a raw invite token, creates a user, and increments the use count.
func RedeemInviteLink(db *database.DB, rawToken, username, displayName string) (*model.User, error) {
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var id string
	var maxUses, useCount int
	var expiresAt sql.NullString

	err = tx.QueryRow(
		`SELECT id, max_uses, use_count, expires_at FROM invite_links WHERE token = ?`,
		tokenHash,
	).Scan(&id, &maxUses, &useCount, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid invite token")
	}
	if err != nil {
		return nil, fmt.Errorf("query invite: %w", err)
	}

	// Check expiry.
	if expiresAt.Valid {
		exp, err := time.Parse(time.RFC3339, expiresAt.String)
		if err == nil && time.Now().After(exp) {
			return nil, fmt.Errorf("invite link has expired")
		}
	}

	// Check uses.
	if useCount >= maxUses {
		return nil, fmt.Errorf("invite link has reached maximum uses")
	}

	// Create user. Username is chosen by the person and validated here
	// (docs/adr/013): normalized, format-checked, reserved-word-checked,
	// and confirmed available — a friendly error, not a UNIQUE violation.
	username, err = PrepareUsername(tx, username)
	if err != nil {
		return nil, err
	}

	userID := NewUUIDv7()
	now := time.Now().UTC().Format(time.RFC3339)
	if displayName == "" {
		displayName = username
	}
	role := roleForNewUser(tx)

	apID := ap.UserAPID(ap.GetDomain(), userID)
	_, err = tx.Exec(
		`INSERT INTO users (id, username, display_name, role, created_at, updated_at, ap_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, username, displayName, role, now, now, apID,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	if role == "admin" {
		log.Printf("auth: first account %q created — bootstrapped as instance admin", username)
	}

	// Increment use count.
	_, err = tx.Exec(`UPDATE invite_links SET use_count = use_count + 1 WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("increment use count: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Generate keypair for ActivityPub federation.
	ap.EnsureUserKeypair(db, userID)

	return &model.User{
		ID:          userID,
		Username:    username,
		DisplayName: displayName,
		Role:        role,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}
