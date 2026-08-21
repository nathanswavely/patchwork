# ADR 059: The handle is a subscribe option

Date: 2026-08-21. Status: **proposed**. Builds on ADR 042 (the patch
profile's overflow), ADR 049 (state only what you enforce), ADR 006
(people are not the product), ADR 023 (the Label). Companion to ADR 058,
which drew the same line for a protocol we do not run; this one draws it
for the protocol we do. Carries a consequence from ADR 060.

## Context

Federation is finished at the protocol level and absent from the product.
`internal/ap/convert.go` sets `PreferredUsername: node.Slug` on an
`Organization` actor with an inbox, an outbox, a followers collection, and
a WebFinger record. The Selvage has been reachable at
`@the-selvage@arts.lancaster.example` since the AP layer shipped.

No surface has ever said so. The SPA contains the word "federation" twice,
both incidental: a seamrip note in `AdminQuiltSettings.svelte:360` and a
visibility description in `PatchSettingsInfo.svelte:180`. Neither tells
anybody that a patch has an address. `federation.enabled` defaults to
`false`, so on most instances the address is not merely unspoken but
unreachable.

This is ADR 049's hole in its purest form — a capability enforced and
never stated — and it costs something specific. The fediverse's onboarding
asks a newcomer to choose a server before they know what for, then hands
them an empty timeline. Patchwork's people arrive for a local reason and
their feed is full on the first day, because it is their own scene.
Federation here is a property of a thing they already made rather than a
thing they must first understand. That advantage is worth nothing while
the handle is unspeakable.

## Decision

**The handle is a third row in Subscribe. That is the whole feature.**

The slot already exists and was cut for this exact reason. ADR 042 put
Subscribe in the patch profile's overflow because the ICS and RSS feeds
"were reachable only from inside the workspace's Events tab, hiding a
public feature from the public page." `PatchOverflow.svelte` renders them
as two labelled rows with a copy button each. The handle is the same class
of fact, hidden the same way, one layer further down.

- A row reading **Mastodon** / `@slug@domain` / Copy, beside Calendar
  (ICS) and RSS.
- The same row in `PatchEvents.svelte`'s Subscribe panel, which already
  carries the same pair.
- **No Federation tab, no settings screen, no toggle.** ADR 042 reserved
  the overflow for acts that are "real but rare." This is one, and the
  relationship row stays untouched.

**The word is "Mastodon", and never "fediverse."** One names an
application a person may have heard of; the other names a category they
are not trying to join. The copy says what the row does — follow this
patch's events from Mastodon — and explains no protocol. A person who
knows what a fediverse handle is will recognize one on sight; a person who
does not is being offered a third way to subscribe to a calendar, which is
what this actually is.

**Both gates are already computed.**

- `node.visibility === 'public'`. `PatchEvents.svelte:162` already derives
  `feedAvailable` exactly this way, and it is the same line federation
  draws — a private patch does not federate.
- `instance.federation`. `GET /api/v1/instance` already returns it
  (`internal/handler/instance.go:23` and `:115`).

So the row itself is **frontend-only**: no endpoint, no column, no
migration. The disclosure below is the one server-side line this needs.

**Patches only. Person handles stay unspoken.** WebFinger resolves users
before nodes (`internal/handler/webfinger.go`), so people are already
addressable and already share a single namespace with patch slugs — a user
`nathan` and a patch slugged `nathan` collide, and the person wins.
Publishing person handles would put that collision in front of people and
would start federating individuals in a product that decided memberships
never do (ADR 006). Patches are the gateway; people are not the product.
The collision is neither created nor repaired here; it is left alone
deliberately, and noted so the next person finds it.

## The thing that must be settled before the address is handed out

`APNodeInbox` (`internal/handler/inbox.go:231–241`) dispatches `Follow`
and `Undo`. Its `default` branch logs `unhandled activity type` and
answers **`202 Accepted`**. A person who replies to an event announcement
from Mastodon is told, affirmatively, that their reply was accepted. It is
discarded.

That is survivable while nobody knows the address. It is not survivable in
a feature whose entire purpose is to hand the address out, to exactly the
newcomers who will not suspect it.

**The disclosure belongs in the actor's summary, not only in our modal.**
This is the part worth getting right: the person about to reply is on
Mastodon, looking at the patch's profile there. They will never open our
Subscribe modal. `NodeToActor` sets `Summary: node.Description`, and
Mastodon renders that summary as the profile bio — so appending one
sentence to the summary of a federated patch actor is the only version of
this disclosure that reaches the audience it is for. The modal row carries
the same sentence for the person on our side who is about to share the
handle.

That the actor is typed `Organization` helps: a patch already presents as
an org account rather than a person, which is the correct expectation for
a followable calendar.

**Considered: answering unhandled activities with a 4xx** so the sending
server surfaces a delivery failure instead of a false accept. More honest
at the protocol layer, and rejected for two reasons. It is not established
that a failed reply delivery ever reaches the human who wrote it —
delivery is fire-and-forget from the author's side — so it likely trades a
false accept for a silent drop with extra steps. And accepting unknown
activity types is the conventional inbox behaviour; diverging from it to
send a message that nobody reads is the wrong place to spend interop
risk. This should be re-checked against the live Mastodon setup that
verified interop on 2026-07-13 before anyone acts on it.

**Not decided here: building an inbox.** Accepting replies is a real
feature with moderation consequences — who sees them, who deletes them,
how they meet ADR 023's stewardship and the report flow. It is not a
prerequisite for publishing a handle. A calendar you can follow is an
honest thing to be. A conversation that swallows what you say is not, and
one sentence is the difference.

## Considered and rejected

**A Federation tab in patch settings.** The instinct is to make it
configurable per patch. There is nothing to configure: the actor exists,
the visibility gate already decides whether it federates, and a per-patch
toggle would be a fourth privacy control disagreeing with
`nodes.visibility`. It would also teach the word "federation" to somebody
who came here to post a show.

**Flipping `federation.enabled` to true by default**, so the row is
usually live. Rejected on the footgun `config.go:240` already warns
about: AP IDs minted against a wrong domain are **permanent**. Off by
default is right. The fix is the SMTP pattern — warn in the dashboard,
never refuse to start — so an admin learns that their patches have
addresses nobody can reach. That belongs with the Label (ADR 023), which
already exists to state how a quilt is run, and is left to its own
decision rather than smuggled in here.

**A reach receipt after publishing an event** ("visible to 40 followers,
12 from other sites"). Probably the strongest motivator in the idea, and
out of scope: it needs a follower count broken down by locality, which is
a query and an endpoint, not a row in a modal. Worth its own ADR if this
one lands well.

## Consequences

- **The address is the quilt's, not the patch's, and the row must say
  so.** ADR 060: `ap_followers` and `nodes.ap_id` stay behind on a
  seamrip, so `@the-selvage@arts.lancaster.example` does not survive the
  patch forking to another instance — the followers this row recruits are
  lost in exactly the scenario the fork exists for. That is a reason to
  publish the handle *with the caveat*, never a reason to withhold it: an
  address that might move still beats an address nobody knows exists, and
  a community that knows what it is building on can decide for itself.
  One clause in the same copy that carries the reply disclosure.
- Nothing here creates a table or a column, so the ADR 002 boundary is
  untouched.
- **A frontend test cannot verify this.** CLAUDE.md is explicit that every
  frontend test asserts against source text, so the suite can confirm the
  row's markup exists and cannot confirm anybody can read it. The handle
  row and the disclosure need reading in a browser, on a patch that is
  public and one that is not.
- If the row lands, the natural next surfaces are the Label line and the
  reach receipt — in that order, since the first is a statement and the
  second is a query.
