# Amendments adopted elsewhere

docs/adr/052 decided that a patch declares where its decisions happen, and
that the question is asked twice — once of leadership, once of proposals.
Only the leadership half shipped: `proposal_venue` was deliberately left
out rather than landing as a field with no attestation behind it. This is
the other half.

It turned out not to be the mirror image. Working through what a
"proposal attestation" would actually do, it splits into two features
wearing one name, and only one of them is attestation.

We decided **`proposal_venue` gates the attestation of amendments — texts
a meeting adopted — and nothing else.**

- **A minute book is a different feature and gets its own decision.**
  Recording "the members approved buying the kiln" changes no state. Its
  value is entirely that it is a public, federated, portable record, which
  is real and is plausibly much of why an established community would use
  Patchwork at all. But it is not attestation-shaped, and the tell is the
  gate: docs/adr/052 gated attestation because otherwise "attesting would
  be a way around the vote." For a record that changes nothing there is no
  vote to go around, so the gate would be protecting against nothing.
  Bolting a minute book under `proposal_venue` would mean inventing a
  restriction to guard an absence.

- **The lining is not attestable, and this is not a new decision.**
  docs/adr/037 already says "the only thing that changes a lining's body
  is a passed amendment proposal," and an attestation is not one.

  Worth recording why that is right rather than merely binding. "Amended
  lining" today means *this community voted to change the baseline*. If an
  attestation could diverge a lining, the badge would look identical
  whether a patch had held a real vote under real terms or one admin had
  asserted a meeting happened — quietly weakening an existing signal, and
  the anti-discrimination baseline is what it guards. So a patch that
  decides everything at meetings still comes back into Patchwork to touch
  its lining: one hard rule surviving every configuration, the same shape
  as the last-admin floor.

- **Prose is attestable; the governance rules are not.** A meeting adopts
  a *text* — a charter, bylaws, an operating agreement.
  `governance-rules.json` is not a text anyone adopts; it is the
  machine-readable configuration of how Patchwork behaves, and the
  structured editor is its only writer. No minutes read "we set
  `quorum_percent` to 50"; they read that the quorum is now half the
  membership, and afterwards somebody updates the system to match. Those
  are two acts, and collapsing them is what creates a hole.

  On a patch that decides elsewhere, a rules change is therefore a
  **direct change** in docs/adr/041's existing sense: an admin applies it,
  and the record says an admin applied it. Honest, and it claims nothing
  about a vote.

  **This closes a two-step bypass of the leadership gate.** A patch with
  `proposal_venue: elsewhere` and `leadership_venue: patchwork` could
  otherwise attest a rules amendment flipping `leadership_venue` to
  `elsewhere`, and then attest a council — one admin's assertion, and the
  gate docs/adr/052 built for leadership is gone, reached entirely through
  the proposals side. Excluding the rules file closes it structurally
  rather than by a special case, because the rules file simply is not the
  kind of thing an attestation is about.

- **An attestation asserts the whole current text and checks no base.**
  A proposal is drafted *against* Patchwork's copy, which is why
  `base_sha` (migration 016) exists: a document that moved underneath a
  draft genuinely invalidates it. An attestation is the opposite
  direction — the community telling Patchwork what its bylaws now say.
  Patchwork's copy is not a base to build on; it is a possibly-stale cache
  being corrected, and checking it would let a stale cache refuse the
  truth.

  The case this is for: a co-op that joined a year ago, took the Formal
  template's bylaws, and has amended them at three AGMs since without
  recording any of it. Patchwork holds the shipped template. Their first
  attestation has almost no relation to it, and under conflict detection
  that reads as an error rather than as the moment Patchwork's copy stops
  being fiction.

  So the diff is a *view*, never a gate; the first attestation on a
  document is a correction and its diff will be large; and no special
  "first attestation" case has to be detected, because every attestation
  replaces and the first simply replaces more. A record of an adopted text
  is not a delta.

  **The cost: an attestation can land on top of an in-flight amendment
  proposal**, whose diff is then against something else. `base_sha` exists
  to catch exactly that. We take the trade — a community's own text should
  win over a draft — but an open amendment proposal on an attested
  document gets a visible notice that the ground moved, in the same spirit
  as docs/adr/047's mid-vote rules notice.

- **Declaring the venue elsewhere removes the ballot and keeps the
  discussion.** This is what makes the gate mean anything. If Patchwork
  votes still bound on an elsewhere-patch, that patch would have both, and
  an admin who disliked where a tally was heading could attest a meeting
  result instead — the bypass restored one field further along.

  So a proposal on such a patch can still be raised, discussed, argued
  over and revised; what it does not have is a tally. The decision comes
  back as an attestation. The leadership half already works this way:
  `leadership_venue: elsewhere` refuses nomination and hand-promotion,
  because the record is what promotes. Discussion also stands on its own
  — docs/adr/048 established that deliberation runs concurrently with
  voting rather than before it — so removing the ballot leaves a working
  surface rather than a stub.

  **A patch that flips the venue mid-vote does not kill the vote.**
  docs/adr/047 already answers this: a vote keeps the terms it opened
  with, so an open ballot finishes and applies. The venue governs new
  proposals.

  **The cost is the straw poll**, and it is a real loss: a community that
  decides at meetings may well want to sound members out beforehand. An
  explicitly advisory *poll* — a tally that says out loud that it decides
  nothing — is a separate feature with its own question, and inventing it
  here would be exactly the over-cleverness that the first draft of
  docs/adr/051 was rewritten to remove.

- **An attestation may name a document Patchwork does not have.** A
  meeting can adopt a charter this instance was never templated with, and
  refusing it would mean a community may only record amendments to
  documents Patchwork happened to guess at. The text is written with
  `DirectEdit`, which already writes a document and its commit without a
  proposal branch, so a missing document is created rather than refused.

## Open

- Whether an attested amendment **federates** as its own activity, or only
  through the governance timeline it shares. docs/adr/052 left the same
  question for the leadership half; both should be answered together.
- The **minute book**: whether Patchwork records decisions that change
  nothing, and if so under what name and what gate. Named here as a
  separate decision rather than an omission.
- The **advisory poll**, as above.

**Status: adopted as a design boundary — implementation is backlog.**
Nothing here is built; `proposal_venue` still does not exist, and that
remains correct until the attestation it gates does.
