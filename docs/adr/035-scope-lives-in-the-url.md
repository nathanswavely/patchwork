# ADR 035: Scope lives in the URL

Date: 2026-07-24. Status: accepted. Decided while grilling the My Quilt
routing rethink after the back-button bug: scope changed the view but
not the address, so history and bookmarks couldn't see it.

## Context

Scope — the switch between My Quilt and the whole quilt (CONTEXT.md) —
was in-memory `$state` in App.svelte, never in the URL. Three
consequences fell out of that one fact:

- **Back/forward couldn't restore it.** Flipping scope while already on
  `/` pushed no history entry (the router early-returns on a same-path
  navigate), so a scope change was invisible to the browser. Quilt →
  patch → back could land on a different scope than you left.
- **Nothing was bookmarkable.** A person who mostly lives in their own
  quilt had no address to save or share for it.
- **The default was a stateful effect**, not a decision: an
  auth-conditional `$effect` set logged-in users to My Quilt, and that
  same class of "reassert the scope" logic is what made back-navigation
  unpredictable.

The fix is to make the URL the single source of truth for scope. The
question the grill resolved was *how* to shape that URL without
inventing a second "place" concept — My Quilt is a lens over the home
quilt, not another quilt (only follows blend across quilts; ADR 024).

## Decision

**Scope is a path suffix on each discovery surface, both values named,
the whole quilt unmarked.** The shape is `/[surface]/[scope]`, both
parts optional:

```
quilt  → /              /my
map    → /map           /map/my
events → /events        /events/my
```

The whole quilt is the *unmarked default* — it keeps the URLs it already
had (`/`, `/map`, `/events`), so every live bookmark and federated link
survives with zero redirects. My Quilt is the marked variant: the `my`
suffix. There is deliberately **no `/all` token** — a whole-quilt URL
distinct from `/` would be the only place `/` could redirect *to*, and
we don't want that redirect (see the default below). All routes are
static, so `/events/my` outranks `/events/:id` on the router's existing
specificity rule, and a UUIDv7 event id is never the literal `my`.

**`/` is the whole quilt for everyone, always.** No auth-conditional
redirect anywhere. This is what makes the original bug *structurally*
impossible rather than merely fixed: with scope carried only by the URL
and no logic that silently reasserts it, there is nothing left to
misfire. The old "logged-in defaults to My Quilt" behavior is dropped in
favor of discovery-first — newcomer and regular alike open on the whole
community, one bookmarkable click from their own quilt.

**Personalization returns as an explicit, opt-in account setting, not
magic.** A server-side per-person preference ("start on My Quilt")
travels across devices. Its one safe semantic: it fires **once, at cold
load, only on a bare `/` entry, via `replaceRoute`** — so the back stack
starts at `/my` (a bare `/` was never pushed) and the preference never
re-fires when a person later navigates to `/` deliberately. Gating it to
mount, rather than "whenever the path is `/`", is what keeps the whole
quilt reachable and keeps back/forward operating on real URLs only. The
URL stays the source of truth; the preference only chooses the *starting*
URL.

**Flipping scope preserves the surface.** The switcher swaps the scope
segment in place — from `/map` to `/map/my`, from `/events/my` to
`/events` — landing on the scope root (`/` or `/my`) only when the
current page has no scope-able surface (a patch profile). Scope reads as
"same view, other quilt," not a "go home" button, which is the switch it
claims to be.

**Patch URLs are untouched.** `/patches/:slug` stays canonical (ADR
003). Scope roots and patch URLs are orthogonal; the redesign needs no
change there, and the patch URL is a federated, externally-linked,
inbox-resident address whose rename would cost far more than the
brevity would buy.

## Considered options

- **Query param (`/?scope=my`, `/map?scope=my`)**: rejected. Puts scope
  in the URL and fixes the bugs, but `/?scope=my` is a poor bookmark and
  the param would have to ride every surface — the suffix reads as a
  place, which is what a person wants to save.
- **Scope-first prefix (`/my`, `/my/map`, `/my/events`) with an `/all`
  mirror**: rejected. Reads Reddit-literal and `/my/events` phrases
  better than `/events/my`, but it forces an `/all` token, breaks the
  live `/map` and `/events` URLs (redirects), and needs `/` to redirect
  by auth to feel right — reintroducing exactly the stateful default the
  bug came from.
- **Keep the auth-conditional default (log-in lands on My Quilt) with a
  real whole-quilt URL**: rejected as the baseline. The opt-in account
  setting gets the same outcome for people who want it, without making
  the default a piece of logic that reasserts scope behind the URL's
  back.
- **Rename patches to `/p/:slug`**: rejected. Aesthetic gain against
  real, permanent, outward-facing cost (the AP `url` property,
  notification/email links already sent, a paste-follow regex that must
  accept `/patches/` from other quilts forever, 141 call sites, and
  superseding ADR 003). The full word is also more on-brand — "patch" is
  the core noun the URL teaches.

## Consequences

- The discovery route table grows from the bare surfaces to their `/my`
  variants (`/my`, `/map/my`, `/events/my`); the bare forms stay exactly
  as they were, so no existing link redirects.
- `changeScope`'s in-memory `$state` and the auth-conditional
  default-scope `$effect` in App.svelte are removed; scope is derived
  from the matched route instead. SocialHome, EventsPage, and the map
  keep reading a single `quiltScope` value — only its *source* moves from
  state to URL.
- The switcher's `selectScope` stops always navigating to `/`; it
  composes the current surface with the chosen scope.
- A migration adds the per-person "start on My Quilt" preference with a
  `GET/PATCH /users/me/*` endpoint and an Account Settings toggle; being
  account-backed, it belongs to the personal-export boundary in a
  seamrip (ADR 002/012), not the instance's data.
- CONTEXT.md is unchanged: "scope", "My Quilt", and "whole quilt" still
  mean what they meant — scope simply became addressable, which is
  implementation, not vocabulary.
