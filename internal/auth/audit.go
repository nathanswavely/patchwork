package auth

import (
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// LogAuditEvent writes an entry to the audit_log table.
func LogAuditEvent(db *database.DB, userID, action, entityType, entityID, metadata, ip string) {
	id := NewUUIDv7()
	if metadata == "" {
		metadata = "{}"
	}
	db.Exec(
		`INSERT INTO audit_log (id, user_id, action, entity_type, entity_id, metadata, ip_address) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, userID, action, entityType, entityID, metadata, ip,
	)
}
