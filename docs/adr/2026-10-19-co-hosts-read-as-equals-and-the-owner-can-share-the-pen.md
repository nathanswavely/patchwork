# Co-hosts read as equals, and the owner can share the pen

Date: 2026-10-19. Status: **accepted**; implemented. Amends docs/adr/032
(event links: one owner, two consents), which named "an explicit co-editing
grant on a link" as a possible future extension. This is that extension,
plus two changes to how links are made and shown.

## Context

The reference instance has a show on 2026-11-14: a dual album release for two
bands, a third band on the bill, at a bar that is a patch of its own. Four
patches put it on. Under docs/adr/032 one of them holds the row and the other
three link to it, and in practice that went wrong in three places.

The control was hard to find. Linking lived on the event page, behind a small
"Link a patch" button, and only after the event existed. The admin who made
the event looked for it on the Edit Event form, where the other fields are,
and it was not there.

The page told the wrong story. It said "Hosted by" the patch holding the row
and put everyone else on a "with" line underneath, which reads as a headliner
and its guests. For this show that is false, and it is false for most shows
that are a shared night rather than one patch's event.

And only one patch could fix a typo. A linked band that needed to change the
set time had to ask the patch holding the row to do it.

## Decision

**One patch still holds the row.** Everything docs/adr/032 gave the owner
stays with it: the review queue, source authority over an imported event,
deleting the event, detaching it from a feed. It keeps the name **owner** in
code and in this record; "steward" was considered and is taken (CONTEXT.md:
the people accountable for a quilt). What changes is that the owner stops
being the only patch the page calls a host. Co-ownership is still rejected, for 032's reasons: every check would have to
reason about a set, and a feed import can never mint co-owners.

**Co-hosts are chosen in the event form, at creation and when editing.** The
form has a co-hosts field for anyone who speaks for the hosting patch (the
same standing the link handshake already asks for). Saving the form sends
one link request per co-host, through the same handshake as before. A patch
the person also speaks for confirms at once; any other patch's admins
confirm before the event appears on their calendar. Nothing about consent
changes: a stranger who could add co-hosts without their agreement could
paint events onto any patch's calendar, and that is the thing 032's second
consent exists to stop. The event page keeps its own control, because the
other side of the handshake (a band asking onto a venue's event) does not
get the form.

**The page names every confirmed co-host on one line.** "Hosted by A, B, C
and D", the owner first, then the confirmed links in the order they were made,
then cross-quilt mentions with their host. Which one is the owner is back-office
information: who holds the row is visible in who can delete it, not in how
the event is introduced.

**A confirmed link can carry edit rights** (`event_links.can_edit`). The
owner's side grants it, either in the link request or afterwards with
`PATCH /api/v1/events/{id}/links/{nodeId}`, and only the owner's side can
take it back. A request from the linked side that asks for it is refused
rather than quietly downgraded. The receiving side does not accept it
separately: they already consented to the link, and edit rights are given to
them rather than exposing them to anything.

It takes effect only while the link is confirmed and the linked patch is
active or unclaimed, and it reaches whoever speaks for the linked patch
(`userSpeaksForNode`: its admins, or a trusted contributor while it is
unclaimed). It is not transitive. An editor cannot grant it to anyone else,
because edit rights that spread one hop at a time are rights nobody decided
to give.

**An editor edits everything on the form except who the event is for.** The
visibility tier is read against the owner's own Follower Permissions
(docs/adr/2026-09-19-an-event-says-who-it-is-for-within-what-the-patch-allows.md),
so a linked patch setting it would be deciding the audience of another
patch's calendar. A PATCH from an editor that changes visibility is refused;
one that sends the stored value back is accepted, because the form spreads
every field it loaded into its submission.

**An imported event stays read-only to everyone, editors included,** until
it is detached (docs/adr/031). The next sync would undo an editor's change
otherwise.

**Granting and revoking notify nobody** (docs/adr/093). The grant shows on
the event: an editor sees the Edit button the next time they open it.

**Edit rights travel with the link** in a seamrip, as one more column on a
row that already travels, and they are visible in the API only to the two
sides of that link, like `requested_by`.

## Considered and not done

**Role labels on links** ("headliner", "opener", "sponsor"). 032 kept links
flat, the way patches are flat, and nothing here changes that. A label is a
taxonomy that fits one kind of community, and a band would have to consent
to being called the opener as well as to the link.

**Letting an editor delete the event.** An editor can remove their own
patch's link at any time, which is what severing looks like from their side.
Deleting removes the event from every co-host's calendar, and that stays with
the patch that holds the row.

**Moving ownership.** Handing the row to another co-host would be a
real feature with its own questions (does the review queue move, does the
feed provenance), and nobody has asked for it.
