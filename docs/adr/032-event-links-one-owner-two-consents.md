# Event links: one owner, two consents

A band and a venue are both patches, and a gig belongs to both — but
`events.node_id` is singular, so today the second patch either duplicates
the event or goes without. The fix must also survive events that arrive
from event sources (ADR 031), where no human authored the row.

The decisions:

- **One owner plus linked patches, not co-ownership.** Every event keeps
  its single owning patch; other local patches attach through a flat
  `event_links` row. Edit rights, review queues (ADR 026), and
  source-authority (ADR 031) stay with the owner — a linked patch gets
  presence, not control. Links carry no role labels ("performer",
  "sponsor"): flat links, like flat patches; nuance goes in the
  description. Co-ownership was considered and rejected: every
  permission check and queue would have to reason about a set, and a
  feed import could never legitimately mint co-owners. An explicit
  co-editing grant on a link is a possible future extension, not part
  of this design.
- **A link takes two consents.** Admins on either side may propose —
  the venue tags the band, or the band requests onto the venue's event —
  and admins on the *other* side confirm. Pending links are invisible:
  no calendar placement, no feeds, no affinity, just a notification.
  A link is an assertion about another patch's public face (ADR 030's
  claims logic), so it never lands without that patch's hand on it.
  One person adminning both sides confirms instantly.
- **The sync never touches links.** We considered auto-confirming links
  asserted by an attached source and rejected it: the vouch (ADR 031)
  is the *source-owning* patch's admins vouching, and feed content is
  third-party text — a sloppy or hostile feed could paint events onto
  another patch's calendar with no one on that side consenting. Even
  auto-*proposing* is out; imported events are ordinary linkable events,
  and humans initiate every link.
- **Duplicates merge only by human choice.** When both patches' sources
  carry the same real-world gig, two rows exist. The link-request flow
  offers to absorb the requester's duplicate (fuzzy match is only ever
  a suggestion): on confirmation the duplicate is deleted and, if
  imported, skip-listed (`event_source_skips`) so the reconciler never
  resurrects it. Unmerged duplicates may persist forever; that is the
  status quo and it is fine.
- **A confirmed link is full presence.** The event appears on the linked
  patch's calendar page, public `.ics`/`.rss` feeds, followers' My Quilt,
  and the map (one marker, both patches); it counts toward `event_count`
  and tile size (ADR 015 — a gig is earned activity); it feeds placement
  affinity at the shared-event tier (×2), eventually subsuming the
  `created_by` heuristic it proxies. Over ActivityPub the linked patch's
  actor sends `Announce` of the owner's event — never `Create`, the
  object stays attributed to the owner.
- **Either side severs unilaterally.** One tap, no confirmation, no
  cooldown — consent is continuous (ADR 012's instinct). Links ride the
  event row: owner deletion or reconciler cancellation cascades. Only
  `active` events are linkable; visibility follows the event with no
  separate knob on the link.
- **Cross-quilt is a mention, not a link.** Pasting a *remote* patch URL
  into the add-patch flow yields a display-only **cross-quilt mention**:
  a labeled doorway on the event page (ADR 024 — objects blend, places
  don't). No handshake — we don't do cross-instance consent — and
  therefore none of the surfaces above; it has the standing of naming
  the band in the description, owner-editable. A *local* patch URL
  pasted there routes into the real handshake, never a consent-free
  mention. Real cross-quilt links are deferred.
