package notifications

// Category groups notification types for admin-level patch configuration.
type Category string

const (
	CategoryProposals  Category = "proposals"
	CategoryGovernance Category = "governance"
	CategoryMembership Category = "membership"
	CategoryEvents     Category = "events"
	CategoryAdmin      Category = "admin"
)

// AllCategories returns every category in display order.
func AllCategories() []CategoryInfo {
	return []CategoryInfo{
		{CategoryProposals, "Proposals", "New proposals, voting updates, deadlines"},
		{CategoryGovernance, "Governance", "Document and rules changes"},
		{CategoryMembership, "Membership", "Join/leave notifications for admins"},
		{CategoryEvents, "Events", "Event creation, updates, reminders"},
		{CategoryAdmin, "Admin", "Claim requests, submissions"},
	}
}

// CategoryInfo holds display metadata for a category.
type CategoryInfo struct {
	ID          Category `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
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

	AdminClaimRequest    NotificationType = "admin.claim_request"
	AdminSubmission      NotificationType = "admin.submission"
	AdminEventSubmission NotificationType = "admin.event_submission"
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
	AudienceAllMembers   Audience = iota // All active members + admins of the patch
	AudienceAdminsOnly                   // Only patch admins
	AudienceSpecificUser                 // A single user (e.g., proposal author)
	AudienceParticipants                 // Users who voted/commented on a proposal
	AudienceSiteAdmins                   // Instance-level admins
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

	GovernanceDocUpdated:   {CategoryGovernance, "Document updated", AudienceAllMembers, PriorityNormal},
	GovernanceRulesChanged: {CategoryGovernance, "Rules changed", AudienceAllMembers, PriorityHigh},

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

	AdminClaimRequest:    {CategoryAdmin, "New patch claim request", AudienceSiteAdmins, PriorityHigh},
	AdminSubmission:      {CategoryAdmin, "New patch submission", AudienceSiteAdmins, PriorityNormal},
	AdminEventSubmission: {CategoryAdmin, "New event submission", AudienceSiteAdmins, PriorityNormal},
}

// DefaultEnabled returns whether a channel should be on by default for a given type.
func DefaultEnabled(t NotificationType, channel string) bool {
	meta, ok := TypeRegistry[t]
	if !ok {
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
		GovernanceDocUpdated, GovernanceRulesChanged,
		MembershipJoined, MembershipRequest, MembershipApproved, MembershipRoleChanged, MembershipBanned, MembershipReinstated,
		EventCreated, EventReminder, EventUpdated, EventCancelled,
		EventSuggested, EventSubmissionApproved, EventSubmissionRejected,
		AdminClaimRequest, AdminSubmission, AdminEventSubmission,
	}
	for _, t := range allTypes {
		if TypeRegistry[t].Category == cat {
			types = append(types, t)
		}
	}
	return types
}
