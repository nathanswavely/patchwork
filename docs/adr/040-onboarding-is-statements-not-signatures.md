# Onboarding is statements, not signatures

Date: 2026-07-24. Status: accepted; amended by docs/adr/068 — "no in-app
help center" stands, but the About page is no longer the *single*
explaining surface: /governance is admitted under a two-part test,
because a permission-gated subsystem cannot be taught by the UI that
hides it.

Designing first-contact for every audience at once — anonymous visitors,
new accounts, new patch members, new patch owners — surfaced a question
that kept recurring in different clothes: at which moments does a person
*sign* something, and at which are they simply *shown* something? The
existing surfaces disagreed with each other. Account creation carried a
real consent line (docs/adr/028), the Welcome flow carried a checkbox
("I agree to the community standards") that was hand-paraphrased from
the lining, bound to nothing, and recorded nowhere, and joining a patch
— the one act that actually places a person under a patch's governance —
carried no ceremony at all.

## Decision

**There is exactly one signature in the system: account creation.** The
User Agreement and Privacy Policy consent at signup (docs/adr/028) is
the single moment a person agrees to anything. The shipped User
Agreement default is rewritten values-forward so the signature covers
what actually matters here: what a quilt is, that patches govern
themselves starting from the lining (linked), and the conduct expected
of anyone interacting with the site — follower, member, or visitor with
an account. The anti-discrimination baseline binds site-wide through
this document, not through membership in any patch.

**Every other onboarding moment is a statement — shown, not signed:**

- The **intro card** greets an anonymous first landing on any public
  surface, compact on deep links, dismissed once and gone; the standing
  path to the **About page** ("What is Patchwork?", `/about`) lives in
  the global bar. The About page is orientation (what/how) and never
  trades jobs with the Label (who/costs); it exposes the Label inline as
  the community's own voice.
- **Welcome** (`/welcome`, now auth-gated and reachable only from
  signup/invite completion) loses its checkbox. Its first step
  compresses to orientation plus the agreement's gist — the person
  signed one screen ago; re-signing is theater.
- The **join sheet** stands between "Become Member" and membership: the
  patch's membership policy, its lining state including an amended
  lining, and its published charters — strictly a lens over what that
  viewer could already see (docs/adr/036 visibility is never bypassed).
  On approval-required patches it carries one optional intro message to
  the admins. No checkbox: joining informed is the agreement.
- The **unlock panel** meets the new member on first workspace visit
  with what membership just made visible — members-only charters,
  proposals and their vote. The full governance introduction happens
  *after* acceptance because that is when the documents become readable.
  Nobody agrees to documents they weren't allowed to read.
- The **setup checklist** gives a new patch admin (creator and
  claim-completer alike) the derived-state list of what a patch with
  footing looks like. State, not stored progress; panel, not wizard.

**No per-surface consent is recorded.** No membership carries an
"agreed" flag; no join is time-stamped as an act of assent. Enforcement
legitimacy rests on the account-level agreement plus the lining being
public and identifiable (docs/adr/037) — "the standards were signed
once, and the baseline was always visible" — not on a stack of
per-context signatures.

**No in-app help center.** The About page is the single standing prose
surface; training is carried by mechanisms maintained as UI (vocab
subtitles, empty states, the surfaces above), which the same PRs that
change behavior must update. Recurring real questions earn an FAQ
section on the About page; a question asked once earns a UI fix. The
organizer-facing prose ("should my community run a quilt?") lives in
the repo (`docs/START-A-QUILT.md`), not in the app.

## Alternatives rejected

- **Keep the Welcome checkbox and record it.** Double-signing the same
  agreement minutes apart is consent theater, and theater trains
  dismissal. The checkbox also bound to a paraphrase that had already
  drifted from the lining it imitated.
- **A per-patch agreement ceremony at join** (checkbox recorded on the
  membership). Rejected for register: docs/adr/037 deliberately made
  patch *creation* a statement, not a checkbox; joining must not be more
  contractual than founding. Rejected for coherence: the documents a
  member would be "agreeing to" are largely unreadable until after
  acceptance (docs/adr/036).
- **A help center.** Where young products send questions their UI
  failed to answer; goes stale the moment it is written.

## Consequences

- The threshold flows (`/login`, `/invite/:token`, `/signup/complete`,
  `/welcome`) share a minimal shell — quilt mark as the exit home —
  instead of being shell-less traps; the global bar gains a Sign Up
  entry (collapsing to Sign Up alone on narrow screens, the unified
  auth page covering sign-in), and the standing "What is Patchwork?"
  affordance for anonymous visitors lives in the sidebar.
- The Welcome checkbox's disappearance moves the standards affirmation
  wholly into the User Agreement; its shipped default must actually
  carry that weight before the checkbox is removed.
- The join sheet needs one new column (the pending membership's intro
  message) and no new visibility machinery.
- Fake-quilt imagery leaves the flows: identity is the quilt mark, and
  the About page's hero is a live miniature of the instance's real
  quilt.
