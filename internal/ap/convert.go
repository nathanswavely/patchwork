package ap

import (
	"fmt"

	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// NodeToActor converts a Patchwork Node to an ActivityPub Actor (Organization).
func NodeToActor(node model.Node, domain string) Actor {
	baseURL := fmt.Sprintf("https://%s", domain)
	apID := fmt.Sprintf("%s/ap/nodes/%s", baseURL, node.ID)

	return Actor{
		Context:           Context,
		Type:              "Organization",
		ID:                apID,
		Name:              node.Name,
		PreferredUsername: node.Slug,
		Summary:           node.Description,
		URL:               fmt.Sprintf("%s/patches/%s", baseURL, node.Slug),
		Inbox:             apID + "/inbox",
		Outbox:            apID + "/outbox",
		Followers:         apID + "/followers",
		Following:         apID + "/following",
	}
}

// EventToObject converts a Patchwork Event to an ActivityPub Object (Event).
func EventToObject(event model.Event, domain string) Object {
	baseURL := fmt.Sprintf("https://%s", domain)
	apID := fmt.Sprintf("%s/ap/events/%s", baseURL, event.ID)

	obj := Object{
		Context:   Context,
		Type:      "Event",
		ID:        apID,
		Name:      event.Title,
		Content:   event.Description,
		URL:       fmt.Sprintf("%s/events/%s", baseURL, event.ID),
		StartTime: event.StartsAt,
	}

	if event.EndsAt != nil {
		obj.EndTime = *event.EndsAt
	}

	if event.Location != "" || (event.Latitude != nil && event.Longitude != nil) {
		place := &Place{Type: "Place", Name: event.Location}
		if event.Latitude != nil {
			place.Latitude = *event.Latitude
		}
		if event.Longitude != nil {
			place.Longitude = *event.Longitude
		}
		obj.Location = place
	}

	return obj
}

// UserToActor converts a Patchwork User to an ActivityPub Actor (Person).
func UserToActor(user model.User, domain string) Actor {
	baseURL := fmt.Sprintf("https://%s", domain)
	apID := fmt.Sprintf("%s/ap/users/%s", baseURL, user.ID)

	return Actor{
		Context:           Context,
		Type:              "Person",
		ID:                apID,
		Name:              user.DisplayName,
		PreferredUsername: user.Username,
		Summary:           user.Bio,
		URL:               fmt.Sprintf("%s/users/%s", baseURL, user.Username),
		Inbox:             apID + "/inbox",
		Outbox:            apID + "/outbox",
		Followers:         apID + "/followers",
		Following:         apID + "/following",
	}
}
