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
	TrustedContributor bool    `json:"trusted_contributor"`
	SuspendedAt        *string `json:"suspended_at,omitempty"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
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
	LeadershipModel     string `json:"leadership_model,omitempty"`
	SuccessionMethod    string `json:"succession_method,omitempty"`
	AdminTermMonths     int    `json:"admin_term_months,omitempty"`
	MaxAdmins           int    `json:"max_admins,omitempty"`
	InactivityDays      int    `json:"inactivity_days,omitempty"`
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
	ID               string      `json:"id"`
	OwnerID          string      `json:"owner_id"`
	Name             string      `json:"name"`
	Slug             string      `json:"slug"`
	Description      string      `json:"description"`
	Latitude         *float64    `json:"latitude,omitempty"`
	Longitude        *float64    `json:"longitude,omitempty"`
	Address          string      `json:"address"`
	Website          string      `json:"website"`
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
	CreatedAt              string               `json:"created_at"`
	UpdatedAt              string               `json:"updated_at"`
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
	Recurrence  string   `json:"recurrence"`
	Visibility  string   `json:"visibility"`
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
}

type Proposal struct {
	ID             string  `json:"id"`
	NodeID         string  `json:"node_id"`
	AuthorID       string  `json:"author_id"`
	Title          string  `json:"title"`
	Body           string  `json:"body"`
	Status         string  `json:"status"`
	State          string  `json:"state"`
	ProposalType   string  `json:"proposal_type"`
	DurationHours  int     `json:"duration_hours"`
	VotingEndsAt   *string `json:"voting_ends_at,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	TargetDoc      string  `json:"target_doc,omitempty"`
	ProposedBranch string  `json:"proposed_branch,omitempty"`
	ProposedBody   string  `json:"proposed_body,omitempty"`
	ProposedTitle  string  `json:"proposed_title,omitempty"`
	GitSHA         string  `json:"git_sha,omitempty"`
	BaseSHA        string  `json:"base_sha,omitempty"`
	AppliedAt      *string `json:"applied_at,omitempty"`
	AppliedBy      *string `json:"applied_by,omitempty"`
}

type GovernanceDoc struct {
	ID        string `json:"id"`
	NodeID    string `json:"node_id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Version   int    `json:"version"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
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
