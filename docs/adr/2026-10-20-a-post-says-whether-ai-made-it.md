# A post says whether AI made it

Date: 2026-10-20. Status: **accepted**; implemented 2026-10-29 (migration
`20261029T133533_ai_disclosure.sql`, `internal/handler/ai_disclosure.go`). The
visible copy waits on the maintainer's review, as "Not decided here" says.
Paired with
docs/adr/2026-10-20-the-site-says-where-ai-is.md. Applies docs/adr/082's
confirm step (nothing is accepted by silence), docs/adr/037's badge and
per-user discovery filter, docs/adr/007 (an image is a reference), and
docs/adr/039 (an unclaimed patch carries no governance).

## Context

The reference instance's policy is transparency: content made with AI is
welcome when it says so, and not welcome when it does not. That covers
prose, images and music. Patchwork hosts no audio, so music reaches it only
as an event or as a patch whose work it is.

A policy nobody can act on is a wish. Members need a way to say what they
made with AI, readers need to see it, and a report needs something to point
at when a post that obviously came from a model claims nothing.

The obvious mechanism is a checkbox reading "I used AI". It has three
problems:

- **Unchecked is an answer by silence.** Everyone who scrolls past it has
  claimed their post is human-made. docs/adr/082 refused the same shape for
  map markers.
- **Two answers are too few.** A person who writes in another language and
  runs it through a translator, or leans on a grammar tool, would have to
  choose between the label a generated newsletter wears and a lie.
- **Not everything here was written here.** Events from calendar feeds, the
  CSV upload and federated posts were written elsewhere. Nobody on this site
  can answer for them.

Separately, some patches *are* AI work: a newsletter a model writes, a music
project made with generators, an event aggregator that has a model write its
listings. The policy welcomes them. A visitor should know which they are
before following, without reading every post.

## Decision

**1. An instance switch, `ai_disclosure`: `off`, `optional` or `required`.**
Stored in `instance_settings`, set at Admin → Settings, rendered into the
paired page's project part. The software ships `optional`: the question
appears everywhere it applies and blocks nothing, because the question costs
little and transparency is a project value. The reference instance sets
`required`. In `required` the server refuses a create without an answer (a
400 naming the field), so an API client cannot skip what the form asks.

**2. The question has three answers, and none is preselected.**

| Value | Form label | Shown to readers |
|---|---|---|
| `none` | No AI | nothing |
| `assisted` | AI helped (editing, cleanup, translation) | nothing |
| `generated` | AI made this | a "Made with AI" tag |
| `NULL` | not asked, or not stated | nothing, or provenance (see 5) |

`assisted` is recorded and not shown. It exists so that an honest person
using a translator or a grammar tool has a true answer that is not the
generated label. The policy is about what a model *made*.

**3. Images are asked separately.** Patches and events carry an image as a
URL plus required alt text (`internal/handler/image_ref.go`). The image gets
its own `image_ai` answer beside the alt text, with the same three values,
because a flyer's artwork and its description are often made by different
people and tools. Patchwork never fetches the image, so the answer is the
only thing anyone here can know about it.

**4. Asked on the surfaces that publish, not on every text box.**

- events: description (`events.ai_use`) and image (`events.image_ai`)
- patches: description (`nodes.ai_use`) and image (`nodes.image_ai`),
  at creation, in Patch Settings, and on the suggestion form, where the
  suggester answers for what they typed
- proposals, including amendments: the proposed text (`proposals.ai_use`)
- direct charter edits: recorded as an `AI-Use:` trailer on the commit,
  shown in the charter history view, since a charter's revisions already
  live in git
- notices (`notices.ai_use`)

Comments, replies, join messages, bios and admin-only fields are covered by
the written policy and carry no question. A required question on every reply
becomes reflexive clicking, and a label clicked reflexively means nothing
anywhere.

**5. Content written elsewhere carries provenance, not a label.** Events from
an event source, from the CSV upload, and from federation are stored with
`ai_use` NULL and show where they came from ("From Tellus360's calendar"),
which they already do for event sources. The CSV format gains an optional
`ai_use` column for an uploader who knows. Nothing here asserts on behalf of
a calendar it did not write.

**6. A patch may declare that its work uses AI.** `nodes.ai_work`:
`NULL` (not stated), `0` or `1`, asked at creation and in Patch Settings as
*Does the work this patch makes or publishes rely heavily on AI-generated
content?* with examples (a newsletter written by a model, generated music or
art, listings a model writes). A `1` puts a public badge on the patch page
and its cards. As with the amended-lining badge, each person may hide
declared patches from their own discovery (`users.hide_ai_work`). There is
no instance-wide hide: the reference policy welcomes labelled work, and an
instance that wants to exclude it is writing a different policy, which its
moderation can enforce. The quilt draws no mark for it: the quilt shows how
people connect, and this would make it a map of who to avoid.

The declaration does not replace per-post answers. A declared patch's posts
are still asked, and nothing is pre-filled from the declaration.

Unclaimed patches carry no declaration (docs/adr/039: they carry no
governance either). Claiming asks the question. Changing the answer is
audited as `node.ai_work_set`, so a patch cannot quietly drop the badge.

**7. Answers are honor-system, and the report gives a false one a
consequence.** A new report reason, `unlabelled_ai`, applies to every surface
in 4 and to patches. The first moderation action is `label`, which sets the
answer to `generated` and records who did it, the way `reset_appearance`
corrects rather than punishes. Removal is for repeat or deliberate cases and
is the moderator's call, not a rule. On the noticeboard the patch's own
admins act, as for every noticeboard report.

**8. The answer travels with its row.** It is pre-filled on edit, included in
the personal export and both seamrips (columns travel with their tables), and
written into the content of a federated `generated` post as a closing line,
since ActivityPub has no field for it.

## Consequences

- Readers can tell what a model made, and a member who uses a translator is
  not marked for it.
- `required` adds one question to five forms on the reference instance. That
  is the cost, and it is paid where something is published.
- Existing rows are NULL and show nothing. Nobody is retroactively labelled,
  and on a `required` instance the question appears the next time a row is
  edited. Patch admins see a one-time prompt in the workspace to answer the
  patch-level declaration.
- Every surface in 4 gains a column, so every form that spreads a payload
  back into a submission must send it, or an unrelated edit resets it.

## Not decided here

- The exact visible copy ("Made with AI", the badge text, the question
  wording). The reference instance's maintainer writes it, and the strings
  enter the ledger as `human`.
- Whether `assisted` should ever be shown, for instance on an item's own
  page. Recorded so the answer can change without a migration.
- Bios. Left to the written policy for now.
