# Patchwork

**Be the Pattern.** Open-source, self-hostable community organizing
infrastructure. One small server, one SQLite file, a whole local scene.

Patchwork is a living map of the communities in your area. Every band,
venue, collective, co-op, and club is a **patch**. Patches fit together into
a **quilt**: a visualization of your scene where groups that share people
sit closer together. Real communities connect through the people who show up
to both, so that's what the layout reflects.

No algorithms, no ads, no engagement mechanics, no data harvesting.
Patchwork is run by your neighbors, for your neighbors. If that ever stops
being true, the community can take its data and walk — see
[Seamrip](#seamrip-export--import) below.

## What it does

- **The quilt.** A D3 treemap of every public patch, sized by activity and
  placed by member overlap. Zoom in, filter by tag, search, click through to
  any community. The same data also drives a Leaflet street map.
- **Patches.** Every group gets a profile: description, links, location,
  members, events, governance documents. Membership comes in three honest
  roles: **admin** runs it, **member** participates and votes, **follower**
  watches and gets notified. A band with three admins and two hundred
  followers fits. So does a consensus-run co-op.
- **Events.** Patches host events. People find them on the quilt, the
  map, and in their feeds.
- **Governance, when you want it.** Proposals run on a real state machine
  (discussion → voting → in effect), with threaded discussion, revisions,
  and vote tallies. Governance documents (*charters*) keep full version
  history and diffs, mirrored into per-patch **git repositories**. A band
  never has to touch any of it. A coalition can run itself entirely on it.
  Every new patch ships with the quilt's baseline lining — anti-discrimination
  included.
- **No-password auth.** Invite links (no email server needed), magic links
  (if you configure SMTP), and passkeys for returning users.
- **Seamrip.** The governance safety valve. Any admin can export the whole
  community as plain JSON: people, patches, memberships, events, proposals,
  votes, governance history. Anyone can import that into a fresh instance
  with every relationship intact. If leadership goes sideways, the community
  forks and keeps going. Power stays with the people in the quilt.
- **Multi-quilt and federation.** The web app can merge several Patchwork
  instances into one view. An optional ActivityPub layer (HTTP signatures,
  WebFinger, inbox/outbox) lets quilts talk to the fediverse.

## Why it exists

Local scenes organize on rented land: platform groups, chat apps, and
spreadsheets that can vanish, get throttled, or get sold overnight. Patchwork
bets the other way. It's community infrastructure you own, simple enough to
run on a Raspberry Pi and honest enough to walk away from. The governance
model mirrors how open source already works: followers become members become
admins, and the whole ladder is visible to everyone.

The reference instance maps the Lancaster, PA arts scene, but Patchwork is
white-label from day one. Any community can deploy its own quilt.

## Quickstart

Requirements: Go 1.25+, Node 20+ (to build the frontend), or just Docker.

```bash
# Clone, configure, run
git clone <this-repo> && cd patchwork
cp patchwork.yaml.example patchwork.yaml   # edit name, domain, geography

# Build the frontend once, then the single binary serves everything
cd web && npm ci && npm run build && cd ..
make seed    # optional: demo data — a fictional Lancaster arts scene
make run     # serves API + SPA on the configured port
```

### Demo data: try it as different people

`make seed` (and `make seed-force` to wipe and re-seed) load a dense
fictional dataset — patches across the governance spectrum (open
collectives, approval-required co-ops, invite-only bands), events,
proposals mid-vote, unclaimed venues awaiting claim. Every person and
organization in it is invented; only the geography is real
(docs/adr/009). It exists for local dev, the e2e suite, and evaluating
Patchwork — never seed a real instance (the seeder refuses if it sees a
database with real users).

The seed also creates long-lived dev sessions so you can experience the
instance from any rung of the contributor ladder. Paste one of these in
your browser console, then reload:

```js
document.cookie = "patchwork_session=dev-admin-token; path=/";     // instance admin
document.cookie = "patchwork_session=dev-organizer-token; path=/"; // patch admin
document.cookie = "patchwork_session=dev-active-token; path=/";    // active member
document.cookie = "patchwork_session=dev-lurker-token; path=/";    // follower
document.cookie = "patchwork_session=dev-new-token; path=/";       // brand-new account
```

Or with Docker Compose (includes Caddy for automatic HTTPS):

```bash
docker compose up -d
```

Everything lives in one SQLite file (`data/patchwork.db`) plus a `data/`
directory of governance git repos. Back those up and you've backed up the
community.

## Configuration

One file: `patchwork.yaml`. Instance identity, geography for the map, SMTP
(optional: invite links and passkeys work without it), feature-module hints,
community submissions, multi-quilt CORS, and the federation toggle. See
`patchwork.yaml.example` for the annotated version.

## Seamrip: export & import

```bash
make export                 # -> ./export/*.json
make import IN=./export/    # -> fresh DB, new IDs, same community
```

Admins can also download the archive from the dashboard
(`GET /api/v1/admin/export`). What travels and what deliberately doesn't
(credentials, keys, federation identity) is documented in
`docs/adr/002-seamrip-boundary.md`.

## Principles

- Single process, single binary, no external services. Runs on a Pi 4.
- SQLite with WAL. Cursor pagination. UUIDv7 IDs.
- Config over code. Communities customize via `patchwork.yaml`.
- Flat patches, no hierarchies. Connections are inferred, never drawn.
- Governance is a spectrum, not a type.
- Antifascist by design: the default lining includes an anti-discrimination
  baseline.
- The exit is part of the product (seamrip).

## Project status

Beta. Backend and frontend test suites are green (Go tests, 55 unit tests,
213 Playwright e2e tests). Design decisions are recorded in `docs/adr/`
(`docs/adr/README.md` is the index).

## Development

```bash
make dev        # Go backend + Vite dev server with hot reload
make test       # go test + vitest
make test-e2e   # Playwright suite (seeds a test DB)
```

Backend vocabulary is generic (nodes, events, proposals). The UI speaks
textile (patches, quilts, charters). The mapping lives in
`CONTEXT.md`; design decisions live in `docs/adr/`; `CLAUDE.md` orients
coding agents.

## How it's built

Patchwork has one maintainer, working heavily with Claude Code —
AI-assisted commits say so in their trailers, and every change is reviewed
before merge. The test suites and the ADR record in `docs/adr/` are what
keep that workflow honest, and they apply to human and AI-written code
alike.

## Contributing

Issues and pull requests are welcome — [CONTRIBUTING.md](CONTRIBUTING.md)
has the dev setup and conventions. The project follows a
[code of conduct](CODE_OF_CONDUCT.md). Security issues go through
[SECURITY.md](SECURITY.md), never the public tracker.

## License

Patchwork is free software under the [GNU Affero General Public License
v3.0](LICENSE) (AGPL-3.0). The license fits the project's ethos. Anyone can
self-host, study, and seamrip (fork) it, and anyone who runs a modified
Patchwork as a service has to offer their changes back to the communities
using it.
