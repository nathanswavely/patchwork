package notifications

import (
	"log"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Event describes something that happened and should generate notifications.
type Event struct {
	Type     NotificationType
	NodeID   string // Patch context (empty for site-level admin notifications)
	NodeSlug string // For building links
	NodeName string // For display / email subject
	ActorID  string // Who triggered the event (excluded from recipients)
	TargetID string // Specific user for AudienceSpecificUser
	EntityID string // Proposal/event/doc ID for building links
	Title    string // Notification title
	Body     string // Notification body text
	Link     string // In-app link path
}

// Notifier is the main entry point for sending notifications.
type Notifier struct {
	DB       *database.DB
	Channels []Channel
}

// NewNotifier creates a Notifier with the in-app channel always present.
// Additional channels (email, etc.) can be appended to Channels.
func NewNotifier(db *database.DB) *Notifier {
	return &Notifier{
		DB:       db,
		Channels: []Channel{&InAppChannel{}},
	}
}

// AvailableChannels returns the names of all configured channels.
func (n *Notifier) AvailableChannels() []string {
	var names []string
	for _, ch := range n.Channels {
		if ch.Available() {
			names = append(names, ch.Name())
		}
	}
	return names
}

// Notify resolves recipients, checks preferences, and delivers via all channels.
func (n *Notifier) Notify(event Event) {
	meta, ok := TypeRegistry[event.Type]
	if !ok {
		log.Printf("notifications: unknown type %q", event.Type)
		return
	}

	// 1. Check patch-level category config.
	if event.NodeID != "" && !IsCategoryEnabled(n.DB, event.NodeID, meta.Category) {
		return
	}

	// 2. Resolve recipients.
	recipients := n.resolveRecipients(event, meta.Audience)

	// 3. Filter out actor.
	filtered := make([]string, 0, len(recipients))
	for _, uid := range recipients {
		if uid != event.ActorID {
			filtered = append(filtered, uid)
		}
	}

	if len(filtered) == 0 {
		return
	}

	// 4. For each recipient × channel, check preferences and deliver.
	for _, uid := range filtered {
		for _, ch := range n.Channels {
			if !ch.Available() {
				continue
			}
			if IsChannelEnabled(n.DB, uid, event.Type, ch.Name()) {
				ch.Send(n.DB, uid, event)
			}
		}
	}
}

// resolveRecipients returns user IDs based on audience type.
func (n *Notifier) resolveRecipients(event Event, audience Audience) []string {
	switch audience {
	case AudienceSpecificUser:
		if event.TargetID != "" {
			return []string{event.TargetID}
		}
		return nil

	case AudienceAdminsOnly:
		return n.queryUserIDs(
			`SELECT user_id FROM memberships WHERE node_id = ? AND status = 'active' AND role = 'admin'`,
			event.NodeID,
		)

	case AudienceAllMembers:
		return n.queryUserIDs(
			`SELECT user_id FROM memberships WHERE node_id = ? AND status = 'active' AND role IN ('admin', 'member')`,
			event.NodeID,
		)

	case AudienceParticipants:
		if event.EntityID == "" {
			return nil
		}
		// Voters + commenters + proposal author.
		return n.queryUserIDs(
			`SELECT DISTINCT user_id FROM (
				SELECT user_id FROM votes WHERE proposal_id = ?
				UNION
				SELECT author_id AS user_id FROM proposal_comments WHERE proposal_id = ?
				UNION
				SELECT author_id AS user_id FROM proposals WHERE id = ?
			)`,
			event.EntityID, event.EntityID, event.EntityID,
		)

	case AudienceSiteAdmins:
		return n.queryUserIDs(`SELECT id FROM users WHERE role = 'admin'`)

	default:
		return nil
	}
}

func (n *Notifier) queryUserIDs(query string, args ...interface{}) []string {
	rows, err := n.DB.Query(query, args...)
	if err != nil {
		log.Printf("notifications: query recipients: %v", err)
		return nil
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
