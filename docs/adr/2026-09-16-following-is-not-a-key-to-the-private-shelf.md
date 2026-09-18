# ADR: Following is not a key to the private shelf

**Status:** accepted, 2026-09-16

## Context

`follower_permissions.charters` decides whether a follower may read the
charters a patch chose not to publish. Three things about it were true at once,
and together they meant that on a default patch, anybody who clicked Follow
read the private shelf.

**It shipped on.** Migration 012 added the column with
`DEFAULT '{"events":true,"proposals":true,"charters":true,"members":true}'`,
and `ALTER TABLE ADD COLUMN` wrote that into every row that already existed.
`DefaultRules()` said the same, and so did three of the four governance
templates. Every patch granted it and none decided to.

**It stopped meaning what it meant.** When the key was written it also gated
*published* charters, so on-by-default was sensible: it stood for "this patch
has governance worth showing you". docs/adr/036 made publishing a deliberate
per-document act and the key narrowed to the members-only shelf alone.
Nothing revisited the default, so a grant that once meant "show followers our
public governance" quietly became "show followers what we did not publish".

**Following costs nothing.** It needs no approval, and no membership policy
consents to it: anyone can follow any public patch regardless of membership
policy. So on an `invite_only` patch, one whose whole door is an invitation,
the private shelf was one click away from any signed-in stranger. Confirmed
against the handlers on `open`, `approval_required` and `invite_only` alike.

**And the same key opened the repository.** docs/adr/110 gated the git
transport on `canReadPatchDocs`, reasoning that a clone takes the whole shelf
so it should ask the whole-shelf question. That is right about the shelf and
wrong about the audience. A clone takes every doc body, its revision history,
its diffs, **and every commit's author**, which is the enumeration of a patch's
people that docs/adr/006 withholds from followers and docs/adr/086 withholds
from everyone. `deleted_accounts.go` had already written the rule down:
`viewerIsInPatchRoom` is "deliberately narrower than canReadPatchDocs, which
admits a follower holding the charters permission. A follower is an observer,
not a member, and the people in a patch are not theirs to enumerate." The
transport enumerated them anyway.

**The commits were worse than 110 believed.** ADR 110 states that a commit is
authored `DisplayName <username@patchwork.local>`. Four call sites disagreed:
`CreateBranch` and `MergeBranch` in `proposals.go` (three of them) and
`revisions.go` passed `user.Email` straight into the signature. So the
repository carried members' **real email addresses**, the one field the
personal export withholds as authentication-adjacent, that the member seamrip
refuses by travelling people as stubs (docs/adr/089), and that no API surface
hands one person about another.

Two of the six call sites already built the non-routable form by hand. The
convention existed; it was simply not the one that got copied.

## Decision

**The clone door asks `viewerIsInPatchRoom`, not `canReadPatchDocs`.** Active
admins and members of the patch, plus an instance admin: the same line
docs/adr/006 draws for hidden memberships, for the same reason, now asked at
the door that can enumerate them. This narrows docs/adr/110's "a follower is
inside or outside by the patch's own answer". The patch's answer still decides
the shelf, and no longer decides the repository.

**The charters key keeps its narrower job.** A follower the patch grants it
still reads members-only charters over REST, one document at a time, with
hidden memberships and tombstones substituted the way every other API surface
substitutes them. Nothing about published charters changes; they were never
gated on this key.

**It is off by default**, in all five places the default lived:
`DefaultRules()`, the casual, collaborative and formal templates,
`scanFollowerPermissions`, and the rules editor's `fp.charters !== false`. A
patch that wants it says so. The editor's read was flipped to `=== true`
specifically: reading it like its three neighbours is what showed a ticked box
to a patch that had never chosen.

**Existing patches are closed too, and told.** Migration 075 moves the rows.
But `nodes.follower_permissions` is a *cache* of `governance-rules.json` in the
patch's own repo, and `SyncRulesToDB` rewrites the column from that file
whenever a rules change lands, so a migration alone would be undone by the next
unrelated amendment. `CloseFollowerChartersDefault` runs at startup, writes the
rules file as well as the row, and notifies each patch's admins. Modelled on
docs/adr/037's lining rollout: a default the project owns moved, so it moves
everywhere, at startup, announced rather than asked. The commit is authored
"Patchwork System" so the history does not show an admin editing rules nobody
edited.

This is Patchwork changing a patch's stated rules without a proposal, which is
ordinarily the thing this project refuses to do. It is justified narrowly: the
value being changed was never chosen by anyone, it predates the narrowing that
gave it its current meaning, and the change only ever subtracts. The notice is
what keeps it honest, and re-granting is one rules edit away.

**Only the charters key moves.** `events`, `proposals` and `members` say what a
follower sees of a patch's public life. They are each patch's own business and
are left exactly as found.

**A commit carries a derived address, never a real one.** `commitIdentity`
returns `DisplayName <username@patchwork.local>`, which is what docs/adr/110
already said was true, and is the single place that decision is made. `.local`
is reserved and can never resolve (RFC 6762), so the address stays a stable
identity for git tooling and reaches nobody.

## Consequences

**Already-written commits keep the addresses they were written with.** History
is not rewritten: the SHAs are recorded in `proposals.git_sha`, and rewriting
them would break that and every existing clone. The clone gate is what contains
them, which is the other half of why it had to narrow rather than merely
tighten.

**Migration 012's column `DEFAULT` still says `charters:true`** and is
deliberately left alone. Changing a default in SQLite means rebuilding `nodes`:
42 columns, five named indexes and 18 inbound foreign keys, on a live instance,
for a value no production path reads. Instead
`TestNoInsertIntoNodesInheritsTheColumnDefault` fails the build on any `INSERT
INTO nodes` that omits the column, and `CloseFollowerChartersDefault` is the
backstop if one ever lands. That test immediately found three inserts in
`unclaimed.go` and two in the seed that were inheriting the grant, which is the
argument for it.

**The row default and the read default had disagreed.** `CreateNode` writes
`'{}'`, which `canReadPatchDocs` unmarshals to `charters:false` while
`scanFollowerPermissions` filled in `true`, so Patch Settings showed a grant
the server refused. Both now say false and the two agree.

**`createTestNode` now writes the column explicitly**, as `CreateNode` does. A
helper that inherited the column default was not testing the product, and that
is how a follower reading an invite-only patch's private charters survived a
green suite.

`internal/handler/follower_charters_default_test.go` and
`commit_identity_internal_test.go` hold these answers, and
`TestGovernanceCloneNeverFollowsTheChartersGrant` replaces docs/adr/110's
`TestGovernanceCloneFollowsTheChartersGrant`, which asserted the behaviour this
ADR removes.
