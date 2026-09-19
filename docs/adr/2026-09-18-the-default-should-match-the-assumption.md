# The default should match the assumption

Date: 2026-09-18. Status: **accepted**. Extends docs/adr/095 and corrects
its decision 7; narrows the reach of docs/adr/037's pin without weakening
it; departs from the grandfathering precedent of docs/adr/036 and
docs/adr/095 and says why.

## Context

A patch admin on the live instance looked at their own patch's profile and
saw what they expected: a Members section naming the one person in it, and a
Governance section listing the patch's charter. What they did not know is
that a signed-out stranger saw exactly the same page. Several admins had
left their roster public without ever deciding to, and when asked, every one
of them **assumed it was not public**.

That is the whole finding, and it is not a bug in any single surface. Every
control involved works as documented:

- `nodes.public_member_list` defaults to `everyone` (migration 069), which
  docs/adr/095 chose deliberately because it was what every existing patch
  already did.
- `governance_docs.visibility` defaults to `members` (docs/adr/036), so
  charters were never the leak.
- `memberships.visible` defaults to visible (docs/adr/006), and the member
  owns it.

Each default is defensible alone and the composition is not. A patch is born
enumerable, and the setting that would change that is not offered at
creation at all — `POST /api/v1/nodes` does not accept `public_member_list`,
so the field is reachable only from Patch Settings afterwards, by an admin
who already suspects there is something to find.

docs/adr/095 anticipated the group-level want and built the control for it.
What it did not ask is which side of that control a patch should wake up on.
This ADR answers that: **the default should match the assumption.** When the
people using a privacy control consistently believe it is already set, the
belief is the requirement, and shipping the opposite means the product is
quietly wrong about its users for as long as it stands.

### What was actually exposed

Auditing the public reads turned up more than the profile shows, including
two surfaces no setting in the product could have covered:

| Surface | Route | Before | Names people? |
|---|---|---|---|
| Member list | `GET /nodes/{slug}/members` | gated by `public_member_list` and `memberships.visible` | yes |
| Counts | every node payload | public, deliberately (docs/adr/095 §3) | no |
| Charters | `GET /nodes/{slug}/governance`, `/governance/{id}` | members-only by default (docs/adr/036) | no |
| The lining | same list | pinned public (docs/adr/037) | no |
| Proposals | `GET /nodes/{slug}/proposals` | `AuthOptional`, **no room gate**, full body | **yes** — `author_name` |
| Proposal detail | `GET /proposals/{id}` | `AuthOptional` | **yes** — plus `target_user_name` |
| Proposal comments | `GET /proposals/{id}/comments` | **bare mount, no auth wrapper at all** | **yes** — every commenter |
| Attestations | `GET /nodes/{slug}/attestations` | `AuthOptional`, public by design (docs/adr/052) | **yes** — `attestation_names` |
| Governance record | `GET /nodes/{slug}/governance/record` | `AuthOptional` | **yes** — author, applier, decliner |
| Vote rosters | proposal detail | gated on `viewerIsInPatchRoom` + `m.visible` | gated |
| Charter git repo | `.../governance.git/info/refs` | `viewerIsInPatchRoom`, whole-repo (docs/adr/110) | gated |

So the documents were never the problem. **The deliberation was.** A patch's
charters are private by default and its arguments about them are not, which
is backwards: a charter is a finished statement a community chose to make,
and a proposal thread is the unfinished argument that produced it.

docs/adr/095 saw the edge of this and left it — extending the gate to
authorship "trades against the public legibility of governance, which is
most of why proposals are a public read at all — and it is deliberately not
answered here." This answers it.

## Decisions

**1. Withhold the data, never merely the section.** Every change below
changes what the server hands over. Nothing here hides a section over a read
that stays open. The test is that an admin flips the setting, a stranger
runs `curl`, and the names are gone.

This is stated first because the product already contains the other kind.
`follower_permissions.members` hides the Members *tab* from signed-in
followers over a read that stays fully public, and docs/adr/095 names it: "a
workspace tidiness key, not a privacy control, **and the two must not be
merged**." Building a second one here would make the reported bug worse
rather than better, because an admin looking at an empty section would
acquire a false belief the product had actively confirmed.

**2. `public_governance_record`, two rungs, defaulting `nobody`.** One
patch-level column, `everyone` or `nobody`, owned by the patch's admins and
edited in Patch Settings beside `public_member_list`. It covers the patch's
deliberation: proposals (list, detail, body), proposal comments,
attestations and their names, amendment attestations, and the governance
record endpoint. It does **not** cover charters, which carry their own
per-document visibility under docs/adr/036 and are published one at a time;
it does not cover the lining; and it does not cover counts.

Two rungs, where its sibling has three. `public_member_list` can offer
`admins` as a middle rung because a roster is a list of names with a natural
subset. A deliberation record is prose that names people *inside itself* — a
nomination carries `target_user_name` and typically names its subject in its
own title — so "proposals visible, authors hidden" would publish "Nominate
kenr for the open seat" with the author redacted and the subject in the
headline. That is the shape docs/adr/095 §5 already refused for the mirror
case, on the grounds that a setting which says "our organisers are not named
here" while four other surfaces name them is worse than not offering it.

**3. The two controls stay two.** "May this patch be enumerated" and "may
this patch's decisions be read" are different questions, and docs/adr/095
spent its argument establishing that a patch can be public in the ways that
matter while being non-enumerable. A tenants' union may want its roster down
and its decisions loud. They ship as a **pair of defaults**, so a new admin
meets one closed door, and they sit on one settings screen, so opening them
is one decision made twice rather than a hunt.

**4. Both are accepted at creation.** `POST /api/v1/nodes` takes both
fields. A control a patch can only reach after it exists is a control most
patches never reach.

**5. Followers are outsiders, and the tab is suppressed.** `nobody` means
the room: active admins and members, plus instance admins. This matches
docs/adr/095's "a follower is an outsider for this purpose" exactly, which
is most of the value of shipping the pair — "outside the room" means one
thing on both controls. Following is frictionless by design, so treating a
follower as an insider would let anyone self-admit to a closed record by
clicking Follow, which makes the control worth nothing.

`follower_permissions.proposals` is **not** touched and does not become the
gate. It remains workspace tidiness, per decision 1. But it must now read
the record setting to decide whether to render at all: a patch with
`public_governance_record: nobody` and `proposals: true` would otherwise
show a follower a tab over a room the server has emptied. That is F-052
inverted — there, `charters: false` hid a room that had public documents in
it — and the fix is suppression rather than an honest empty state, because
the tab is navigation and a door onto nothing is a false door (docs/adr/042).

**6. A withheld record answers 200 and states the setting.** Not 404, not
403. `GET /nodes/{slug}/proposals` returns an empty `items` with
`public_governance_record` beside it, exactly as `ListMembers` returns
`public_member_list` — because, in docs/adr/095's words, "a client that only
sees an empty array cannot tell 'no members yet' from a withheld list, and
those two want opposite copy." The glimpse already has both sentences
drafted. An individual proposal fetched by id **404s**: without the list
there is no legitimate way to hold that id, and that matches how a
members-only charter behaves.

The objection that 200 leaks "this patch has a closed record" does not bite
here the way it does for a noticeboard (docs/adr/081) or a drafting charter
(docs/adr/036). The patch is public, its tile is on the quilt sized by its
member count, and `public_member_list` already publishes its own value to
anyone who asks. A patch whose existence or activity must be secret wants
`visibility: private`, which is docs/adr/036's own answer to the same
objection.

**7. A membership must not travel as a fact, or as an inference.** This
corrects docs/adr/095 decision 7, which said "Nothing federates. Memberships
have never travelled on an AP actor, so there is no `publicMembers` field to
add and no remote surface to gate."

True of the actor document, false in effect. Proposals are broadcast to node
followers on create, and every ballot emits a `gv:Vote` carrying the voter's
actor ID. **You cannot vote on a patch unless you are a member of it**, so a
`gv:Vote` naming an actor and a node is a membership assertion in all but
name. Memberships federate, one ballot at a time.

So `public_governance_record: nobody` gates federation: no `Create` for
proposals, no `gv:Vote`, no `gv:ResolveProposal`. Without this the setting is
a REST-only fiction — the record closed to a browser and delivered in full to
every remote follower, over a wire with no session to check, in copies that
never come back.

A second defect surfaced from the same audit and is **out of scope here**,
being wrong today for every patch regardless of this work: the REST vote
roster is redacted for a member with `visible = 0` while the AP broadcast of
that same ballot names them, so docs/adr/006's switch is already overruled by
federation. It is fixed separately so the two diffs stay reviewable apart.

**8. The lining follows its status — on the profile, and nowhere else.**
docs/adr/037 pins the lining public, undeletable and title-immutable. That
pin is **not** weakened. The lining remains publicly readable at its own URL
in every case, and accountability is untouched.

What changes is one composition rule: the profile glimpse omits the lining
row when the patch's lining is **pristine or stale**, and includes it when
**diverged**. Without this, every consequence above leaves the reported
screenshot intact — roster withheld, proposals withheld, and a Governance
section still showing "Community Standards v1" on every patch on the quilt.

This is presentation, and decision 1 says presentation fixes are a trap. The
line between them: **withhold the data when there is a privacy interest;
adjust presentation when there is only a signal interest.** A pristine lining
names nobody and is byte-identical on every patch, so no one can form a false
belief about their safety from its absence — there is no privacy claim to be
falsely reassured about. There is only noise: because *every* patch publishes
a lining, seeing one tells a reader nothing. Under this rule a lining on a
profile means "this patch wrote its own," which is precisely the fact
docs/adr/037 wants legible. The pin gets narrower in reach and the signal
gets stronger.

The cost is that a visitor can no longer verify patch-by-patch from the
profile that a patch carries the baseline; they infer it from the absence of
the "Amended lining" badge. That is the same fact by inference rather than by
reading, and `lining_status` is computed server-side from the body hash
independently of document visibility, so the badge is unaffected.

**9. Existing patches are retracted, not grandfathered.** The migration sets
every existing patch to the closed value on both controls.

This departs from precedent twice. docs/adr/036 pinned pre-existing charters
public because "silently retracting live charters would be its own surprise.
The default governs what comes next, not what shipped," and docs/adr/095
defaulted `everyone` on the same reasoning. Both are departed from
deliberately, for two reasons.

The first is that grandfathering does not solve the reported problem. The
patches that already exist *are* the problem; a default governing only future
patches helps nobody currently exposed.

The second is that the two errors are not the same size, and Patchwork has
already accepted this asymmetry in this exact place. docs/adr/095 §6 made the
roster control an admin-immediate lever rather than a governed rule,
justifying the power on the grounds that "the worst an admin can do with it
is hide people, never expose them." That is the same trade applied to a
migration instead of a button: **a wrong retraction costs one click, a wrong
exposure cannot be recalled.**

It also distinguishes this from docs/adr/036, which concerned text a
community wrote and chose to publish. Nobody chose to publish a roster, and
`public_governance_record` has never existed, so no patch has ever chosen
public deliberation — it received it. ADR 036's rule protects decisions;
there are none here to protect.

The audit log cannot narrow this. `node.update` is logged with an empty `{}`
detail, so there is no way to distinguish a patch that deliberately set
`everyone` from one that never looked. Some genuine choices are being
overridden, and they cannot be identified or apologised to individually. That
is the price and it is accepted.

The instance is in early beta with few patches and one operator, who is
notifying the affected admins personally. No in-product notification
machinery is built for a one-time migration; the release note carries
`breaking: true` for anyone self-hosting later.

**10. Creation states it once; Patch Settings states it always.** The create
form carries one sentence naming both defaults. Not a section — docs/adr/037
earned creation-form real estate for the lining because it is a standing
commitment a patch cannot undo, and a default flippable in Patch Settings
does not deserve equal weight. Patch Settings shows both current values
plainly, which is where an admin who goes looking arrives.

The setup checklist is deliberately **not** used. A checklist frames its
contents as tasks to complete, and "publish your member list" is not a task
but a choice most patches should decline; listing it would manufacture
pressure toward the exposure this ADR removes.

**11. The seamrip boundary closes in the same direction.** Two gaps would
otherwise defeat the retraction with a file:

- `def("public_member_list", "everyone")` is the import fallback for archives
  predating the column. Importing a pre-069 archive after this ships would
  restore every roster in it to public. Both fallbacks import as the
  **closed** value: an archive that predates a privacy control cannot consent
  to the exposed one.
- `sqlVisibleProposals` in the member-view rules carries proposals from any
  visible node, on a comment stating the assumption this ADR overturns — "a
  proposal is public deliberation." It gains the room predicate the roster
  rules already use, so proposals travel from patches with an open record
  plus patches the viewer is in.

`TestEveryTableHasAMemberViewRule` will not catch the second: `proposals`
*has* a rule, and it is now the wrong one. The build stays green while the
boundary drifts, which is the failure docs/adr/002 exists to prevent, and the
reason it is written down here.

## Consequences

**The reported screenshot resolves.** A new patch shows a stranger About and
Events. Members collapses, Governance collapses with the lining row, and the
counts still say how many.

**Discovery is unaffected, which is what makes this cheap.** Placement
affinity and member counts are computed directly from `memberships` in
`internal/handler/tree.go` with no visibility filter. A closed patch keeps
its tile size, its inferred threads, and its placement near its kind. Only
identities are withheld — docs/adr/095 §3 holding under a much heavier load
than it was written for.

**A community organizing platform now defaults its governance record
closed.** This is the real cost and it should not be minimised: open
deliberation is most of why proposals were a public read, and the patches
that govern in the open are now the ones that had to say so. The judgement is
that a record a community chose to publish is worth more than one it
published by not knowing, and that the second kind was most of what existed.

**Federated copies already delivered stay delivered.** Retraction cannot
recall what remote instances hold. For a beta with limited federation this is
close to nothing, and it is the one consequence here that no setting can
undo.

**Public links break.** A patch that pointed anyone at its proposals has those
URLs go dark on upgrade.

**docs/adr/095 §7 is corrected**, and docs/adr/037's pin is narrowed in reach
but not in force. The proposal-comments route is brought under `AuthOptional`
and gated; it was mounted bare, with no viewer at all, which is how a design
that never intended world-readable discussion shipped one.

**Not solved, and named so it is not mistaken for a gap.** docs/adr/095's
known limit stands: this buys non-enumerability, not anonymity. An event
still names who posted it. A patch using these controls for safety should
read the settings copy, which says so in one line rather than letting the
control over-promise.
