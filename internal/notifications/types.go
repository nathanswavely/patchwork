package notifications

// Category groups notification types for admin-level patch configuration.
type Category string

const (
	CategoryProposals  Category = "proposals"
	CategoryGovernance Category = "governance"
	CategoryMembership Category = "membership"
	CategoryEvents     Category = "events"
	CategoryAdmin      Category = "admin"
	// The one category that is not a patch's own emission (docs/adr/076).
	// Every other notification in this system is a consequence of a
	// relationship the recipient already holds; this one is the quilt
	// speaking about who arrived. It is kept separate rather than folded
	// into Events or Admin so the exception stays legible, and the next
	// person proposing a broadcast has to argue for it rather than inherit
	// it.
	CategoryQuilt Category = "quilt"
	// The noticeboard (docs/adr/081). Its own category, named for the
	// surface, so a busy patch's board can be silenced per patch without
	// touching its events or proposals — the per-patch config and the
	// per-user preferences both key on category.
	CategoryNoticeboard Category = "noticeboard"
)

// AllCategories returns every category in display order.
func AllCategories() []CategoryInfo {
	return []CategoryInfo{
		{CategoryProposals, "Proposals", "New proposals, voting updates, deadlines", true},
		{CategoryGovernance, "Governance", "Document and rules changes", true},
		{CategoryMembership, "Membership", "Join/leave notifications for admins", true},
		{CategoryEvents, "Events", "Event creation, updates, reminders", true},
		{CategoryAdmin, "Admin", "Claim requests, submissions", true},
		{CategoryNoticeboard, "Noticeboard", "A notice whose author chose to tell members, replies on notices you're in, and reports for admins", true},
		{CategoryQuilt, "The quilt", "A monthly note naming the patches that joined. Off unless you ask for it, and the one notification here that is not about you.", false},
	}
}

// CategoryInfo holds display metadata for a category.
type CategoryInfo struct {
	ID          Category `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	// PatchScoped is whether a patch admin may switch this category off for
	// their own patch. Every category is a patch's own emission except the
	// bulletin, which belongs to no patch — offering a patch a switch over
	// it would imply it had one.
	PatchScoped bool `json:"-"`
}

// PatchCategories returns the categories a patch admin can configure: all of
// them except the broadcast, which no patch emits (docs/adr/076).
func PatchCategories() []CategoryInfo {
	var out []CategoryInfo
	for _, ci := range AllCategories() {
		if ci.PatchScoped {
			out = append(out, ci)
		}
	}
	return out
}

// NotificationType is the specific notification type string stored in the DB.
type NotificationType string

const (
	ProposalNew          NotificationType = "proposal.new"
	ProposalVoting       NotificationType = "proposal.voting"
	ProposalVoteReceived NotificationType = "proposal.vote_received"
	ProposalApproved     NotificationType = "proposal.approved"
	ProposalRejected     NotificationType = "proposal.rejected"
	ProposalApplied      NotificationType = "proposal.applied"
	ProposalComment      NotificationType = "proposal.comment"
	ProposalDeadline     NotificationType = "proposal.deadline"

	GovernanceDocUpdated   NotificationType = "governance.doc_updated"
	GovernanceRulesChanged NotificationType = "governance.rules_changed"
	// GovernanceRulesChangedMidVote is the same edit landing while votes are
	// open. Those votes keep the terms they opened with (docs/adr/047), so the
	// new rules do not reach them — which is time-sensitive in a way a routine
	// rules edit is not, since those votes close. Separate from
	// GovernanceRulesChanged so it can be muted separately and mailed
	// separately: preferences key on the type, and email defaults on for high
	// priority only. The two never both fire for one edit.
	GovernanceRulesChangedMidVote NotificationType = "governance.rules_changed_midvote"
	// LiningUpdated fires when a stale lining auto-updates to the current
	// shipped text (docs/adr/037). Notified, never asked.
	LiningUpdated NotificationType = "governance.lining_updated"
	// GovernanceInactivityWarning tells an admin their seat is at risk before
	// it goes, which is the whole point of the warning: the shipped succession
	// plan gives them the gap between day 30 and day 60 to answer.
	GovernanceInactivityWarning NotificationType = "governance.inactivity_warning"
	// GovernanceSuccessionNeeded reaches instance admins on a patch whose
	// succession policy asks them to step in — the one policy Patchwork
	// cannot carry out on its own.
	GovernanceSuccessionNeeded NotificationType = "governance.succession_needed"

	MembershipJoined      NotificationType = "membership.joined"
	MembershipRequest     NotificationType = "membership.request"
	MembershipApproved    NotificationType = "membership.approved"
	MembershipRoleChanged NotificationType = "membership.role_changed"
	MembershipBanned      NotificationType = "membership.banned"
	MembershipReinstated  NotificationType = "membership.reinstated"

	EventCreated   NotificationType = "event.created"
	EventReminder  NotificationType = "event.reminder"
	EventUpdated   NotificationType = "event.updated"
	EventCancelled NotificationType = "event.cancelled"

	// Event submissions (docs/adr/026): suggestions to an active patch go
	// to its admins; submissions to unclaimed patches go to site admins.
	EventSuggested          NotificationType = "event.suggested"
	EventSubmissionApproved NotificationType = "event.submission_approved"
	EventSubmissionRejected NotificationType = "event.submission_rejected"

	// Event links (docs/adr/032): the confirming side's admins get the
	// request; the linked patch's admins hear when it lands. Requests to
	// an unclaimed confirming side go to site admins, who hold those
	// calendars in trust.
	EventLinkRequested NotificationType = "event.link_request"
	EventLinkConfirmed NotificationType = "event.link_confirmed"

	// A new listing matched a program this patch is credited with
	// (docs/adr/063). Not a link and not an event of theirs — an offer to
	// propose one. Crediting back-fills silently; only offers arriving
	// afterward announce, which is docs/adr/056's rule for crosswalk
	// entries applied to programs.
	ProgramOffer NotificationType = "program.offer"

	AdminClaimRequest     NotificationType = "admin.claim_request"
	AdminSubmission       NotificationType = "admin.submission"
	AdminEventSubmission  NotificationType = "admin.event_submission"
	AdminEventLinkRequest NotificationType = "admin.event_link_request"

	// Claim lifecycle (docs/adr/039): a verified or approved claim opens a
	// 14-day, single-use window into patch setup — these are claimant-facing,
	// unlike AdminClaimRequest above.
	ClaimApproved      NotificationType = "claim.approved"
	ClaimSetupExpiring NotificationType = "claim.setup_expiring"

	// The bulletin (docs/adr/076): the one broadcast this system sends.
	QuiltBulletin NotificationType = "quilt.bulletin"

	// The noticeboard (docs/adr/081). A notice is born quiet: NoticePosted
	// fires only when its author checked "Tell members". A reply reaches
	// the notice's participants — its author and those who already replied
	// — and nobody else; there is no way to summon someone in. A report
	// reaches the patch's admins, who are the room's stewards.
	NoticePosted   NotificationType = "notice.posted"
	NoticeReply    NotificationType = "notice.reply"
	NoticeReported NotificationType = "notice.reported"
)

// Priority determines default channel behavior.
type Priority int

const (
	PriorityLow    Priority = 0 // in-app on, email off
	PriorityNormal Priority = 1 // in-app on, email off
	PriorityHigh   Priority = 2 // in-app on, email on (if available)
)

// Audience determines how recipients are resolved.
type Audience int

const (
	AudienceAllMembers         Audience = iota // All active members + admins of the patch
	AudienceAdminsOnly                         // Only patch admins
	AudienceSpecificUser                       // A single user (e.g., proposal author)
	AudienceParticipants                       // Users who voted/commented on a proposal
	AudienceSiteAdmins                         // Instance-level admins
	AudienceSubscribers                        // Everyone who asked for the bulletin (docs/adr/076)
	AudienceNoticeParticipants                 // A notice's author and everyone who replied (docs/adr/081)
)

// TypeMeta holds static metadata for each notification type.
type TypeMeta struct {
	Category Category
	Label    string
	Audience Audience
	Priority Priority
}

// TypeRegistry is the single source of truth for all notification types.
var TypeRegistry = map[NotificationType]TypeMeta{
	ProposalNew:          {CategoryProposals, "New proposal in your patch", AudienceAllMembers, PriorityNormal},
	ProposalVoting:       {CategoryProposals, "Voting has started", AudienceAllMembers, PriorityNormal},
	ProposalVoteReceived: {CategoryProposals, "Vote received on your proposal", AudienceSpecificUser, PriorityLow},
	ProposalApproved:     {CategoryProposals, "Proposal approved", AudienceAllMembers, PriorityHigh},
	ProposalRejected:     {CategoryProposals, "Proposal rejected", AudienceAllMembers, PriorityHigh},
	ProposalApplied:      {CategoryProposals, "Amendment applied", AudienceAllMembers, PriorityHigh},
	ProposalComment:      {CategoryProposals, "Comment on a proposal you're in", AudienceParticipants, PriorityNormal},
	ProposalDeadline:     {CategoryProposals, "Voting ends in 24 hours", AudienceAllMembers, PriorityHigh},

	GovernanceDocUpdated: {CategoryGovernance, "Document updated", AudienceAllMembers, PriorityNormal},
	// Normal, not high: a patch tuning its own rules is significant but not
	// time-sensitive, and email defaults on for high priority. Mailing every
	// member on every config edit is how a whole category gets filtered, and
	// then the mid-vote notice below gets filtered with it.
	GovernanceRulesChanged:        {CategoryGovernance, "Rules changed", AudienceAllMembers, PriorityNormal},
	GovernanceRulesChangedMidVote: {CategoryGovernance, "Rules changed while votes are open", AudienceAllMembers, PriorityHigh},
	LiningUpdated:                 {CategoryGovernance, "The lining was updated", AudienceAllMembers, PriorityNormal},
	GovernanceInactivityWarning:   {CategoryGovernance, "Your admin seat is inactive", AudienceSpecificUser, PriorityHigh},
	GovernanceSuccessionNeeded:    {CategoryAdmin, "A patch has no admins left", AudienceSiteAdmins, PriorityHigh},

	MembershipJoined:      {CategoryMembership, "New member joined", AudienceAdminsOnly, PriorityNormal},
	MembershipRequest:     {CategoryMembership, "Membership request pending", AudienceAdminsOnly, PriorityHigh},
	MembershipApproved:    {CategoryMembership, "Your membership was approved", AudienceSpecificUser, PriorityHigh},
	MembershipRoleChanged: {CategoryMembership, "Your role was changed", AudienceSpecificUser, PriorityHigh},
	MembershipBanned:      {CategoryMembership, "You have been removed", AudienceSpecificUser, PriorityHigh},
	MembershipReinstated:  {CategoryMembership, "You have been reinstated", AudienceSpecificUser, PriorityHigh},

	EventCreated:   {CategoryEvents, "New event", AudienceAllMembers, PriorityNormal},
	EventReminder:  {CategoryEvents, "Event starts in 24 hours", AudienceAllMembers, PriorityHigh},
	EventUpdated:   {CategoryEvents, "Event details changed", AudienceAllMembers, PriorityLow},
	EventCancelled: {CategoryEvents, "Event cancelled", AudienceAllMembers, PriorityHigh},

	EventSuggested:          {CategoryEvents, "Event suggested to your patch", AudienceAdminsOnly, PriorityHigh},
	EventSubmissionApproved: {CategoryEvents, "Your event was approved", AudienceSpecificUser, PriorityHigh},
	EventSubmissionRejected: {CategoryEvents, "Your event was declined", AudienceSpecificUser, PriorityNormal},

	EventLinkRequested: {CategoryEvents, "Event link request for your patch", AudienceAdminsOnly, PriorityHigh},
	EventLinkConfirmed: {CategoryEvents, "Event link confirmed", AudienceAdminsOnly, PriorityNormal},

	// Normal, not high: an offer is an invitation to act, and the event is
	// already on the quilt under its venue whether or not anyone acts.
	ProgramOffer: {CategoryEvents, "A listing matched one of your programs", AudienceAdminsOnly, PriorityNormal},

	// Normal, not high, for the first two: the checkbox must not become a
	// "send everyone an email" button. Whether the bell reaches a mailbox
	// belongs to the recipient's preference, not the author (docs/adr/081).
	NoticePosted:   {CategoryNoticeboard, "A notice for your patch", AudienceAllMembers, PriorityNormal},
	NoticeReply:    {CategoryNoticeboard, "Reply on a notice you're in", AudienceNoticeParticipants, PriorityNormal},
	NoticeReported: {CategoryNoticeboard, "A notice or reply was reported", AudienceAdminsOnly, PriorityHigh},

	AdminClaimRequest:     {CategoryAdmin, "New patch claim request", AudienceSiteAdmins, PriorityHigh},
	AdminSubmission:       {CategoryAdmin, "New patch submission", AudienceSiteAdmins, PriorityNormal},
	AdminEventSubmission:  {CategoryAdmin, "New event submission", AudienceSiteAdmins, PriorityNormal},
	AdminEventLinkRequest: {CategoryAdmin, "Event link request (unclaimed patch)", AudienceSiteAdmins, PriorityNormal},

	ClaimApproved: {CategoryAdmin, "Your claim was approved", AudienceSpecificUser, PriorityHigh},

	// Priority is not what gates this one — DefaultEnabled refuses it on
	// every channel until a person turns it on, so the priority here only
	// says what it is once they have: a monthly note, not an interruption.
	QuiltBulletin:      {CategoryQuilt, "New patches this month", AudienceSubscribers, PriorityLow},
	ClaimSetupExpiring: {CategoryAdmin, "Your claim's setup window is closing", AudienceSpecificUser, PriorityHigh},
}

// selfNotifyingTypes reach their own actor. Notifications normally skip the
// person who caused them — nobody needs telling what they just did — but a
// review-queue notification reports the state of a queue, which is true no
// matter who filled it. It also meant a lone site admin's queues notified
// nobody at all: they are the only recipient and, when they submit, the only
// actor. Patch-level queues need no equivalent, since anyone who could be a
// recipient there posts directly instead of submitting.
var selfNotifyingTypes = map[NotificationType]bool{
	AdminClaimRequest:     true,
	AdminSubmission:       true,
	AdminEventSubmission:  true,
	AdminEventLinkRequest: true,
}

// NotifiesSelf reports whether a type is delivered to its own actor.
func NotifiesSelf(t NotificationType) bool {
	return selfNotifyingTypes[t]
}

// DefaultEnabled returns whether a channel should be on by default for a given type.
func DefaultEnabled(t NotificationType, channel string) bool {
	meta, ok := TypeRegistry[t]
	if !ok {
		return false
	}
	// The bulletin ships off, on every channel (docs/adr/076). Opt-in is the
	// whole of what keeps the front door's promise true: the person decided
	// this should reach them, and the app deciding is exactly what "no
	// algorithm runs it" rules out. In-app defaults on for everything else
	// below, which is why this cannot be expressed as a priority.
	if t == QuiltBulletin {
		return false
	}
	switch channel {
	case "in_app":
		return true // Always default on for in-app.
	case "email":
		return meta.Priority == PriorityHigh
	default:
		return false
	}
}

// TypesForCategory returns all notification types belonging to a category, in order.
func TypesForCategory(cat Category) []NotificationType {
	var types []NotificationType
	// Maintain a stable order by iterating a known list.
	allTypes := []NotificationType{
		ProposalNew, ProposalVoting, ProposalVoteReceived, ProposalApproved,
		ProposalRejected, ProposalApplied, ProposalComment, ProposalDeadline,
		GovernanceDocUpdated, GovernanceRulesChanged, GovernanceRulesChangedMidVote, LiningUpdated,
		GovernanceInactivityWarning,
		MembershipJoined, MembershipRequest, MembershipApproved, MembershipRoleChanged, MembershipBanned, MembershipReinstated,
		EventCreated, EventReminder, EventUpdated, EventCancelled,
		EventSuggested, EventSubmissionApproved, EventSubmissionRejected,
		EventLinkRequested, EventLinkConfirmed, ProgramOffer,
		AdminClaimRequest, AdminSubmission, AdminEventSubmission, AdminEventLinkRequest,
		GovernanceSuccessionNeeded,
		ClaimApproved, ClaimSetupExpiring,
		QuiltBulletin,
		NoticePosted, NoticeReply, NoticeReported,
	}
	for _, t := range allTypes {
		if TypeRegistry[t].Category == cat {
			types = append(types, t)
		}
	}
	return types
}
