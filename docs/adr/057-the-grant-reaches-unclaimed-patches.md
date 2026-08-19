# ADR 057: The trusted-contributor grant reaches unclaimed patches

Date: 2026-08-19. Status: accepted. Amends ADR 032 (standing in the link
handshake is no longer admins-only). Decided while grilling the reuse of
the patch picker for the event page's "link a patch" control.

## Context

ADR 032 gave the link handshake one standing rule: "admins on either side
propose, admins on the *other* side confirm." `userSpeaksForNode` encodes
it as instance-admin-or-patch-admin, and it is the same predicate for both
halves of the handshake.

That rule leaves community-submitted events with no one to link them. An
event on an unclaimed patch (ADR 026) has no patch admins by definition —
the organization has not claimed it. The instance admin can act, because
they hold unclaimed patches' calendars in trust. Nobody else can: a
trusted contributor may *record* the event, but the moment it wants a
second patch on it, the button does not render. `canAct` is false, so the
person best placed to know that the gig was a co-bill is shown no control
at all.

This is the seam the grant already runs along everywhere else. ADR 026
gave trusted contributors the right to record events on unclaimed patches
without review; ADR 056 gave an aggregator that same standing, described
there as "ADR 026's trusted-contributor grant made non-human." Links were
the one place the seam stopped.

## Decision

**A trusted contributor speaks for a patch when that patch is unclaimed.**
One condition on `userSpeaksForNode`, alongside the instance-admin and
patch-admin cases. Nothing else moves: `initiated_by` stays the
owner/linked binary, no schema changes, no new notification type.

Two properties survive verbatim:

- **Worth nothing on active patches.** A trusted contributor gets no
  standing on a claimed patch's event, on either side. The glossary's
  "orthogonal to patch roles: not a rung between member and admin" is
  still true.
- **A link never lands without the other side's hand on it.** Where the
  other side is a claimed patch, its own admins still confirm. Where it is
  another unclaimed patch, the hand is the same hand that already holds
  that calendar in trust.

On two unclaimed patches a trusted contributor speaks for both sides, so
the handshake self-confirms. That is deliberate: they can already create
the events on both patches, and asserting that two patches share a gig is
the weaker claim of the two.

## Considered and rejected

**Propose anywhere, confirm nothing.** Split `userSpeaksForNode` into a
propose predicate and a confirm predicate, letting a trusted contributor
propose a link on any event — including between two claimed patches — while
never confirming one. It reads as the safer rule and is not, for two
reasons. It contradicts "worth nothing on active patches," quietly turning
the grant into a rung on the ladder after all. And a third-party proposal
has no true `initiated_by` value: neither "waiting for The Selvage to
confirm this link" nor "The Selvage asked to link to this event" is a true
sentence when neither side asked for anything. It needs a third state, new
copy, and a rule for which side confirms first — machinery for a case
nobody requested.

**Leave it alone.** Coherent, and it was the status quo for a reason. It
means the events with the least institutional attention are the ones that
can never be linked, which inverts who the feature is for.

## Consequences

Revoking the grant leaves any pending links that person proposed still
pending. They are ordinary rows and either side may remove them; nothing
needs to cascade.
