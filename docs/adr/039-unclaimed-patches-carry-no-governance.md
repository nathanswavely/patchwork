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
