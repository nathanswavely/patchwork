# An act the instance role let through says so

Date: 2026-10-20. Status: **accepted, implemented.** Applies docs/adr/115 while
its implementation waits (issue #286).

## Context

docs/adr/115 decided that an instance admin's reach into a patch is custody,
not rank, and deferred removing the reach until the reference instance has
claimed patches run by people other than its operator. It named the cost of
waiting plainly: the status quo is "an undocumented break-glass". An instance
admin holding no role in a patch could rename it, rewrite its charters, drop
its events or change who runs it, and the audit entry was the same one the
patch's own admin would have written. `node.update` recorded the field names
and nothing else.

The trigger has not been met, so the reach stays. What does not have to stay
is the silence.

## Decision

**Every audit entry for an act on a patch says when the instance role is what
let it through.** The entry's metadata carries `"acting_without_role": true`
when the actor is an instance admin who does not hold the standing that act
otherwise requires in that patch: admin for an admin's act, the member/admin
standing for a direct publish, any role (or a follower's permission) for a
comment. An act on the actor's own event, comment or proposal is theirs and
is not marked.

- **Absent, never false.** Every other entry keeps its existing shape, and a
  query for the reach is a query for the key.
- **Custody is marked too.** Reviewing an unclaimed patch's queue and restoring
  a vacated patch's admin are acts ADR 115 keeps, and they are still the
  instance role acting. The mark says what let the act through, not whether
  ADR 115 approves of it.
- **Asked before the act.** `UpdateMember` can seat the caller as an admin of
  the patch they were reaching into. Asking afterwards would record the reach
  as the standing it had just created.
- **One predicate.** `actingWithoutRole` (`internal/handler/audit_reach.go`)
  is the only place the question is answered. It refuses and allows nothing,
  and it is declared in `instance_admin_reach.go` as `reachRecordsIt`.

The sites that built their metadata by string concatenation now go through
`LogAuditEventJSON`. A feed URL, a crosswalk name key or a hold decision that
contained a quote could write an entry that was not JSON at all.

### The guard

`cmd/patchwork`'s `TestEveryRouteAnInstanceAdminReachesIsDeclared` already
walks every route as an instance admin holding no role. It now also requires
every audit entry that caller writes to carry the mark, unless the action is
about no patch (`auditAboutNoPatch`: today only the instance invite link). An
entry written on a new reaching route without the mark fails the build.
`TestAnActTheInstanceRoleLetThroughSaysSo` checks the other direction: the
patch's own admin, and an instance admin who is also the patch's admin, are
never marked.

## Consequences

**Who can see the mark is unchanged.** The audit log is `AdminRequired`, so
the mark reaches the instance admins (shown in the admin panel's audit view
as raw metadata) and, through the personal export, the person who acted. It
does not reach the patch. The governance record reads audit rows into fixed
fields and passes none of the metadata through. Whether a patch should see
what the instance role did inside it is docs/adr/091's question, and it stays
open.

**It records only what is audited.** Reads are not audited, so an instance
admin reading a members-only charter or a closed governance record leaves no
entry, marked or otherwise. A few writes keep no audit entry at all (a
feed's manual sync, offer dismissal). Removing the reach is #286. This ADR
only makes the writes that are recorded honest about how they happened.

**When #286 lands the mark does not go away.** Custody of unclaimed and
vacated patches remains, and archiving remains. Those acts still go through
on the instance role, and their entries should keep saying so.
