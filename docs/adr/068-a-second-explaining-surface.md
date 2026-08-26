# A second explaining surface, and the test that keeps it the last one

docs/adr/040 ruled that the About page is "the single standing prose
surface" and that there is no in-app help center, because a help center
"goes stale the moment it is written." It named the alternative:
"training is carried by mechanisms maintained as UI (vocab subtitles,
empty states, the surfaces above), which the same PRs that change
behavior must update."

That rule has a gap, found by writing About's own copy. The new "How does
it work?" section ends by telling an anonymous reader that patches can
elect leaders, vote on policies, and attest to shared values, and offers
to explain further. There is nowhere to send them. The mechanism ADR 040
prescribes cannot serve this reader, because governance is
permission-gated: `canSeeGovernance` on the patch profile hides every
governance surface from non-members, and follower permissions can hide it
from followers too (docs/adr/050). Empty states and vocab subtitles only
teach people already standing in the room. Nobody can learn how a vote
works by using a product that will not show them a vote.

## The distinction ADR 040 was actually drawing

Every other public prose route in the app renders an artifact that exists
independently of the page. `/lining` renders the lining. `/label` renders
the Label. `/privacy` and `/terms` render legal documents. The page is a
viewer; the thing it shows is the thing itself.

`/about` is the sole exception, and that is what made it "the single
standing prose surface": it is the only route that *explains* rather than
displays. The ban was on explanation without an artifact underneath it,
which is the form that rots, because nothing fails when the prose and the
behavior drift apart.

## Decision

**A route at `/governance` explains patch governance to the public, and
it is the second and last explaining surface admitted without meeting the
two-part test below.**

The page is **project-owned and fixed** — no `instance_settings` key, no
admin editor, the lining's model (docs/adr/037) rather than the legal
documents' (docs/adr/028). The line between those two precedents is who
is liable for the claim: privacy and terms are the instance's legal
exposure, so the instance must be able to speak; a description of what
the software does is the project's claim. An admin who could edit it
could describe voting that does not happen, on a platform surface, in the
platform's voice — the exact failure docs/adr/049 found six times.

Its spine is the three axes a patch configures, not a tour of features:
who decides (the electorate), how a decision is made (direct change, a
vote, or elsewhere), and who leads (maintainer, meritocratic, elected).
Feature tours have no natural stopping point and grow a section per
release. Axes change far less often — elections shipped under
docs/adr/051 and slotted into "who leads" without restructuring anything.

It covers **patch** governance only. Quilt-level accountability already
has a surface in the Label, and a page with two subjects is two pages to
maintain.

## The test for any surface after this one

A standing explaining surface is permitted only when both hold:

1. **The subsystem is permission-gated from the reader who needs it**, so
   ADR 040's UI-carries-training mechanism has nothing to offer them.
2. **Its claims are pinned by a test to an enumeration in code**, so the
   prose cannot silently fall behind the behavior.

Governance passes both. A page called "Getting started", "Tips", or "FAQ"
passes neither, which is the point — the test is written to fail the
things ADR 040 was right about.

The second prong is real here rather than aspirational, and it is why the
spine is axes rather than features: axes are enumerated in source and
features are not. `governance-page.test.js` reads `DECISION_OPTIONS` from
`StructuredRulesEditor.svelte` and the leadership `models` map from
`GovernanceOverview.svelte`, and asserts the page names every value in
each. A fifth decision method turns the suite red until the page covers
it.

**Its limit, stated so nobody trusts it further than it goes:** the test
catches *added* values, not changed *behavior*. If elections moved from
approval voting to ranked choice, the enumeration would be unchanged and
the test would stay green over stale prose. Nothing cheap catches that.
What it does catch is the likeliest rot, since governance has grown by
adding options three times running — docs/adr/049 retracted six unread
fields, docs/adr/050 found the same disease in follower permissions, and
docs/adr/051 built the three succession mechanics.

## Considered and rejected

- **Fold it into About.** Honors ADR 040 literally and needs no new
  route. Rejected on length and on framing: About runs ~600 words after
  the rewrite, governance orientation is another 500-700, and doubling
  the page buries the orientation every visitor needs under the depth
  only some want. About's own sentence frames it correctly as an opt-in
  second click.
- **A repo doc linked from About.** Where docs/START-A-QUILT.md correctly
  lives, and it keeps the binary clean. Rejected because the reader is a
  member on a live quilt, not an organizer evaluating a deployment, and
  sending that person to a code host to learn what a proposal is fails
  them.
- **Documentation hosted on a site the project author owns.** Rejected
  on white-label grounds: the About page ships in every binary, so every
  community that seamrips would carry a link to one person's site in
  their own public copy. A project whose pitch is that no company stands
  behind it cannot ship a link to a company.
- **An admin-editable page with shipped defaults**, the legal-documents
  pattern. Rejected above — it hands instances the ability to narrate
  platform behavior they do not control.
- **No page, and cut the sentence from About.** The cheapest option and
  genuinely on the table, since ADR 040 would stay untouched. Rejected
  because the gap is real: governance is the most valuable thing
  Patchwork does that a prospective member cannot see before joining, and
  refusing to explain it protects a rule at the expense of the reader it
  was written for.
