# ADR 003: One URL scheme per screen — governance URLs are canonical, /manage is removed

Date: 2026-07-13. Status: accepted.

## Context

The SPA carried two complete URL families rendering identical screens:
`/patches/:slug/manage/governance|members|events|settings/*` and
`/patches/:slug/governance|members|events|settings/*`, plus two older
aliases (`/charters/*`, `/proposals/*`). PatchShell had a `manageMode`
prop that was hardcoded `true` at its only render site, leaving a whole
dead branch; ~50 route registrations dispatched to the same components.

## Decision

**Canonical:** `/patches/:slug/governance/*`, `/members`, `/events`,
`/settings/*`. These win on every axis: 30+ in-app link generators and
the backend's notification links already target them, and the mission
frames governance as community participation (follower → member → admin),
not "management" — members vote at a governance URL, admins configure at
a settings URL.

**Removed:** the `/manage/*` mirror, `/charters/*`, `/proposals/*`, and
`/governance-setup`. They redirect (history-replacing) to their canonical
equivalents so old links keep working; they are not separate routes with
duplicate dispatch.

PatchShell loses `manageMode`: one tab set (Governance, Members, Events,
plus Settings for admins), one basePath. The "Managing" badge renders for
admins based on `is_admin`, not on which URL family they arrived by.

## Consequences

- Roughly half the patch-scoped route registrations and route-name sets
  in App.svelte disappear; PatchShell's dead branch goes with them.
- Old `/manage/*` bookmarks land on the canonical URL via redirect.
- e2e specs assert the canonical scheme only.
