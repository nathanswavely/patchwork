# ADR 024: Cross-quilt following — switcher to browse, home-side follows, federation as the upgrade

Date: 2026-07-20. Status: **accepted**. This turns the multi-quilt stub
(Connected Quilts settings, `?registry=`, CORS, the never-imported
`multiQuilt.js`) into a real feature. Numbering note: originally claimed
ADR 023 and migration 027, but the Label branch merged to main claiming
both first — this side renumbered to ADR 024 / migration 028 at merge
time, per CLAUDE.md's collision rule (the unmerged side moves).

## Context

Multi-quilt shipped as plumbing without a payoff: you could follow a quilt
in settings and nothing anywhere changed. The original sketch ("query all
instances, merge the view") hides a hard fact: the quilt treemap's layout
*is* member-overlap affinity, computed per instance — and affinity cannot
cross instances in a public-only model. A merged treemap of whole quilts
would be a lie about connection, the one thing the quilt is honest about.

Meanwhile the repo already contains the real thing multi-quilt was
reaching for: a working ActivityPub layer (actors, signed delivery,
Follow/Undo handling, a retrying delivery worker, Mastodon-verified).

## Decisions

**Objects blend, places don't.** (Revised 2026-07-20, same day, after
using the first build.) The original decision here was an in-app quilt
switcher: swap the whole discovery surface to another quilt's public
data, one quilt at a time. Built, it felt wrong in a way the design
review missed — **URL dishonesty**. Our address showing their quilt is a
lie: the shell keeps home's theme while wearing their name, the crumb
confuses, a copied link deep-links into a nested identity. So the line
moved: a whole quilt is a *place*, and places are visited at their own
address — every switcher entry for another quilt is a doorway (new tab,
their site), unconditionally. What still renders in-app is *objects*: a
single remote patch you have a relationship with (or are deciding to)
appears as a **remote patch card** — a card about their patch framed in
its quilt's sashing color, where Follow lives, with doorways for
everything deeper. The same line Mastodon draws around remote profiles.

Since browsing happens on their soil where Follow cannot exist (they
don't know you), the follow path is **paste-a-link**: the home instance
recognizes a pasted patch URL (discovery search, and an explicit
affordance in Connected Quilts settings) and opens its remote patch
card. A Mastodon-style remote-follow interstitial on the source site is
future work — it requires both instances upgraded; paste-a-link works
against any version.

No view ever mingles two quilts' patches on affinity terms. The only
multi-quilt canvas is My Quilt, and it merges *relationships*, not
quilts: remote patches you follow, grouped by source quilt in
sashing-framed regions (CONTEXT.md: sashing). Quilts are peers — once
two regions exist, every region gets sashing, home included.

**A remote follow is a row at home that upgrades to a federated Follow.**
Considered: localStorage (doesn't survive a device, silently strands
relationships), pure AP (hard-requires `federation.enabled` on both ends,
so following would mysteriously fail against half the fediverse of
quilts), home-DB only (rebuilds a follow mechanism while a real one sits
unused in `internal/ap`). Chosen: the follower's home instance stores the
follow (syncs across devices, renders offline from a snapshot) and, when
both instances federate, upgrades it over ActivityPub. Federation is a
progressive enhancement, never a requirement. The row is the truth the UI
renders; the AP state is the truth the network shares.

**The instance relays the Follow; the person never does.** The obvious
mechanism — the person's own actor sends the Follow — was rejected after
noticing it breaks a documented promise: follows are private ("only the
person sees their own follows", CONTEXT.md), yet AP followers collections
publicly enumerate follower actor IDs (`internal/handler/ap.go` serves
ours that way, and remote quilts run the same code). A person quietly
following a sensitive patch elsewhere would become publicly listable
there. Instead each quilt gets one service actor (the Mastodon
instance-actor pattern): it sends one Follow when the first local person
follows a remote patch and one Undo(Follow) when the last unfollows.
Events arrive once; the home instance fans out notifications privately
from its own rows. Cost, accepted: a remote patch's federated follower
count tallies quilts, not people.

**The notification loop ships whole.** Event creation now broadcasts
`Create` to a patch's AP followers (governance docs and proposals already
did; events — the thing followers most want — did not), and the inbox
learns to turn inbound `Create` from a followed remote patch into a
notification via `internal/notifications`. Without both halves the hybrid
is just a bookmark table.

**Connecting quilts has two tiers.** Neighbor quilts are instance-level:
the admin curates them in Quilt settings, and every visitor — anonymous
included — sees them in the switcher. A community declaring its own
adjacency is a different act from a person following a hometown quilt
their community doesn't care about; personal connected quilts layer on
top, account-backed. `?registry=` survives as a session-only overlay — a
discovery flyer, not a data source. localStorage retires from this job.

**Only the person ends a follow.** From outside, a deleted patch and a
patch that went private are indistinguishable — both vanish from public
data. Auto-pruning would erase a real relationship because someone
flipped a privacy switch. A missing patch renders from its stored
snapshot, marked, with unfollow offered. Likewise, disconnecting a quilt
never cascades to its follows: connection is a browsing convenience,
follows are relationships, and My Quilt regions are driven by follows
alone.

## Consequences

- Migration 028: remote follows (keyed by the remote patch's `ap_id`,
  with domain, slug, and a display snapshot refreshed on successful
  fetches), personal connected quilts, and instance neighbor quilts.
- Migration 028 also adds the instance service actor (keypair, actor
  document); unfollow emits `Undo(Follow)` only when the last local
  follower of that remote patch leaves.
- Event *updates and cancellations* should eventually broadcast too, or
  federated followers hold stale events; acceptable gap at ship, worth
  closing soon.
- Remote patches are read-only in-app: public detail renders locally
  (framed in the source quilt's sashing), Follow posts home, and
  everything deeper is a labeled doorway to the patch's own instance.
  Mutations remain never-cross-origin.
- The notification payoff requires the *remote* instance to run a version
  with event broadcasting. Against older quilts, follows are silent watches
  — by design, not by failure.
- `multi_quilt: false` on a peer is treated as consent withheld — not
  from being pointed at (every quilt is entered through a doorway now),
  but from *blending*: its My Quilt region draws from stored snapshots
  only, and its patches' remote cards fall back to pure doorways.
  Server-side proxying of a CORS-declining peer was considered and
  rejected — CORS-off is the only "don't embed my community elsewhere"
  signal an instance has, and we honor it.
