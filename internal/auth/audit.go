package auth

import (
	"log"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// LogAuditEvent writes an entry to the audit_log table.
func LogAuditEvent(db *database.DB, userID, action, entityType, entityID, metadata, ip string) {
	id := NewUUIDv7()
	if metadata == "" {
		metadata = "{}"
	}
	// An actorless entry — the clock resolved a vote, seated a council,
	// lapsed a proposal — has no user. `user_id` is a nullable foreign key,
	// and '' is not NULL: with foreign keys on, the insert was refused and
	// the error dropped, so every clock-driven audit entry the product
	// thought it wrote (election.resolved, election.unsettled,
	// proposal.resolved, membership.ratified) was silently lost.
	var actor interface{} = userID
	if userID == "" {
		actor = nil
	}
	if _, err := db.Exec(
		`INSERT INTO audit_log (id, user_id, action, entity_type, entity_id, metadata, ip_address) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, actor, action, entityType, entityID, metadata, ip,
	); err != nil {
		log.Printf("audit: %s on %s %s not recorded: %v", action, entityType, entityID, err)
	}
}
