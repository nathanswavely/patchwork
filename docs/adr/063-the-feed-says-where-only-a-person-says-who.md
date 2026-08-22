# ADR 063: The feed says where; only a person says who

Date: 2026-08-21. Status: accepted. Discharges the Known gap in ADR 056
("a patch cannot discover that a venue's routed calendar names it") and
draws the line ADR 056 named and deferred: text may suggest to a human
even though it may never route. Extends ADR 032; amends nothing.

## Context

ADR 056 shipped the crosswalk and predicted its own gap. Lancaster hit it
in the first week. Six upcoming listings sit under the unrouted name
`Lancaster Marriott at Penn Square`, and every one of them is the African
American Heritage Walking Tour, which the African American Historical
Society runs. Routing the name is correct — the tour genuinely happens at
the hotel — and it makes a hotel's tile the sole home of another
organization's program.

Four facts from the live feed (186 events on 2026-08-21; ADR 056 counted
196 across 55 names, so it moves):

- **No `ORGANIZER`, on any event.** Neither `internal/eventsource` nor the
  feed has ever carried one.
- **The string "African American Historical Society" appears nowhere** —
  not in a title, a description, or `CATEGORIES`, which is empty on every
  event. There is no text to match. Whoever knows the tour is theirs knows
  it from living in Lancaster.
- **No `RRULE`. 186 events, 186 distinct UIDs.** Tockify embeds a series
  id in the UID and it does not help: the seven tours carry seven
  different series ids. Grouping by series collapses 186 listings to 154.
- **Grouping by title collapses them to 111**, and by location to 49.

So the gap is not a matching problem and cannot be closed by better
matching. Recognition is the scarce input, and only a person has it.

## Decision

**1. A program is a recurring title in a feed, credited by a person to a
patch.** It groups listings the way a name does, and that is the whole of
the resemblance: a name is read from `LOCATION` and decides ownership, a
program is read from `SUMMARY` and decides nothing. What a program does is
offer its events to the credited patch, whose admins propose an ordinary
ADR 032 link that the owning patch confirms.

**2. A name is the only key that routes.** Not because a location is a
venue — the backend has no venue concept, and the result either way is two
patches on one event — but because `event_sources.node_id` is the owner
and every listing carries both a place and a title. If both keys could
route, a listing matched by both would answer to two sources and the
reconciler would have two masters. Location wins the seat on the evidence:
49 keys cover what 110 titles cover, venues get misspelt where titles get
renamed, and `Latin Dance Night at West Art` is ADR 056's standing proof
that a title cannot say whose event it is.

**3. Text may suggest and may never route.** The line ADR 056 declined to
draw, drawn. It holds because of where it terminates, not because the
matching is careful: a program ends at a link the named patch confirms, so
a wrong program is declined, where a wrong route lands silently. The
matching may be as crude as equality on a title, and is.

**4. The sync writes no links, not even proposals.** ADR 032's "humans
initiate every link" stays literally true rather than approximately. A
credited program produces an **offer**, which is a query result — events
routed from listings whose title matches a program credited to this patch,
minus those already linked, minus dismissals. Nothing is stored, so there
is nothing for the reconciler to write, and decision 3 is enforced by
absence rather than by discipline. Crediting also back-fills instantly
over events already routed, with no wait for a sync. The one stored thing
is a dismissal, for ADR 056's reason: otherwise the next sync re-offers it
and the same refusal is owed every hour.

**5. Programs are promoted, never enumerated.** A program exists only
because someone opened a name's listings, recognized what they were, and
said so. Listing every distinct title would post 110 rows next to 49, some
75 of them one-offs, and would demand a decision about `Latin Dance Night
at West Art` whose right answer is "nothing." The admin panel shows no
judgement nobody made.

**6. Programs and names share one list.** They are the same kind of
object — a key grouping listings, awaiting one human judgement — and the
list is what the instance admin has not yet decided about. Recognition
happens in the drawer, where ADR 056 already put the evidence (title,
time, description, the publisher's own link); the result surfaces at the
top level, where decisions live. A program's row says what it is waiting
on, which is how the ordering below stays visible instead of silent.

**7. A program is inert until its name is routed.** Ownership precedes
attribution: with no event there is nothing to offer. This makes an
organization's visibility depend on its venue being minted, which is
uncomfortable and correct — the alternative is a tour that appears on the
society's calendar and nowhere at the hotel it happens in. It also
disposes of the interaction with ignore: an ignored name never routes, so
its programs could never have offered anything, and hiding them with it
costs nothing.

**8. Crediting takes standing over the credited patch and nothing else.**
`userSpeaksForNode` unchanged — instance admin, the patch's own admins, or
a trusted contributor while it is unclaimed (ADR 057). No new permission
concept, because a program is a claim about who *you* are: crediting the
society asserts nothing about the hotel, whose calendar is untouched until
a link is proposed and its admins confirm. A patch's admins hunting their
own programs need a corpus of **routed** names, which is the opposite of
what `ListAggregatorNames` serves today — that endpoint returns unrouted
names, for a patch looking for names that mean itself.

**9. Crediting means one thing regardless of who does it.** A self-credit
takes the same path as any other: it produces offers, and whoever credited
acts on them. Letting a self-credit propose links directly was considered
and rejected — it makes two flavours of program, and the standing half
would auto-propose future listings, which is decision 4 broken from the
inside.

**10. Offers surface where the aggregator's other effects do, and announce
on ADR 056's rule.** Patch Settings → Sources already carries the
crosswalk entries pointing at a patch and its held duplicates; offers are
the same kind of thing and want the same page. Crediting back-fills
silently — nobody wants six notifications for a decision they just made —
and each new offer afterward sends `program.offer` to the credited patch's
admins. Not notifying at all was rejected: standing is the whole value of
a program, and an offer that waits on a settings-page visit is a one-time
bulk apply with extra machinery.

## Considered and rejected

**Grouping by UID or series id.** Recommended and abandoned inside an hour
once the feed was actually read. Tockify emits no `RRULE` and gave the one
tour seven series ids; the whole feed collapses 186 → 154. A feed with real
recurrence would make this work. This is not one.

**Enumerating every title as a row.** See decision 5. Manufactures some 35
questions to collect maybe 5 answers.

**Letting a program own the event** — the tour becomes the society's, the
hotel keeps nothing. Rejected twice: on `events.node_id` being singular
with no source pointing at the new owner, and because a person asking what
is on at the Marriott would get a hole where a thing that is literally
happening there should be.

**A standing rule that creates links at sync.** ADR 057 makes a trusted
contributor speak for both sides of an unclaimed-to-unclaimed handshake,
deliberately. Combined with ADR 056's aggregator being "the grant made
non-human," a rule firing at sync would propose and confirm in one tick
and nobody would ever be asked. Text matching would have routed, wearing a
link instead of a `node_id`.

**Calling the offer a candidate.** ADR 056's Known gap used the word.
CONTEXT.md gives it to elections — a person on a ballot — across fourteen
files, and its entry already flags the term as collision-prone. Read that
sentence in ADR 056 as describing programs.

**Letting the owning patch credit somebody else.** The hotel's admins know
their own calendar, and ADR 032 already lets them tag the society on one
event at a time — with the society's confirmation built in. What they must
not have is the *standing* half: a program pointed at a patch that did not
ask generates a fresh offer for every future listing, and dismissals are
per-event, so the refusal is owed forever. Credit is for the credited.

**Recruiting the society instead.** ADR 056's answer was that a host can
attach its own event source. It presumes a machine-readable calendar, and
an organization that had one would not be publishing through the tourism
bureau. Still the better outcome where it is available, and a program does
not preclude it.

## Consequences

The heaviest tiles on a city-calendar quilt will be venues, because ADR
015 sizes by activity — but the scale is smaller than it sounds, and an
early draft of this ADR overstated it. In this feed the busiest name is
Binns Park at 46 listings, a public square the city genuinely programs,
then two arts venues at 15 and an improv troupe at 12 that the feed files
as a location. The Marriott sits at 7, and 26 of 49 names carry a single
listing. A confirmed link grows both tiles (ADR 032), so crediting
programs mitigates what is left.

**Hosting and running weigh the same, held rather than settled**
(2026-08-21). This is already what ships: `event_count` sums a patch's own
active events and its confirmed links with no weighting
(`internal/handler/tree.go`), so a venue's tile grows for a night it hosts
exactly as the organizer's does for running it. Keeping it that way is a
decision and not an oversight — a venue that opens its room *is* doing
community work, which is ADR 015's premise rather than a loophole in it,
and this feed does not embarrass the rule: its busiest name is a public
square the city genuinely programs, not a hall nobody claims. What would
reopen it is a venue carrying a hundred nights it does not run, and no
quilt has one. Weighting a linked event below an owned one is the lever if
one ever does, and it belongs to ADR 015.

Declining to route a venue is not a way to manage this. By decision 7 it
suppresses the events entirely — the credited patch gets nothing either —
so it trades a small tile for the listings themselves. Ignore is for names
that mean no organization, which is what ADR 056 built it for.

A program is scoped to one name. In this feed exactly one title of 110
appears under two names, so the cost is a second program credited to the
same patch — already the ordinary shape, since the society's other tour
runs seven times at Crispus Attucks Community Center under a different
title.

Both travel in a seamrip on ADR 056's terms: a program is the same
community labour as a crosswalk entry, and worth no more without a steward
to vouch for the aggregator carrying it.
