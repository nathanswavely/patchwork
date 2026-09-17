# ADR 039: Unclaimed patches carry no governance; claims complete through setup

Date: 2026-07-24. Status: accepted. Decided while grilling the fallout of
the v0.7.1 lining rollout, which created lining docs on every unclaimed
patch.

## Context

CONTEXT.md already rules that an unclaimed patch is follow-only because
"membership in an organization that hasn't admitted anyone is a
fabricated relationship." The lining rollout (ADR 037) violated the same
principle from a different angle: it stamped a consent document onto
patches where nobody had ever performed the act of consenting. ADR 037's
own adoption rule — shown out loud at creation, never a surprise — has no
subject on a directory listing the instance admin bulk-loaded.

It also wasted real resources: every unclaimed patch carried a lining row
and a governance mirror repo that no one would read or govern by, and the
claim flow inherited a patch with governance the claimant never saw.

## Decision

**Governance exists only for active patches.** An unclaimed patch has no
lining doc, no governance repo, and no governance surface on its page —
absence, not a "not yet adopted" state. The amended-lining discovery
filter never treats a governance-free unclaimed patch as amended;
unclaimed patches are outside lining semantics, not in violation of them.

**A claim is creation with prepopulated fields, not a handoff.** The flow:

1. Suggestion (unchanged): users suggest listings (`pending_review`,
   admin-approved); instance admins and trusted contributors create
   unclaimed patches directly.
2. Claim (unchanged, ADR 030): an assertion of ownership, proven by any
   verification method or admin review. Concurrent claims race; first
   proof wins.
3. **Patch setup (new):** a verified or approved claim is a single-use,
   expiring (14 days, with a reminder) right to enter the patch creation
   flow prepopulated with the listing's data. Until setup is submitted
   the patch is still unclaimed to every visitor — no "awaiting setup"
   badge, or the claim becomes the reservation ADR 030 forbids. On
   expiry the patch is simply claimable again.
4. **Setup submit is the creation moment:** status flips to active, the
   claimant becomes admin, the lining is adopted out loud, and the
   governance repo is created — with the then-current lining text
   ("agreed to the baseline, whatever it currently says").

**Two fields are locked in setup:** the slug (the patch's existing public
address — bookmarked, followed, and minted into its AP id) and the
verification domain (the trust anchor the claim just proved). Everything
else creation allows, setup allows.

**Amended 2026-09-16, on the reference instance's first real claim — "who
can join" is one of the things creation allows.** Setup shipped without it,
on the reasoning that the listing already carried a membership policy and
setup could inherit it. Every listing is written `open` (`unclaimed.go`), the
value decides nothing while the patch is a listing — an unclaimed patch takes
followers only — and nobody has ever been asked: the submitter is not the
patch, and at submission there is no patch to ask. The claim is where that
value stops being inert. The Candy Factory went live admitting anyone, and
its admin could not find the control, because membership policy is governance
and lives in the rules file, not in Patch Settings.

Worse, setup's own template picker was being overruled. The absorb step that
copies the row's live membership settings into the freshly forked rules file
was written for ordinary creation, where the row carries the creator's
answer. On this path it carried the listing's, so a claimant who picked
Minimal — whose rules say `invite_only` — got `open`.

So setup asks, seeded from the chosen template rather than starting blank
(the template already states a policy; a fourth opinion would be one too
many), and the seed stops following the template the moment the claimant
answers for themselves. `POST /claims/{id}/setup` takes `membership_policy`,
validated like creation's, and falls back to the template's own value for a
client that sends none — never to the row's. Listings are now written
`invite_only` so that the one thing an unchosen value can still do is fail
closed. Patch Settings → Info states the current policy and links to the
rules editor: the control stays in one place, but it stops being unfindable.

**Cleanup is one migration, no standing machinery.** Migration 039
deletes lining docs on non-active patches unconditionally — whatever sits
there is fabricated consent by definition, since no member could have
adopted or amended it. Orphaned mirror repos are removed by a one-off
operation on the affected instance, not by permanent reconcile code: the
state can't arise again, so no code should exist to clean it.

## Considered options

- **Keep linings on unclaimed patches, marked "not yet adopted"**:
  rejected. Invents a fourth lining state for what is really the absence
  of a party to the agreement; the glossary's fabricated-relationship
  rule says absence is the honest model.
- **Adoption at claim submission**: rejected. Proving you own a venue and
  agreeing to community norms are unrelated acts; gluing consent to the
  claim form only made sense before setup existed to hold it. Concurrent
  claimants would each "adopt" a document that never materializes.
- **Approval activates the patch immediately, setup as an optional
  settings pass**: rejected. The patch would be active with an admin
  before anyone adopted the lining — governance materializing
  sight-unseen in an admin's approval click, which re-breaks the rule
  this ADR exists to fix.
- **A permanent startup reconcile deleting orphaned governance repos**:
  rejected. Point-forward: post-refactor the state cannot recur, and
  dead cleanup machinery running on every boot of every instance is a
  worse cost than one manual `rm` on the one instance that has the
  orphans.

## Addendum, 2026-09-16: the no-badge rule is about visitors

"No 'awaiting setup' badge" above is a rule about **the patch**. It is what
keeps an approved claim from becoming the reservation ADR 030 forbids: a
visitor must not be able to read a patch as spoken for, because until setup
is submitted nobody runs it and it is still claimable by anyone whose proof
lands first.

Read as a rule about *every* surface, it left the two parties to the claim
with nowhere to see their own business. The admin panel listed `pending`
only, so approving a claim removed it from the one page an admin could
return to — the approval's only record was a toast. And My Patches is built
from memberships, which an approved claimant does not hold until
`activateClaimedNode` runs at setup, so the claimant's only trace was the
notification announcing it. A 14-day window could close in silence at both
ends. It read as a failed write, which is how this came up.

So: the patch says nothing, and the two parties see their own. The admin
panel gained an "Approved, awaiting setup" section with each claim's expiry;
My Patches gained a "Claiming" section with the claimant's own claims,
carrying the setup link and saying out loud that the patch still reads
unclaimed to everyone else. Neither is reachable without being somebody —
instance admin, or the claimant — and neither changes what the patch shows.

`GET /api/v1/users/me/claims` serves the second. Both listings run the lazy
expiry sweep first (`expireAllPastDueApprovedClaims`), because a right that
has lapsed must not be listed as one still held.

## Addendum, 2026-09-16: `approved` is not "awaiting setup"

The claim row deliberately never changes when setup succeeds: a second
attempt is refused because the patch is no longer unclaimed, not because the
claim moved on (`SetupClaim`). So `status = 'approved'` records only that the
claim cleared review. What says setup happened is the *node*, which is now
`active`.

The section added above asked the claim and not the node, so a claimant who
finished setup stayed in "Approved, awaiting setup" forever, telling instance
admins to keep waiting on a patch that was already live and run by the person
named beside it. A claimed venue sat in that queue on the Lancaster instance.
The lazy expiry sweeps had the same gap from the other side: once the window
passed they would rewrite a claim that had *succeeded* into one that lapsed.

The rule, then: any surface that means "awaiting setup" joins `nodes` and
requires `status = 'unclaimed'`. `MyClaims` and the expiry reminder already
did. `ListClaims` and both sweeps now do, via `claimNodeStillUnclaimed`.
`RequestClaim`'s approved-claim block needs no join because it refuses
anything but an unclaimed patch several lines earlier.
