# ADR 075: Discovery asks a question; the quilt shows everything

Date: 2026-08-31. Status: accepted. Decided while grilling discoverability
on the home quilt.

## Context

The home quilt renders forty-nine tiles and the full tag vocabulary as
chips, and asks nothing. A newcomer's problem there is not that they cannot
narrow — it is that nobody has asked them anything, so no tile has a claim
to being the one they click. Better filters do not fix that; they are a
volume tool, and volume is not yet the complaint.

The fix was already written and then locked away. `web/src/pages/Welcome.svelte`
runs a complete discovery flow: a shortlist of the eight **most-worn tags on
this quilt** (ranked by `node_count` from `GET /api/v1/tags`, unworn ones
alphabetical after, "show all" for the rest), then the patches wearing them
with follow buttons inline. Its own comment states the doctrine — "never a
baked-in vocabulary — white-label quilts surface their own categories."

It is shown once. `web/src/lib/onboarding.js` dismisses it into localStorage
forever; ADR 040 gated `/welcome` to signup and invite completion. Anonymous
visitors never see it, returning people cannot reach it, and every existing
person on the live reference instance has already spent their single use.
The best discovery surface in the app is the one almost nobody may use.

Meanwhile the discovery chips render `getAllTags()` — raw vocabulary order.
Same data, same store, two treatments, and discovery got the worse one.

The standing temptation is a recommender: "people who follow this also
follow…". That is the shape every platform reaches for, and the intro card
promises every anonymous visitor, in three sentences on first landing,
"that no algorithm runs it" (ADR 040).

## Decision

**Welcome is two things fused, and they split.** Step 1 is post-signup
orientation — the agreement's gist, shown to someone who signed one screen
ago — and stays auth-gated and one-shot exactly as ADR 040 requires. Steps 2
and 3 are discovery, have nothing to do with agreement, and are promoted to
a standing public route: re-enterable, reachable by anonymous visitors, with
its own entry in the left rail. **ADR 040 is not amended** — its gating was
always about the orientation, which merely happened to share a file.

**Discovery mode asks one question and shows a short answer.** That is the
whole distinction from `/`, and it is the Zillow lesson properly read:
Zillow's front door does not render four thousand listings with facets
alongside — it asks "where?", and everything after is a consequence of the
answer. Google Maps asks the same question by knowing where you stand. Both
defer the wall of results until you have said something.

Two things follow and are accepted on purpose:

- **Discovery mode has no quilt canvas.** A canvas is the show-everything
  gesture; putting one here would collapse the mode back into `/`.
- **It ends in follows, not navigation.** People leave with relationships,
  which is what makes My Quilt worth having on the next visit.

**The engine is what this quilt wears and what is happening on it — never
what other people follow.** Suggestion ranks by tag usage on this instance,
identical for every viewer, anonymous included, verifiable in one click
("nine patches wear this"). Where genuine rotation is wanted, the honest
rotating fact is **upcoming events** — Google Maps' "Open now": a fact about
the world, self-refreshing as the calendar moves, the same for everyone, and
an answer to what a newcomer is actually asking.

**Co-follow recommendation is refused, and the reason is correctness rather
than taste.** "People who follow Tellus360 also follow…" is computed from the
follower sets of a forty-nine-patch quilt; at community scale that is often a
handful of people, and a co-follow signal derived from three humans is a
membership disclosure. ADR 033 refused instance-wide people search on exactly
this ground — "a small quilt must not be enumerable by typing a first name" —
and ADR 006 gave each person a switch over whether a membership is visible at
all. A recommender would leak in aggregate what those two decisions protect
directly.

## Considered options

- **Kill the mode; fold usage-ranked chips and quilt order into `/`.**
  Genuinely considered, and the strongest alternative: the mechanics
  (tag toggles, follow hearts) already exist on the home quilt, so the mode
  risks being a second copy of it. Rejected because the quilt's gesture is
  show-everything and the missing act is *asking* — a gesture the quilt
  cannot make without ceasing to be a quilt.
- **Amend ADR 040 to un-gate `/welcome`.** Rejected: it drags post-signup
  orientation onto a public surface, where the agreement's gist is addressed
  to nobody.
- **Co-follow suggestions, even shown transparently.** Rejected above. A
  visible explanation does not undo a disclosure.
- **A seeded shuffle for rotation.** Rejected: it rotates, but it makes a
  claim it cannot explain, and "why is this one first" would have no answer.
- **Leave the flow one-shot and improve it in place.** Rejected — being
  one-shot is the defect.

## Consequences

- Two routes where there was one; Welcome hands off to discovery mode rather
  than containing it.
- Discovery mode is reachable anonymously, so its follow buttons route to
  sign-in — the pattern `SocialHome.toggleFollow` already uses.
- The usage-ranked shortlist should also become the chips' order on the
  discovery surfaces; raw vocabulary order was never a decision either.
- Every existing person on the live instance gains access through the
  standing entry point. This is why the flow, not onboarding, is the right
  home for anything offered at its end (ADR 076).
- No per-person state anywhere in the flow: nothing is stored about who saw
  what, and the surface behaves identically for a stranger and a regular.
