# Follower permissions gate taking part, not reading

`follower_permissions` is `{events, proposals, charters, members}` and it
looks like an access-control list. Three quarters of it isn't one.

Only `fp.Charters` is read anywhere in Go — once, in `canReadPatchDocs`,
where it decides whether a follower may open a members-visibility charter
(docs/adr/036). `fp.Events`, `fp.Proposals`, and `fp.Members` are read
nowhere on the server. They are checked in the SPA, where each hides a
workspace tab: `PatchEvents.svelte` and `GovernanceList.svelte` compute a
`permissionDenied` and render a refusal.

The data behind those tabs is public. `GET /api/v1/nodes/{slug}/proposals`
and `GET /api/v1/proposals/{id}` are `AuthOptional`;
`GET /api/v1/proposals/{id}/comments` carries no middleware at all. So a
patch that switches `proposals` off has hidden a tab from its own
followers while leaving the same threads readable by anyone who is not
signed in. The setting made a follower see *less than a passer-by*.

That is the mechanism ADR 044 described as "a **read** gate," reasoning
from the one field that is. The description was already load-bearing:
ADR 042 had cleared a follower-inclusive helper for two more doors on the
grounds that "comments and proposal participation legitimately admit
followers under `follower_permissions`," ADR 044 corrected that by
pointing out no such field exists, and both were arguing about the
reading of a mechanism neither had checked the enforcement of.

docs/adr/049 decided the general form of this while this was being
written: "Patchwork states as fact only what Patchwork enforces." It
swept `GovernanceConfig` and found six fields stored, serialized,
rendered as prose, and read by nothing. It did not sweep
`follower_permissions`, which is a different struct on the same node row
with the same disease — and worse in one way, because these four have a
UI that calls them permissions rather than merely describing them.

ADR 049 resolved its six by enforcing one and retracting the claims of
five. The same two moves apply here, and land differently: `proposals`
is the one worth enforcing, and what gets retracted is not a sentence on
a page but the sense that any of this hides anything.

We decided **`follower_permissions` gates participation and navigation,
not readability**:

- **A patch's public pages stay public, and this setting never changes
  that.** Patchwork reads openly and gates writing — that is the shape of
  the whole product, and a follower-scoped read gate over data the
  anonymous internet already has is not a boundary, it is a costume.
  Making these real read gates would mean pulling proposals, events, and
  member lists out of public reach for everyone, which is a different and
  much larger decision about what a patch is.

- **Switching `proposals` off does mean something, and it is enforceable
  where it means it.** A patch saying "our proposals are not a follower
  matter" is talking about taking part. `CreateComment` now honours it:
  a follower may comment on a patch's proposals unless the patch has
  turned that off. This was the one place the setting could bite and
  didn't — the workspace hid the tab, the API took the comment.

- **Reading is untouched, deliberately, and a test says so.** The
  anonymous read of a comment thread on a patch with `proposals: false`
  returns 200 and is meant to. Gating participation is not a claim that
  the data is private, and the next person to find this asymmetry should
  find it asserted rather than rediscover it as a bug.

- **`charters` keeps working as it does.** It is the odd one out and it
  is the odd one out for a reason: charters carry per-document
  visibility, so there is something real to withhold. The other three
  have nothing to withhold.

**Considered and rejected: enforcing all four server-side.** It is the
reading the name invites, and it is impossible to do coherently. A
follower is a *more* engaged visitor than an anonymous one; a gate that
gives them less is inverted. We would have had to choose between
breaking the public read — which federation, feeds, the quilt, and
cross-quilt following all depend on — and shipping a check that hides a
tab from the only people who have identified themselves.

**Considered and rejected: deleting the three that do nothing.** Tempting,
and it would have been right if they did nothing. They do something small
and real: they set what a patch's workspace offers a follower, which is a
legitimate thing for a patch to decide. What was wrong was the word
"permission" doing duty for "what this rung sees in the workspace."

**Open:** the rules editor still labels the group "Follower Permissions"
with four bare checkboxes, which is the phrasing that made all of this
confusing in the first place. Renaming it is a copy decision, not a code
one, and it wants deciding rather than guessing.
