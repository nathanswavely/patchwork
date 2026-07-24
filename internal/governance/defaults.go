package governance

// ValidTemplates lists the available governance templates.
var ValidTemplates = []string{"minimal", "casual", "collaborative", "formal"}

// TemplateInfo describes a governance template for API/UI consumption.
type TemplateInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Leadership  string   `json:"leadership"`
	BestFor     string   `json:"best_for"`
	Documents   []string `json:"documents"`
}

// TemplateList returns metadata for all available templates.
func TemplateList() []TemplateInfo {
	return []TemplateInfo{
		{
			ID:          "minimal",
			Name:        "Minimal",
			Description: "Just a listing. You run it, no overhead.",
			Leadership:  "Maintainer",
			BestFor:     "Bands, solo artists, pop-up projects",
			Documents:   []string{"community-standards.md"},
		},
		{
			ID:          "casual",
			Name:        "Casual",
			Description: "Small crew, lightweight process. Majority rules.",
			Leadership:  "Maintainer → Meritocratic",
			BestFor:     "Small collectives, meetups, studios (5–20 people)",
			Documents:   []string{"community-standards.md", "operating-agreement.md"},
		},
		{
			ID:          "collaborative",
			Name:        "Collaborative",
			Description: "Open community, structured process. Earn trust through contribution.",
			Leadership:  "Meritocratic",
			BestFor:     "Venues, co-ops, makerspaces, community radio (20–100 people)",
			Documents:   []string{"community-standards.md", "operating-agreement.md", "financial-transparency.md", "conflict-resolution.md"},
		},
		{
			ID:          "formal",
			Name:        "Formal",
			Description: "Coalition-scale governance. Elected council, term limits, full accountability.",
			Leadership:  "Elected Council",
			BestFor:     "Arts districts, mutual aid networks, coalitions (100+ people)",
			Documents:   []string{"community-standards.md", "charter.md", "bylaws.md", "financial-transparency.md", "conflict-resolution.md", "succession-plan.md"},
		},
	}
}

// defaultInstanceFiles returns all files for the instance-level governance repo,
// including the lining (community-standards.md) and all template subdirectories.
func defaultInstanceFiles() map[string]string {
	return map[string]string{
		// The lining — shared baseline for all patches. Same content as the
		// canonical governance_docs row every new node gets (docs/adr/011).
		"community-standards.md": DefaultLiningBody,
		// Root-level default rules (backward compat)
		"governance-rules.json": rulesMinimal,

		// Minimal template
		"templates/minimal/governance-rules.json": rulesMinimal,

		// Casual template
		"templates/casual/governance-rules.json":  rulesCasual,
		"templates/casual/operating-agreement.md": operatingAgreementCasual,

		// Collaborative template
		"templates/collaborative/governance-rules.json":      rulesCollaborative,
		"templates/collaborative/operating-agreement.md":     operatingAgreementCollaborative,
		"templates/collaborative/financial-transparency.md":  financialTransparency,
		"templates/collaborative/conflict-resolution.md":     conflictResolution,

		// Formal template
		"templates/formal/governance-rules.json":     rulesFormal,
		"templates/formal/charter.md":                charter,
		"templates/formal/bylaws.md":                 bylaws,
		"templates/formal/financial-transparency.md": financialTransparencyFormal,
		"templates/formal/conflict-resolution.md":    conflictResolutionFormal,
		"templates/formal/succession-plan.md":        successionPlan,
	}
}

// ExportDefaultFiles returns all default instance files (exported for API use).
func ExportDefaultFiles() map[string]string {
	return defaultInstanceFiles()
}

// templateFiles returns just the files for a specific template (without the templates/ prefix).
func templateFiles(template string) map[string]string {
	all := defaultInstanceFiles()
	prefix := "templates/" + template + "/"
	result := map[string]string{}
	for path, content := range all {
		if len(path) > len(prefix) && path[:len(prefix)] == prefix {
			filename := path[len(prefix):]
			result[filename] = content
		}
	}
	return result
}

// ============================================================
// THE LINING — shared community standards baseline
// ============================================================

// DefaultLiningTitle and DefaultLiningBody are the single identity of the
// default lining (docs/adr/011). CreateNode inserts a governance_docs row
// with this title and body, and the governance repo fork writes the same
// body to the file this title slugifies to (community-standards.md). The
// DB row is canonical; the git file is its history mirror. Keep them fed
// from these two constants only — a second copy is how the pre-ADR-011
// drift happened.
const DefaultLiningTitle = "Community Standards"

// DefaultLiningBody is the current shipped lining text — the head of the
// lineage in lining.go (docs/adr/037). It deliberately has no top-level
// heading: the title lives in the governance_docs row (and the constant
// above), and git file content must equal the DB body verbatim so the two
// stores stay diffably equal.
var DefaultLiningBody = CurrentLiningBody()

// ============================================================
// GOVERNANCE RULES (JSON) — one per template
// ============================================================

const rulesMinimal = `{
  "decision_method": "admin",
  "quorum_percent": 0,
  "default_vote_duration_hours": 0,
  "amendment_threshold": "majority",
  "amendment_auto_apply": true,
  "succession_policy": "longest_tenure",
  "min_voting_tenure_days": 0,
  "leadership_model": "maintainer",
  "succession_method": "founder_designate",
  "admin_term_months": 0,
  "max_admins": 1,
  "inactivity_days": 0,
  "membership_policy": "invite_only",
  "follower_permissions": {
    "events": true,
    "proposals": false,
    "charters": false,
    "members": false
  }
}
`

const rulesCasual = `{
  "decision_method": "majority",
  "quorum_percent": 0,
  "default_vote_duration_hours": 72,
  "amendment_threshold": "majority",
  "amendment_auto_apply": true,
  "succession_policy": "longest_tenure",
  "min_voting_tenure_days": 0,
  "leadership_model": "maintainer",
  "succession_method": "admin_nominate",
  "admin_term_months": 0,
  "max_admins": 3,
  "inactivity_days": 90,
  "membership_policy": "open",
  "follower_permissions": {
    "events": true,
    "proposals": true,
    "charters": true,
    "members": true
  }
}
`

const rulesCollaborative = `{
  "decision_method": "majority",
  "quorum_percent": 25,
  "default_vote_duration_hours": 168,
  "amendment_threshold": "supermajority",
  "amendment_auto_apply": true,
  "succession_policy": "nomination",
  "min_voting_tenure_days": 7,
  "leadership_model": "meritocratic",
  "succession_method": "admin_nominate",
  "admin_term_months": 0,
  "max_admins": 5,
  "inactivity_days": 60,
  "membership_policy": "approval_required",
  "follower_permissions": {
    "events": true,
    "proposals": true,
    "charters": true,
    "members": true
  }
}
`

const rulesFormal = `{
  "decision_method": "consensus",
  "quorum_percent": 50,
  "default_vote_duration_hours": 336,
  "amendment_threshold": "consensus",
  "amendment_auto_apply": false,
  "succession_policy": "election",
  "min_voting_tenure_days": 30,
  "leadership_model": "elected",
  "succession_method": "election",
  "admin_term_months": 12,
  "max_admins": 7,
  "inactivity_days": 30,
  "membership_policy": "approval_required",
  "follower_permissions": {
    "events": true,
    "proposals": true,
    "charters": true,
    "members": true
  }
}
`

// ============================================================
// TEMPLATE DOCUMENTS — Casual
// ============================================================

const operatingAgreementCasual = `# Operating Agreement

## Who We Are

This patch is a group of people who share a common interest and want to coordinate without a lot of overhead. We keep things simple.

## How We Make Decisions

- Any member can propose something by submitting a proposal.
- Proposals are open for 3 days. Majority vote wins.
- No quorum requirement. If you care, vote.
- The admins (maintainers) handle day-to-day decisions that don't need a vote.

## Membership

- Anyone can join. This patch is open.
- Members can vote on proposals and participate fully.
- Followers can see what we're up to without participating in governance.
- Admins manage the patch, approve changes, and keep things running.

## Leaving

- You can leave at any time. No hard feelings.
- If you want to take the pattern and start something new, seamrip it.

## Changing This Agreement

- Any member can propose changes to this document through a proposal.
- Changes pass by majority vote.
`

// ============================================================
// TEMPLATE DOCUMENTS — Collaborative
// ============================================================

const operatingAgreementCollaborative = `# Operating Agreement

## Mission

This patch exists to serve our community through shared space, resources, and collective decision-making. We believe that the people who do the work should have a voice in how things run.

## Leadership Model: Meritocratic

We follow a meritocratic model inspired by open source communities. Authority is earned through sustained contribution, not elected or inherited.

- **Followers** are community members who stay informed. They can attend events and observe governance.
- **Members** have demonstrated commitment to the patch. They can vote on proposals and participate in governance.
- **Admins** are experienced members who have been nominated by existing admins and ratified by the community. They manage day-to-day operations.

### Becoming an Admin

Existing admins may nominate any active member for an admin role. The nomination is posted as a proposal. The community has 7 days to discuss and vote. A majority of active members must approve.

### Stepping Down

Admins can step down at any time. When an admin steps down, remaining admins nominate a successor from active members.

## Decision Making

- **Day-to-day decisions**: Admins handle routine operations.
- **Proposals**: Any member can submit a proposal for community decisions.
- **Voting period**: 7 days for most proposals.
- **Quorum**: 25% of active members must participate for a vote to be valid.
- **Passing threshold**: Simple majority for most decisions. Supermajority (2/3) for amendments to governance documents.

## Membership

- Membership requires approval. Apply and an admin will review your request.
- Members who haven't participated in 60 days may be contacted about their status.
- Admins inactive for 60 days will be asked to step down or re-engage.

## Amendments

Any member can propose changes to this operating agreement. Amendments require a supermajority (2/3) vote with 25% quorum.
`

const financialTransparency = `# Financial Transparency

## Principle

Money is where trust dies in community. Every dollar in and every dollar out should be visible to every member, in real records anyone can open at any time. Nobody should have to wait for a treasurer's report at a meeting.

## Income

All income (dues, grants, donations, event revenue, sales) is recorded with:
- Amount
- Source
- Date
- Purpose or restriction (if any)

## Spending

All spending is recorded with:
- Amount
- Recipient
- Date
- What it was for
- Who approved it

## Approval Process

- **Under $50**: Any admin can approve.
- **$50–$500**: Requires two admin approvals.
- **Over $500**: Requires a proposal and community vote.

These thresholds can be adjusted by amending this document.

## Reporting

A financial summary is posted monthly and is always accessible to members. Anyone can ask questions about any transaction at any time.

## Amendments

Changes to financial policies require a supermajority vote.
`

const conflictResolution = `# Conflict Resolution

## Principle

Conflict is normal. How we handle it is what matters. This process exists so that disagreements don't fester into fractures.

## Step 1: Direct Conversation

If you have an issue with someone, talk to them directly first. Most conflicts come from misunderstanding, not malice. Be specific about what happened and what you need.

## Step 2: Mediation

If direct conversation doesn't resolve it, ask a neutral member or admin to mediate. The mediator's job is to listen to both sides and help find common ground, not to judge.

## Step 3: Formal Complaint

If mediation doesn't work, submit a formal complaint to the admins. Include:
- What happened (specific actions, not character judgments)
- When it happened
- What resolution you're seeking

Admins will respond within 7 days with a proposed resolution.

## Step 4: Community Review

If you disagree with the admin's resolution, you can escalate to a community review. The situation is presented (with identities protected where possible) as a proposal. The community votes on the resolution.

## Consequences

Depending on severity: warning, temporary suspension, permanent removal. Violations of the community standards (the lining) may result in immediate action.

## Confidentiality

Details of conflicts are shared only with the people directly involved and those handling the resolution. Public discussions use anonymized descriptions.
`

// ============================================================
// TEMPLATE DOCUMENTS — Formal
// ============================================================

const charter = `# Charter

## Preamble

This charter establishes the governance framework for this patch. It is the foundational document that defines our mission, structure, and principles. All other governance documents build on this charter.

## Mission

We exist to serve our community through collective organizing, shared resources, and transparent self-governance. We believe that the people who build the culture should govern the culture.

## Values

- **Transparency**: All decisions, finances, and governance are public to members.
- **Accountability**: Power rotates. No one stays in charge indefinitely.
- **Inclusion**: We actively work to include voices that are typically excluded.
- **Self-determination**: We govern ourselves. We don't answer to external authorities that don't share our values.
- **Right to seamrip**: Any group can take our patterns and build their own quilt.

## Organizational Structure

### The Council

This patch is governed by an elected council of up to 7 admins. The council handles day-to-day decisions, facilitates community governance, and is accountable to the membership.

### Members

Members are the foundation. They vote on proposals, elect the council, and can propose amendments to any governance document. Membership requires approval and a 30-day tenure before voting rights activate.

### Followers

Followers stay informed about events and activities. They can participate in discussions but cannot vote.

## Relationship to the Quilt

This patch is part of a larger patchwork. We agree to uphold the community standards (the lining) and to participate in good faith with other patches on the quilt.
`

const bylaws = `# Bylaws

## Article 1: Elections

### 1.1 Council Elections
Elections are held annually. All members who have been active for at least 30 days are eligible to vote.

### 1.2 Nominations
Any member may nominate themselves or another member for a council seat. Nominations open 14 days before the election and close 7 days before.

### 1.3 Voting
Elections use ranked-choice voting. Each member ranks candidates in order of preference. Voting is open for 14 days.

### 1.4 Term Limits
Council members serve 12-month terms. There is no limit on the number of terms, but consecutive terms require re-election.

## Article 2: Council Operations

### 2.1 Quorum
A majority of council members must be present (or voting) for council decisions.

### 2.2 Decision Making
The council operates by consensus when possible. If consensus cannot be reached within a reasonable timeframe, decisions fall back to a supermajority (2/3) vote of council members.

### 2.3 Removal
A council member can be removed by a supermajority vote of the full membership. A removal proposal requires signatures from at least 10% of active members to be considered.

## Article 3: Membership

### 3.1 Joining
New members must be approved by a council member. Approval should be granted to anyone who commits to the community standards and participates in good faith.

### 3.2 Voting Rights
Voting rights activate after 30 days of active membership. This ensures voters have context for the decisions they're making.

### 3.3 Leaving
Members may leave at any time. Membership data is retained for 30 days in case of return.

## Article 4: Amendments

### 4.1 Proposing Amendments
Any member can propose an amendment to these bylaws or any governance document by submitting a proposal.

### 4.2 Review Period
Amendment proposals are open for 14 days of discussion before voting begins.

### 4.3 Approval
Amendments to the bylaws require consensus. If consensus cannot be reached, a supermajority (2/3) with 50% quorum is required.

### 4.4 Implementation
Approved amendments are not auto-applied. A council member must review and merge the change to ensure consistency with other governance documents.

## Article 5: Dissolution and Seamrip

### 5.1 Dissolution
This patch may be dissolved by a consensus vote of the membership with at least 50% quorum.

### 5.2 Seamrip
Any group of members may seamrip (take the governance documents, tools, and their data) and start a new patch. This is a right the parent patch must facilitate in good faith.

### 5.3 Assets
Upon dissolution, remaining assets are distributed as determined by the final membership vote. If no consensus is reached, assets are donated to a community organization chosen by the council.
`

const financialTransparencyFormal = `# Financial Transparency

## Principle

Every dollar in and every dollar out is recorded and visible to every member. Transparency here is structural, written into the rules rather than left to whoever happens to hold the books. This document establishes the rules for how money moves through this patch.

## Income

All income (dues, grants, donations, event revenue, sponsorships, sales) is recorded with:
- Amount
- Source
- Date received
- Purpose or restriction (if any)
- Council member who acknowledged receipt

## Spending

All spending is recorded with:
- Amount
- Recipient
- Date
- Description of what it was for
- Which council member approved it
- Budget line item it falls under

## Approval Tiers

| Amount | Required Approval |
|--------|------------------|
| Under $100 | Any council member |
| $100–$1,000 | Two council members |
| $1,000–$5,000 | Council vote (majority) |
| Over $5,000 | Community vote (proposal) |

## Budgeting

The council proposes an annual budget as a proposal. The budget must be approved by a majority vote with 50% quorum. Mid-year budget amendments follow the same process.

## Audit

An annual financial review is conducted by a member who is not on the council. The review is published to all members.

## Reporting

- Monthly financial summaries posted to governance docs
- Annual comprehensive report with review
- Any member can request details on any transaction at any time

## Amendments

Changes to financial policies require a supermajority vote with 50% quorum.
`

const conflictResolutionFormal = `# Conflict Resolution

## Principle

This patch is committed to resolving conflicts fairly, transparently, and with respect for all involved. This process exists to protect people, not to punish them.

## Scope

This process applies to conflicts between members, between members and admins, and between patches. It does not replace legal processes where applicable.

## Step 1: Direct Resolution

Parties are encouraged to resolve conflicts directly. Be specific about actions and impacts, not character judgments.

## Step 2: Mediation

If direct resolution fails, either party may request mediation. A neutral mediator is selected from a pool of trained members or, if none are available, an admin not involved in the conflict.

The mediator:
- Meets separately with each party
- Facilitates a joint conversation
- Helps identify common ground and next steps
- Documents the agreement (shared only with involved parties)

## Step 3: Formal Grievance

If mediation does not resolve the conflict, a formal grievance may be filed with the council. The grievance must include:
- Specific actions that caused harm
- Dates and context
- Steps already taken to resolve
- Requested resolution

The council responds within 14 days with a proposed resolution.

## Step 4: Appeal

If either party disagrees with the council's resolution, they may appeal to the full membership. The appeal is presented as a proposal with identities protected where possible. The community votes on the resolution.

## Emergency Actions

In cases of immediate safety concern, any admin may take emergency action (temporary suspension) without waiting for the full process. Emergency actions must be reviewed by the council within 48 hours.

## Consequences

- Verbal or written warning
- Required mediation or training
- Temporary suspension (with defined end date)
- Permanent removal (requires council vote, appealable to membership)

## Confidentiality

All conflict resolution proceedings are confidential. Public communications use anonymized descriptions. Participants may share their own experience but should not share others' private information.

## Record Keeping

Records of formal grievances are maintained by the council for 2 years. They are accessible only to current council members and the involved parties.
`

const successionPlan = `# Succession Plan

## Purpose

Power must rotate. This document ensures that leadership transitions are planned, transparent, and don't depend on any single person.

## Regular Succession: Elections

### Schedule
Council elections are held annually. The election timeline:
- **Day 1–14**: Nomination period. Any member can nominate themselves or another member.
- **Day 15–28**: Campaign/discussion period. Candidates share their vision.
- **Day 29–42**: Voting period (14 days). Ranked-choice voting.
- **Day 43**: Results announced. New council seated.

### Transition
Outgoing council members have 7 days to transfer responsibilities, access, and context to incoming members. A transition document is published to governance docs.

## Emergency Succession

### Single Vacancy
If one council seat becomes vacant mid-term, the council may appoint a replacement from active members. The appointment must be ratified by the community within 14 days.

### Multiple Vacancies
If more than half the council seats are vacant, an emergency election is triggered immediately using the regular election process with compressed timelines (7 days each phase instead of 14).

### Total Vacancy
If all council seats are vacant (the "bus factor" scenario), the three longest-tenured active members become interim admins. They must initiate an emergency election within 7 days.

## Inactivity

### Detection
Council members who have not participated in governance (votes, proposals, discussions) for 30 consecutive days are contacted.

### Process
- Day 30: Notification sent to inactive council member.
- Day 45: If no response, the council discusses the situation.
- Day 60: If still inactive, the seat is declared vacant and succession procedures begin.

## Seamrip Succession

If a group of members seamrips to create a new patch, they carry their governance documents with them. The original patch's succession plan continues to operate independently.

## Amendments

Changes to the succession plan require consensus. If consensus cannot be reached, a supermajority (2/3) with 50% quorum is required.
`
