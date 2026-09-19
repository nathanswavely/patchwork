# ADR 114: A patch may ask for a word

Date: 2026-09-16. Status: accepted. Amends docs/adr/021.

## Context

ADR 021 settled that tags are the only classification system and that the
vocabulary is instance-curated: patch admins pick from the list, unknown
names are rejected at the API boundary, "never auto-created."
`resolveTagIDs` enforces that at four call sites, and CONTEXT.md said flatly
that only instance admins change the list.

The holding was right and the mechanic has been quietly failing for two
months. Migration 026 backfilled 18 tags. The live Lancaster vocabulary now
carries 49, with `brewery`, `coffee`, `drinks`, `piercing`, `tattoo`,
`podcast` and `horticulture` added between 2026-08-16 and 2026-09-16. Every
one of them arrived the same way: somebody wanted a word, could not add it,
said so out of band, and the instance admin typed it into the Add tag form.

The curation is worth keeping. The out-of-band step is not. It costs the
admin a conversation per word, and a person creating a patch at 11pm does
not have that conversation: they pick the nearest wrong tag or none at all,
and a patch with no tags is a patch with nothing but people-overlap to place
it, which for a brand-new patch is nothing at all. ADR 021 added the
placement term specifically for that cold-start case, and an untagged patch
gets no benefit from it.

This ADR keeps the decision and changes the route: the person who wants the
word proposes it in the moment, and the instance admin still decides.

## Decision

**A suggested tag is a pending row in the one vocabulary, not a second
table.** `tags` gains `status` (`approved` | `pending` | `rejected`) and
`suggested_by`. `node_tags` is untouched: the proposing patch wears the word
from the moment it is proposed, and the pending-ness is a property of the
word, not of the attachment. The user-facing term is **suggested tag**
(CONTEXT.md); "tag submission" is an avoid word, because in this codebase a
person submits patches and events (docs/adr/026), which are content they
will own, and a word in shared language is owned by nobody.

**One vocabulary, one decision.** Two patches proposing the same word share
one row, and approving it publishes on both at once, including the one whose
admin was never asked. What is under review is a *word*, not whether a given
patch is honestly describing itself. The second question is moderation of
patch content, with its own queue and its own owner.

**"Tags" means approved everywhere.** `GET /nodes/{slug}` keeps `tags` as
the approved, public array and adds `pending_tags`, sent only to that
patch's admins: not to members, not to followers, not to an instance admin
holding no role there. A pending word does not filter, does not derive a
motif, and contributes nothing to the placement affinity term, because two
patches pulled together by a word that is later declined would spring apart
for a reason no viewer could see.

**No client can destroy a suggestion by omission.** `setNodeTags` scopes its
delete to approved tag ids. `PatchSettingsInfo.svelte` spreads `node.tags`
back into a wholesale PATCH, so without that scoping a patch admin editing
tags for any unrelated reason would silently delete their own pending
suggestion. Withdrawing one is an explicit `DELETE`, never a side effect.

**Suggestions travel on their own request field**, `suggest_tags`, alongside
`tags`. An unknown name inside `tags` stays a 400 at every call site. If an
unknown name meant "suggest this," a typo would coin a word: `visualarts`
would stop being an immediate, fixable error and become queue noise plus a
chip nobody meant. `POST /api/v1/submissions` keeps rejecting outright: a
stranger proposing an unclaimed patch has no stake in the vocabulary, and
unclaimed patches have no admins, so the pending chip would have nobody to
appear to. The instance admin already sets tags when approving that
submission.

**Names are normalized server-side on every write path, the admin's
included**, and `tags.name` gains `COLLATE NOCASE`. Normalization is trim,
NFC, whitespace to hyphen, lowercase, trimmed hyphens, 32 characters, and it
is not ASCII-only: a Spanish or Japanese quilt needs its own alphabet, and
ADR 021's white-label reasoning forbids baking Lancaster's in. Until now the
only lowercasing anywhere was `newName.trim().toLowerCase()` in the admin
page's client code, so `Music` could sit beside `music` in a column whose
`UNIQUE` constraint looked like it prevented exactly that.

**Rejecting spends the word.** `status = 'rejected'`, pending attachments
deleted, and a later suggestion of the same name is refused with a sentence
that says an admin already declined it, not a generic conflict. One
rejection settles a word; it does not need re-deciding every week.

**The admin's own Add tag form resurrects.** `POST /admin/tags` on a
rejected or pending name upserts it to approved instead of returning 409.
Without this, an admin who declined `zine` in January and needs it in April
gets "tag already exists" for a tag that is visibly absent from their own
vocabulary page: the UI and the constraint disagreeing, with only the
constraint right. Deleting a tag keeps the meaning ADR 021 gave it, strips
it from every patch, and frees the name to be suggested again. Delete and
reject are different verbs with different aftermaths, and the queue never
uses delete.

**Approving may rename, so approving may merge.** The admin can retype
`Live Music` as `live-music` before approving, which can land on an approved
tag, a rejected one, or another pending one. All three merge: re-point the
attachments, respect `node_tags`'s composite key where the patch already
wore the target, keep the existing position, drop the emptied row. The same
path serves the suggest side, where a proposed name that normalizes onto an
existing approved tag is not a suggestion at all but an ordinary pick,
attached immediately with no queue entry.

**The queue travels in an admin seamrip and not in a member one.** ADR 002
calls the admin export a custody transfer, and the pending queue is instance
state like any other; a fork that dropped it would silently promote or lose
in-flight words. `seamrip.go`'s `SELECT id, name, motif, created_at FROM
tags` must gain `status` in the same commit the column exists, or every
pending and rejected row lands on the fork as live vocabulary. The member
seamrip carries approved rows only with `suggested_by` nulled, and its rule
string in `memberview.go`, which justified carrying every tag on the grounds
that "the tag list is a public read," is corrected: that is true of approved
rows and false of the two new states.

## Considered options

- **Per-patch suggestion records in their own table** (approve the word, but
  strip it from this patch): rejected. It buys the admin one extra power at
  the cost of a second table, a second write path on approval, and a
  rendering source for pending chips that is not `node_tags`. Curating a
  word and judging whether a patch describes itself honestly are different
  jobs; the second is a report.
- **Unknown names in `tags` become suggestions**: rejected. A typo coins a
  word, and the `tags` array would behave differently depending on which
  endpoint received it, which is the request-side version of the ambiguity
  the split `tags` / `pending_tags` response exists to avoid.
- **Reject deletes the row**: rejected. The name frees up, the same person
  proposes it again tomorrow, and the admin re-reviews it forever while the
  suggestor is never told why their chip keeps evaporating.
- **Keep the hard rejection and curate out of band**: rejected, but it is
  the status quo and it does work, slowly. The 49-tag vocabulary is evidence
  both ways: the words do arrive, and every one of them cost a conversation.
- **An instance toggle**: considered and declined. The approval step is
  already the control, and a setting invites the argument back every release
  (docs/adr/093's reasoning about event notifications, applied here).
- **Suggestions from the patch submission path**: rejected. It nests a
  pending thing inside a pending thing: rejecting the patch orphans the
  word, approving the patch but not the word leaves nobody to tell, and the
  proposer is not an admin of anything, so the feedback loop this design
  runs on does not exist for them.

## Consequences

- **No spam cap ships, on purpose.** The queue is admin-only and the blast
  radius before approval is one chip on the proposer's own patch, so the
  cost of abuse is queue depth and an admin reading something unpleasant. If
  that happens, the cap is a count of outstanding pending rows per account,
  and this paragraph is the one to delete. A wordlist filter is not the
  answer and never was: the admin is the filter.
- `ListTags` is unauthenticated, so its approved-only filter is a disclosure
  boundary rather than a cosmetic one, and so is the `node_count` subquery.
- A rejected word is invisible on the vocabulary page while still occupying
  its name. An admin who does not know about the resurrect path will
  experience Add tag succeeding on a word they do not remember declining.
- `suggested_by` joins `users`, so every read of it goes through
  `displayNameExpr` / `usernameExpr`. Writing the COALESCE by hand leaks a
  retired handle into the admin queue and nothing fails (docs/adr/086).
  Account deletion does not remove suggestions: a coined word is a
  vocabulary act, not the person alone, and by then it may be worn by a
  dozen patches. A pending suggestion whose only proposer deleted their
  account stays in the queue, tombstoned, and the admin still decides.
- **The boundary guards held, and they are finer-grained than the
  table-level ones this ADR was drafted against.** Adding four columns to
  `tags` failed three tests before a line of this was reviewed:
  `TestEveryColumnHasABoundaryDecision` demanded a travel decision for each
  new column, `TestEveryTableHasAMemberViewRule` refused a member-view rule
  naming a column the table does not export, and
  `TestEveryPersonalTableHasADeletionRule` noticed that `suggested_by` makes
  `tags` a table holding rows about a person and demanded the
  purged/kept/emptied answer (it is kept, above). The lesson is the opposite
  of the one drafted here: a column that changes what a table means does not
  slip past, and the place to record the reasoning is the rule itself.
- Three notification types join the event-submission triad they are modelled
  on: `admin.tag_suggestion`, `tag.suggestion_approved`,
  `tag.suggestion_rejected`. The approval notice carries both names, because
  telling someone a word they never typed was approved is worse than
  silence.
- ADR 021 is amended, not superseded. Its holding stands in full: one
  classification system, an instance-curated vocabulary, nothing
  auto-created. What changes is the route by which a word reaches the admin
  who decides.
