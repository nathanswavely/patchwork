package handler

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
