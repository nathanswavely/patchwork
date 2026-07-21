# ADR 022: Discovery narrowing is three independent lenses

Date: 2026-07-20. Status: accepted. Decided while grilling the tag-filter
UI rethink after PR #67.

## Context

PR #67 made the quilt store's long-dead tag filter reachable by adding a
chip row to the home quilt's cards pane. It shipped with two structural
problems: the row scrolls poorly on desktop and disappears entirely on
mobile outside the list view — the control lived on one surface while the
state it set narrowed several.

The rethink exposed a pre-existing version of the same disease: the
"search lives on the quilt" doctrine (typing was inert off the quilt;
Enter jumped there) coexisted with a shared search-query store that
EventsPage read anyway — so a query typed on the quilt silently followed
you to Events and narrowed the list with no visible explanation. Discovery
had accumulated three narrowing mechanisms (scope, tags, query) with three
different lifecycles, three different homes, and no shared discipline.

## Decision

Discovery narrowing is **three independent lenses — scope, filter, and
query** — governed by one rule set:

- **Independent.** Touching one lens never changes another. Switching
  scope keeps the filter; clearing the query keeps the filter; no lens is
  cleared as a side effect. Composition is explained where it bites: an
  empty result set names the active lenses ("No patches match your filter
  in My Quilt") with a one-step clear and, where scope participates, a
  scope switch.
- **Applied in place, on every discovery surface.** Treemap, cards, map,
  and events all narrow live under the same lenses. The "search lives on
  the quilt" jump is retired: typing on the Events page filters events.
- **Never silently active.** The filter is toggled in the **filter card**,
  anchored beneath the discovery search and opened on focus — empty query
  included, because tags serve exactly the person who doesn't know what to
  type. When the card is closed, active filters announce themselves: a
  count chip in the search input on desktop (also a click-target back into
  the card), a bell-style count badge on the mobile shelf's search button.
  On mobile the card renders inside the existing search takeover.
- **Session-ephemeral filter.** The filter outlives the search
  *interaction*, not the *visit*: reload clears it. Scope-adjacent
  localStorage persistence was rejected — a quilt pre-narrowed by last
  week's forgotten tap is the silent-lens failure in its worst form, and
  adding persistence later is the easy direction; removing it is not.
- **Union semantics, plain chips.** Multiple tags OR together — on a
  curated vocabulary where patches wear one or two tags, intersection is
  nearly always empty. Chips carry no per-tag counts in v1: counts differ
  per scope, the tags endpoint doesn't serve them, and live-narrowing
  results are the honest count.
- **Discovery-only.** The scoped finder (workspace, admin panel) shares
  the global bar's slot but never sprouts the filter card. The PR #67 chip
  row is removed; the filter card is the only place tags are toggled.

## Considered options

- **Filter as query refinement** (tags live and die with the search
  interaction): rejected. EventsPage already read the shared state
  independently of any query, and a filter you can only hold while the
  search box is open can't serve "show me the music corner of the quilt"
  browsing.
- **Scope switch clears the filter**: rejected. Mechanically prevents the
  empty-composition trap but makes filters second-class and destroys
  built-up state because the person peeked at the other scope. The empty
  state explains instead.
- **Keep the jump-to-quilt doctrine and stop Events reading the query**:
  coherent, rejected. The unified model is one sentence — "the search box
  narrows whatever discovery surface you're on" — and matches what the
  store already did.
- **Per-tag counts, AND semantics, localStorage persistence**: each
  rejected for v1 as recorded above; all three are additive if real usage
  argues for them.

## Consequences

- The indicator discipline binds future discovery surfaces: anything new
  that renders under the lenses must either show the filter or show the
  count. This is the cost of standing state, accepted deliberately.
- Empty states become lens-aware (naming filter and scope) rather than
  generic.
- The `filterTags` prop plumbing survives; the control moves from
  SocialHome's cards pane into the search surface (SocialShell), which
  owns query state already.
- Implementation must land after PR #68 (tag write path, ADR 021), which
  touches the same store and App shell — this ADR deliberately claims 022
  with #68 in flight, per the number-claiming rule in CLAUDE.md.
