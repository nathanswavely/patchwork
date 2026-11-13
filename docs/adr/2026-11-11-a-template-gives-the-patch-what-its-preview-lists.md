# A template gives the patch what its preview lists

**Status:** accepted, 2026-11-11

## Context

The template preview at patch creation lists the documents a template
includes. For Formal it said "Included documents (6)", and a patch made from
Formal had one: the lining. The governance simulation filed this as F-018.
Two founders looked for the five that never arrived.

The cause is a split between two stores. `ForkForNode` wrote the template's
documents (charter, bylaws, financial transparency, conflict resolution,
succession plan, and the smaller templates' operating agreements) into the
patch's git repo, and nothing wrote them into `governance_docs`. The table is
the canonical store and the repo its mirror (docs/adr/011, docs/adr/084), so
every surface that lists a patch's documents read the table and found the
lining alone.

The documents were also wrong. Read against the mechanics the product now
runs, the Formal bylaws described ranked-choice voting, a nomination window
that closes a week before the election, and recall by petition; the
succession plan described a campaign phase, an emergency election with
compressed timelines, and three interim admins for an emptied council. None
of those exist. Delivering the documents as written would have made the
patch's own governing text the most authoritative wrong description of it.

## Decision

**First the text was audited, claim by claim, against the code.** Every
sentence that says how Patchwork behaves was checked under its own
template's rules, and each one the code does not keep was rewritten to say
what the code does. Sentences that state a community's own policy, which
the software never claims to run (spending tiers, mediation steps, a
45-day council conversation), were left alone unless they contradicted a
mechanic. The corrections:

| Document | Said | Now says |
|---|---|---|
| Bylaws, Succession Plan | ranked-choice voting | approval voting; most approvals win, a candidate needs one, half the electorate must take part |
| Bylaws | nominations close 7 days before the election | 14 days of nominations, then voting opens |
| Succession Plan | days 15 to 28 campaign, 29 to 42 voting, seated day 43 | days 1 to 14 nominations, 15 to 28 voting, seated when voting closes |
| Bylaws, Succession Plan | (silent) | holdover: an election that settles nothing leaves the sitting council |
| Succession Plan | council appoints to a vacancy, community ratifies in 14 days | an admin nominates, members confirm, the nominee serves out that seat's term |
| Succession Plan | emergency election with 7-day phases | empty seats are filled by nomination or at the next election |
| Succession Plan | three longest-tenured members become interim admins | an emptied elected council's election opens shortly, any member may stand (interim offers are the maintainer model's, docs/adr/2026-10-19-an-interim-admin-is-asked-first.md) |
| Bylaws, Succession Plan, Financial Transparency (Formal) | consensus, else two thirds with 50% quorum | consensus only: nobody rejects, one approves, half take part |
| Financial Transparency (Formal) | budget by majority with 50% quorum | Formal decides every proposal by consensus |
| Bylaws | removal by supermajority with a 10% petition | any member may propose a removal; if it passes, the council removes the role |
| Bylaws | council quorum and council votes, stated as mechanics | stated as the council's own practice; Patchwork holds no council-only vote |
| Bylaws | membership data kept 30 days in case of return | a member who rejoins starts as a new member and waits the 30 days again |
| Charter | an elected council of up to 7 | as many seats as the council has set up (`max_admins` caps nothing, docs/adr/051) |
| Charter, Bylaws | 30-day voting tenure | the same, plus docs/adr/098: no wait in the patch's own first 30 days |
| Operating Agreement (Collaborative) | a majority of active members must approve a nomination | more approvals than rejections, with a quarter of the electorate taking part |
| Operating Agreement (Collaborative) | inactive admins asked to step down at 60 days | reminded at 60 days, role ends at 120 |
| Operating Agreement (Collaborative) | any admin can step down | except the last, until another is confirmed |
| Operating Agreement (Casual) | admins approve changes | votes decide; admins carry out what passes |
| Operating Agreement (Casual) | (silent) | the 90 and 180 day inactivity clock, the named successor, and the interim offer |
| Operating Agreement (Casual) | leave at any time | the last admin names a successor first |
| Casual, Collaborative, Formal | the patch is open / requires approval | who can join is set in the governance rules, since the creation form's answer overrides the template's |
| Conflict Resolution (both) | temporary suspension | removal, which an admin can reverse; Patchwork's ban has no end date |
| Formal follower lines | followers "participate in discussions" | followers cannot vote (what else they may do is a setting, and a document should not restate a setting that can change under it) |

`TestTemplateDocumentsMakeNoClaimTheCodeDoesNotKeep` holds the refuted
phrases out, each with the reason, so a claim can only come back with the
mechanic it describes.

**Then each document is a row from the moment the patch exists.**
`CreateTemplateDocs` inserts one `governance_docs` row per template document
at creation and at claim setup (docs/adr/039), after the lining.

- **Members-only.** A template's text is a starting point the patch has not
  yet said it means, so it gets the default every new charter gets
  (docs/adr/036). Publishing stays a deliberate act at Governance, Documents.
- **The lining is untouched.** It is still made by `CreateDefaultLining`,
  public, `kind = 'lining'`, from the shipped lineage (docs/adr/037).
- **One text, two stores, equal from the first commit.** The documents are
  now title and body pairs in `internal/governance/defaults.go`, with no
  top-level heading in the body for the reason the lining has none: the title
  lives on the row. The fork writes each body to `Filename(title)`, so the
  git file and the row are byte-for-byte the same. Where the fork fails, the
  rows stand and the startup repair builds the repo from them.
- **The preview is the list.** `GET /api/v1/templates/{id}` returns the
  lining and the template's documents in the order the Documents page lists
  them, with the titles and bodies the patch is born with. It used to walk a
  map, so the order also changed between loads. The Documents list breaks
  same-millisecond ties by `rowid`, so the order written is the order shown.

**Patches that already exist are not backfilled.** They never had these
documents on their shelf. Nobody there read them, adopted them or published
them, and a document appearing now would be an act on the patch's record
that nobody took, on a shelf that belongs to the community. That is the line
between these and the lining, which is project-owned and updates itself by
design (docs/adr/037). It is also the text: backfilling from each repo would
publish the very claims this record corrects as that patch's governing text,
and backfilling the corrected text would hand a community words it has
never seen in place of the ones it was shown. A patch that wants one adds it
at Governance, Documents, where the corrected text is a preview away.

## Consequences

- A Formal patch is born with six documents and its Documents page says so.
  The preview's count is a promise the creation path keeps, tested for every
  template.
- Older patches keep the template files in their repos, as files no row
  mirrors. A clone shows them; the canonical list does not, and nothing in
  the product reads them. If such a patch later creates a document with the
  same title, the database to git mirror overwrites the file, which is the
  right outcome. Removing them would be a commit nobody made, and the repair
  pass never deletes (docs/adr/084).
- `cmd/seed` still forks templates into git without rows; its patches carry
  their own fixture charters and are in the same position as older patches.
- `internal/notifications/interim_offers.go` still cites the old succession
  plan's "three longest-tenured active members" as the source of
  `interimSeats`. The number is right for the maintainer model; the sentence
  it quotes is no longer in any shipped document.
- The audit found one mechanics question it could not settle from the text:
  leaving or being demoted may not empty a seat on an elected patch. That is
  a question about seats, recorded for the session that owns them, and no
  document here makes a claim that depends on it.
