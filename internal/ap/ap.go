// Package ap provides ActivityPub-compatible types and conversion functions.
// This is Phase A — data model only, no HTTP endpoints yet.
package ap

// Context is the ActivityPub JSON-LD context URL.
const Context = "https://www.w3.org/ns/activitystreams"

// Actor represents an ActivityPub Actor (Person, Organization, etc.).
type Actor struct {
	Context           string `json:"@context"`
	Type              string `json:"type"`
	ID                string `json:"id"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferredUsername,omitempty"`
	Summary           string `json:"summary,omitempty"`
	URL               string `json:"url,omitempty"`
	Inbox             string `json:"inbox"`
	Outbox            string `json:"outbox"`
	Followers         string `json:"followers"`
	Following         string `json:"following"`
}

// Object represents an ActivityPub Object (Event, Note, etc.).
type Object struct {
	Context   string `json:"@context"`
	Type      string `json:"type"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Content   string `json:"content,omitempty"`
	URL       string `json:"url,omitempty"`
	StartTime string `json:"startTime,omitempty"`
	EndTime   string `json:"endTime,omitempty"`
	Location  *Place `json:"location,omitempty"`
}

// Place represents an ActivityPub Place for location data.
type Place struct {
	Type      string  `json:"type"`
	Name      string  `json:"name,omitempty"`
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
}

// Activity represents an ActivityPub Activity (Create, Update, Delete, etc.).
type Activity struct {
	Context string      `json:"@context"`
	Type    string      `json:"type"`
	ID      string      `json:"id"`
	Actor   string      `json:"actor"`
	Object  interface{} `json:"object"`
}

// Collection represents an ActivityPub Collection.
type Collection struct {
	Context    string        `json:"@context"`
	Type       string        `json:"type"`
	ID         string        `json:"id"`
	TotalItems int           `json:"totalItems"`
	Items      []interface{} `json:"items,omitempty"`
}

// OrderedCollection represents an ActivityPub OrderedCollection.
type OrderedCollection struct {
	Context      string        `json:"@context"`
	Type         string        `json:"type"`
	ID           string        `json:"id"`
	TotalItems   int           `json:"totalItems"`
	OrderedItems []interface{} `json:"orderedItems,omitempty"`
}
