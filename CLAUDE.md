# Patchwork

Open source, self-hostable community organizing platform. Go + SQLite + Svelte. "Be the Pattern."

## What This Is

Patchwork is a platform for grassroots communities to organize, govern, and discover each other. The reference implementation is for the Lancaster, PA arts scene, but it's white-label from day one — any community can seamrip (fork) and deploy their own.

## Tech Stack

- **Backend:** Go (net/http stdlib router), single binary
- **Database:** SQLite with WAL mode (see PRAGMA config below)
- **Frontend:** Svelte SPA, compiled to static assets, embedded in Go binary via embed.FS
- **Reverse proxy:** Caddy (auto-HTTPS via Let's Encrypt)
- **Deployment:** Docker Compose (single container + Caddy)
- **Auth:** Magic links (SMTP, optional) + invite links (no SMTP) + WebAuthn/passkeys (go-webauthn/webauthn)
- **Maps:** Leaflet (no API key)
- **Visualization:** D3.js treemap (quilt view — the signature visual)

## Project Structure

```
patchwork/
├── cmd/
│   ├── patchwork/          # main.go entry point
│   ├── seed/               # seed data generator
│   ├── export/             # data export CLI (seamrip)
│   └── import/             # data import CLI (seamrip)
├── internal/
│   ├── config/             # patchwork.yaml loading
│   ├── database/           # SQLite connection, migrations, PRAGMA init
│   ├── auth/               # magic links, invite links, WebAuthn, sessions
│   ├── handler/            # HTTP handlers (nodes, events, proposals, admin, tree,
│   │                       #   governance hub, claims/unclaimed, notifications, AP inbox,
│   │                       #   user profiles)
│   ├── middleware/          # auth, rate limiting, CSRF, CORS, logging
│   ├── model/              # Go structs for all entities
│   ├── ap/                 # ActivityPub: actors, HTTP signatures, keypairs, delivery worker
│   ├── eventsource/        # event sources: ICS fetch/parse/expand, sync reconciler, worker (docs/adr/031)
│   ├── safehttp/           # SSRF-guarded HTTP client shared by ap and eventsource
│   ├── governance/         # git-backed charter repos, templates, rules, defaults
│   ├── notifications/      # notification channels, email, reminder worker
│   ├── weblink/            # the SPA paths Go emits (notification/email/feed links)
│   └── seamrip/            # export/import portability boundary (docs/adr/002)
├── migrations/             # SQL migration files (sequential numbered; 006 intentionally absent)
├── docs/                   # DEPLOYMENT.md, adr/ (decision records; adr/README.md is the index)
├── web/                    # Svelte project (npm, builds to web/dist/)
├── CONTEXT.md              # canonical vocabulary glossary (backend vs UI terms)
├── docker-compose.yaml
├── Dockerfile
├── Caddyfile
├── Makefile
├── patchwork.yaml.example  # example config
└── CLAUDE.md
```

### Claiming a number (ADRs and migrations)

Both `docs/adr/` and `migrations/` are sequentially numbered, and both are
claimed by branches that can't see each other. Two branches each reading
"the highest number on disk" will pick the same next one and both be right
locally. Migrations collide loudly at merge; ADRs collide *silently* —
duplicate numbers merge clean and leave every `docs/adr/0NN` citation
ambiguous. Two ADR 017s reached main this way.

Before claiming a number, check what's in flight, not just what's on disk:

```sh
git ls-tree --name-only origin/main docs/adr/   # or migrations/
gh pr list --state open                          # branches that haven't merged yet
git branch --list                                # local branches, incl. other worktrees'
```

Local branches matter as much as PRs: an unpushed worktree branch claimed
ADR 038 invisibly to a session that checked only origin/main and open PRs.

**That check has a shelf life.** It tells you the number was free when you
claimed it, not that it still is. ADR 063 was claimed against a clean
board — main topped out at 062, no open PRs, no other branches — and was
wrong three hours later when another PR merged its own 063 while the first
sat in review. **Re-check before merge, not only before claiming**, and
re-check again after any merge of main into a long-lived branch.

When running parallel agents or worktrees on one repo, **assign each its
number up front** rather than letting each pick. If a collision does land,
renumber the side with fewer inbound references — or, when one side has
already merged, the unmerged one — and update every citation in the same
commit: `git mv` so history survives, then sweep `*.md`, `*.go`,
`*.svelte`, `*.js`, `*.sql` for the old number.

Sweeping is where this bites a second time. Once both ADRs exist in one
tree, **a blanket find-and-replace is wrong**: files legitimately cite the
other side's number too. Renumbering 063 → 064 meant 17 citations moved
and two files kept theirs, because `internal/eventsource/sync.go` and
`PatchSettingsSources.svelte` each cited *both* ADRs after the merge.
Anchor those edits on the surrounding sentence, not on the number.

Numbers are never reused once merged: `migrations/006` is intentionally
absent, and a retired ADR keeps its number and gets a status line.

**The same collision has a code form, and git is equally blind to it.**
Two branches each added a `jsonString` helper — one in
`internal/handler/programs.go`, one in `internal/handler/event_sources.go`
— so the merge was clean and the package stopped compiling. A green PR
does not stay green just because main merged without conflicts: **build
and run the suites after merging main**, before treating a quiet merge as
a working one. Shared helper names, new columns on a table two branches
both touched, and a route registered twice all fail this way.

### Verifying frontend changes

There is no Svelte render library in this project — every frontend test asserts against **source text**, so the suite cannot catch a rendering bug. Run the app and read the page for anything user-visible. In one session four of five browser checks found real defects the green suite had missed, including a page reciting a voting mechanic its patch does not run.

Two gotchas that recurred: Svelte trims whitespace at the head of an `{#if}` body, so `{formatDay(d)}{#if x} · more{/if}` renders as "2026· more" — use `{' · '}` as an expression. And an endpoint whose payload a form spreads back into a submission must send **every** field, or the ones it drops get silently reset.

## Data Model

### Core Principle: Flat Patches, Inferred Connections

All communities are **patches** — flat, equal entities. A band and a venue are both patches. There is no hierarchy (no parent/child nodes). Connections between patches are **inferred from shared members**, not manually created. Patches with more overlapping members are considered more strongly connected and are placed closer together in the quilt visualization.

### Patches (nodes table)

Every community, collective, band, venue, or group is a patch. Patches differ not by "type" but by **governance complexity**:

- A band: invite-only, all members are admins, no proposals or governance docs needed
- A venue: open membership, has organizers and members, uses proposals for decisions
- A coalition: approval-required, full governance with charters and proposals

The governance features (proposals, governance docs) are available to all patches but optional. The `membership_policy` field drives access control.

Governance docs (charters) carry their own per-document visibility — `members` (the default for a new doc) or `public` (docs/adr/036). Publishing is a deliberate act at Governance → Documents, not the state a doc is born in. Only public docs federate, and amendment proposals withhold the mirrored charter text (`proposed_body`, `current_doc_content`) from viewers who can't read the target charter. `nodes.visibility` is a separate, patch-level thing: private keeps a patch off the quilt, search, the map, public feeds, and federation, but a direct link still opens its page.

The **lining** is the exception to all of the above (docs/adr/037): `governance_docs.kind='lining'`, identified by column not title, project-owned (text ships in the binary — `internal/governance/lining.go` holds the versioned lineage; no instance override, fork the repo instead). Pinned public, title immutable, undeletable, body changed only by amendment proposal. A patch whose lining matches no shipped version is **diverged** — it wears a public "Amended lining" badge and can be filtered out of discovery by per-user setting (`users.hide_amended_linings`) or instance policy (`hide_amended_linings` in instance_settings; strictest wins). Stale linings (older shipped versions) auto-update at startup with member notification. Updating the lining's text = appending to the lineage in `lining.go` — a release, not a migration.

### Membership Roles

Three relationships a person can have with a patch:

| Role | What it means | Can do |
|------|--------------|--------|
| **Admin** | Runs the patch | Edit profile, manage members, create events, proposals |
| **Member** | Active participant | Vote on proposals, participate, listed publicly |
| **Follower** | Interested observer | Sees events in feed, gets notified, no voting rights |

Following is frictionless: anyone can follow any public patch regardless of membership policy.

On an `elected` patch, admin is additionally seated: a **seat** holds the role for a term. See "Governance: Leadership, Succession, and Venue" below.

### Governance: Leadership, Succession, and Venue (docs/adr/051, 052, 053)

**How admins are made** follows `leadership_model` — three models, three mechanics, and Patchwork runs exactly the one the patch names:

| Model | Seats | Terms | How an admin is made |
|---|---|---|---|
| `maintainer` | no | no | designates a successor (`nodes.designated_successor_id`) |
| `meritocratic` | no | no | admins nominate (`proposals.target_user_id`), community ratifies |
| `elected` | yes | yes | calendared election, approval voting over a slate |

A **seat** (`seats` table) is a governed admin position that outlives its holder; `seats.term_ends_at` is the patch's election calendar. Dueness is *derived* from it, never stored — one fact in one place, and what makes staggering a policy rather than machinery. An election is a proposal carrying candidates (`seats_contested > 0`), the one proposal not born voting: it takes nominations first, then votes. **Holdover** is the rule for every way an election can settle nothing — no candidates, quorum unmet, nobody approved — the sitting council stays. No clock ever removes anybody.

**Where decisions happen** is declared per patch and asked twice: `leadership_venue` and `proposal_venue`, each `patchwork` (default) or `elsewhere`. Elsewhere, Patchwork conducts nothing and records an **attestation** instead — a community's statement that it decided this at a venue Patchwork was not. Two kinds: leadership (`attestations` + `attestation_names`) and amendment (`amendment_attestations`). The record may name anyone; the effect lands only on members.

`proposal_venue: elsewhere` removes the ballot and keeps the discussion — a proposal is born `state = 'elsewhere'` with `voting_ends_at` NULL, so nothing resolves it on a clock. The two halves only work together: a patch with both a tally and an attestation would let an admin who disliked where a tally was heading record a meeting result instead.

**Never attestable:** the lining (docs/adr/037 — checked by `kind`, not by title) and `governance-rules.json`, which is machine configuration rather than a text anyone adopts. Excluding the rules file closes a two-step route around the leadership gate. On an elsewhere patch a rules change is a **direct change** an admin applies.

### Inferred Threads & Placement Affinity

A **thread** — the user-facing connection concept — is inferred from shared admin/member overlap only; followers don't create threads. **Placement affinity**, the internal weight table the quilt layout runs on (`internal/handler/tree.go`), is broader: shared admins/members ×3, shared event participation ×2, shared followers ×1, plus a weak shared-tag term (mass-scaled toward the larger patch's member count, capped below one shared member) so brand-new patches with no people-overlap still land near their kind. Tag attraction is a placement detail, never a thread. The tree endpoint returns these links and the frontend layout engine places strongly connected patches adjacently in the quilt treemap.

### User Profiles & Membership Visibility (ADR 006)

People have public profile pages at `/users/{username}` — name, avatar, bio, and visible memberships with role chips (the contributor ladder made legible). Follows never appear on profiles. Each membership has a per-membership visibility switch owned by the member: one switch controls both whether the membership shows on the profile and whether the person shows in the patch's public member list — the two surfaces never disagree. Hidden memberships remain visible to that patch's admins/members inside the workspace. Memberships never federate (AP actors carry identity only). There is deliberately no instance-wide people search — people are discovered through patches.

### Patch Appearance (ADR 004, ADR 029) and how a tile is drawn (ADR 066)

Patch admins choose their tile's look on the quilt: a **block** — curated (named quilt pattern — Pinwheel, Ohio Star, Log Cabin…) or **drafted** in the block drafter (grid 1×1–10×10, ≤24 seams between wall anchors, pieces colored by bundle slot — docs/adr/029) — plus a rotation and a **bundle** of 1–6 fabrics off the curated fabric wall (`web/src/lib/fabricWall.js`; the classic palettes are pre-cut bundles). All stored as JSON in `nodes.appearance` (migration 018 replaced the old `theme` column); a drafted block is inline data, not an entity. Unset appearance = hash-assigned from the patch ID (stable but not chosen). Bundle slot 0 (= palette primary) is the patch's identity color on cards and labels. Edited at Patch Settings → Appearance. Moderation: the `reset_appearance` report action nulls a patch's appearance — the quilt decides again.

**What a patch chooses is separate from how the quilt draws it** (docs/adr/066). The line between two tiles is a **tile seam** (plain "seam" is the block drafter's, sewn *within* a block — CONTEXT.md disambiguates), and it belongs to the boundary, not to either tile: the wobble comes from a lattice both neighbours read (`web/src/lib/quiltCloth.js`), so tiles meet exactly and the seam is ink stroked on top in *screen* pixels rather than a gap left underneath in world units. Blocks stretch onto that lattice instead of being drawn square and clipped — clipping left the fabric short of 23% of the tile boundary. Bevel, puff and raking light are strokes along the seam, not per-tile layers; folds bend the lattice and are shaded from the same height field. Two rules worth keeping: **intensity may vary per tile, direction never may** (the old per-tile shadow layer randomized its light *angle*, which cancels the illusion of a surface), and geometry is batched **by paint within a spatial chunk** — merging the whole quilt into one path per colour cut nodes 84× and moved the frame time not at all, because a path is only culled when its bounding box misses the viewport.

## Naming Convention

Backend uses generic terms. UI uses textile vocabulary:

| Backend | UI (Textile) |
|---------|-------------|
| Node | Patch |
| Event | Event ("pin" retired — docs/adr/027) |
| Instance | Patchwork |
| Proposal | Proposal ("baste request" retired) |
| Governance Doc | Charter ("lining" names only the shared baseline charter) |
| Fork | Seamrip |
| Inferred connection | Thread |

All code, variable names, database columns, and API endpoints use the backend terms. The Svelte frontend handles the mapping to textile vocabulary. The full canonical glossary — including shell/navigation terms (global bar, context crumb, workspace, scoped finder) and appearance terms (tile, palette, block, identity color) — lives in `CONTEXT.md`; when in doubt about a word, that file wins.

**Removed concepts:** Stitches (child nodes), explicit Edges (manual connections), Moderator role, "Pin" as the UI word for events (docs/adr/027 — it collided with map pins and pinned posts; the UI says "event"), "Baste request" as the UI word for proposals and "Lining" as the generic word for governance docs (the UI says "proposal" and "charter"; "the lining" survives only as the name of the shared baseline charter). See "Design Rationale" below.

## Database

SQLite. Every connection must set these PRAGMAs on open:

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA cache_size = -64000;
PRAGMA wal_autocheckpoint = 1000;
```

On startup (before opening the pool): `PRAGMA integrity_check;` and `PRAGMA wal_checkpoint(TRUNCATE);`

All IDs are UUIDv7 (time-sortable). All timestamps are ISO 8601 TEXT. All list endpoints use cursor-based pagination (not offset).

## Auth Model

Three auth paths, zero passwords:

1. **Invite link** (primary for grassroots): admin generates a single-use URL, shares out-of-band (Signal, flyer, etc). User clicks, creates account, enrolls passkey. No SMTP needed.
2. **Magic link** (requires SMTP): user enters email, receives link, clicks to auth.
3. **Passkey** (returning users): WebAuthn ceremony, no network dependency.

SMTP is optional. Without it, invite links + passkeys still work (magic links print to the server log for local dev). Patchwork warns in the dashboard but doesn't refuse to start.

Bootstrap: the first account created on a fresh instance automatically becomes the instance admin (`internal/auth/bootstrap.go`).

## API

REST at `/api/v1/*`. JSON bodies. Auth via HTTP-only Secure SameSite=Lax session cookie. All mutations require auth. Public reads are unauthenticated.

Key endpoints:
- `GET /api/v1/nodes/tree` — flat list of patches with member/event counts, sorted by member-affinity for treemap rendering
- `GET /api/v1/nodes/{slug}` — single patch detail
- `POST /api/v1/nodes/{slug}/join` — join or follow a patch (body: `{"role": "follower"}` for follow)
- `GET /api/v1/users/{username}` — public user profile (visible memberships only)
- `PATCH /api/v1/users/me/memberships/{nodeId}` — flip a membership's visibility switch
- `GET /api/v1/events` — **omitting `from` means upcoming.** The list sorts `starts_at` ascending, so an unbounded list would be a patch's *oldest* events; three surfaces headed "upcoming events" shipped that bug at once. Pass `include_past=true` for the whole calendar (workspace calendar, scoped search, "any events yet" probes). An explicit `from` always wins, including one in the past. Date-only `from`/`to` are widened to the instants they mean, in UTC (docs/adr/045)
- `POST /api/v1/events` — members/admins post directly; anyone else submits for review (`status: pending_review`) per docs/adr/026; trusted contributors (users flag) post directly to unclaimed patches
- `PATCH /api/v1/events/{id}/review` — approve/reject an event submission (instance admin for unclaimed patches, patch admins for active)
- `GET /api/v1/admin/event-submissions`, `GET /api/v1/nodes/{slug}/event-submissions` — the two review queues
- `POST /api/v1/nodes/{slug}/claim`, `GET /api/v1/nodes/{slug}/claims/mine`, `POST /api/v1/claims/{id}/{verify|withdraw|resend-email}`, `GET|POST /api/v1/claims/verify-email` — claiming an unclaimed patch (docs/adr/030): concurrent claims (one open per user per patch), self-verification (DNS / meta tag / email) anchored on the vetted `nodes.verification_domain`, admin review via `GET/PATCH /api/v1/admin/claims`
- `GET|POST /api/v1/nodes/{slug}/event-sources`, `DELETE|POST .../{id}[/sync]`, `POST /api/v1/events/{id}/detach` — event sources (docs/adr/031): owner-attached calendar feeds (ICS; Squarespace and schema.org-marked events pages auto-detected), synced hourly; imported events publish directly, are read-only until detached
- `POST /api/v1/nodes/{slug}/events/bulk` — event upload (CSV door): admin-only batch create, all-or-nothing validation, title+start dedup, silent (no notify/AP burst)
- `GET /api/v1/nodes/{slug}/events.{ics|rss}` — public subscribable feeds per patch; `GET /api/v1/feeds/{secret}/events.ics` + `GET|POST|DELETE /api/v1/users/me/feed-secret` — the personal My Quilt calendar behind a regenerable URL secret
- `GET /api/v1/admin/export` — zip download of all instance data (admin only)
- `PUT /api/v1/admin/users/{id}/email` — set a user's email address (docs/adr/072): the in-product repair for an account with no way back in, and for a typo'd address. Its own route rather than a field on `PATCH /api/v1/admin/users/{id}`, because pointing an account at a mailbox lets whoever holds that mailbox magic-link in — so it takes the step-up gate (docs/adr/017) that promotion takes. Normalized through `auth.NormalizeEmail` (sign-in is an exact `WHERE email = ?` match), refuses an address another account holds, audited as `admin.user_email_set`, and announced to the old address as well as the new one
- `GET /api/v1/instance/icon` — the public quilt icon, rendered to SVG from the drafted design (docs/adr/043); an instance that has drafted none wears a starter block assigned from its name
- `GET|PATCH /api/v1/admin/settings`, `POST /api/v1/admin/wipe` — quilt settings: rename/description overrides, the icon design (`icon_design`: a drafted block plus its fabrics; `null` clears it), danger-zone wipe (docs/adr/014, docs/adr/043)
- `GET /api/v1/legal/{privacy|terms}` — public legal documents: shipped defaults or admin overrides, rendered at /privacy and /terms; admin editing via `GET /api/v1/admin/legal` + `PUT|DELETE /api/v1/admin/legal/{doc}` (docs/adr/028)
- `PUT|DELETE /api/v1/nodes/{slug}/successor` — maintainer succession (docs/adr/051); step-up gated
- `POST /api/v1/proposals/{id}/candidates`, `PUT /api/v1/proposals/{id}/ballot` — elections: standing is a member act, the ballot is a PUT of the whole approved set (approval voting replaces wholesale)
- `GET|POST /api/v1/nodes/{slug}/attestations`, `PATCH /api/v1/nodes/{slug}/attestation-names/{id}` — leadership decided elsewhere (docs/adr/052); public read, step-up write
- `GET|POST /api/v1/nodes/{slug}/amendment-attestations` — texts a meeting adopted (docs/adr/053); replaces the whole charter, checks no base
- `GET /api/v1/nodes/{slug}/governance/rules` — **sends the whole rule set, and must.** The editor builds its submission by spreading what it loaded, so a field this drops is a field the next unrelated rules edit resets to its default

## Multi-Quilt / Cross-Quilt Following (docs/adr/024)

**Objects blend, places don't.** Another quilt as a *place* is always
visited at its own address: every switcher entry for another quilt is a
doorway (new tab, their site) — no surface ever renders another quilt's
whole view inside this one's URL. What blends in-app is *objects*: **My
Quilt** merges relationships — patches you follow on other quilts render
grouped by source quilt in **sashing**-framed regions (each quilt's own
branding color), alongside a merged event feed and map markers with source
chips — and a single remote patch renders as a read-only **remote patch
card** at `/quilts/{host}/patches/{slug}` (reached from My Quilt tiles,
notifications, and pasted patch links — the discovery search and
Connected Quilts settings recognize a pasted `https://host/patches/slug`
URL; this is the follow path, since browsing happens on their soil where
Follow can't exist).

- **Two connection tiers:** instance admins curate public **neighbor
  quilts** (`neighbor_quilts` table, admin panel → Neighbors, exposed on
  `GET /api/v1/instance` for every visitor, anonymous included); signed-in
  users add personal **connected quilts** (`user_quilts`,
  `/api/v1/users/me/quilts`, Settings → Connected Quilts).
- **Remote follows** live on the follower's home instance
  (`remote_follows`, `/api/v1/users/me/remote-follows`) with a display
  snapshot so tiles render even when the remote quilt is unreachable.
  Never auto-deleted — deletion and going-private are indistinguishable
  from outside. When federation is enabled on both ends, the **instance
  service actor** (an Application actor, `/ap/instance`) relays ONE AP
  Follow per remote patch — never the person's own actor, so nobody is
  enumerable in a remote followers collection. Event creation broadcasts
  `Create` to AP followers; the instance inbox turns inbound `Create`
  into `remote.event` notifications.
- **Doorways:** a quilt with `multi_quilt: false` declines cross-quilt
  *reads* (blending); it can still be neighbored/connected — its My Quilt
  region renders from snapshots and its patches' cards fall back to pure
  doorways. Never proxied around. On any remote patch card, joining/RSVPs
  are doorways to the remote site.
- **Registry-driven:** SPA accepts `?registry=<url>` — a session-only
  overlay on the switcher, never persisted (a discovery flyer).
- **CORS:** public GET endpoints return `Access-Control-Allow-Origin: *`
  when `multi_quilt: true`. Mutations are never cross-origin (the follow
  mutation posts to the person's home instance).

**Cross-quilt browsing is public-only.** Private nodes don't appear.

Registry JSON format:
```json
{
  "name": "Lancaster Community",
  "quilts": [
    { "url": "https://arts.lancaster.example", "tags": ["arts"] },
    { "url": "https://discgolf.lancaster.example", "tags": ["sports"] }
  ]
}
```

## ActivityPub (implemented)

Federation is live at the protocol level: actor documents, outboxes,
followers collections, WebFinger (`/.well-known/webfinger`), an inbox that
handles `Follow`/`Undo(Follow)`, HTTP-signature signing on outbound
deliveries and verification on inbound ones (with Date-skew replay window
and remote key caching), and a retrying delivery worker — all in
`internal/ap` and mounted in `cmd/patchwork/main.go`. Keypairs and `ap_id`s
are backfilled on startup, and stale-domain `ap_id`s are healed to the
configured domain. `federation.enabled` in patchwork.yaml gates the AP,
WebFinger, and governance git-transport mounts plus the delivery worker.
Live Mastodon interop (webfinger resolve, signed Follow/Accept both ways)
was verified 2026-07-13.

## Data Export / Seamrip

- `make export` — exports instance data to `./export/` as JSON files
- `make import IN=./export/` — imports into a fresh database with new UUIDs
- `GET /api/v1/admin/export` — admin endpoint returning a zip download

The seamrip mechanism is a governance safety valve: if a community's leadership goes sideways, members can fork the data to a new instance. The portability boundary (what travels, what stays) is defined once in `internal/seamrip` and documented in docs/adr/002 — memberships travel, so the fork keeps its inferred threads; keys, sessions, and AP identity do not.

**Adding a table means deciding whether it travels.** `TestEveryTableHasABoundaryDecision` requires every table in the schema to be either in `Tables()` or in an explicit stays-behind list with a reason — a new table fails the build until someone chooses. This exists because migration 050's `seats` silently didn't travel, and since election dueness is derived from `seats.term_ends_at`, a forked elected patch stopped holding elections forever.

## Key Principles

- Single-process. No Redis, no queues, no workers.
- No external service dependencies. Runs fully self-contained.
- Config over code. Community customization via patchwork.yaml.
- Antifascist by design. Default lining includes anti-discrimination baseline.
- The Go binary must run on a Raspberry Pi 4 with 2GB RAM.
- Distroless Docker image. Non-root container. SQLite file perms 600.
- Governance mirrors open source: the contributor ladder (follower → member → admin) is a core pattern.

## Design Rationale

### Why flat patches (no hierarchy)

The original model had "container" nodes (patches) with "leaf" children (stitches). In practice, community organizers don't think in nested hierarchies. A band and a venue are equally valid community entities — one shouldn't be subordinate to the other. Flat patches are honest about how real communities organize: as a network of peers, not a tree.

### Why inferred connections (no explicit edges)

Nobody will manually create a "thread" between two patches — that's admin busywork. But if 8 people are members of both Gallery Row and The Selvage, that IS a real connection, and it happened organically. The quilt visualization places patches with strong member overlap closer together, so spatial proximity communicates connection without needing a separate graph view.

### Why follower role (replacing moderator)

The admin/member/follower model maps directly to proven open source governance: maintainer/contributor/user. A band has 200 followers (fans) and 3 admins (bandmates). A venue has 500 followers and 15 active members. The moderator role was just an admin with fewer permissions — not a distinct relationship. Follower is a distinct relationship: interested observer with no voting rights but notification access.

### Why succession follows the leadership model

A patch's charter already tells members how admins are made — the three `leadership_model` values each promise a different mechanic. Before docs/adr/051 all three promised and none delivered. One universal mechanic was considered and rejected: an annual meeting suits a co-op and not a band, and picking one would have made two of the three descriptions lies. The first draft invented ad-hoc election-calling with an anti-griefing nomination filter; it was thrown out as over-clever. Small community governance is a solved problem — calendared elections, holdover, and a designated successor are the boring answers, and they are the ones shipped.

### Why governance is a spectrum, not a type

Instead of labeling patches as "community" vs "collective", the governance features (proposals, governance docs) are available to all but optional. A band just doesn't use them. This is driven by the existing membership_policy and whether governance docs/proposals exist — no new type field needed.
