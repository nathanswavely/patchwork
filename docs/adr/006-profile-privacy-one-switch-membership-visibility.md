# Profiles: public page, lean actor, one membership-visibility switch

People get public profile pages at `/users/{username}` (the URL the AP
browser redirect has pointed at since the actor endpoints shipped): name,
avatar, bio, links, and visible memberships with role chips. Anyone can
read a profile, including anonymous visitors — the person's AP actor doc
already publishes the identity fields, so gating the page would protect
nothing.

Membership disclosure is controlled by one per-membership switch, owned by
the member, that governs both directions at once: whether the membership
appears on the person's profile and whether the person appears in the
patch's public member list. Default visible. A hidden membership is still
seen by that patch's admins and members inside the workspace. Both
surfaces always agree because a profile-only hide is privacy theater —
member lists are public API, so anyone could rebuild the aggregation; the
switch is only honest if flipping it removes the fact from every public
surface the instance controls.

The AP actor stays lean: identity only (name, bio, avatar, outbox,
followers). Memberships never enter actor docs or activities, because
remote servers cache actors indefinitely and a hide must take effect
immediately everywhere we control. Follows never appear publicly anywhere
— follower status is non-public today, and following is meant to be
frictionless on an organizing platform.

People re-enter search only through scoped finders (a workspace's members,
the admin panel's users), landing on profiles. There is no instance-wide
people search: discovery stays patch-centric, and a public user directory
is exactly the correlation tool the threat model worries about.

## Considered options

- Profile-only hide toggle (member lists unchanged): rejected — the hidden
  membership stays publicly readable on the patch page, so the toggle is
  comfort, not protection.
- Opt-in membership listing (profiles empty by default): rejected — guts
  the contributor-ladder legibility the platform is built around, while
  the same data stays public on member lists anyway.
- Federating memberships as actor profile fields (Mastodon-style
  attachments): rejected — remote caches would hold affiliations after a
  member hides them; unretractable disclosure is wrong for organizers.
- Login-gated profiles: rejected — the actor doc already serves the same
  identity fields publicly, invite links make accounts cheap, and
  logged-out browsing is how communities get discovered.
- Instance-wide people search: rejected for v1 — it is a user directory,
  the one-click correlation surface the membership switch exists to limit.
