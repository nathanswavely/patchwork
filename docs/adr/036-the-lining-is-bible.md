# The lining is bible — project-owned, always public, amendable only in the open

ADR 035 gave every governance doc a visibility switch, members-only by
default. That quietly gutted the lining's founding purpose: the shared
baseline every patch adopts at creation — the anti-discrimination floor —
was suddenly a document a patch could hide, or retitle, or silently
direct-edit. A baseline nobody can see holds nothing together.

We decided the lining is a different kind of document from every other
charter, and the differences are enforced, not requested:

- **Project-owned.** The lining's text ships in the Patchwork binary and
  is written by the project. There is no instance-admin override — the
  legal-documents pattern (docs/adr/028: shipped defaults, admin
  overrides) deliberately does NOT apply here. The lining is the bare
  minimum for what a quilt should be; it is friction against
  organizations that diverge from the project's values. A quilt that
  wants a different baseline forks the open source repo — that is the
  intended escape hatch, and its cost is the point.
- **Identified by a `kind` column, not a title.** `governance_docs.kind`
  is `'lining'` for the row every patch gets at creation, `'charter'`
  otherwise (backfilled by matching the historical title constants).
  Title matching was the only identity before, and the title was
  editable — one rename and every rule below goes blind. The column is
  deliberately extensible; only `'lining'` carries behavior today.
- **Pinned public, title immutable, undeletable, amendment-only.** The
  visibility switch (ADR 035) refuses `kind='lining'`; so does a title
  edit and a direct body edit (`PUT /governance/{id}`). The only path
  that changes a lining's body is a passed amendment proposal — a voted,
  recorded, notified act. A lone admin's silent 2am edit is not a
  community owning its divergence.
- **Amendable, but the divergence is broadcast.** We considered locking
  the lining entirely (it matches "bible", and its own text says patches
  "can't override" it). Rejected: the court of public opinion should
  prevail over decreed values. A patch may amend its lining — and the
  amendment is public, the patch wears an **"Amended lining"** badge, and
  both individual users (personal setting) and instance admins (quilt
  policy) can filter amended-lining patches out of discovery. Filtering
  uses the same surface set as patch-level private — quilt, search, map,
  public feeds; events follow, except for the patch's own members and
  followers — and the same caveat: a direct link always works, because
  accountability requires the divergence stay inspectable. Strictest
  wins: the user setting can hide what quilt policy shows, never reveal
  what it hides.
- **Divergence is a property of the text, not the history.** The binary
  carries the full lineage of shipped lining versions (bodies; hashes
  derived). A patch whose lining matches the current version is pristine;
  matching an older version is *stale*, not amended; matching nothing is
  *diverged* (the badge state). Shipping a new version therefore never
  changes anyone's badge, and a patch that amends and later reverts
  word-for-word to a shipped version is pristine again — redemption is
  possible, and git still remembers.
- **Stale linings auto-update.** On startup after an upgrade, every stale
  lining is rewritten to the current text — version bump, git mirror
  commit, `lining.updated` notification to members. Notified, never
  asked: a pristine patch never chose its lining text; it agreed to the
  baseline, whatever it currently says. Opt-in adoption was rejected
  because it fragments the fleet by apathy and makes the lining's first
  sentence ("every patch on this quilt agrees to these standards") false
  within two releases. The consent story is the asymmetry: a patch that
  wants out of auto-updates amends the lining and wears the badge.
- **Creation says all of this out loud.** The create-patch form carries a
  dedicated lining section — every patch starts with the lining; it is
  always public; amendments are public and badge the patch — with the
  full text one drawer away. A statement, not a checkbox: the standing
  consequence is the friction, not consent theater at the door.

The shipped lineage starts at version 1 = the project author's
handwritten text. The earlier AI-drafted body that live instances carry
is not part of the lineage; a one-time startup heal (keyed on that body's
hash, held outside the lineage) rewrites matching rows in place. The
git mirrors' history retains the old text — accepted for now; a planned
pre-launch data reset would erase it, and purpose-built git-history
rewriting was rejected as falsification machinery with one use.

Consequences: "Amended lining" is a first-class user-facing state
(CONTEXT.md); the lining is the one document ADR 035's members-only
default can never touch; amendment proposals targeting a lining never
redact their text (it is public by definition); and every future edit to
the lining's text is a lineage append that auto-propagates to all
pristine patches on the next deploy — updating the bible is a release,
not a migration.
