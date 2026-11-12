package governance

// ValidTemplates lists the available governance templates.
var ValidTemplates = []string{"minimal", "casual", "collaborative", "formal"}

// TemplateInfo describes a governance template for API/UI consumption.
//
// The card says who decides and how, in that order, because that is the
// question a founder is answering when they pick one; how big a group it
// suits was the previous lead, and a size routed a forty-member venue board
// away from the one template that elects a council (docs/adr/104, amended by
// docs/adr/2026-11-11-a-template-card-says-who-decides.md). Elsewhere is the
// line every card carries: a community that decides at a meeting can pick
// any of the four and record its decisions here.
type TemplateInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Leadership  string `json:"leadership"`
	BestFor     string `json:"best_for"`
	Elsewhere   string `json:"elsewhere"`
	// Documents are the filenames a patch made from this template holds,
	// the lining first. Each one is a governance_docs row from the moment
	// the patch exists, so this is a promise the Documents page keeps.
	Documents []string `json:"documents"`
}

// elsewhereLine is the same on every card: where decisions happen is a
// setting every patch has, whichever template it starts from.
const elsewhereLine = "Decisions can be made at a meeting instead, and recorded here."

// TemplateList returns metadata for all available templates. The create
// form carries the same words (web/src/pages/PatchForm.svelte), and
// TestTemplateCardsMatchTheServer fails when the two drift apart.
func TemplateList() []TemplateInfo {
	list := []TemplateInfo{
		{
			ID:          "minimal",
			Name:        "Minimal",
			Description: "You decide. Members can propose, and you approve or decline, or ask them to vote first.",
			Leadership:  "Maintainer",
			BestFor:     "Bands, solo artists, pop-up projects",
		},
		{
			ID:          "casual",
			Name:        "Casual",
			Description: "Members vote for three days, and a proposal passes with more approvals than rejections. The admins run things day to day.",
			Leadership:  "Maintainer",
			BestFor:     "Small collectives, meetups, studios",
		},
		{
			ID:          "collaborative",
			Name:        "Collaborative",
			Description: "Members vote for a week, and a quarter of them must take part. Admins nominate new admins, and members confirm them.",
			Leadership:  "Meritocratic",
			BestFor:     "Venues, co-ops, makerspaces, community radio",
		},
		{
			ID:          "formal",
			Name:        "Formal",
			Description: "Members elect a council for one-year terms. A proposal needs half the members to vote, and fails if anyone rejects it.",
			Leadership:  "Elected council",
			BestFor:     "Boards, coalitions, arts districts, mutual aid networks",
		},
	}
	for i := range list {
		list[i].Elsewhere = elsewhereLine
		list[i].Documents = []string{Filename(DefaultLiningTitle)}
		for _, d := range templateDocs(list[i].ID) {
			list[i].Documents = append(list[i].Documents, Filename(d.Title))
		}
	}
	return list
}

// TemplateDoc is one document a template gives a new patch, beside the
// lining. The title is the governance_docs row's title and the body is its
// body, verbatim; the git file is Filename(Title) holding the same body, so
// the two stores are diffably equal from the first commit (docs/adr/011).
// Bodies carry no top-level heading for the same reason the lining's does
// not: the title lives on the row.
type TemplateDoc struct {
	Title string
	Body  string
}

// templateDocs is each template's documents, in the order a founder reads
// them. Minimal has none of its own; it is the lining alone.
func templateDocs(template string) []TemplateDoc {
	switch template {
	case "casual":
		return []TemplateDoc{
			{"Operating Agreement", operatingAgreementCasual},
		}
	case "collaborative":
		return []TemplateDoc{
			{"Operating Agreement", operatingAgreementCollaborative},
			{"Financial Transparency", financialTransparency},
			{"Conflict Resolution", conflictResolution},
		}
	case "formal":
		return []TemplateDoc{
			{"Charter", charter},
			{"Bylaws", bylaws},
			{"Financial Transparency", financialTransparencyFormal},
			{"Conflict Resolution", conflictResolutionFormal},
			{"Succession Plan", successionPlan},
		}
	}
	return nil
}

// ResolveTemplate names the template a creation request actually gets.
// Empty and unknown names both fall back to casual, as ForkForNode always
// has; the rules file and the documents must come from the same template,
// so every caller asks here rather than repeating the fallback.
func ResolveTemplate(template string) string {
	for _, t := range ValidTemplates {
		if t == template {
			return t
		}
	}
	return "casual"
}

// TemplateDocuments returns the documents a patch created from template
// holds beside the lining, after ResolveTemplate's fallback.
func TemplateDocuments(template string) []TemplateDoc {
	return templateDocs(ResolveTemplate(template))
}

// defaultInstanceFiles returns all files for the instance-level governance repo,
// including the lining (community-standards.md) and all template subdirectories.
func defaultInstanceFiles() map[string]string {
	files := map[string]string{
		// The lining — shared baseline for all patches. Same content as the
		// canonical governance_docs row every new node gets (docs/adr/011).
		Filename(DefaultLiningTitle): DefaultLiningBody,
		// Root-level default rules (backward compat)
		"governance-rules.json": rulesMinimal,

		"templates/minimal/governance-rules.json":       rulesMinimal,
		"templates/casual/governance-rules.json":        rulesCasual,
		"templates/collaborative/governance-rules.json": rulesCollaborative,
		"templates/formal/governance-rules.json":        rulesFormal,
	}
	for _, t := range ValidTemplates {
		for _, d := range templateDocs(t) {
			files["templates/"+t+"/"+Filename(d.Title)] = d.Body
		}
	}
	return files
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
    "charters": false,
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
    "charters": false,
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
    "charters": false,
    "members": true
  }
}
`

// ============================================================
// TEMPLATE DOCUMENTS — Casual
// ============================================================

const operatingAgreementCasual = `## Who We Are

This patch is a group of people who share a common interest and want to coordinate without a lot of overhead.

## How We Make Decisions

- Any member can propose something by submitting a proposal.
- Proposals are open for 3 days. A proposal passes when it has more approvals than rejections.
- There is no quorum. Every vote cast counts, however few there are.
- The admins handle day-to-day decisions that don't need a vote, and carry out the proposals that pass.

## Membership

- Who can join is set in this patch's governance rules.
- Members can vote on proposals and take part fully.
- Followers can follow along. They cannot vote.
- Admins manage the patch and keep things running. An admin can make another member an admin.

## Admins Who Go Quiet

- An admin who has not voted on, made or commented on a proposal here for 90 days is reminded. At 180 days their admin role ends, and they stay on as a member.
- If that leaves the patch with no admin, the successor an admin named takes over. With nobody named, up to three members who are still active here are asked, longest-standing first, whether they will step in. Each who says yes becomes an admin.

## Leaving

- You can leave at any time. The last admin names a successor before leaving.
- If you want to take the pattern and start something new, seamrip it.

## Changing This Agreement

- Any member can propose changes to this document through a proposal.
- A change passes with more approvals than rejections, and takes effect when it passes.
`

// ============================================================
// TEMPLATE DOCUMENTS — Collaborative
// ============================================================

const operatingAgreementCollaborative = `## Mission

This patch exists to serve our community through shared space, resources, and collective decision-making. We believe that the people who do the work should have a voice in how things run.

## Leadership Model: Meritocratic

We follow a meritocratic model inspired by open source communities. Authority is earned through sustained contribution, not elected or inherited.

- **Followers** stay informed. They can see events and follow along, and cannot vote.
- **Members** have joined and made some commitment to the patch. They can vote on proposals and take part in governance.
- **Admins** are the people who started the patch, and members who have been nominated by an admin and confirmed by the community. They manage day-to-day operations.

### Becoming an Admin

An admin may nominate any active member for an admin role. The nomination is posted as a proposal, and members have 7 days to discuss and vote. It passes with more approvals than rejections, provided at least a quarter of the members who can vote take part. The nominee becomes an admin when it passes.

### Stepping Down

Admins can step down at any time, except the last one: the patch keeps at least one admin, so the last admin stays until another has been confirmed. Remaining admins nominate a successor from active members.

## Decision Making

- **Day-to-day decisions**: Admins handle routine operations.
- **Proposals**: Any member can submit a proposal for community decisions.
- **Who can vote**: Members and admins who have been here for 7 days. In the patch's own first week there is no wait.
- **Voting period**: 7 days.
- **Quorum**: At least 25% of the members who can vote must take part, abstentions included. A proposal that closes short of that decides nothing.
- **Passing threshold**: More approvals than rejections for most decisions. For changes to governance documents, at least two thirds of the votes for or against must be for.

## Membership

- Who can join is set in this patch's governance rules.
- Members who haven't taken part in 60 days may be contacted about their status.
- An admin who has not voted on, made or commented on a proposal here for 60 days is reminded. At 120 days their admin role ends, and they stay on as a member.

## Amendments

Any member can propose changes to this operating agreement. An amendment passes when at least two thirds of the votes for or against are for, with 25% quorum, and takes effect when it passes.
`

const financialTransparency = `## Principle

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

Changes to financial policies need at least two thirds of the votes for or against to be for.
`

const conflictResolution = `## Principle

Conflict is not desired, but it is expected. This process exists so that disagreements don't become schisms.

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

Depending on severity: a warning, or removal from the patch. An admin can reverse a removal, so a removal meant to be temporary is reversed when its time is up. Violations of the community standards (the lining) may result in immediate action.

## Confidentiality

Details of conflicts are shared only with the people directly involved and those handling the resolution. Public discussions should use anonymized descriptions.
`

// ============================================================
// TEMPLATE DOCUMENTS — Formal
// ============================================================

const charter = `## Preamble

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

This patch is governed by an elected council. The council has as many seats as it has set up, and each seat is held for a 12-month term. The council handles day-to-day decisions, facilitates community governance, and is accountable to the membership.

### Members

Members are the foundation. They vote on proposals, elect the council, and can propose amendments to any governance document. Voting rights begin after 30 days of membership. In the patch's own first 30 days there is no wait, so the people who start it can vote from the start.

### Followers

Followers stay informed about events and activities. They cannot vote.

## Relationship to the Quilt

This patch is part of a larger patchwork. We agree to uphold the community standards (the lining) and to participate in good faith with other patches on the quilt.
`

const bylaws = `## Article 1: Elections

### 1.1 Council Elections
Each council seat is elected for a 12-month term. The election for a seat opens four weeks before its term ends. Members who have been here for at least 30 days can vote.

### 1.2 Nominations
Any member may nominate themselves or another member for a council seat. Nominations are open for 14 days, and voting opens when they close.

### 1.3 Voting
Elections use approval voting. Each member approves as many candidates as they like, and the candidates with the most approvals take the seats. A candidate needs at least one approval, and at least half of the members who can vote must take part. Voting is open for 14 days.

### 1.4 Terms
Council members serve 12-month terms. There is no limit on the number of terms, but each term requires re-election. If an election settles nothing (no candidates, too few voters, or nobody approved), the sitting council stays until one does.

## Article 2: Council Operations

### 2.1 Council Decisions
Proposals on Patchwork go to every member who can vote, not to the council alone. When the council decides something among itself, a majority of council members must take part.

### 2.2 Decision Making
The council works by consensus when possible. If consensus cannot be reached within a reasonable time, a council decision needs two thirds of council members.

### 2.3 Removal
Any member can propose that a council member be removed. If the proposal passes, the council removes that member's admin role.

## Article 3: Membership

### 3.1 Joining
Who can join is set in this patch's governance rules. Where joining needs approval, a council member approves each request, and approval should be granted to anyone who commits to the community standards and participates in good faith.

### 3.2 Voting Rights
Voting rights begin after 30 days of membership. This ensures voters have context for the decisions they're making. In the patch's own first 30 days there is no wait.

### 3.3 Leaving
Members may leave at any time. A member who leaves and comes back rejoins as a new member, and their 30 days start again.

## Article 4: Amendments

### 4.1 Proposing Amendments
Any member can propose an amendment to these bylaws or any governance document by submitting a proposal.

### 4.2 Review Period
Amendment proposals are open for 14 days. Discussion and voting run over
that same window: comment on the proposal, and change your vote, at any
point before it closes.

### 4.3 Approval
An amendment passes only if nobody votes to reject it, at least one member approves it, and at least half of the members who can vote take part.

### 4.4 Implementation
Approved amendments are not applied automatically. A council member reviews the change and applies it, to ensure consistency with other governance documents.

## Article 5: Dissolution and Seamrip

### 5.1 Dissolution
This patch may be dissolved by a proposal that passes under the rules in 4.3.

### 5.2 Seamrip
Any group of members may seamrip (take the governance documents, tools, and their data) and start a new patch. This is a right the parent patch must facilitate in good faith.

### 5.3 Assets
Upon dissolution, remaining assets are distributed as determined by the final membership vote. If no consensus is reached, assets are donated to a community organization chosen by the council.
`

const financialTransparencyFormal = `## Principle

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

The council proposes an annual budget as a proposal. Like any proposal here, it passes only if nobody votes to reject it, at least one member approves it, and at least half of the members who can vote take part. Mid-year budget amendments follow the same process.

## Audit

An annual financial review is conducted by a member who is not on the council. The review is published to all members.

## Reporting

- Monthly financial summaries posted to governance docs
- Annual comprehensive report with review
- Any member can request details on any transaction at any time

## Amendments

Changes to financial policies pass only if nobody votes to reject them, at least one member approves, and at least half of the members who can vote take part.
`

const conflictResolutionFormal = `## Principle

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

In cases of immediate safety concern, any admin may remove someone from the patch without waiting for the full process. Emergency actions must be reviewed by the council within 48 hours, and the council reverses a removal it does not uphold.

## Consequences

- Verbal or written warning
- Required mediation or training
- Removal for a defined period, which the council reverses when the period ends
- Permanent removal (requires council vote, appealable to membership)

## Confidentiality

All conflict resolution proceedings are confidential. Public communications use anonymized descriptions. Participants may share their own experience but should not share others' private information.

## Record Keeping

Records of formal grievances are maintained by the council for 2 years. They are accessible only to current council members and the involved parties.
`

const successionPlan = `## Purpose

Power must rotate. This document ensures that leadership transitions are planned, transparent, and don't depend on any single person.

## Regular Succession: Elections

### Schedule
Each council seat is elected for a 12-month term, and its election opens four weeks before the term ends:
- **Days 1 to 14**: Nominations. Any member can nominate themselves or another member, and candidates can post a short statement.
- **Days 15 to 28**: Voting (14 days), by approval voting.
- **When voting closes**: Results are announced and the new council is seated.

If an election settles nothing, the sitting council stays until one does. No clock removes anybody.

### Transition
Outgoing council members have 7 days to transfer responsibilities, access, and context to incoming members. A transition document is published to governance docs.

## Vacancies

### A Seat Falls Empty Mid-Term
An admin nominates a member for the empty seat. If members confirm the nomination by proposal, the nominee takes the seat for the rest of its term.

### Several Seats Fall Empty
Each empty seat is filled the same way, by nomination, or at the next election.

### Every Seat Falls Empty
If every seat is vacated for inactivity, an election opens shortly, and any member may stand.

## Inactivity

### Detection
A council member who has not voted on, made or commented on a proposal for 30 consecutive days is contacted.

### Process
- Day 30: Notification sent to the inactive council member.
- Day 45: If no response, the council discusses the situation.
- Day 60: If still inactive, the seat is declared vacant and the steps above apply.

## Seamrip Succession

If a group of members seamrips to create a new patch, they carry their governance documents with them. The original patch's succession plan continues to operate independently.

## Amendments

Changes to the succession plan pass only if nobody votes to reject them, at least one member approves, and at least half of the members who can vote take part.
`
