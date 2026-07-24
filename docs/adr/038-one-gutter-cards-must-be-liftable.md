# One gutter per screen; a card must be liftable

Copy-heavy pages read badly on a phone, and the cause was compounding
padding nobody owned. On a 375px screen the charter rendered at **269px**
of measure — the workspace gutter (`.work-content`, `padding: 1.5rem 2rem
2rem`, with no mobile override anywhere) plus a hand-rolled card wrapping
the document body. Account and Security Settings came in at 285px, the
Label and the legal documents at 287px. Comfortable prose is 45–75
characters; every long-form page in the app sat around 36. Meanwhile
`.card` was used 20 times while `background: var(--color-surface)` was
hand-rolled 87 times across 54 files, so there was no shared idea of what
a card *was* — which is why each one picked its own padding.

An earlier attempt at this (issue #17) decided the container owns padding
and wrote it into a code comment on `.social-main`. The two most
copy-heavy pages in the app, the Label and the legal documents, both
re-pad on top of it. A rule enforced by a comment is not enforced.

We decided:

- **The gutter is a component, not a convention.** The inset between the
  viewport edge and content is set once, by the shell, through a single
  primitive. A page or a surface inside it never re-applies one. Making
  it a component is the whole point: "don't re-pad" failed as a comment,
  but a second gutter is visible as a second component, and a reviewer
  can see it. A card's inner padding is a *different token on a different
  scale* — collapsing the gutter for a phone must not shrink what's
  inside the cards.

- **A card must be liftable — "could this move?"** A card holds something
  that would still make sense on a different page: a patch tile, an
  event, a member, a proposal in a list. A settings section is not a
  card; it *is* the page, and it separates with a heading and a rule.
  This deletes boxes rather than resizing them — including the charter's
  `.doc-body`, three sections each in Account and Security Settings, four
  in AdminLabel, three in AdminQuiltSettings, the whole-page forms in
  EventForm/PatchForm/ProposalForm, and the mutually-exclusive page
  *states* boxed in SignupComplete and ClaimVerifyEmail. The pattern that
  replaces them already exists and is already shipping: PatchSettingsInfo,
  PatchSettingsNotifications, and NotificationPreferences use bare
  `padding: 0.75rem 0` rows with no cards.

- **"Interruption" is a second, closed category.** A danger zone, a
  warning callout, an unsaved-changes banner — loud on purpose because it
  breaks the reading flow. Named explicitly and kept small, because
  without it "but this one needs a box" is a loophole wide enough to
  re-admit every page section as a card.

- **Parameterized, not configurable — no density setting.** Tokens are
  CSS custom properties overridable at a scope below `:root`, so a future
  density toggle, a native shell, or a large-text mode is a substitution
  rather than a refactor; `data-density` on `<html>` would sit exactly
  where `data-theme` already does. But nothing user-facing or
  admin-facing ships. "Config over code" in Patchwork means *community*
  customization — names, colors, governance, vocabulary. Those are
  identity. Gutter width is craft, and an admin-facing control for it
  buys a permanent multiplier on the screenshot and e2e matrix in
  exchange for a setting nobody touches. A seamrip still retunes its
  whole quilt by editing the token block, which is the same override path
  `--color-primary` already has and which also has no admin UI.

- **Fluid `clamp()` tokens; no spacing breakpoints.** Spacing retunes
  continuously with the viewport rather than at breakpoints. This is
  primarily about keeping the seam clean: if media queries redefined
  tokens on `:root`, a later `[data-density]` on `<html>` would outrank
  them and silently win on desktop too. With no spacing media queries
  there is nothing for the density layer to fight, so it stays purely
  additive. It also degrades better at the 320px sizes where the measure
  is worst, at the cost of values a reviewer can't eyeball.

- **Semantic tokens, no numeric ramp.** `--pw-gutter`, `--pw-pad`,
  `--pw-stack`, `--pw-gap`, `--pw-measure`. A `--pw-space-3` carries no
  rule about where it belongs, so every component would still pick by
  taste and the drift would return, merely tokenized. A semantic token is
  reviewable: "that's a gutter inside a gutter" is a sentence you can
  say. **`--pw-stack` deliberately does not shrink on mobile** while the
  horizontal tokens collapse — with the card borders deleted, vertical
  separation is the only thing left saying a section ended. This is also
  why a single density multiplier was rejected: it cannot express
  "collapse the gutter, keep the rhythm."

- **The system governs layout spacing, not component-internal spacing.**
  A badge's pill padding, a button's `--btn-pad-*` (already tokenized
  separately, and correctly), a diff gutter's `3ch` — those are part of a
  component's own drawing, not the page's rhythm. Without this line the
  five-token system becomes forty and the sweep becomes all 455 `padding`
  declarations in the tree.

Consequences: `--pw-measure` collapses five contradictory content widths
(`640px` ×26, `520px`, `800px`, `1000px`, and none at all) onto one
`ch`-based value — the measure the codebase already got right in exactly
one place, `max-width: 60ch` in AdminLegal, whose public counterpart
LegalDoc renders the same prose at `640px`. Because `.card` is bypassed
4-to-1 by hand-rolled surfaces, the sweep is 54 files rather than a
change to one class. If a deployment later wants roomier text for its
audience, that is a fork or a PR, not a toggle. MarkdownRenderer's
`0.9rem` body copy is a typography question that sits outside this
decision's stop line and is left open.
