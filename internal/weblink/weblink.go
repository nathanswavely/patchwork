// Package weblink builds the in-app paths that notifications, notification
// emails, and public feeds point people at.
//
// The SPA's route table (web/src/App.svelte) is the only authority on what
// these look like, and nothing in Go can see it. Ten call sites each
// concatenated their own version of an event's path, all ten agreed, and all
// ten were wrong: `/patches/{slug}/events/{id}` was never a registered
// route, so every event notification, every reminder email, and every ICS
// and RSS item landed on the home quilt instead of the event (issue #56).
// A doc's path drifted the same way, into the proposal detail route.
//
// So: one function per surface, the route it targets named beside it. When a
// route moves, this file moves with it, and weblink_test.go is the list to
// check against App.svelte.
package weblink

// Patch is a patch's public profile — SPA route /patches/:slug.
func Patch(slug string) string {
	return "/patches/" + slug
}

// PatchEvents is a patch's workspace events tab — /patches/:slug/events.
func PatchEvents(slug string) string {
	return Patch(slug) + "/events"
}

// PatchMembers is a patch's workspace members tab — /patches/:slug/members.
func PatchMembers(slug string) string {
	return Patch(slug) + "/members"
}

// PatchMembersPending is the members tab filtered to pending join requests.
func PatchMembersPending(slug string) string {
	return PatchMembers(slug) + "?status=pending"
}

// PatchSetup is the post-claim setup flow — /patches/:slug/setup.
func PatchSetup(slug string) string {
	return Patch(slug) + "/setup"
}

// PatchSources is a patch's event-source settings — /patches/:slug/settings/sources.
// Where the aggregator's effects on a patch gather: the crosswalk entries
// pointing at it, its held duplicates, and the offers its programs make
// (docs/adr/063).
func PatchSources(slug string) string {
	return Patch(slug) + "/settings/sources"
}

// Event is an event's detail page — SPA route /events/:id.
//
// Events are addressed globally, not under the patch that owns them: one
// event can be linked to several patches (docs/adr/032), so there is no one
// patch to scope the URL to.
func Event(id string) string {
	return "/events/" + id
}

// PatchGovernance is a patch's governance hub, where its rules are stated —
// /patches/:slug/governance. Registered after the /governance/:id route in
// App.svelte, so it is the bare path and nothing more: appending an id here
// lands on a proposal, not on the hub.
func PatchGovernance(slug string) string {
	return Patch(slug) + "/governance"
}

// Proposal is a proposal's detail page — /patches/:slug/governance/:id.
func Proposal(slug, proposalID string) string {
	return Patch(slug) + "/governance/" + proposalID
}

// GovernanceDoc is a charter's detail page —
// /patches/:slug/governance/docs/:id. The `docs/` segment is what separates
// it from Proposal above; both take a bare UUID, so dropping it silently
// renders a charter's id as a proposal that does not exist.
func GovernanceDoc(slug, docID string) string {
	return Patch(slug) + "/governance/docs/" + docID
}

// RemotePatch is the read-only card for a patch on another quilt —
// /quilts/:host/patches/:slug (docs/adr/024).
func RemotePatch(host, slug string) string {
	return "/quilts/" + host + Patch(slug)
}

// Absolute turns an in-app path into a full URL on this instance, for the
// surfaces that leave the app: notification emails and public feeds.
func Absolute(domain, path string) string {
	return "https://" + domain + path
}
