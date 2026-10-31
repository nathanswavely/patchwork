package notifications

import (
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/auth"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Channel is a pluggable notification delivery mechanism.
type Channel interface {
	// Name returns the channel identifier stored in preferences (e.g., "in_app", "email").
	Name() string
	// Available reports whether this channel is configured and usable.
	Available() bool
	// Send delivers a notification to one recipient.
	Send(db *database.DB, recipientID string, event Event)
}

// InAppChannel writes to the notifications table. Always available.
type InAppChannel struct{}

func (c *InAppChannel) Name() string      { return "in_app" }
func (c *InAppChannel) Available() bool    { return true }
func (c *InAppChannel) Send(db *database.DB, recipientID string, event Event) {
	id := auth.NewUUIDv7()
	nodeID := event.NodeID
	if nodeID == "" {
		nodeID = NodeIDFromLink(db, event.Link)
	}
	db.Exec(
		`INSERT INTO notifications (id, user_id, type, title, body, link, node_id) VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''))`,
		id, recipientID, string(event.Type), event.Title, event.Body, event.Link, nodeID,
	)
}

// NodeIDFromLink finds the patch a notification link points into, for an
// event that did not name one. It is the match the notification_patch
// migration backfilled with: a /patches/{slug} link, bounded by '/' or '?' so
// "press" never matches "press-two", or an event's own patch for the
// canonical /events/{id} shape. Empty when the link is neither, and empty
// when the lookup fails: a notification with no patch named is what every
// row was before, so a miss is never worth refusing the notification over.
func NodeIDFromLink(db *database.DB, link string) string {
	var row interface{ Scan(...any) error }
	switch {
	case strings.HasPrefix(link, "/patches/"):
		row = db.QueryRow(`SELECT id FROM nodes
			WHERE ? = '/patches/' || slug OR ? LIKE '/patches/' || slug || '/%' OR ? LIKE '/patches/' || slug || '?%'
			LIMIT 1`, link, link, link)
	case strings.HasPrefix(link, "/events/"):
		row = db.QueryRow(`SELECT node_id FROM events
			WHERE ? = '/events/' || id OR ? LIKE '/events/' || id || '?%'
			LIMIT 1`, link, link)
	default:
		return ""
	}
	var id string
	if err := row.Scan(&id); err != nil {
		return ""
	}
	return id
}
