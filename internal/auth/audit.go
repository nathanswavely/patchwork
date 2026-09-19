package auth

import (
	"encoding/json"
	"log"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// LogAuditEventJSON writes an entry to the audit_log table, marshaling
// payload to JSON rather than trusting a caller to have hand-built a valid
// JSON string. The inputs behind today's hand-built payloads are all
// validated or fixed-set, so nothing is unsafe yet — but a hand-built
// literal is a landmine for the day a free-text field (a join message, a
// display name, a reason) joins one of them. On a marshal error the event
// is still recorded, with the error message as the payload, because a
// malformed payload is a bug to fix, not a reason to drop the audit line.
func LogAuditEventJSON(db *database.DB, userID, action, entityType, entityID string, payload any, ip string) {
	metadata := "{}"
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			log.Printf("audit: %s on %s %s payload not marshaled: %v", action, entityType, entityID, err)
			if eb, merr := json.Marshal(map[string]string{"marshal_error": err.Error()}); merr == nil {
				metadata = string(eb)
			}
		} else {
			metadata = string(b)
		}
	}
	LogAuditEvent(db, userID, action, entityType, entityID, metadata, ip)
}

// LogAuditEvent writes an entry to the audit_log table. metadata must
// already be a valid JSON object string; prefer LogAuditEventJSON, which
// marshals a Go value instead of trusting a hand-built literal.
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
