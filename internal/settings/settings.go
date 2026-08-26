// Package settings reads and writes the instance_settings table: the
// community-editable identity overrides introduced by docs/adr/014.
//
// A value here overrides the corresponding patchwork.yaml field at request
// time; the yaml value remains the bootstrap default. Deployment concerns
// (domain, ports, SMTP, federation) deliberately have no keys here.
package settings

import (
	"strings"
	"sync/atomic"
	"time"

	// The distroless image ships no tzdata, and a configured zone name has
	// to resolve wherever the binary runs.
	_ "time/tzdata"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Setting keys. Keep this list short on purpose — every key added here is
// a promise that the instance admin can change it from the UI.
const (
	KeyName        = "instance_name"
	KeyDescription = "instance_description"

	// The quilt icon is a drafted block, stored as JSON — a block plus the
	// fabrics it is pieced from (docs/adr/043). No key means the quilt
	// wears a starter block assigned from its name.
	KeyIconDesign = "icon_design"

	// Legal documents (docs/adr/028): a stored value replaces the shipped
	// default template wholesale; no key means the default is in effect.
	KeyLegalPrivacy = "legal_privacy"
	KeyLegalTerms   = "legal_terms"

	// Quilt policy (docs/adr/037): "true" hides amended-lining patches from
	// discovery for everyone. The per-user twin is users.hide_amended_linings;
	// strictest wins — the user switch can hide more, never reveal more.
	KeyHideAmendedLinings = "hide_amended_linings"

	// Where this quilt keeps time (docs/adr/045): an IANA zone name, the
	// bottom rung of the chain an event's zone resolves through. Overrides
	// geographic.timezone in patchwork.yaml. Editable here rather than only
	// in yaml because getting it wrong is visible — every event renders
	// hours off — and the fix should not need a redeploy.
	KeyTimezone = "instance_timezone"
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

// timezoneDefault holds the patchwork.yaml zone, recorded once at startup.
//
// The other Effective* helpers take the whole config, which works because
// their callers already hold one. This one is read while building event
// payloads — the events list, a patch profile, every feed — and threading
// a config pointer through all of them to reach a single string would be
// churn in service of consistency alone. The domain has the same shape and
// the same answer: ap.SetDomain at startup, ap.GetDomain() at the call.
var timezoneDefault atomic.Pointer[string]

// SetTimezoneDefault records the configured zone. Called once at startup,
// before anything serves a request or syncs a feed.
func SetTimezoneDefault(name string) {
	name = strings.TrimSpace(name)
	timezoneDefault.Store(&name)
}

// EffectiveTimezone returns the quilt's zone: the admin's override, else
// the configured default, else UTC. Never empty — a caller resolving an
// event's zone needs a terminating rung, and "no answer" is not one.
func EffectiveTimezone(db *database.DB) string {
	if v, ok := Get(db, KeyTimezone); ok {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	if p := timezoneDefault.Load(); p != nil && *p != "" {
		return *p
	}
	return "UTC"
}

// ValidTimezone reports whether name is a zone this binary can resolve.
// The distroless image carries no tzdata, so the answer comes from the
// embedded copy rather than the host.
func ValidTimezone(name string) bool {
	_, err := time.LoadLocation(strings.TrimSpace(name))
	return err == nil
}
