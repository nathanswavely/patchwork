# ADR 032: Search is a dropdown; narrowing is explicit

Date: 2026-07-22. Status: accepted. Amends ADR 022. Decided while
grilling the search rethink after the lens-persistence bug (search
survived navigation invisibly).

## Context

ADR 022 made the search query a lens: typing on a discovery surface
narrowed it live, in place. That was coherent when discovery *was* the
product — typing-as-interaction made the quilt feel alive under your
hands. But the same search box now persists across a growing social
shell (patch profiles, event detail, dashboard…), where typing was inert
until Enter jumped you to the quilt — opaque, and the source of the bug
where a query kept narrowing the quilt after its input had unmounted.
Meanwhile the workspace and admin contexts already had the right shape:
an autocomplete over the context's entities (the "scoped finder").

The three-lens discipline of ADR 022 (independent, never silently
active, session-ephemeral) was never the problem. Typing-as-lens-setter
was.

## Decision

**One search posture everywhere: an autocomplete dropdown.** The global
bar's search is the same interaction on every screen — type, see typed
results, pick one, land there. Context decides the corpus: discovery
searches all public patches and upcoming events; a workspace searches
its own members, proposals, documents, events; the admin panel its
users, reports, submissions. Typing never narrows any surface as a side
effect.

**No instance-wide people search — reaffirmed.** People appear in search
only where the context legitimizes the result (a workspace's member
list, the admin panel). A small quilt must not be enumerable by typing a
first name. Pasted profile/patch URLs still resolve: possessing the URL
means the person was found through a legitimate surface.

**The query lens survives as an explicit act.** The discovery dropdown
ends with one action row — "Show matches on the quilt" — which applies
the query as a standing **search chip** and lands on the quilt. This is
the only way a query becomes narrowing state; it answers the spatial
question ("show me everything zine-ish in its neighborhoods") that a
result list cannot.

**The filter moves out of the search box onto the surfaces it
narrows.** The full tag vocabulary renders as toggleable **filter
chips** on every discovery surface — the control and the indicator are
the same thing, living where the narrowing bites. The filter card is
retired. Placement follows the surface type:

- *Desktop canvases (quilt, map):* the chip row overlays the canvas's
  top edge, bounded by the canvas width — wrapping to further lines
  rather than extending into the cards pane's column. A peer element on
  the results' plane, not global-bar chrome.
- *Desktop page surfaces (events list, cards list):* the chips are
  their own component at the top of the page flow, wrapping.
- *Mobile canvases:* a badged button opposite the Label's info button,
  opening the chips as a sheet above the nav row.
- *Mobile page surfaces:* the top-of-flow component, collapsed to the
  badged button by default.

The chips collapse to a single badged button under **one shared
persisted preference** (localStorage, sidebar-collapse pattern): open
by default on desktop, closed by default on mobile, governing every
in-flow and overlay home at once. The mobile canvas *sheet* is exempt —
sheets are open-while-using, dismissed-when-done, not a preference.
The search chip rides among the tag chips in every form.

**Announced where it bites, so never revoked.** The filter only ever
narrows discovery surfaces, and every discovery surface now wears the
chips (or their badged button) — so state can survive navigation
anywhere without ever being silently active: quilt → patch → back keeps
your narrowed quilt, and the chips are standing right on it. Reload is
the only reset (session-ephemeral, unchanged from ADR 022). The
route-based reset shipped earlier the same day (clear lenses on leaving
discovery) is superseded: it cured invisibility by amputation; the
on-surface chips cure it by visibility. Non-discovery pages carry no
indicator — nothing on them is narrowed.

**Client-side, provider-pattern data.** The discovery corpus is fetched
once on first focus (the nodes tree + upcoming events), cached for the
visit, filtered in memory — extending the existing finder-provider
pattern (ADR 005 scale doctrine: community-scale data, no server index,
no per-keystroke requests). The search deliberately does not find deep
past events; the events page's date controls are the tool for
archaeology.

**Vocabulary: "search", not "finder".** The standard word wins where
textile vocabulary adds nothing. "Scoped finder" is retired in prose;
code keeps the finder naming (WorkspaceSearch, finderProviders) — not
worth the churn.

## Considered options

- **Two postures (lens on discovery, dropdown elsewhere)**: rejected.
  Honest about context but makes one control behave two ways, and the
  live-narrowing posture is what produced the silent-lens bug class.
  One sentence — "search is a dropdown; the quilt narrows only when you
  ask" — beats a conditional.
- **Kill the query lens entirely**: rejected. Tags cover curated
  browsing, but free-text spatial browsing ("everything with 'zine'")
  has no tag. Kept behind an explicit row instead.
- **Server-side FTS endpoint**: rejected for now. Finds everything ever
  and scales past community size, but adds an index to maintain and
  breaks the one-fetch pattern. Additive later if a real instance
  outgrows client-side.
- **People in the discovery dropdown**: rejected; would overturn the
  ADR 006-adjacent enumeration protection for no articulated need.

## Consequences

- ADR 022's lens rule set survives with one amendment: the query lens is
  set explicitly (search chip), not by typing; the filter's home moves
  from the search-anchored card to on-surface chips. Independence, the
  indicator discipline, union semantics, and session-ephemerality all
  stand.
- The mobile search takeover no longer carries the filter card; the
  shelf search button loses its filter badge (the canvas button and
  top-of-flow component carry the count instead).
- The chips render the full vocabulary — this leans on ADR 021's
  curated-vocabulary restraint. A vocabulary that outgrows a couple of
  wrapped lines is an admin-restraint problem before it is a UI one.
- EventsPage and SocialHome keep reading the shared filter state
  unchanged — only how that state is set moves.
- The route-based lens reset in App.svelte is removed; the
  input/store sync in SocialShell stays (the box must always show the
  truth).
