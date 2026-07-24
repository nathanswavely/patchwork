package ap

import "fmt"

// domain holds the configured instance domain for AP ID generation.
var domain string

// SetDomain sets the domain used for generating AP IDs.
// Called once on startup from main.go after loading config.
func SetDomain(d string) { domain = d }

// GetDomain returns the configured domain, defaulting to "localhost".
func GetDomain() string {
	if domain == "" {
		return "localhost"
	}
	return domain
}

// APID generates a stable ActivityPub ID for an entity.
func APID(domain, entityType, entityID string) string {
	if domain == "" {
		domain = "localhost"
	}
	return fmt.Sprintf("https://%s/ap/%s/%s", domain, entityType, entityID)
}

// UserAPID generates an AP ID for a user.
func UserAPID(domain, userID string) string {
	return APID(domain, "users", userID)
}

// NodeAPID generates an AP ID for a node (patch).
func NodeAPID(domain, nodeID string) string {
	return APID(domain, "nodes", nodeID)
}

// EventAPID generates an AP ID for an event.
func EventAPID(domain, eventID string) string {
	return APID(domain, "events", eventID)
}

// ProposalAPID generates an AP ID for a proposal.
func ProposalAPID(domain, proposalID string) string {
	return APID(domain, "proposals", proposalID)
}
