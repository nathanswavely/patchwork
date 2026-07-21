package ap_test

import (
	"encoding/json"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/ap"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

func TestNodeToActor(t *testing.T) {
	node := model.Node{
		ID:          "test-node-id",
		Name:        "Lancaster Arts",
		Slug:        "lancaster-arts",
		Description: "An arts community",
	}

	actor := ap.NodeToActor(node, "arts.lancaster.pw")

	if actor.Context != "https://www.w3.org/ns/activitystreams" {
		t.Errorf("expected AP context, got %s", actor.Context)
	}
	if actor.Type != "Organization" {
		t.Errorf("expected Organization, got %s", actor.Type)
	}
	if actor.ID != "https://arts.lancaster.pw/ap/nodes/test-node-id" {
		t.Errorf("unexpected ID: %s", actor.ID)
	}
	if actor.Name != "Lancaster Arts" {
		t.Errorf("unexpected Name: %s", actor.Name)
	}
	if actor.PreferredUsername != "lancaster-arts" {
		t.Errorf("expected PreferredUsername=lancaster-arts, got %s", actor.PreferredUsername)
	}
	if actor.Inbox != "https://arts.lancaster.pw/ap/nodes/test-node-id/inbox" {
		t.Errorf("unexpected Inbox: %s", actor.Inbox)
	}

	// Ensure it's valid JSON.
	data, err := json.Marshal(actor)
	if err != nil {
		t.Fatalf("marshal actor: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal actor: %v", err)
	}
	if parsed["@context"] != "https://www.w3.org/ns/activitystreams" {
		t.Errorf("@context missing in JSON-LD output")
	}
}

func TestEventToObject(t *testing.T) {
	lat := 40.0379
	lng := -76.3055
	endsAt := "2026-04-01T18:00:00Z"
	event := model.Event{
		ID:          "test-event-id",
		Title:       "Gallery Opening",
		Description: "A gallery opening event",
		Location:    "Gallery Row",
		Latitude:    &lat,
		Longitude:   &lng,
		StartsAt:    "2026-04-01T10:00:00Z",
		EndsAt:      &endsAt,
	}

	obj := ap.EventToObject(event, "arts.lancaster.pw")

	if obj.Type != "Event" {
		t.Errorf("expected Event, got %s", obj.Type)
	}
	if obj.StartTime != "2026-04-01T10:00:00Z" {
		t.Errorf("unexpected StartTime: %s", obj.StartTime)
	}
	if obj.EndTime != "2026-04-01T18:00:00Z" {
		t.Errorf("unexpected EndTime: %s", obj.EndTime)
	}
	if obj.Location == nil {
		t.Fatal("expected location to be set")
	}
	if obj.Location.Name != "Gallery Row" {
		t.Errorf("unexpected location name: %s", obj.Location.Name)
	}
}

func TestUserToActor(t *testing.T) {
	user := model.User{
		ID:          "test-user-id",
		Username:    "jdoe",
		DisplayName: "Jane Doe",
		Bio:         "Artist and community organizer",
	}

	actor := ap.UserToActor(user, "arts.lancaster.pw")

	if actor.Type != "Person" {
		t.Errorf("expected Person, got %s", actor.Type)
	}
	if actor.Name != "Jane Doe" {
		t.Errorf("unexpected Name: %s", actor.Name)
	}
	if actor.PreferredUsername != "jdoe" {
		t.Errorf("expected PreferredUsername=jdoe, got %s", actor.PreferredUsername)
	}
	if actor.URL != "https://arts.lancaster.pw/users/jdoe" {
		t.Errorf("unexpected URL: %s", actor.URL)
	}
}

func TestNodeToActor_HasInboxOutbox(t *testing.T) {
	node := model.Node{
		ID:   "node-inbox-test",
		Name: "Test Node",
		Slug: "test-node",
	}

	actor := ap.NodeToActor(node, "example.com")

	if actor.Inbox != "https://example.com/ap/nodes/node-inbox-test/inbox" {
		t.Errorf("unexpected Inbox: %s", actor.Inbox)
	}
	if actor.Outbox != "https://example.com/ap/nodes/node-inbox-test/outbox" {
		t.Errorf("unexpected Outbox: %s", actor.Outbox)
	}
	if actor.Followers != "https://example.com/ap/nodes/node-inbox-test/followers" {
		t.Errorf("unexpected Followers: %s", actor.Followers)
	}
	if actor.Following != "https://example.com/ap/nodes/node-inbox-test/following" {
		t.Errorf("unexpected Following: %s", actor.Following)
	}
}

func TestUserToActor_HasInboxOutbox(t *testing.T) {
	user := model.User{
		ID:          "user-inbox-test",
		Username:    "tester",
		DisplayName: "Tester",
	}

	actor := ap.UserToActor(user, "example.com")

	if actor.Inbox != "https://example.com/ap/users/user-inbox-test/inbox" {
		t.Errorf("unexpected Inbox: %s", actor.Inbox)
	}
	if actor.Outbox != "https://example.com/ap/users/user-inbox-test/outbox" {
		t.Errorf("unexpected Outbox: %s", actor.Outbox)
	}
	if actor.Followers != "https://example.com/ap/users/user-inbox-test/followers" {
		t.Errorf("unexpected Followers: %s", actor.Followers)
	}
	if actor.Following != "https://example.com/ap/users/user-inbox-test/following" {
		t.Errorf("unexpected Following: %s", actor.Following)
	}
}

func TestEventToObject_HasAPID(t *testing.T) {
	event := model.Event{
		ID:       "event-apid-test",
		Title:    "Test Event",
		StartsAt: "2026-04-01T10:00:00Z",
	}

	obj := ap.EventToObject(event, "example.com")

	if obj.ID != "https://example.com/ap/events/event-apid-test" {
		t.Errorf("unexpected AP ID: %s", obj.ID)
	}
}

func TestNodeToActor_JSONMarshal(t *testing.T) {
	node := model.Node{
		ID:          "json-test-id",
		Name:        "JSON Test",
		Slug:        "json-test",
		Description: "Testing JSON output",
	}

	actor := ap.NodeToActor(node, "example.com")

	data, err := json.Marshal(actor)
	if err != nil {
		t.Fatalf("marshal actor: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal actor: %v", err)
	}

	if parsed["@context"] != "https://www.w3.org/ns/activitystreams" {
		t.Errorf("@context missing or wrong in JSON output: %v", parsed["@context"])
	}
	if parsed["type"] != "Organization" {
		t.Errorf("type missing or wrong in JSON output: %v", parsed["type"])
	}
	if parsed["preferredUsername"] != "json-test" {
		t.Errorf("preferredUsername missing or wrong in JSON output: %v", parsed["preferredUsername"])
	}
}
