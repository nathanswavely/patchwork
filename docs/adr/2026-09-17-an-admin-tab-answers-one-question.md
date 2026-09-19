# ADR: An admin tab answers one question

Date: 2026-09-17. Status: **accepted**. Amends ADR 005, which gave the admin
panel its tab row and said nothing about how many tabs it could hold.

## Context

ADR 005 made the admin panel a full-screen takeover with a single tab row,
the same treatment a patch workspace gets. A patch workspace has at most
five tabs, and the two dense ones (Governance, Settings) each carry a
sidebar. The admin panel had no such second level, so every feature that
needed an admin page added a tab. By ADR 114 there were fifteen: Overview,
Quilt, Neighbors, Label, Legal, Tags, Users, Reports, Submissions, Event
submissions, Aggregators, Claims, Archived, Audit Log, Prove admin. The row
scrolled on a laptop, and the ordering encoded the sequence in which the
features had been built rather than anything an admin would think in.

Sorted by the question an admin arrives with, the fifteen fall into four:

- *What is waiting on me?* Reports, patch submissions, event submissions,
  claims, and the suggested-tag queue that ADR 114 had put at the top of
  the Tags page. All the same shape: a pending list with approve and
  reject.
- *How is this quilt configured?* Quilt settings, the Label, legal
  documents, the tag vocabulary, neighbors, aggregators. The exact
  analogue of a patch's Settings.
- *Who is on it?* Users, and the archived patches.
- *What happened, and can I prove it?* The audit log, and the attestation
  tool of ADR 087.

The Overview of PR #296 already answers the first question at a glance,
with a count per queue read from `GET /api/v1/admin/overview`.

## Decision

The admin panel has five tabs, and a tab answers one question:

| Tab | Sections |
|---|---|
| Overview | none |
| Review | Reports · Patch submissions · Event submissions · Claims · Suggested tags |
| Users | none |
| Settings | Quilt · Label · Legal · Tags · Neighbors · Aggregators · Usage · Archived patches · Prove admin |
| Audit log | none |

Review and Settings render their sections in the same `SettingsShell` a
patch's Settings and Governance use. A section's URL is its tab's plus one
segment (`/admin/review/claims`, `/admin/settings/legal`), and a tab's bare
URL lands on its first section, since a tab with sections has no page of
its own. The tab and section lists live in `web/src/lib/adminPanel.js` as
pure data, the way `patchWorkspace.js` holds the workspace's, so the shape
is asserted without rendering.

The Review tab wears the total of what is waiting, and each Review section
wears its own count. Both are read from the Overview's inbox, not counted
again: a badge that counts differently from the page under it is worse
than no badge.

The suggested-tag queue becomes a Review section with its own page.
Deciding a word is a queue's work and curating the vocabulary is a
setting's; the two pages link to each other.

The flat scheme survives as a redirect. `/admin/*` is a wildcard alias
behind every explicit route, mapping each retired path onto the section
its page became, and answering not-found for a path that never existed.
The five notification links Go emits, the two patch-side links to the
claims queue, the Overview's own links and the admin finder all point at
the nested paths directly; the redirect is for what is already in
somebody's inbox or bookmarks.

Two placements are judgment calls. Archived patches is a list rather than
a setting, but it has nowhere better to go short of building a general
admin Patches page, which nothing has asked for. Prove admin is rare and
step-up gated, so it sits last under Settings, beside the danger zone.

## Considered options

- **Collapse only the configuration cluster** into a Settings sidebar and
  leave the queues flat: eight tabs. Rejected. The queue pages are the ones
  an admin opens daily, and they were the ones scattered.
- **One merged inbox** listing every pending item with a type filter,
  instead of a Review sidebar. Best for scanning, and the most work: five
  endpoints with five action vocabularies merged client-side, a row
  component per type. Left open for later; the Review sidebar does not
  preclude it, since the Overview already lists the queues side by side.
- **A left rail for the whole panel, no tab row.** What most admin
  consoles do. Rejected as a departure from ADR 005's one-tab-row rule
  that the patch workspace follows, with its own phone treatment to
  invent.
- **Keep the flat URLs and change only the chrome.** Cheaper, but then the
  URL and the navigation disagree about where a page lives, and the
  suggested-tag page would have had no address distinct from the
  vocabulary's. ADR 003's one-URL-per-screen rule wanted the nesting.

## Consequences

- Fifteen tabs become five, with a sidebar under two. The row fits.
- `web/src/lib/adminPanel.js` is the one place the panel's shape is
  stated. A new admin page is a section of Review or Settings, or the case
  for a sixth tab has to be made here.
- `SettingsShell` accepts an optional `count` per section. Nothing but the
  Review sidebar passes one.
- The old flat paths are aliases, so nothing that carried one breaks, but
  nothing new should emit one.
- CONTEXT.md's Admin panel entry describes the five tabs; "Quilt settings"
  moves to `/admin/settings/quilt`.
