package handler

import "github.com/patchwork-toolkit/patchwork/internal/database"

// SetRequireSignature toggles inbound AP signature verification for tests and
// returns the previous value so callers can restore it.
func SetRequireSignature(v bool) bool {
	prev := requireSignature
	requireSignature = v
	return prev
}

// CheckActorBinding exposes the signature/actor binding check to tests.
func CheckActorBinding(verifiedActor string, activity map[string]interface{}) error {
	return checkActorBinding(verifiedActor, activity)
}

// Suggested tags (docs/adr/114). These expose the unexported tag layer so the
// boundary rules can be asserted from the external test package.

func NormalizeTagNameForTest(raw string) string { return normalizeTagName(raw) }

func ResolveTagIDsForTest(db *database.DB, names []string) ([]string, string) {
	return resolveTagIDs(db, names)
}

func SetNodeTagsForTest(db *database.DB, nodeID string, tagIDs []string) error {
	return setNodeTags(db, nodeID, tagIDs)
}

func SuggestTagsForNodeForTest(db *database.DB, nodeID, userID string, names []string) (int, string) {
	return suggestTagsForNode(db, nodeID, userID, names)
}
