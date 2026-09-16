# 110. A clone takes the whole shelf

**Status:** accepted, 2026-09-16

## Context

docs/adr/036 gave every governance doc its own visibility — `public` or
`members`, and `members` is what a new doc is born as. Publishing is a
deliberate act. The REST layer keeps that promise row by row:
`ListGovernanceDocs` appends `AND visibility = 'public'` for anyone who is not
being handed the whole shelf, and a withheld doc contributes no row and no
number a visitor could subtract to learn it exists.

The git smart-HTTP transport kept no such promise. `GET
/api/v1/nodes/{slug}/governance.git/info/refs` and `POST
.../git-upload-pack` were mounted with no auth middleware at all, resolved the
slug through `NodeIDFromSlug` — which answers for any active or unclaimed
patch, private ones included — and then opened the bare repo and advertised
every ref. Meanwhile `DirectEdit` mirrors a doc into that repo on every
content change **regardless of its visibility**. So one unauthenticated
command took every members-only charter of any patch whose slug you could
guess, with its full revision history and its diffs:

```sh
git clone https://<instance>/api/v1/nodes/<slug>/governance.git
```

This was found in an outside source-level review on 2026-09-16 and confirmed
here before the fix. It was latent rather than live — the build running on
lancasterpatchwork.org predates the transport, and the route 404s there — but
the next deploy is what would have shipped it.

**The bodies are not the whole of it.** A commit in these repos is authored
`DisplayName <username@patchwork.local>`. A clone therefore also names who
edited what and when — which is exactly the question the per-membership
visibility switch (docs/adr/006) and the tombstone rule (docs/adr/086) each
exist to govern, and neither of them can reach inside a packfile. A retired
username lives in git history for ever, where `displayNameExpr` has nothing to
substitute.

## Decision

**The transport is gated on the whole-shelf rule, not the per-document one.**
`GitHTTPHandler` no longer takes a resolver; it takes an authorizer,
`handler.GovernanceRepoNodeID`, which resolves the slug and then asks
`canReadPatchDocs` — the same function that decides whether a viewer is handed
the members-only shelf in the REST listing. One rule, stated once, asked at
both doors.

There is no half-clone, so there is no caller who may be handed half of one.
A repository cannot be filtered the way a listing can: git serves objects, and
the objects are the history. Trying to serve a subset means building a
different repository, which is the next section.

**Resolution and refusal are one callback returning one answer.** "No such
patch" and "not for you" both come back as `""` and leave as the same 404 with
the same body. Two answers would be an oracle for the existence of a private
patch, which is the thing private is for.

**A follower is inside or outside by the patch's own answer** —
`follower_permissions.charters` (docs/adr/050) — because that is what
`canReadPatchDocs` already means. The transport keeps no second opinion about
followers.

**Anonymous clone is gone, on purpose.** Nothing in the tree consumed it: no
UI advertises a clone URL, no federation path clones a remote governance repo,
and the deployed build never served one. A public charter is already readable
over REST and over ActivityPub. What anonymous clone offered on top of that
was the history and the authorship — the two things this ADR is about.

## Consequences

A member clones with their session cookie, which `git` will carry if told to:

```sh
git -c http.extraHeader="Cookie: patchwork_session=<token>" clone https://<instance>/api/v1/nodes/<slug>/governance.git
```

That is a member's affordance, not a pleasant one, and it is the honest state
of the route until the mirror below exists.

**What this does not fix, and what would.** The gate closes the disclosure; it
does not make the repo publishable. A public mirror is the shape that would:
`governance.Repair` already rebuilds a repo from the canonical
`governance_docs` rows (docs/adr/084), so a second, public-only repo built the
same way — holding only `visibility = 'public'` docs, authored by a neutral
committer rather than by the people who edited them — could be served
anonymously without contradicting docs/adr/006 or docs/adr/086. It would need
a rebuild when a doc's visibility changes, and it would give a fork's egress
story (docs/adr/012) a public door that does not depend on a session cookie.
Backlog, not built.

**Mirroring stays as it is.** Members-only docs keep going into the repo, so a
patch's own history stays whole for the people whose history it is. The
boundary is at the door, not at the write.

`TestGovernanceCloneIsRefusedToEveryoneOutsideTheRoom` and its neighbours in
`internal/handler/governance_git_transport_test.go` run the transport the way
`main.go` mounts it and hold every one of these answers in place.
