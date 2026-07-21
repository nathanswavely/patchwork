# One global bar with contextual slots; workspaces take over below it

> Amended by ADR 003: the `/patches/:slug/manage/*` URL family this ADR
> mentions was retired — workspaces render (unchanged) at the canonical
> `/patches/:slug/governance|members|events|settings` URLs.

The app has three contexts: discovery (the quilt), patch workspaces
(/patches/:slug/manage), and the admin panel (/admin). Previously a
workspace rendered as a child of the discovery shell — global top bar +
discovery rail + workspace header + section nav stacked four layers of
chrome, and the most prominent affordances (scope switcher, patch search)
were about leaving the context the user was working in.

We adopted the GitHub-repository model (analogy, not vocabulary — see
below): exactly one slim global bar persists on every screen, holding a
context crumb (quilt mark alone in discovery; mark / patch name in a
workspace; mark / Administration in the admin panel), a contextual search,
and the bell + user menu, which never move. Below the bar, a workspace
takes the full screen with its own single tab row (Governance · Members ·
Events · Settings[admin-only], relationship cluster at the right end);
the discovery rail, scope switcher, and mobile bottom-pill nav do not
exist inside workspaces or the admin panel. Exits are the mark (→ quilt)
and "View profile" (→ public patch page). Workspace root is Governance.

Workspace search is a scoped finder, not an index: the workspace's
entities (members, proposals, documents, events; admin panel: users,
reports, submissions) are fetched once on first focus via existing list
endpoints and filtered client-side — zero new backend, sized for
community-scale data and the Raspberry Pi target. The provider seam allows
a server-side search endpoint later without changing the component.

## Considered options

- Full takeover (no global chrome in workspaces): rejected — notifications
  and account access matter most during governance work, and it forks the
  top bar into two implementations.
- Keeping the discovery shell and compressing the workspace header:
  rejected — discovery nav is irrelevant to managing a patch, and the
  squeeze was the complaint.
- Naming the workspace "repo": rejected — "repo" already denotes the real
  git-backed governance repositories (governance.ForkForNode); reusing it
  for a UI surface would collide inside the governance subsystem. GitHub
  remains the named design analogy only.
