# 117. A follower is not a quieter member

**Status:** accepted, 2026-09-17

## Context

Follower and member are different relationships. CONTEXT.md says so, the
contributor ladder says so, and `member_count` counts admins and members and
never followers. Three surfaces said otherwise, and together they let a patch
admin hand a follower the admin role without that person ever asking to join.

Found in use, on the live build, by clicking Follow on a patch and then finding
the follower row in Patch Settings with an editable role dropdown on it.

**The listing had no notion of category.** `GET /nodes/{slug}/members` returned
every active row. For an outsider that was already filtered to visible
member and admin rows, because a follower relationship is never public
(docs/adr/006), so nobody outside noticed. For an insider the roster was the
membership plus the followers, under a heading that said Active Members, above
a count that excluded them. The page said 2 rows and 1 member and both were
"right".

**The node payload had the same slip in a flag.** `is_member` was set for any
active row, followers included. Six components had each written their own
guard against it:

> Not `isMember`: the node payload sets is_member for followers too

Six guards and one miss is the shape of a wart that should have been fixed at
the source. The miss was `ProposalDetail`, which passed the follower-inclusive
flag into the comment gate. That one happened to be *correct* — commenting is
the one governance act a follower holds (docs/adr/044) and `CreateComment`
names all three roles — but it was correct by accident, resting on a flag that
meant something wider than its name.

**And the role control never checked the person.** `UpdateMember` validated
that the new role was one of three words. It never validated the transition, so
`follower → admin` was a 200. Two other paths that make an admin both refuse a
follower by name:

- `isActivePatchPerson`: "A follower is never eligible to hold a seat — admin
  is a rung on the member ladder, not a role handed sideways to an observer."
- `validateNomination`: "a nominee must be an active member of this patch"

So on a meritocratic patch, nominating a follower was refused while the
dropdown did it silently. That is docs/adr/100's failure exactly, one rule
over: a control that quietly outranks the mechanism the governance page
advertises.

## Decision

**"/members" means the membership.** The listing returns admins and members,
the same two roles `member_count` counts. `?role=follower` asks for followers,
and `member` and `admin` are accepted as exact filters. An unknown value falls
back to the membership rather than 400ing, matching how `statusFilter` treats a
typo: a listing is a read, and the safe reading of a word we do not know is the
narrower one. The outsider clause is unchanged and still ANDed on top, so
`?role=follower` hands an outsider nothing.

**`is_member` means the membership.** A caller that wants "has any standing
here" reads `membership_role`, which is set for every active row and empty for
everyone else. `ProposalDetail`'s comment gate now says `membershipRole` is
non-empty, which is what it always meant.

**A follower is not promoted; a follower joins.** `UpdateMember` refuses any
transition out of `follower`. The reasoning is already in the function one
relationship over, for invited rows (docs/adr/098): the person has not said
yes, and a role set here is what they would become without anyone having asked
them. The two paths that make a member both put the question to the person, and
this control was the only one that skipped them. The refusal names both:
invite them, or let them use "Become a member".

**Demotion is untouched.** An admin may still drop a member to follower.
Ending a relationship needs nobody's consent the way starting one does, which
is the asymmetry the last-admin floor and meritocratic demotion already run on.

**Patch Settings lists the two apart**, followers with no role control and a
line saying why. The server is what enforces this; the page is the same rule
where a person can see it, because a UI fix over an open API is what let this
exist.

## Consequences

**An insider's public member list no longer includes followers.** That page is
headed by a member count that never counted them, so the list now agrees with
its own header. Followers remain counted, in the "N following" figure beside
it. An outsider sees no change at all.

**A member still cannot change their own role**, which was true before this and
is now tested. It rests on two gates: `UpdateMember` requires patch-admin or
instance-admin, and `UpdateMyMembership` decodes only `visible`. Neither had a
test saying it was deliberate.

**What this does not close.** An instance admin can still promote themselves on
any patch, because `UpdateMember` accepts `user.Role == "admin"` and the node
payload sets `is_admin` for them. That is the bypass docs/adr/115 exists to end
and deliberately defers. It is how this was found, and it is untouched here.

`internal/handler/follower_is_not_a_member_test.go` holds each of these.
`TestRoleChange` lost its `follower → admin` leg, which asserted the behaviour
this removes, and `TestPublicMemberListHidesHiddenAndFollowers` gained a case
proving an insider can still reach followers one listing over.
