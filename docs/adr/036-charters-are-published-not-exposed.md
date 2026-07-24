# Charters are published, not exposed — per-document visibility

Governance docs were world-readable with no switch and no session:
`GET /api/v1/nodes/{slug}/governance` and `GET /api/v1/governance/{id}`
were mounted bare, so every charter a patch had ever saved — including
the auto-created default lining a patch gets at creation, before anyone
has read it — was public the instant it was written. The patch-level
`nodes.visibility` didn't cover it: a public patch is the normal case,
and it published the patch's rules along with its name.

That is backwards for how communities actually write rules. A charter
is drafted, argued over, and half-finished for weeks before it is
something the community wants strangers reading. Publishing should be a
thing a patch *does*, not the state a document is born in.

We decided **each governance doc carries its own visibility**, defaulting
to members-only.

- **Two values, `public` and `members`** (`governance_docs.visibility`,
  migration 036). Not a third "unlisted" tier: the reason to hide a
  charter is that it isn't finished, and a link-shareable draft would
  reintroduce exactly the accidental publication this fixes.
- **New docs default to `members`; rows that already existed are pinned
  to `public`.** Those were public under the old rule and communities
  may already be pointing people at them; silently retracting live
  charters would be its own surprise. The default governs what comes
  next, not what shipped.
- **"Members" means the patch's admins and members**, plus instance
  admins, plus followers when the patch's `follower_permissions.charters`
  is on — the same knob the workspace UI already reads, so the two
  surfaces can't disagree. Signed-out visitors never qualify. The four
  read endpoints (list, get, versions, diff) moved from bare mounts to
  `AuthOptional` to have a viewer to check.
- **A hidden doc 404s rather than 403s.** That a patch is drafting a
  code of conduct is itself part of what it hasn't published.
- **Only public docs federate.** The AP object at `/ap/governance/{id}`
  filters on `visibility = 'public'`, and the `Update` broadcast on edit
  is skipped for members-only docs. The fediverse has no session to
  check a membership against, so members-only is off the wire entirely.
- **Amendment proposals inherit the target charter's visibility for the
  document text only.** Proposals are a public read, and an amendment
  carries `proposed_body` — the full new text of the doc it targets — so
  without this a members-only charter is one amendment away from being
  world-readable. `proposed_body`, `proposed_title`,
  `current_doc_content`, and revision snapshots are withheld from
  viewers who can't read the charter (`doc_text_hidden` tells the UI
  why). The proposal's own title, the proposer's rationale, and the vote
  tally stay public: that is what the author posted, knowing proposals
  are public deliberation.
- **Flipping visibility is not an amendment.** The text didn't change,
  so it earns no version bump, no git commit, and no notification — just
  an audit entry (`governance.visibility`). A version history where half
  the entries are "someone toggled a switch" is a worse history.

The control lives where the documents do: Governance → Documents, one
Publish / Make members only button per row, with the current state as a
chip. Not in Patch Settings — an admin decides this while looking at the
document, not while editing the patch's address.

Alongside it, the patch-level visibility control in Patch Settings was
rewritten from a bare Public/Private select to two labelled choices that
each state their consequence, plus the caveat the old control implied but
never kept: private keeps a patch from being *found* — off the quilt,
search, the map, public feeds, and federation — but anyone holding a
direct link can still open its page. Naming a state is not the same as
saying what it does.

Consequences: a fresh patch's default lining is now members-only, so a
community that wants its anti-discrimination baseline visible to
newcomers has to publish it — deliberate, but one more step at a moment
when the admin has many. The governance overview's document count now
counts only what the viewer can open, so the same patch reports
different counts to a member and a visitor; that is the honest number,
since a count of unreachable docs reads as a broken link. Seeded demo
charters are inserted `public` explicitly — demo data exists to be read
by a signed-out visitor.
