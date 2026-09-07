# ADR 083: A contact item is shared one patch at a time

Date: 2026-09-07. Status: **accepted**; implemented, superseding
docs/adr/080. Sits beside docs/adr/006 (one membership-visibility switch)
without amending it.

## Context

docs/adr/080 shipped three days ago and answered the organizer's question —
"I want to share my number, but only to certain patches I'm part of" — with
one card of three columns and one boolean per membership, shown in one
place. Reading it back against the code found three faults, and only the
third is a bug.

**The granularity is wrong.** One boolean means every room a person shares
with receives byte-identical fields. Ana wants her band to have her number,
the volunteer crew to have email only, and the coalition to have neither;
under 080 her only move is to delete her phone from the card, which takes
it from the band too. The `note` field is already absorbing the pressure —
080 concedes people type `"+1 717 555 0100, Signal only"` into the phone
field, which is a channel smuggled into a value because there is nowhere
else to put it.

**One surface is not the same as the right surface.** 080 chose the Members
room and rejected the profile because a viewer-dependent profile "puts a
reachability check on a page that is public by construction." That cost is
real, but it was weighed as though the profile would *widen* the audience.
It does not: the set of people who can read a shared item is identical
either way. What the profile changes is where that audience can look.

**The room could not carry it.** `parsePaginationParams` defaults to 20 and
`PatchMembers.svelte` never follows `next_cursor`, so a patch past twenty
people showed twenty — and reported the loaded array's length as the member
count. Every shared card below row 20 has been unreachable since 080
shipped.

Two more constraints bound the design. Editing was already split — card
content on `/settings`, switches on `/settings/patches` — which is two
editors over one truth. And an admin can promote a follower to member
unilaterally, and a patch's door can be changed from `open` to
`approval_required` by its admins: any rule that shares *by standing
condition* can be triggered by somebody other than the person whose number
it is.

## Decision

**1. The card is a set of typed items.** A contact item carries a kind
(phone, email, handle, note), a value, an optional label, and a position.
No fixed length, no required item, empty is the normal starting state. The
kind is a fact about the item rather than a column it happens to sit in,
which is what lets a surface render `tel:` and `mailto:` without guessing.

**2. Sharing is per item, per patch, and always an act.** Every pairing is
decided on its own. There is deliberately **no share-with-everything** —
no "all", no "all vetted patches", no rule evaluated later. A patch joined
next year starts shared with nothing.

This is the decision the rest follows from, and it was reached by trying
the opposite. A standing choice forces the design to answer for the
future: a promotion moves a person into a room they did not walk into, an
admin switching the door from open to approval-required moves a room
inside a condition already chosen, and a patch anyone can join means a
stranger clicking Join can read a number. Each has a patch — carve out
promotions, notify on door changes, warn about open patches — and the
patches compound until nobody can predict the system. Removing the
standing choice removes all three questions at once, and costs a
convenience nobody asked for.

**3. Two surfaces, one audience.** A shared item appears in the patch's
Members room and on the person's profile, under one predicate: the viewer
is an active admin or member of a patch the item is shared with. The
profile is a **window onto that audience, never a wider one** — it shows a
visitor exactly what they could have read by walking into a room they
already share.

**4. The profile never names the granting patch.** An item shared through
a private patch or a hidden membership shows the same as any other. The
page cannot cite its own reason without disclosing a membership that
docs/adr/006 keeps off it, so it does not try; copy is audience-shaped
("shared with people you organize with"), never provenance-shaped.

**5. Grants are patch-first; item-first writes may only reduce exposure.**
Sharing is done from a patch — the Members room, or My Patches — because
that is where the intent forms. The card editor owns the items and shows
where each is shared, with one action: stop sharing everywhere. A
multi-select of every patch you are in, offered per item, is "all" wearing
a disguise, and is not built.

**6. Ending a membership deletes its shares.** Leaving, being banned, and
demotion to follower all drop the share rows. Rejoining starts from
nothing. Keeping them dormant would make *rejoining* an act that discloses
a phone number without a decision — the same fault as a standing rule,
time-shifted. This follows the precedent at `memberships.go:653`, where
rejecting a request clears `join_message` rather than parking it.

**7. One rendering of a person.** The **person card** shows a person
wherever they are named, and the shared items are a section inside it that
appears only for a viewer in a shared room. It follows the pattern
docs/adr/078 already set for the patch card — the same rendering
everywhere, previewing where there is a pointer and opening on tap where
there is not — rather than introducing a modal as a second answer to a
question this codebase settled. The profile is the person card at full
page size.

**8. It travels, and nothing federates.** Items and shares both move in a
seamrip, for 080's reason: the card travels beside what shows it, so a
fork shows each item to exactly the rooms the person chose. The AP actor
is built from named fields and gains none.

## Considered options

- **Keep one card, add the profile surface.** Rejected: the surface was
  the smaller half of the complaint. Granularity is what people are
  working around.
- **"All", or "all my vetted patches", as standing choices.** Considered
  at length and rejected — see decision 2. The named-option form fixed the
  honesty problem (a chosen option is not a silent exception) and left the
  substantive one: a convenience nobody asked for that makes a person's
  disclosure depend on other people's later actions.
- **Item-first granting via a multi-select.** Rejected — see decision 5.
- **Click-to-reveal in the Members room, for harvest resistance.**
  Rejected as theater: the endpoint ships every card in the page payload,
  so anyone in the room reads them all from the JSON regardless. The
  honest version — omit cards from the list, per-member fetch,
  rate-limited — was rejected too: it slows every legitimate reader to
  inconvenience someone already admitted to the room, and the real control
  is the per-item choice upstream. Items are shown in the person card for
  **layout** reasons, which is a good reason, and not offered as a
  security boundary, which it is not.
- **Keeping shares dormant across a departure.** Rejected — see decision 6.
- **A reusable contact modal.** Rejected — see decision 7.

## Consequences

- A migration adds two tables. Both must be entered in `internal/seamrip`
  or `TestEveryTableHasABoundaryDecision` fails the build; the answer for
  both is that they travel.
- Migration 062's data has an exact path forward: each non-empty column
  becomes an item, and every membership with `share_contact = 1` gains a
  share row per item. Nobody's disclosure changes by a field on upgrade.
  `share_contact` and the three `users.contact_*` columns retire with it.
- **`GET /api/v1/users/{username}` stops being viewer-independent.** It
  needs `Vary: Cookie` and can no longer be shared-cached. Under
  multi-quilt CORS the response carries `Access-Control-Allow-Origin: *`,
  so a browser sends no credentials cross-origin and a remote quilt sees
  the public view — **a shared item never appears on a remote patch card
  or in a merged view, even to someone who is in the room.** That is the
  correct outcome (reachability is this quilt's fact to disclose, and
  docs/adr/024 keeps cross-quilt browsing public-only) and it is recorded
  here so it is not later mistaken for a bug.
- `GET /api/v1/nodes/{slug}/members` gains real totals and the client
  gains cursor follow-through. This is a fix 080 silently depended on and
  never had, and it lands whatever else is built.
- **Disclosure is not recall.** Anyone who read an item before it was
  unshared still has it. No surface may say "revoke".
- The person card is new shared UI, and has to be worth showing for the
  many people who have shared nothing — name, avatar, role mark, standing,
  a way to the profile.
- CONTEXT.md gains *contact item*, splits *contact sharing* out of
  *contact card*, and gains *person card* beside *patch card*.

## What building it found

Two bugs that existed only where this decision met another, and neither
branch could have seen alone.

**A card outlived the person.** docs/adr/086 deletes an account by keeping
the users row as a tombstone, so every FK declared ON DELETE CASCADE never
fires — that ADR carries the schema's intent out by hand for
`seats.holder_id`, and `contact_items.user_id` was not on the list because
the table did not exist when the list was written. Deleting the memberships
ended every disclosure, so nothing was visible; but ending a disclosure is
not erasing a value, and a card is the person rather than an act.

**A fork arrived unreachable.** `make import` builds a database after the
process has started, so the startup conversion has already run against an
empty schema and does not run again. Every card in a pre-066 archive would
have landed in columns nothing reads — a silent loss in the one mechanism a
community has for leaving with what is theirs (docs/adr/002), with the fork
coming up looking complete.

Both have the same shape, and it is worth stating for the next table
somebody adds: **a new table joins the cross-cutting machinery only when
someone adds it by hand.** Deletion, import, and export each keep a list.
Only the seamrip boundary has a test that fails when you forget
(`TestEveryTableHasABoundaryDecision`); deletion and import do not.
