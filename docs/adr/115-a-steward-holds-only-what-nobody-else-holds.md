# ADR 115: A steward holds only what nobody else holds

Date: 2026-09-16. Status: **accepted; implementation deliberately deferred**,
see "Deferred on purpose" below. Raised when the reference instance took its
first patch claim and its operator found he held every admin page of a patch he
only followed.

## Context

Every admin-gated node endpoint accepts `user.Role == "admin"`. About thirty
sites spell it the same way, and `GET /api/v1/nodes/{slug}` reports it back:

```go
// nodes.go:686
// Global admins can manage any patch (matches the write-side authz: every
// admin-gated node endpoint accepts user.Role == "admin"). Surface that so
// the UI shows the management surface.
if user.Role == "admin" { resp["is_admin"] = true }
```

That comment is accurate. The payload flag is not a display bug; it truthfully
reports real write authority. So an instance admin holding no role in a patch
can rename it, rewrite its charters, change its membership policy, own its
event feeds, read its members-only documents, and see the memberships its
members chose to hide.

**This was never decided.** It spread by being copied from the site next to it,
and two comments record the moment the copy outran its own justification.
`event_links.go:26` justifies and contradicts itself inside one sentence:

> "Speaking for a patch is admin territory: patch admins on active patches,
> the instance admin everywhere (who holds unclaimed patches' calendars in
> trust)."

The parenthetical justifies unclaimed. The code says everywhere.
`event_submissions.go:165`, three lines below code that correctly implements
ADR 026's unclaimed/active split, asserts the habit as though it were the rule:

> "Unclaimed calendars are reviewed by the instance admin alone; active
> calendars by that patch's admins (global admin override applies, as on every
> node endpoint)."

ADR 026 says the opposite in as many words: *"never the instance admin,"* and
the grant *"must not reach into active patches, where it would let instance
authority override patch autonomy."*

The principle was already written down three times before this ADR. ADR 026:
review is owed to whoever owns the calendar, and the instance admin owns the
unclaimed ones *"in trust."* ADR 057: *"The instance admin can act, because
they hold unclaimed patches' calendars in trust."* CONTEXT.md, "Instance
admin": *"Curates instance-wide options; does not override per-patch choices."*

And it was discovered again, independently, at twelve places that had to work
around the payload flag to be correct. Five in Go (`proposals.go` at three
doors, `notices.go`, `memberships.go:447`) and seven in Svelte
(`patchWorkspace.js`, `RulesProposalEditor.svelte`, `GovernanceOverview.svelte`,
`PatchProfileGlimpses.svelte`, `PatchProfileHead.svelte`,
`PatchRelationship.svelte`, `EventLinks.svelte`). Twelve authors wrote the same
sentence twelve ways. That is what a wrong abstraction looks like from inside.

## Decision

**An instance admin's reach into a patch is custody, not rank.** It exists
exactly while the patch has no admin of its own, and it ends the moment
somebody holds the role.

Custody is not a synonym for unclaimed. A patch reaches "nobody holds this"
two ways, and they are not the same job.

| State | Condition | Reach |
|---|---|---|
| **Unclaimed** | never claimed, no admins by definition | the calendar, as ADRs 026, 031, 056 and 057 already grant: events, sources, links, submissions, bulk upload |
| **Vacated** | claimed, zero active admins | `UpdateMember` only: put an admin back, and nothing else |
| **Held** | at least one active admin | none |

The vacated row is not hypothetical. Inactivity vacates seats (docs/adr/102),
and on `elected` with no term end, on `meritocratic`, and on `maintainer` with
no successor named, a patch lands at zero admins and cannot refill itself.
`internal/notifications/inactivity.go` already tells the instance admins and
already says why: *"UpdateMember lets an instance admin restore one there."*

The two custodies differ because the situations do. An unclaimed patch has no
community inside it, so somebody must keep its calendar alive or the quilt
looks dead exactly where it most needs to look alive (ADR 026). A vacated patch
has members, a charter, a history and a room. It has lost its officers, not its
community. Running it was never the steward's job, so the steward's only power
there is to hand the keys back.

**`is_admin` in the node payload means admin of this patch.** One meaning, one
flag. The twelve workarounds are deleted, not extended.

### Named exceptions, each for a stated reason

- **Archive** (`DeleteNode`). Archiving removes a patch from the quilt rather
  than editing it, which is instance curation, and it is the moderation lever
  of last resort. CONTEXT.md already grants it and ADR 034 already makes
  restore instance-admin-only on purpose. Kept, and recorded here so it stops
  inheriting the reflex and starts standing on a reason.
- **Commenting**, at `comments.go:177` and `:192`. `proposals.go:183` already
  settled this: instance admins keep a bypass *"for commenting, which is
  speech."* Deleting somebody else's comment is not speech, so `comments.go:337`
  loses its bypass and routes through the report queue like every other piece
  of content.
- **Every `/api/v1/admin/*` route**, the report actions, claims review, tags,
  the trusted-contributor grant, the danger-zone wipe, and admin email repair.
  These are instance-level objects. Custody does not apply because they were
  never a patch's to hold.

### There is no break-glass

Considered at length and deliberately not built. A steward who must read a
private room does it at the database, out of band, and the software neither
eases that nor pretends to govern it.

The case for building it was real: the act happens anyway, and a feature that
notifies the patch and records the act beats an invisible shell session. It
lost on ADR 023's threat model. That ADR already refuses to treat the admin
role as trustworthy by construction, because *"auto-publishing the names and
faces of everyone with root on an antifascist organizing platform builds a
targeting list out of a trust feature."* The steward is a person under threat.
A documented product feature is a thing a court order can name, a phished
session can use, and a tired admin can press at 2am. Building the key creates
the lock.

ADR 081 already shipped a room with no escalation path and the sky did not
fall. Charters now join it.

**The Label carries the floor instead.** ADR 049 says Patchwork states only
what it enforces, and ADR 023 makes the Label the page where a quilt discloses
how it is run. An undisclosed "whoever holds the machine can read everything"
is true but unstated, which is the opacity ADR 023 exists to refuse. So the
Label says it, plainly, and a community that dislikes the answer can seamrip
before it matters rather than after.

### A guard, because the failure mode is copying

`TestInstanceAdminHoldsNoPatchAuthority` walks every node-scoped route and
asserts that an instance admin holding no role there is refused, with an
explicit allowlist naming the custody cases and the exceptions above. A new
route that inherits the reflex fails the build until somebody names it. This is
the same shape as `TestEveryTableHasABoundaryDecision` and the who-decides
matrix, and it exists for the same reason: the mistake is not hard to make
once, it is hard to stop making thirty times.

## Deferred on purpose

The rule is decided. The code is not changing yet.

Lancaster is weeks old and has just taken its first claim. In that window the
operator wants the reach as a recovery hatch: if something goes badly wrong in
a patch on a live instance with real organizers in it, being able to open the
settings and fix it beats being correct about who holds what. That is a
legitimate call and it is the operator's to make, so this ADR records the
decision and waits.

Two things are worth naming so the wait stays honest.

**The status quo is now an undocumented break-glass.** This ADR argued against
building one, and deferring leaves an ambient, unannounced, unaudited version
in place instead: `node.update` logs detail `{}`, so an instance admin's edit
to a patch they hold no role in is indistinguishable in the record from the
patch's own admin doing it, and the audit log is `AdminRequired` so the patch
cannot read it either. Whatever else happens, that is the weakest form of the
thing and it should not be the resting state for long.

**The reflex spreads while it waits.** The bypass reached thirty sites by being
copied from its neighbour, and every node route written between now and
implementation will copy it again. The guard test is the part that does not
have to wait: landed first as a characterization test, pinning the current set
and failing the build on a thirty-first, it stops the spread without removing
anything. Flipping it to the rule above then becomes an allowlist edit rather
than an archaeology exercise.

**The trigger for revisiting** is launch maturity rather than a date: when the
reference instance has claimed patches run by people other than its operator,
and the operator no longer expects to be the one fixing them. Tracked as
issue #286.

## Considered and rejected

**Rank: keep the reach, fix only disclosure and audit.** The honest version of
the status quo, and cheap. It requires reversing five deliberate removals that
each cite CONTEXT.md, and it makes the glossary's "does not override per-patch
choices" false. Rejected because the project already decided, three times, and
the code simply had not been told.

**Read anything, write nothing.** Draws the line at mutation rather than at
custody. It reads as a moderate compromise and is not one: it leaves the
steward able to read every members-only charter and every hidden membership on
the instance, ambiently, which is precisely the reach an adversary inherits and
precisely what ADR 023's threat model is about.

**One custody, same reach in both states.** Simpler to state and to implement:
one predicate, no branch. Rejected because it hands the steward a members-rich
patch's settings and charters while its chair is empty, and the members are
right there. Losing your officers is not the same as never having had any.

**Vacated grants nothing automatic.** Purest custody: the steward is notified
and may act only if the members ask. Rejected because a patch whose members
also go quiet becomes permanently stuck, and `inactivity.go` already built the
recovery path and already announces it to the people affected.

**Break-glass as a product feature.** Covered above.

## Consequences

The reference instance's operator loses the admin pages of every claimed patch
he does not run. That is the point, and it is the promise made to whoever just
claimed one.

**A report about private content can no longer be judged by reading it.** The
report panel still works, because every action there is entity-first and
consults no patch role: `remove_content`, `reset_appearance`, `remove_image`,
`warn`, `suspend_user`, and archive all take an entity id. What the reviewer
loses is context they could previously browse to. For a members-only charter or
a notice, they act on the report or they do not act. This is a real cost,
accepted knowingly, and it is the same trade ADR 091 made when it refused to
relay the reporter's words: the alternative on offer was worse.

**`canReadPatchDocs` is the largest single change**, because ADR 110 hangs the
git transport off it. Closing it to instance admins closes every doc body,
every revision, every diff and every editor's name in the commit metadata, in
one predicate. That is ADR 110 working as designed: there is no half-clone, so
the door carries the whole rule.

**No migration.** This is authorization only. No schema, no data, no backfill.

**A one-person instance is unaffected**, because its operator is already the
admin of every patch on it. The rule costs nothing where there is nobody to
protect from, and costs exactly what it should where there is.

**The floor is unchanged and now stated.** Whoever holds the SQLite file can
read any room. No permission model alters that. What this ADR changes is that
the software no longer makes it casual, no longer makes it ambient, and no
longer leaves the patch unable to find out.
