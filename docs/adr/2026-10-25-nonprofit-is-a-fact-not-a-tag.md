# Nonprofit is a fact about a patch, not a tag

**Status:** accepted, 2026-10-25

## Context

Issue #448 asked that a patch be markable as a nonprofit, both by whoever
suggests a listing and by the people who run a patch.

The quilt could already do this without any code. An instance admin adds
"nonprofit" to the vocabulary, and both the suggestion form and Patch Settings
offer it in the tag picker. docs/adr/021 says tags are the only
classification, so that would be the default answer.

It is the wrong one, because the quilt acts on tags:

- **Tags place patches.** Every shared tag adds to placement affinity
  (`internal/handler/tree.go`), so every nonprofit would drift toward every
  other one. A nonprofit theatre and a nonprofit food bank are not alike
  because they file the same paperwork.
- **Tags draw tiles.** The first tag with a motif becomes the patch's icon.
  "Nonprofit" would compete for that spot with the words that say what a
  patch actually *is*.

## Decision

**`nodes.nonprofit` is a boolean column, and nothing in placement or motif
derivation reads it.**

ADR 021 still holds. It is about how patches are classified, meaning what
kind of group each one is, and this column doesn't classify. It records how
a group is constituted, the same kind of fact as `founded_at` (docs/adr/098).
It is not a second taxonomy.

- **Self-declared.** Patch admins set it at creation and at Settings → Info.
  A community suggestion carries the suggester's answer. The instance admin
  can correct that answer at approval, as they correct the suggester's tags,
  and a claimant sees it pre-filled at setup. Nothing verifies it, and the
  badge's tooltip says who made the claim: the patch's admins, or whoever
  added the listing.
- **Jurisdiction-neutral.** The word is "nonprofit", not 501(c)(3), because
  the quilt is white-label.
- **Public.** The detail, list and tree payloads all carry it. The tree has
  it so a discovery card can show it, for the same reason the tree carries
  `moved_to`.
- **Travels in a seamrip,** as a fact about the community. Archives written
  before the column import with 0.

## Considered options

- **A tag:** rejected, for the two reasons above.
- **A tag the affinity and motif code skip:** rejected. A tag that the quilt
  ignores is a column with extra steps. It would also make "tags place
  patches" false for one word, and only someone who read the code would know.
- **A registration number (EIN, charity number), with verification:**
  deferred. It is a larger question, about what a quilt vouches for and
  under which country's register, and nobody asked for it.

## Consequences

- There is no filter for it yet. Narrowing the quilt or map to nonprofits is
  a follow-up, and the tree already carries what such a filter would need.
- It does not federate. The AP actor carries identity, and adding this to
  it is a separate decision.
