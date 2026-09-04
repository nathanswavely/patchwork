# ADR 080: A contact card is shared patch by patch

Date: 2026-09-04. Status: **accepted**; implemented. Sits beside docs/adr/006
(one membership-visibility switch) without amending it.

## Context

A community organizer, in a feedback session, asked for the thing every
organizing group ends up doing in a spreadsheet: a way to give the people
you organize with your phone number without giving it to the internet. "I
want to share my number, but only to certain patches I'm part of."

Patchwork had no field for it. A profile carries name, bio, avatar, and
links, and every one of those is public by design — the AP actor publishes
the same fields, so gating the page would protect nothing (docs/adr/006).
The sign-in email is deliberately never shown to anyone but instance admins.
There was nowhere between "public" and "admins only" for a person to put
how they can be reached, and so the number went into a Signal group, a
flyer, or a member's memory.

Three constraints from existing decisions bound the design:

- **ADR 006 has one switch, on purpose.** Membership visibility governs
  both the profile and the public member list at once, and the ADR rejects
  a profile-only hide as privacy theater because the same fact stays
  readable elsewhere. A second toggle that looked like a second visibility
  switch would reopen that argument.
- **No instance-wide people search** (docs/adr/006). A user directory is
  "exactly the correlation tool the threat model worries about." Contact
  details must not create one by the back door.
- **Follower permissions gate taking part, not reading** (docs/adr/050).
  Member lists are public reads; hiding a tab from followers hides nothing
  from the signed-out. Anything shown on the Members room that is *not*
  public has to be gated by an actual check, not by the tab.

## Decision

**1. One contact card on the account: phone, email, note.** Three columns
on `users` (`contact_phone`, `contact_email`, `contact_note`), edited whole
through `contact_card` on `PATCH /api/v1/auth/me` so a form that spreads
the card back can never half-update it. The email is an address to be
reached at, deliberately separate from the sign-in address — sharing how to
reach you must never hand out the key to your account, and the sign-in
address stays admin-only as before. Phone is free text: "+1 717 555 0100,
Signal only" is how people actually fill in a phone field.

**2. One switch per membership: `memberships.share_contact`.** Owned by the
member, flipped on the same endpoint as visibility, default off. Nobody's
number is shared by a migration or a default. It is a **second axis, not a
second visibility switch**: visibility says whether a membership is *known*
publicly; sharing says whether the people already in the room can *reach*
you. The two compose freely — a hidden membership can share its card (the
room already sees the person; ADR 006 says so) and a public one can keep it
private.

**3. The card is shown in exactly one place, to exactly the room.**
`GET /api/v1/nodes/{slug}/members` carries `contact` per member only when
the *viewer* is an active admin or member of that patch, and only for rows
that switched sharing on. It never appears on the profile, never in the
public member list, never in the AP actor. This gate is narrower than the
existing `insider` check on the same endpoint, which admits an instance
admin with no role in the patch to the full member list: an instance admin
curates the quilt, but was not who the person chose to be reachable by.
The two-line difference is the whole point.

**4. A follower cannot share.** The Members room shows cards to admins and
members; a follower's card would be shown to people the follower never
chose. The switch is refused on follower rows and not offered in the UI.

**5. It travels the way `email` does.** The admin seamrip already moves
other people's secrets and is admin-gated for it (CONTEXT.md, *Seamrip*).
The card travels beside the switch that shows it, so a fork shows each
card to exactly the rooms the person chose and no more. Leaving the switch
behind would fail the other way: every card someone had chosen to share
would go silent on the fork.

## Considered options

- **Sharing on the profile, to viewers who share a patch with you.**
  Rejected: it turns the profile into a surface whose content depends on
  who is looking, and it puts a reachability check on a page that is
  public by construction. One surface, the Members room, keeps the rule
  legible.
- **A per-patch card (different number per patch).** Rejected as busywork
  nobody asked for; one card, many switches, is the spreadsheet everyone
  already keeps.
- **Sharing the sign-in email instead of a separate address.** Rejected:
  the sign-in address is a magic-link target. Reachability and account
  access must stay different facts.
- **Defaulting sharing on for invite-only patches** ("a band already
  knows each other"). Rejected: a default is a decision the app made, and
  the whole feature is the person deciding. Off everywhere, always.
- **Showing the card to instance admins too**, matching the member list's
  `insider` rule. Rejected — see decision 3. Instance admins can read the
  database; the API still should not say what the person didn't.
- **Excluding the card from the seamrip.** Rejected — see decision 5. The
  export already carries sign-in emails; refusing the card while shipping
  the email would be a boundary that protects the less sensitive fact.

## Consequences

- Migration 062; `TestEveryColumnHasABoundaryDecision` is satisfied by the
  columns travelling.
- The Members room stops being a pure public read: the same URL returns
  strictly more to a viewer in the room. `TestContactCardIsSharedPatchByPatch`
  holds the viewer matrix (anonymous, follower, instance admin without a
  role, fellow member, the sharer, a member of a different patch).
- The shipped privacy policy gains a paragraph; instances that overrode
  it (docs/adr/028) should say the same thing in their own words.
- Nothing federates. The AP actor is built from named fields
  (`internal/ap/convert.go`) and this does not add one.
- Every surface that says "membership visibility" now has a sibling
  switch beside it. The glossary keeps them apart: *known* versus
  *reachable*.
