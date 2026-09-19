package handler

// Instance-admin reach into a patch (docs/adr/115).
//
// An instance admin's reach into a patch is custody, not rank: it exists
// exactly while the patch has no admin of its own, and ends the moment
// somebody holds the role. That rule is decided and its implementation is
// deliberately deferred (issue #286), so this file is a **characterization**
// of what the handlers do today, not an assertion that they are right.
//
// It exists because of how the reach got here. Nobody decided it. Every
// `user.Role == "admin"` on a node-scoped route was copied from the handler
// next door, about thirty times, past the point where its own justification
// still applied — `event_links.go` grants the reach "everywhere" in a sentence
// whose own parenthetical says "unclaimed", and `event_submissions.go` calls
// it "global admin override ... as on every node endpoint" three lines below
// code implementing ADR 026's rule that active calendars are reviewed by that
// patch's admins, "never the instance admin".
//
// A reflex that spreads by copying will keep spreading while the rule waits.
// So every site is declared here with what ADR 115 says it should end up as,
// and `TestEveryInstanceAdminReachIsDeclared` fails the build on a
// thirty-first. The cost of adding one is now reading this file; the cost of
// adding one silently is gone.
//
// When #286 is implemented this file stops being a snapshot and becomes the
// rule: the `settled` column goes all true, and anything that is not
// `custodyUnclaimed`, `custodyVacated`, `instanceSurface` or `speech` is gone.

// reachDisposition is what docs/adr/115 says an instance admin holding no
// role in a patch should be able to do at one site. It describes the decided
// end state, which is not always what the code does yet — see `settled`.
type reachDisposition int

const (
	// reachNone: nothing. The patch has an admin of its own, so it is held by
	// nobody else, and this check should not consult user.Role at all.
	reachNone reachDisposition = iota
	// reachCustodyUnclaimed: the calendar of a patch nobody has claimed, held
	// in trust (docs/adr/026, 031, 056, 057). Worth nothing once it is
	// claimed.
	reachCustodyUnclaimed
	// reachCustodyVacated: a claimed patch with zero active admins, held only
	// to hand back. The one act is putting an admin in place.
	reachCustodyVacated
	// reachInstanceSurface: not a patch's to hold in the first place, so
	// custody never applied. The instance's own roster, the quilt's own
	// curation.
	reachInstanceSurface
	// reachSpeech: commenting, kept deliberately (proposals.go's
	// CreateProposal comment). Speech is not authority over a patch.
	reachSpeech
)

// instanceAdminReach is every function in this package that compares a
// signed-in user's instance role against "admin". Keyed by "file.go:Func" so
// the entry survives the line moving.
//
// `count` is how many such comparisons the function contains, so that adding
// a second one to a function that already has an entry still fails. `settled`
// is whether the code already does what `end` says; false means issue #286
// changes this site.
var instanceAdminReach = map[string]struct {
	end     reachDisposition
	count   int
	settled bool
	why     string
}{
	"account_deletion.go:DeleteMyAccount": {reachInstanceSurface, 1, true,
		"Refuses the last instance admin's own deletion. About the instance's roster, never about a patch."},

	"aggregators.go:crosswalkAccess": {reachCustodyUnclaimed, 2, true,
		"The one site that already states ADR 115's rule in full, and states it better than the ADR does: " +
			"manage on an unclaimed patch held in trust, suggest into a claimed patch only where its own " +
			"accept_event_suggestions switch is on (the patch's consent, and nothing publishes without its " +
			"admins), neither otherwise. Read its comment before changing anything in this file."},

	"aggregators.go:crosswalkNodeAccess": {reachCustodyUnclaimed, 1, false,
		"Reading and unmapping crosswalk entries. Follows sourceNodeAccess, which narrows to unclaimed."},

	"aggregators.go:DecideAggregatorHold": {reachNone, 1, false,
		"Deciding a hold on a claimed patch's feed is that patch's call."},

	"comments.go:CreateComment": {reachSpeech, 2, true,
		"Commenting on a proposal and the follower-permission gate around it. Kept: speech, not authority."},

	"comments.go:AddReaction": {reachSpeech, 2, true,
		"Reacting to a comment takes the same standing gate CreateComment does, added alongside it " +
			"(issue #321): a role on the patch, and the follower-permission gate around it. Same reasoning " +
			"as its neighbor — a reaction is speech, not authority."},

	"comments.go:RemoveReaction": {reachSpeech, 2, true,
		"Un-reacting, gated the same way as AddReaction even though the delete is already scoped to the " +
			"caller's own row (issue #321)."},

	"comments.go:DeleteComment": {reachNone, 1, false,
		"Deleting somebody else's comment is moderation, not speech. Routes through the report queue like " +
			"every other piece of content."},

	"deleted_accounts.go:viewerIsInPatchRoom": {reachNone, 1, false,
		"Who is in the room. Its own comment cites ADR 006's 'seen by that patch's admins and members " +
			"inside the workspace', and an instance admin holding no role is neither."},

	"event_links.go:userSpeaksForNode": {reachCustodyUnclaimed, 1, false,
		"The link handshake. Its comment already says 'who holds unclaimed patches' calendars in trust' " +
			"and the code says everywhere; ADR 115 is that parenthetical winning."},

	"event_sources.go:sourceNodeAccess": {reachCustodyUnclaimed, 1, false,
		"Owning a patch's calendar feeds."},

	"event_sources.go:DetachEvent": {reachNone, 1, false,
		"Severing an imported event's provenance on a patch that runs itself."},

	"event_submissions.go:ListNodeEventSubmissions": {reachCustodyUnclaimed, 1, false,
		"A patch's own review queue. ADR 026: unclaimed queues are the instance admin's, active ones are " +
			"the patch's."},

	"event_submissions.go:ReviewEventSubmission": {reachCustodyUnclaimed, 2, false,
		"Two branches. The unclaimed one is already right and is ADR 026 working; the active one carries " +
			"the comment that asserts the reflex as the rule. Only the second changes."},

	"event_upload.go:BulkCreateEvents": {reachCustodyUnclaimed, 1, false,
		"The CSV door onto a patch's calendar."},

	"events.go:CreateEvent": {reachCustodyUnclaimed, 1, false,
		"Publishing straight to a calendar. ADR 026's grant 'must not reach into active patches'."},

	"events.go:UpdateEvent": {reachCustodyUnclaimed, 1, false,
		"Editing anything on a patch's calendar."},

	"events.go:DeleteEvent": {reachCustodyUnclaimed, 1, false,
		"Removing an event from a patch's calendar."},

	"events.go:GetEvent": {reachCustodyUnclaimed, 1, false,
		"Reading a submission still pending review. Its comment is already about the unclaimed queue."},

	"governance.go:canReadPatchDocs": {reachNone, 1, false,
		"The largest single site. ADR 110 hangs the git transport off this predicate, so it is not one " +
			"charter: it is every doc body, revision, diff and committer name, in one condition. There is " +
			"no half-clone, so the door carries the whole rule."},

	"governance.go:CreateGovernanceDoc": {reachNone, 1, false,
		"Writing a charter into a patch that runs itself."},

	"governance.go:UpdateGovernanceDoc": {reachNone, 1, false,
		"Rewriting a charter a patch adopted."},

	"memberships.go:ListMembers": {reachNone, 2, false,
		"The pending/banned filter and the `insider` flag. The second exposes hidden memberships and " +
			"followers, and ADR 006 makes that switch the member's own."},

	"memberships.go:UpdateMember": {reachCustodyVacated, 1, false,
		"Putting an admin back. This is the recovery path inactivity.go:624 already names for a patch " +
			"whose seats were vacated (docs/adr/102), which is why it narrows to zero-active-admins " +
			"rather than disappearing."},

	"nodes.go:GetNode": {reachNone, 1, false,
		"Sets is_admin on the payload for any instance admin. Not a display bug: it truthfully reports " +
			"UpdateNode's authz. Twelve sites across Go and Svelte work around this one flag."},

	"nodes.go:UpdateNode": {reachNone, 1, false,
		"Renaming a patch, moving it, changing its membership policy, its visibility, its appearance."},

	"nodes.go:DeleteNode": {reachInstanceSurface, 1, true,
		"Archive. A named exception in ADR 115: it removes a patch from the quilt rather than editing it, " +
			"which is instance curation and the moderation lever of last resort. CONTEXT.md already grants " +
			"it and ADR 034 already makes restore instance-admin-only on purpose."},

	"proposals.go:WithdrawProposal": {reachNone, 1, false,
		"Withdrawing a patch's proposal. Not enumerated in ADR 115's own list, but it is a held patch's " +
			"governance act and the rule reaches it. Note that CreateProposal's comment still describes " +
			"stewardship as 'withdrawing, applying, moderating' — ApplyProposal's bypass is already gone, " +
			"so that sentence is half stale and should be fixed with this site."},

	"proposals.go:UpdateProposal": {reachNone, 1, false,
		"Editing a patch's proposal. Same reasoning as WithdrawProposal, and same stale comment."},
}
