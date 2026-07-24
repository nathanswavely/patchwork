package notifications

import (
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
	db.Exec(
		`INSERT INTO notifications (id, user_id, type, title, body, link) VALUES (?, ?, ?, ?, ?, ?)`,
		id, recipientID, string(event.Type), event.Title, event.Body, event.Link,
	)
}
