// Package settings reads and writes the instance_settings table: the
// community-editable identity overrides introduced by docs/adr/014.
//
// A value here overrides the corresponding patchwork.yaml field at request
// time; the yaml value remains the bootstrap default. Deployment concerns
// (domain, ports, SMTP, federation) deliberately have no keys here.
package settings

import (
	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Setting keys. Keep this list short on purpose — every key added here is
// a promise that the instance admin can change it from the UI.
const (
	KeyName        = "instance_name"
	KeyDescription = "instance_description"
	KeyIconDefault = "icon_default"

	// Legal documents (docs/adr/028): a stored value replaces the shipped
	// default template wholesale; no key means the default is in effect.
	KeyLegalPrivacy = "legal_privacy"
	KeyLegalTerms   = "legal_terms"

	// Quilt policy (docs/adr/037): "true" hides amended-lining patches from
	// discovery for everyone. The per-user twin is users.hide_amended_linings;
	// strictest wins — the user switch can hide more, never reveal more.
	KeyHideAmendedLinings = "hide_amended_linings"
)

// Get returns the stored value for key and whether it exists.
func Get(db *database.DB, key string) (string, bool) {
	var v string
	err := db.QueryRow(`SELECT value FROM instance_settings WHERE key = ?`, key).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}

// GetDetailed returns the stored value and its updated_at timestamp.
func GetDetailed(db *database.DB, key string) (value, updatedAt string, ok bool) {
	err := db.QueryRow(`SELECT value, updated_at FROM instance_settings WHERE key = ?`, key).
		Scan(&value, &updatedAt)
	if err != nil {
		return "", "", false
	}
	return value, updatedAt, true
}

// Set upserts a value for key.
func Set(db *database.DB, key, value string) error {
	_, err := db.Exec(`INSERT INTO instance_settings (key, value, updated_at)
		VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value)
	return err
}

// Unset removes a key, restoring the patchwork.yaml default.
func Unset(db *database.DB, key string) error {
	_, err := db.Exec(`DELETE FROM instance_settings WHERE key = ?`, key)
	return err
}

// EffectiveName returns the DB-overridden instance name, falling back to
// the patchwork.yaml value.
func EffectiveName(db *database.DB, cfg *config.Config) string {
	if v, ok := Get(db, KeyName); ok && v != "" {
		return v
	}
	return cfg.Instance.Name
}

// EffectiveDescription returns the DB-overridden instance description,
// falling back to the patchwork.yaml value.
func EffectiveDescription(db *database.DB, cfg *config.Config) string {
	if v, ok := Get(db, KeyDescription); ok && v != "" {
		return v
	}
	return cfg.Instance.Description
}

// Icon returns the uploaded quilt icon, if any.
func Icon(db *database.DB) (mime string, data []byte, updatedAt string, ok bool) {
	err := db.QueryRow(`SELECT mime, data, updated_at FROM instance_icon WHERE id = 1`).
		Scan(&mime, &data, &updatedAt)
	if err != nil {
		return "", nil, "", false
	}
	return mime, data, updatedAt, true
}

// SetIcon stores (or replaces) the single uploaded quilt icon.
func SetIcon(db *database.DB, mime string, data []byte) error {
	_, err := db.Exec(`INSERT INTO instance_icon (id, mime, data, updated_at)
		VALUES (1, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT(id) DO UPDATE SET mime = excluded.mime, data = excluded.data, updated_at = excluded.updated_at`,
		mime, data)
	return err
}

// DeleteIcon removes the uploaded icon, reverting to a default block.
func DeleteIcon(db *database.DB) error {
	_, err := db.Exec(`DELETE FROM instance_icon WHERE id = 1`)
	return err
}
