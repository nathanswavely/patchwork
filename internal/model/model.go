package model

import "encoding/json"

// Core domain models for Patchwork.
// These map to the database tables defined in 001_initial.sql.

// SystemUserID is the sentinel user that owns unclaimed patches.
const SystemUserID = "00000000-0000-0000-0000-000000000000"

type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email,omitempty"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Bio         string     `json:"bio"`
	AvatarURL   string     `json:"avatar_url"`
	Links       []NodeLink `json:"links,omitempty"`
	Role        string     `json:"role"`
	// TrustedContributor is the instance-level grant from docs/adr/026:
	// events this person records on unclaimed patches skip review. It is
	// orthogonal to patch roles and worth nothing on active patches.
	TrustedContributor bool `json:"trusted_contributor"`
	// StartOnMyQuilt is the per-person landing preference (docs/adr/035):
	// when true, a cold visit to "/" redirects once to "/my". Default false
	// — the whole quilt is the shared default landing.
	StartOnMyQuilt bool `json:"start_on_my_quilt"`
	// HideAmendedLinings is the personal discovery filter (docs/adr/037):
	// hide amended-lining patches from this user's quilt, search, map, and
	// public feeds. Populated by the Me handler, not by session validation.
	HideAmendedLinings bool `json:"hide_amended_linings"`
	// ContactCard is the phone, email and note a person shares patch by
	// patch (docs/adr/080). Populated by the Me handlers only — it never
	// rides along on a login response, a public profile, or an AP actor.
	ContactCard *ContactCard `json:"contact_card,omitempty"`
	SuspendedAt *string      `json:"suspended_at,omitempty"`
	CreatedAt   string       `json:"created_at"`
	UpdatedAt   string       `json:"updated_at"`
}

type Notification struct {
	ID        string  `json:"id"`
	UserID    string  `json:"user_id"`
	Type      string  `json:"type"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	Link      string  `json:"link"`
	ReadAt    *string `json:"read_at,omitempty"`
	CreatedAt string  `json:"created_at"`
}

// RemoteFollow is a person's follow of a patch on another quilt
// (docs/adr/024). Stored on the follower's home instance; the snapshot
// keeps enough public display data to draw the tile while the remote
// quilt is unreachable.
type RemoteFollow struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	QuiltURL  string `json:"quilt_url"`
	NodeAPID  string `json:"node_ap_id"`
	NodeSlug  string `json:"node_slug"`
	NodeName  string `json:"node_name"`
	Snapshot  string `json:"-"` // raw JSON, re-emitted verbatim
	CreatedAt string `json:"created_at"`
}

// UserQuilt is a personal connected quilt — a quilt a signed-in person
// browses via the switcher, on top of the instance's neighbor quilts.
type UserQuilt struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	URL       string `json:"url"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// NeighborQuilt is an instance-level, admin-curated public connection to
// another quilt, visible to every visitor in the switcher.
type NeighborQuilt struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Name      string `json:"name"`
	Position  int    `json:"position"`
	CreatedAt string `json:"created_at"`
}

type Credential struct {
	ID              string `json:"id"`
	UserID          string `json:"user_id"`
	CredentialID    []byte `json:"-"`
	PublicKey       []byte `json:"-"`
	AttestationType string `json:"-"`
	AAGUID          []byte `json:"-"`
	SignCount       uint32 `json:"-"`
	Name            string `json:"name"`
	CreatedAt       string `json:"created_at"`
}

type NodeLink struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

// ContactCard is how a person can be reached, kept once on the account and
// shown only inside the patches whose membership they have switched
// contact sharing on for (docs/adr/080). Email here is an address to be
// reached at, deliberately separate from the sign-in address: sharing how
// to reach you must never hand out the key to your account.
type ContactCard struct {
	Phone string `json:"phone"`
	Email string `json:"email"`
	Note  string `json:"note"`
}

// Empty reports whether the card carries nothing to show.
func (c ContactCard) Empty() bool {
	return c.Phone == "" && c.Email == "" && c.Note == ""
}

type FollowerPermissions struct {
	Events    bool `json:"events"`
	Proposals bool `json:"proposals"`
	Charters  bool `json:"charters"`
	Members   bool `json:"members"`
}

type GovernanceConfig struct {
	DecisionMethod      string `json:"decision_method"`
	QuorumPercent       int    `json:"quorum_percent"`
	DefaultVoteDuration int    `json:"default_vote_duration_hours"`
	AmendmentThreshold  string `json:"amendment_threshold"`
	AmendmentAutoApply  bool   `json:"amendment_auto_apply"`
	SuccessionPolicy    string `json:"succession_policy"`
	MinVotingTenureDays int    `json:"min_voting_tenure_days"`
	// SubjectRecusal bars the person a proposal is *about* from voting on it
	// — a nomination's nominee (docs/adr/051). A term of the contest, so it
	// freezes with the rest of the config when voting opens (docs/adr/047).
	SubjectRecusal  bool   `json:"subject_recusal,omitempty"`
	LeadershipModel string `json:"leadership_model,omitempty"`
	// LeadershipVenue is where admins are actually chosen: "patchwork" (or
	// empty, the default) or "elsewhere" (docs/adr/052). Where it is
	// elsewhere, Patchwork conducts nothing and records attestations
	// instead.
	LeadershipVenue string `json:"leadership_venue,omitempty"`
	// ProposalVenue is where this patch decides the things proposals are
	// about: "patchwork" (or empty, the default) or "elsewhere"
	// (docs/adr/052, docs/adr/053). Elsewhere removes the ballot and keeps
	// the discussion — a proposal can still be raised and argued over, and
	// the decision comes back as an amendment attestation.
	ProposalVenue string `json:"proposal_venue,omitempty"`
	// NominationDays is how long an election takes nominations before voting
	// opens (docs/adr/051). This field is why a pre-voting phase is defensible
	// at all: docs/adr/048 retired `draft` and `discussion` because nothing
	// said how long they lasted or what ended them, and a window with a
	// governed length is the counterexample.
	NominationDays   int    `json:"nomination_days,omitempty"`
	SuccessionMethod string `json:"succession_method,omitempty"`
	AdminTermMonths  int    `json:"admin_term_months,omitempty"`
	MaxAdmins        int    `json:"max_admins,omitempty"`
	InactivityDays   int    `json:"inactivity_days,omitempty"`
}

// Appearance is a patch's chosen tile appearance on the quilt. Fields are
// optional and partial: anything absent stays hash-assigned from the patch
// ID. Palette and slug-valued blocks are opaque slugs — definitions live in
// the frontend registry, and unknown keys fall back to hash assignment at
// render time (see docs/adr/004-tile-appearance-storage-and-registry.md).
type Appearance struct {
	Palette string `json:"palette,omitempty"`
	// Block is either an opaque slug (curated block) or an embedded
	// drafted-block object — grid, seams, piece colors — validated
	// structurally, never aesthetically (docs/adr/029).
	Block    json.RawMessage `json:"block,omitempty"`
	Rotation *int            `json:"rotation,omitempty"`
	// Bundle is the fabrics the tile draws with: 1-6 hex colors, slot zero
	// is the identity color. The fabric wall (which colors the UI offers)
	// is the frontend's concern; the backend validates hex shape only.
	Bundle []string `json:"bundle,omitempty"`
	// Icon is the patch's motif — the mark drawn beside its name on quilt
	// label badges and patch cards. Unset/unknown falls back to tag-derived
	// then the quilt mark.
	Icon string `json:"icon,omitempty"`
}

type Node struct {
	ID          string   `json:"id"`
	OwnerID     string   `json:"owner_id"`
	Name        string   `json:"name"`
	Slug        string   `json:"slug"`
	Description string   `json:"description"`
	Latitude    *float64 `json:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty"`
	Address     string   `json:"address"`
	// Timezone is where this patch keeps time (docs/adr/045), as an IANA
	// name. Unlike an event payload's resolved zone, this is the stored
	// value: empty means the patch inherits the instance's, which is what
	// its settings form needs to show an empty field rather than a
	// pre-filled one nobody chose.
	Timezone string `json:"timezone"`
	Website  string `json:"website"`
	// ImageURL is a reference, never bytes (docs/adr/007): the browser fetches
	// it from wherever the patch keeps it. ImageAlt is required alongside, and
	// is what remains when the bytes go.
	ImageURL         string      `json:"image_url"`
	ImageAlt         string      `json:"image_alt"`
	Links            []NodeLink  `json:"links"`
	Visibility       string      `json:"visibility"`
	MembershipPolicy string      `json:"membership_policy"`
	Appearance       *Appearance `json:"appearance,omitempty"`
	Tags             []string    `json:"tags,omitempty"`
	Status           string      `json:"status,omitempty"`
	SubmittedBy      string      `json:"submitted_by,omitempty"`
	SubmissionSource string      `json:"submission_source,omitempty"`
	// AcceptEventSuggestions is the patch-admin-owned switch for whether
	// non-members may suggest events to this (active) patch (docs/adr/026).
	AcceptEventSuggestions bool                 `json:"accept_event_suggestions"`
	FollowerPermissions    *FollowerPermissions `json:"follower_permissions,omitempty"`
	GovernanceConfig       *GovernanceConfig    `json:"governance_config,omitempty"`
	MemberCount            int                  `json:"member_count,omitempty"`
	FollowerCount          int                  `json:"follower_count,omitempty"`
	// Events not yet started — distinct from the tree endpoint's
	// event_count, which is every active event past and future
	// (CONTEXT.md "Upcoming events"). Set on the single-node detail
	// response only; the tree carries the all-time figure.
	UpcomingEventCount int `json:"upcoming_event_count"`
	// ActivatedAt is when this patch joined the quilt - created, or claimed
	// through patch setup. NULL while it is only a directory listing: an
	// unclaimed patch has not joined, because no community has arrived
	// (docs/adr/076). Distinct from CreatedAt, which is when the row was
	// written, and from UpdatedAt, which every later edit moves.
	ActivatedAt *string `json:"activated_at,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type ClaimRequest struct {
	ID                string  `json:"id"`
	NodeID            string  `json:"node_id"`
	UserID            string  `json:"user_id"`
	Method            string  `json:"method"`
	Evidence          string  `json:"evidence"`
	Status            string  `json:"status"`
	ReviewedBy        *string `json:"reviewed_by,omitempty"`
	ReviewNote        string  `json:"review_note"`
	VerificationToken string  `json:"-"`
	Email             string  `json:"email,omitempty"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

type Event struct {
	ID          string   `json:"id"`
	NodeID      string   `json:"node_id"`
	CreatedBy   string   `json:"created_by"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Location    string   `json:"location"`
	Latitude    *float64 `json:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty"`
	StartsAt    string   `json:"starts_at"`
	EndsAt      *string  `json:"ends_at,omitempty"`
	// Timezone is the IANA zone the event happens in (docs/adr/045).
	// StartsAt stays the instant and stays the sort key; this is the fact
	// that instant encodes — the wall clock the organizer meant.
	//
	// On the wire it is always resolved and never empty: the API collapses
	// event → patch → instance → UTC before it leaves, so a client never
	// reimplements the fallback and never fetches the patch to render the
	// event. Stored, it may be NULL, and NULL means inherit.
	Timezone   string `json:"timezone"`
	Recurrence string `json:"recurrence"`
	Visibility string `json:"visibility"`
	// A flyer or a show photo, held wherever the patch keeps it (docs/adr/007).
	ImageURL string `json:"image_url"`
	ImageAlt string `json:"image_alt"`
	// EventURL is the event's own page out on the web — the venue's
	// listing, the ticket page, the Facebook event (docs/adr/079). Every
	// feed carries one and Patchwork used to drop it; an imported show
	// with no way back to where you buy a ticket is half an event.
	//
	// Distinct from the Patchwork permalink, which is derived from ID and
	// never stored, and from an EventLink, which is a patch's presence on
	// someone else's event (docs/adr/032).
	EventURL string `json:"event_url"`
	// Status is 'active' or 'pending_review' (docs/adr/026). Pending
	// events are submissions awaiting whoever owns the calendar; they
	// never appear in public listings and never federate.
	Status string `json:"status,omitempty"`
	// SourceID marks an imported event (docs/adr/031): the source is
	// authoritative and the event is read-only until detached.
	SourceID  *string `json:"source_id,omitempty"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// EventLink associates an event with a patch beyond its owner
// (docs/adr/032): one side's admins propose, the other side's confirm.
// Pending links are invisible everywhere; a confirmed link is presence,
// not control — the event stays the owner's to edit.
type EventLink struct {
	ID          string `json:"id"`
	EventID     string `json:"event_id"`
	NodeID      string `json:"node_id"`
	Status      string `json:"status"`
	InitiatedBy string `json:"initiated_by"`
	RequestedBy string `json:"requested_by,omitempty"`
	CreatedAt   string `json:"created_at"`
	// Display fields joined from nodes for rendering "with X".
	NodeName string `json:"node_name,omitempty"`
	NodeSlug string `json:"node_slug,omitempty"`
}

// EventMention is a display-only doorway on an event page to a patch on
// another quilt (docs/adr/032). No handshake, no surfaces — the standing
// of naming the band in the description.
type EventMention struct {
	ID      string `json:"id"`
	EventID string `json:"event_id"`
	Host    string `json:"host"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
}

// EventSource is a standing feed a patch pulls events from
// (docs/adr/031). Attached by whoever owns the calendar; attaching is
// vouching for the feed once.
type EventSource struct {
	ID            string  `json:"id"`
	NodeID        string  `json:"node_id"`
	Type          string  `json:"type"`
	URL           string  `json:"url"`
	AddedBy       string  `json:"added_by"`
	Status        string  `json:"status"`
	LastFetchAt   *string `json:"last_fetch_at,omitempty"`
	LastSuccessAt *string `json:"last_success_at,omitempty"`
	LastError     *string `json:"last_error,omitempty"`
	EventCount    int     `json:"event_count"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
	// AggregatorID and NameKey are set when this source is a crosswalk
	// entry — one name inside an aggregator rather than a feed of its own
	// (docs/adr/056). AggregatorName is joined for display.
	AggregatorID   *string `json:"aggregator_id,omitempty"`
	NameKey        *string `json:"name_key,omitempty"`
	AggregatorName string  `json:"aggregator_name,omitempty"`
	DisplayName    string  `json:"display_name,omitempty"`
	// Suggests marks a crosswalk entry that routes into this patch's
	// review queue rather than publishing (docs/adr/056). AddedByName
	// says who pointed it here, which is the whole of why a patch can
	// see entries it did not make.
	Suggests bool `json:"suggests,omitempty"`
	// LocalTimeStampedUTC marks a publisher that emits the venue's wall
	// clock as though it were UTC (docs/adr/073). Not a guess Patchwork
	// makes: the feed's own offset is internally consistent and simply
	// wrong, so only a person comparing the markup against the page can
	// say so.
	LocalTimeStampedUTC bool `json:"local_time_stamped_utc"`
	// SampleStartsAt is one upcoming event from this source, and Timezone
	// the zone its patch resolves to. Together they let the settings page
	// show what the switch above would actually do to a real row before
	// anybody saves it (docs/adr/073).
	SampleStartsAt *string `json:"sample_starts_at,omitempty"`
	Timezone       string  `json:"timezone,omitempty"`
	AddedByName    string  `json:"added_by_name,omitempty"`
	// PendingCount is how many of its items are waiting in the queue.
	PendingCount int `json:"pending_count"`
}

// Aggregator is an instance-level feed that lists events it does not own
// (docs/adr/056). It owns nothing, has no tile, and creates no event
// until a crosswalk entry addresses one.
type Aggregator struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	URL           string  `json:"url"`
	AddedBy       string  `json:"added_by"`
	Paused        bool    `json:"paused"`
	Status        string  `json:"status"`
	LastFetchAt   *string `json:"last_fetch_at,omitempty"`
	LastSuccessAt *string `json:"last_success_at,omitempty"`
	LastError     *string `json:"last_error,omitempty"`
	// ListingCount is what the last successful fetch carried;
	// MappedCount and UnroutedCount split it by whether a crosswalk
	// entry addresses the name.
	ListingCount  int    `json:"listing_count"`
	MappedCount   int    `json:"mapped_count"`
	UnroutedCount int    `json:"unrouted_count"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// UnroutedName is one name an aggregator's listings carry that no
// crosswalk entry addresses. Unrouted is a resting state, not a queue:
// "PA" and "3rd floor atrium" are names that should never be mapped.
type UnroutedName struct {
	AggregatorID   string `json:"aggregator_id"`
	AggregatorName string `json:"aggregator_name"`
	NameKey        string `json:"name_key"`
	DisplayName    string `json:"display_name"`
	Count          int    `json:"count"`
	// NextStartsAt is the soonest listing under this name — enough for an
	// admin to tell a live venue from a name that appeared once in 2019.
	NextStartsAt string   `json:"next_starts_at"`
	SampleTitles []string `json:"sample_titles"`
	// Ignored marks a name judged to mean no organization. Set only on
	// the ignored listing; the working list excludes them.
	Ignored bool `json:"ignored,omitempty"`
}

// AggregatorListing is one item as its feed published it, shown when an
// admin opens a name to decide what it means (docs/adr/056). Never an
// event — a listing becomes one only where a crosswalk entry routes it.
type AggregatorListing struct {
	UID        string `json:"uid"`
	Occurrence string `json:"occurrence"`
	Title      string `json:"title"`
	// TitleKey groups the listings a publisher files under one title, so a
	// reader of this drawer can credit the whole program at once rather
	// than one date at a time (docs/adr/063).
	TitleKey    string  `json:"title_key"`
	Description string  `json:"description"`
	Location    string  `json:"location"`
	StartsAt    string  `json:"starts_at"`
	EndsAt      *string `json:"ends_at,omitempty"`
	URL         string  `json:"url,omitempty"`
}

// AggregatorHold is a listing withheld because the patch already has an
// event at that instant (docs/adr/056). The patch's own event wins until
// one of its admins says the two are different.
type AggregatorHold struct {
	ID             string `json:"id"`
	SourceID       string `json:"source_id"`
	NodeID         string `json:"node_id"`
	UID            string `json:"uid"`
	Occurrence     string `json:"occurrence"`
	RivalEventID   string `json:"rival_event_id"`
	RivalTitle     string `json:"rival_title"`
	Title          string `json:"title"`
	Location       string `json:"location"`
	StartsAt       string `json:"starts_at"`
	AggregatorName string `json:"aggregator_name"`
	CreatedAt      string `json:"created_at"`
}

// AggregatorProgram is a recurring title someone recognized and credited
// to a patch (docs/adr/063). It never routes: the events stay the owning
// patch's, and what the program produces is offers.
type AggregatorProgram struct {
	ID             string `json:"id"`
	AggregatorID   string `json:"aggregator_id"`
	AggregatorName string `json:"aggregator_name"`
	NameKey        string `json:"name_key"`
	// DisplayName is the place this program was recognized under — the
	// row has to say which name it sits beneath, since it is scoped to one.
	DisplayName  string `json:"display_name"`
	TitleKey     string `json:"title_key"`
	DisplayTitle string `json:"display_title"`
	NodeID       string `json:"node_id"`
	NodeName     string `json:"node_name"`
	NodeSlug     string `json:"node_slug"`
	CreditedBy   string `json:"credited_by"`
	CreatedAt    string `json:"created_at"`
	// ListingCount is what the feed currently carries under this title.
	ListingCount int `json:"listing_count"`
	// Routed reports whether this program's name has a crosswalk entry.
	// False means inert — with no events there is nothing to offer — and
	// the row says so rather than looking broken (docs/adr/063).
	Routed bool `json:"routed"`
	// OfferCount is how many of its events are still waiting on someone.
	OfferCount int `json:"offer_count"`
}

// AggregatorOffer is one event a credited program presents to its patch,
// waiting for a person to propose an event link (docs/adr/032). Never
// stored — it is what remains after subtracting links already made and
// offers already declined (docs/adr/063).
type AggregatorOffer struct {
	ProgramID    string `json:"program_id"`
	DisplayTitle string `json:"display_title"`
	EventID      string `json:"event_id"`
	Title        string `json:"title"`
	StartsAt     string `json:"starts_at"`
	Location     string `json:"location"`
	// The patch that owns the event — the one whose admins confirm.
	OwnerNodeID string `json:"owner_node_id"`
	OwnerName   string `json:"owner_name"`
	OwnerSlug   string `json:"owner_slug"`
}

type Membership struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	NodeID string `json:"node_id"`
	Role   string `json:"role"`
	Status string `json:"status"`
	// Visible is the one membership-visibility switch: it controls both the
	// profile's patch list and the patch's public member list (docs/adr/006).
	Visible  bool   `json:"visible"`
	JoinedAt string `json:"joined_at"`
	// JoinMessage is the join sheet's optional intro note to admins on a
	// pending membership request (docs/adr/040). Populated only for pending
	// requests shown to that patch's admins — never in public member
	// listings, and nulled once the request is resolved.
	JoinMessage string `json:"join_message,omitempty"`
	// ShareContact is the per-membership contact-sharing switch
	// (docs/adr/080): when on, this patch's admins and members see the
	// person's contact card in the Members room. Default off. A second
	// axis beside Visible, not a second visibility switch — visibility
	// says whether the membership is known, sharing says whether the
	// people already in the room can reach you.
	ShareContact bool `json:"share_contact"`
}

type Proposal struct {
	ID            string  `json:"id"`
	NodeID        string  `json:"node_id"`
	AuthorID      string  `json:"author_id"`
	Title         string  `json:"title"`
	Body          string  `json:"body"`
	Status        string  `json:"status"`
	State         string  `json:"state"`
	ProposalType  string  `json:"proposal_type"`
	DurationHours int     `json:"duration_hours"`
	VotingEndsAt  *string `json:"voting_ends_at,omitempty"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
	TargetDoc     string  `json:"target_doc,omitempty"`
	// TargetUserID is the person a proposal is *about*, as distinct from
	// AuthorID, who raised it. Set on a meritocratic nomination
	// (docs/adr/051); empty on every proposal that decides a thing rather
	// than a person.
	TargetUserID   string  `json:"target_user_id,omitempty"`
	ProposedBranch string  `json:"proposed_branch,omitempty"`
	ProposedBody   string  `json:"proposed_body,omitempty"`
	ProposedTitle  string  `json:"proposed_title,omitempty"`
	GitSHA         string  `json:"git_sha,omitempty"`
	BaseSHA        string  `json:"base_sha,omitempty"`
	AppliedAt      *string `json:"applied_at,omitempty"`
	AppliedBy      *string `json:"applied_by,omitempty"`
}

type GovernanceDoc struct {
	ID     string `json:"id"`
	NodeID string `json:"node_id"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	// Kind is "charter" or "lining" (docs/adr/037). The lining is the shared
	// baseline every patch adopts at creation; it is machine-identified by
	// this column, never by its title.
	Kind string `json:"kind"`
	// Visibility is "public" or "members" (docs/adr/036). New docs default
	// to members-only; a patch admin publishes each one deliberately. The
	// lining is pinned public (docs/adr/037).
	Visibility string `json:"visibility"`
	Version    int    `json:"version"`
	CreatedBy  string `json:"created_by"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type Vote struct {
	ID         string `json:"id"`
	ProposalID string `json:"proposal_id"`
	UserID     string `json:"user_id"`
	Value      string `json:"value"`
	CreatedAt  string `json:"created_at"`
}

type AuditEntry struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id,omitempty"`
	Action     string `json:"action"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Metadata   string `json:"metadata"`
	IPAddress  string `json:"ip_address"`
	CreatedAt  string `json:"created_at"`
}

type ContentReport struct {
	ID             string  `json:"id"`
	ReporterID     string  `json:"reporter_id"`
	EntityType     string  `json:"entity_type"`
	EntityID       string  `json:"entity_id"`
	Reason         string  `json:"reason"`
	Details        string  `json:"details"`
	Status         string  `json:"status"`
	ReviewedBy     *string `json:"reviewed_by,omitempty"`
	ResolutionNote string  `json:"resolution_note"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}
