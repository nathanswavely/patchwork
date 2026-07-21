package ap

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// newUUIDv7 generates a UUIDv7 (time-sortable) string.
// This is a local copy to avoid an import cycle with the auth package.
func newUUIDv7() string {
	now := time.Now()
	ms := uint64(now.UnixMilli())

	var b [16]byte
	binary.BigEndian.PutUint16(b[0:2], uint16(ms>>32))
	binary.BigEndian.PutUint32(b[2:6], uint32(ms))
	rand.Read(b[6:])
	b[6] = (b[6] & 0x0F) | 0x70
	b[8] = (b[8] & 0x3F) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		b[10:16],
	)
}

// QueueActivity adds an activity to the outbox queue for delivery.
func QueueActivity(db *database.DB, activity map[string]interface{}, targetInbox string) error {
	id := newUUIDv7()
	actJSON, err := json.Marshal(activity)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO ap_outbox_queue (id, activity_json, target_inbox) VALUES (?, ?, ?)`,
		id, string(actJSON), targetInbox,
	)
	return err
}

// BroadcastToFollowers queues an activity for delivery to all followers of a local actor.
func BroadcastToFollowers(db *database.DB, localActorType, localActorID string, activity map[string]interface{}) error {
	rows, err := db.Query(
		`SELECT remote_inbox FROM ap_followers WHERE local_actor_type = ? AND local_actor_id = ? AND accepted = 1 AND remote_inbox IS NOT NULL AND remote_inbox != ''`,
		localActorType, localActorID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var inbox string
		if err := rows.Scan(&inbox); err != nil {
			continue
		}
		if err := QueueActivity(db, activity, inbox); err != nil {
			return err
		}
	}
	return rows.Err()
}

// BuildAcceptFollow builds an Accept(Follow) activity.
func BuildAcceptFollow(localActorID string, followActivity map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Accept",
		"actor":    localActorID,
		"object":   followActivity,
	}
}
