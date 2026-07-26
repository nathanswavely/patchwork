# The patch profile is a window, not a lobby

A patch's public page ended in one centered, wrapping row of equal-weight
pills that held five unrelated kinds of act at once: relationship (Join,
Become Member, Follow, Unfollow, Leave), stewardship (Claim this patch),
contribution (Suggest an event), navigation (Manage, Governance), and
moderation (Report). Nothing in the row said which of them the page
wanted, because the primary slot meant something different in every
state — `Join` on an active patch, `Claim this patch` on an unclaimed
one, where the loudest pixel on the page was aimed at roughly one visitor
in a thousand while `Follow`, the act nearly every visitor should take,
sat in secondary.

The incoherence was not only visual. An instance admin viewing an
unclaimed patch saw four buttons, three of them false for them: `Claim
this patch` with no admin guard at all, `Suggest an event` because
`canSuggest` short-circuits on `isUnclaimed` *before* it checks
direct-post rights (`events.go:272` lets instance admins and trusted
contributors post directly, so the button promised a review queue they
bypass), and `Manage`, which on an unclaimed patch silently lands on
`/events` instead. A stranger on an invite-only patch — the canonical
band setup — got `Join` as the page's primary, walked the join sheet's
ceremony, and received `403 "this node is invite only"` from
`memberships.go:67`; `JoinSheet` knows only `approval_required` versus
everything else. And a plain member, the person who actually turns up and
votes, was offered exactly one relationship control: `Leave`.

Two structural faults sat underneath. The page was **terminal**: charters
and proposals opened a `Modal` rendering `doc.body` inline rather than
navigating to the surfaces that own them, so every glance ended in a
dialog to dismiss — which is why a `Manage` button had to be bolted on to
get anywhere. And the same relationship logic was implemented twice, in
`PatchProfile` and in `PatchShell`'s cluster, where it had already
drifted: the shell renders nothing at all for admins, and the two toast
the same event as "Now a member" and "You are now a member".

We decided:

- **The action row is the relationship row.** It states where the viewer
  stands and offers the next rung, and holds nothing else — no
  navigation, no contribution, no moderation. At most two controls in any
  state. A row where "up" and "out" and "elsewhere" are the same size
  doesn't render the contributor ladder, it renders a fork.

- **Standing is a control, and the exit lives inside it.** "Following" /
  "Member" / "Admin" is the resting form; `Unfollow` and `Leave` are its
  menu. This costs one deliberate extra step to leave and buys a fact the
  page never stated before — today a member can only infer that the page
  knows them by reading the word "Leave" and working backwards. Leaving
  is a member right (ADR 012); a pill wedged beside "Suggest an event"
  was never the surface that right deserved.

- **`Follow` is the stranger's primary everywhere**, claimed and
  unclaimed alike, and **`Join` renders only where it can succeed** —
  never on `invite_only`. Not disabled, not "request an invite": absent.
  Following is frictionless on any public patch by design; joining
  depends on a policy the visitor cannot see. One rule, no
  state-dependent surprise, and the upsell isn't lost — the follower
  state puts the next rung in the primary slot forever after.

- **The rung's word is "Become a member", everywhere.** `Join` named two
  different acts on two different objects: becoming a member of a patch,
  and signing up for the quilt (`IntroCard.svelte:65`), which is the
  first `Join` a newcomer meets. The role words already carry the
  relationships; "Become a member" names the rung in the vocabulary that
  exists. `Join` is left to mean joining the quilt.

- **The profile is a window into the patch's rooms, not a lobby in front
  of them.** One glimpse per workspace surface; the heading is the door;
  items navigate to their real routes and the modals go. Ordered for the
  glance — events, about, members, governance — and deliberately *not* in
  the workspace's tab order, which leads with Governance because that is
  the workspace's root for members. The two orders serve different
  audiences: a flyer's QR code lands a stranger here, and what a stranger
  wants first is what is happening and where it is.
  A glimpse renders when the viewer may **enter that
  room or act in it**, collapsing only when the room is both empty and
  inert for that viewer — otherwise a brand-new patch renders zero doors
  and strands its admin at the moment they most need to get inside, and a
  stranger on an event-less patch that accepts suggestions gets no window
  to suggest through. This adds the members glimpse the page never had,
  though the endpoint is already public (`AuthOptional`) and ADR 006
  deliberately designed that list and the switch governing it.

- **Every door names a room, never a container.** This is the rule that
  makes the page navigable without a `Manage` button, and it is the test
  a future label must pass: `Settings` is fine because it names a room;
  `Manage` was unnameable because it named the container — which is also
  why `CONTEXT.md`'s Workspace entry already lists "manage area" under
  *Avoid*.

- **State belongs in the header; acts belong beside what they act on.**
  The unclaimed state becomes a header notice that states the fact and
  carries the claim inside it, following the Amended lining badge's
  precedent one line above — unboxed, because "Interruption" is a closed
  category for things loud on purpose (ADR 038) and unclaimed is a state,
  not a warning. Nothing on the page said the patch was unclaimed: the
  fact was invisible while the act shouted, and `Claim in progress` was a
  status wearing a button's clothes. `Suggest an event` moves to the
  Events glimpse. `Report`, `Subscribe`, and `Workspace view` move to a
  `⋯` overflow in the header — which also surfaces the per-patch ICS/RSS
  feeds for the first time on a public page; they were reachable only
  from inside the workspace's Events tab.

- **Labels follow the outcome, not the patch's state.** `New event` when
  the person posts directly, `Suggest an event` when it enters review.
  That is ADR 026's own principle — review is owed to whoever owns the
  calendar — rendered honestly, and it fixes the `canSuggest` ordering
  bug rather than papering over it. Naming the outcome also exposed a
  second, older mismatch: `CreateEvent` tested direct-post rights with
  `userHasMembership`, which counts *any* active membership, so a
  **follower's event published straight to the calendar** — ADR 026 grants
  that rung to members and admins. Following is frictionless precisely
  because it carries no write rights, so the server now tests the role
  (`userHasNodeRole(…, "member", "admin")`). The shared helper is left
  alone: comments and proposal participation legitimately admit followers
  under `follower_permissions`, and only the events door was wrong.

**Considered and rejected: collapsing the profile into the workspace tab
row.** The strongest alternative, and the one that dissolves the naming
problem outright — GitHub, the analogy `CONTEXT.md` already uses for the
workspace, has no door because there is no inside to enter; you are on
the repo and your permissions decide the tabs. `/patches/:slug` is today
the only member of its URL family rendered outside `PatchShell`, and that
orphaning is what forced a door into existence. It was rejected because
the profile's whole value is being read at a glance: a shared flyer's QR
code should land on a face, not on a takeover. ADR 005 is therefore
untouched, the two-page split stands, and `PatchShell`'s eye icon back to
the profile keeps its meaning as the way back to the glance.

**Considered and rejected: naming the door.** What was rejected is a
named door as the page's primary way in — the `Manage` slot — not the
word itself in every position. `Workspace` is the architecture word:
accurate about shape, effectively unspoken in the UI before this (the
sole user-facing instance was a sentence in the danger zone), and it
leans "work" when a follower browsing proposals isn't working. `Hall`
carries the right civic register — union hall, grange hall — but will not
read to an organizer who has never met the usage. `Forum` is legible and
civic but promises a message board that does not exist behind it: the
rooms are Governance, Members, Events, Settings, and what discussion
exists is welded to decisions (`proposal_comments`). Worse, it would
corrupt **thread** — the inferred connection between patches, the
signature concept — by making "discussion thread" every visitor's default
reading of the word, and it is the obvious name for a real per-patch
discussion surface should one ever be built. The naming problem was
dissolved rather than solved: name the rooms, which are already named,
and the container never needs a word.

Consequences: the relationship vocabulary must converge across
`PatchProfile` and `PatchShell` rather than being written twice, and the
existing drift between them (admins get no cluster in the shell; the two
toasts disagree) is part of the work, not a follow-up.
`web/src/test/onboarding-workspace-surfaces.test.js` asserts the exact
button markup for `Join` and `Become Member` and will fail by design.
Invite-only patches lose their join affordance entirely, with no "request
an invite" replacement — recruiting for a band stays out-of-band, which
is what invite-only already meant. `Workspace view` makes `workspace` a
user-facing word for the first time, in exactly one place — a deliberate
narrowing of the rejection above, not a contradiction of it: the overflow
is the one position where the container really is the destination, so the
alternative was a vaguer label (`Full view`) that never says where you
land. A word spoken once teaches nothing by itself; it takes its meaning
from the surface it opens. Because a glimpse now renders whenever its
viewer may enter it, every member always has doors, so the item is
redundant by construction and can be dropped later without structural
loss. `Thread`,
`Patch profile`, `Relationship row`, and `Standing control` are added to
`CONTEXT.md` — the first of those had been referenced in the glossary
only as two denials and never defined.
