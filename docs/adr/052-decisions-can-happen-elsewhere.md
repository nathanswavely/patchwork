# Decisions can happen elsewhere, and be recorded here

A co-op with twenty years of bylaws joins a quilt. It has a board of
seven, elected last March at its annual membership meeting, with terms
running to next March. Under everything decided up to docs/adr/051 it has
two options, and both are lies: adopt `elected` and Patchwork immediately
opens an election nobody needs, treating a sitting board as unelected
holdovers eight months early — or don't, and the patch describes a
structure it doesn't have.

The same community amends its bylaws on a show of hands in that same
room. Patchwork's answer for that is a vote it would have to stage.

docs/adr/049 established that Patchwork states as fact only what
Patchwork enforces. This is the same principle pointed the other way:
**Patchwork must not force a community's real process through its own
model.** For an established organization, being the public, federated,
portable *record* of how it is run is the whole value, and changing how
it decides is not on the table. Recording is a legitimate mode, and
probably the more common one outside patches founded here.

We decided **a patch declares where its decisions happen, and results
reached elsewhere are attested here.**

- **A venue is declared, and asked twice.** Once of proposals, once of
  leadership *(the proposals half is decided in docs/adr/053, which found
  it is not the mirror image: what can be attested there is a text a
  meeting adopted, never the lining and never the rules file)* — mirroring the split `GovernanceConfig` already has and
  docs/adr/051's table is built on. The modal established nonprofit is
  *hybrid*, not external: bylaws often require an annual membership
  meeting to elect a board, with quorum in a room and sometimes a legal
  reason for it, while "should we buy a kiln" has no such pressure and is
  exactly what the app is for. One venue would force that community to
  declare something false about half its governance.

  A patch running on a `maintainer` has no leadership venue to declare.
  Designation is not a decision anyone convenes for — naming a successor
  is already a single admin act, and recording it *is* the mechanism
  (docs/adr/051). The venue question only bites where a body votes.

- **Attestation is offered only where a venue says decisions happen
  elsewhere.** This is the gate, and without it the whole idea is a
  bypass: if any admin could attest any outcome, "the community approved
  this" becomes a button and the vote machinery is decorative — 049's
  disease with the polarity reversed. On a hybrid patch an admin can
  attest the election and *cannot* attest the kiln, because that patch's
  proposals are decided here. The gate stays tight at each site without
  any rule beyond the declaration.

- **An attestation is not a direct change, and never worded like one.**
  docs/adr/041's direct change is the closest existing thing — a
  governance act Patchwork didn't vote on, stored as a record so it
  shares one history surface. But its claim is "an admin decided this,
  under rules that allow it." An attestation's claim is "*the membership*
  decided this, somewhere Patchwork wasn't," which is far larger, and a
  reader of the timeline has to be able to tell them apart at a glance.

- **The record may name anyone; the effect lands only on members.** This
  is the decision the rest hangs on, and the first draft got it wrong by
  requiring every name to be a current member.

  A record of what a meeting decided is *the community's own statement
  about itself* — the same standing as an About page listing staff, and
  nobody reads a staff page as fabricating accounts. So the March
  election is recorded complete on day one, all seven names, real date,
  real term ends. The *effect* — holding `admin`, appearing in the member
  count, feeding the quilt's affinity math, receiving notifications — is
  a relationship inside the platform, and that needs the person to have
  arrived and consented.

  A named person who has not joined is marked as such, counted nowhere,
  and holds nothing. When they join, they take up the role the record
  already said they held: the record was never wrong, only not yet fully
  realized.

  **Requiring members-only was rejected on adoption grounds, and the
  reasoning is worth keeping.** It would have made an arriving co-op's
  page understate them for a week — one admin where there are seven —
  at exactly the moment the organizer is deciding whether this is worth
  the trouble. What such a community needs on day one is visible
  legitimacy, and there is no reason it should cost anyone's fabricated
  consent to get it. The line that matters is not "who may be named" but
  "what a name does".

- **Fabricated memberships stay impossible.** CONTEXT.md draws this line
  for unclaimed patches — "membership in an organization that hasn't
  admitted anyone is a fabricated relationship... agreement by an
  organization that hasn't arrived is fabricated consent" — and a council
  seat held by someone who never signed up is that same fabrication one
  level down. Unrealized names never become memberships, never count,
  never touch affinity, and are never quietly upgraded.

- **An attestation is corrected by superseding it, never by editing it.**
  A silently editable record is worthless: if what the meeting decided
  can be rewritten without trace, the one property that makes attestation
  acceptable — that it is public and the people in the room can check it
  — evaporates. An immutable one is worse in a different way, because a
  typo then gets worked around with a second, contradictory record that
  claims no relationship to the first.

  So a correction is a new attestation naming the one it corrects. Both
  stay in the timeline, the later governs, and the state follows it. This
  is how real bodies already work — you do not edit approved minutes, you
  record a correction at the next meeting — and it matches what the repo
  already does: proposals are withdrawn rather than un-said, amendments
  keep git history rather than overwriting.

  A correction that removes someone demotes them, which is a real power
  move and carries the same weight as any other: step-up (docs/adr/017)
  and a notification to the person losing the role. It must not be
  quieter than the member list's demote button.

- **The trust model is exit and moderation, not verification.** Patchwork
  cannot check an attestation and does not pretend to; it records who
  asserted what, and when, in public. The answers to an admin who lies
  are the ones the project already has — the reporting system, seamrip,
  and the standing ability to start a new community — not a verification
  mechanism the platform is in no position to operate.

  The residual risk worth naming is the one aimed *outward* rather than
  at members: a patch listing a prestigious board that never shows up,
  as a legitimacy claim to outsiders who will not read the timeline. The
  mitigations are that unrealized names are visibly marked, counted
  nowhere, and never upgraded without the person arriving.

## Consequences for docs/adr/051

- **"Adopting `elected` starts an election" needs the venue branch.** It
  holds where leadership is decided in Patchwork. Where the venue is
  elsewhere, adoption starts nothing and the first attestation supplies
  the council and its term ends.
- **The cycle scheduler must not schedule where the venue is elsewhere.**
  Otherwise a patch that always attests would collect cycles nobody votes
  in, failing quorum and holding over forever — noise that teaches people
  to ignore governance notifications.
- **`succession_policy` and `inactivity_days` are unaffected.** They are
  about a patch running out of people, not about where it decides.

## Open

- The exact fields of an attestation record, and how an unrealized name
  is stored so it can never be mistaken for a membership row.
- Whether unrealized names expire, or sit indefinitely.
- Whether an attestation federates as its own activity or only through
  the governance timeline it already shares.
- Whether a member — not only an admin — may raise an attestation for the
  patch to confirm. Recording is a secretary's job and admins are the
  obvious holders of it, but a community whose secretary is not an admin
  is not far-fetched.

**Status: adopted as a design boundary — implementation is backlog, and
sequenced before docs/adr/051's phase 3**, because the cycle scheduler
and the adoption path both change shape under it.
