package governance

import (
	"crypto/sha256"
	"encoding/hex"
)

// The lining's shipped lineage (docs/adr/037). The binary carries every
// version the project has ever shipped, in order; the last entry is current.
// Divergence is a property of the text, not the history: a patch's lining is
// pristine if it matches the current entry, stale if it matches an older one,
// and diverged ("amended lining" in the UI) if it matches nothing.
//
// Appending here is how the lining is updated: stale linings auto-update on
// the next startup (AutoUpdateLinings), so an edit to this list is a release,
// not a migration. Never edit an existing entry — that would orphan every
// patch sitting on it.
var liningVersions = []string{
	liningV1,
}

// legacyLiningBodies are pre-lineage texts healed to the current version on
// startup but deliberately NOT part of the shipped record: the drafts that
// existed before the lining was written by hand. A body matching one of these
// counts as stale (so it heals), yet the lineage itself starts at v1.
var legacyLiningBodies = []string{
	legacyLiningDraft,
}

// LiningStatus values. Backend/predicate terms; the UI says "amended lining"
// for diverged (CONTEXT.md).
const (
	LiningPristine = "pristine"
	LiningStale    = "stale"
	LiningDiverged = "diverged"
)

func liningHash(body string) string {
	h := sha256.Sum256([]byte(body))
	return hex.EncodeToString(h[:])
}

// LiningStatus classifies a lining body against the shipped lineage.
func LiningStatus(body string) string {
	h := liningHash(body)
	if h == liningHash(CurrentLiningBody()) {
		return LiningPristine
	}
	for _, v := range liningVersions[:len(liningVersions)-1] {
		if h == liningHash(v) {
			return LiningStale
		}
	}
	for _, v := range legacyLiningBodies {
		if h == liningHash(v) {
			return LiningStale
		}
	}
	return LiningDiverged
}

// CurrentLiningBody returns the current shipped lining text.
func CurrentLiningBody() string {
	return liningVersions[len(liningVersions)-1]
}

// CurrentLiningVersion returns the 1-based lineage position of the current
// text — the "vN" in the auto-update git commit message.
func CurrentLiningVersion() int {
	return len(liningVersions)
}

// liningV1 is the first shipped lining, written by the project author
// (docs/adr/037: the lineage starts at the handwritten text; earlier drafts
// are legacy, healed but unrecorded).
//
// PLACEHOLDER: this is the working draft pending Nathan's final wording. It
// must be replaced by the finalized v1 before this branch ships — the whole
// point of the lineage is that v1 is written by hand.
//
// No top-level heading: the title lives in the governance_docs row
// (DefaultLiningTitle), and git file content must equal the DB body verbatim.
const liningV1 = `This is our lining. This patch, like every patch on this quilt, starts by agreeing to it. This patch runs itself, except in what's written here. It's deliberately short.

## Keep each other safe

This patch, like the quilt it belongs to, is antifascist by design. Nobody gets targeted here for who they are: race, ethnicity, gender, sexuality, disability, age, religion, immigration status, class. A space that tolerates hate turns into a space for hate. So we don't tolerate it.

## Good faith

Argue as hard as you want about ideas and leave the people out of it. Do what you said you'd do. If you can't, say so. Leave people and places a little better than you found them.

## Don't look away

If someone is being harmed, tell a patch admin, and if the patch itself is the problem, tell the quilt's admin. Reporting isn't snitching, and nobody else is coming to handle it.

## Hard limits

Harassment, threats, or hate speech. Outing people, or passing along what they told you in confidence. Contact that keeps coming after someone says stop. Any of these can get a person removed, and a patch that shelters them can go with them.`

// legacyLiningDraft is the pre-lineage text live instances carry (the body
// DefaultLiningBody shipped as before docs/adr/037). Held here only so the
// startup heal recognizes and replaces it; it is not a shipped version.
const legacyLiningDraft = `This is the lining, the baseline that holds this patchwork together. Every patch on this quilt agrees to these standards. Individual patches can build on top of them, but they can't override them.

## Our Commitment

This patchwork is antifascist by design. Everyone gets a welcoming and safe place here, regardless of race, ethnicity, gender identity, sexual orientation, disability, age, religion, immigration status, or socioeconomic background.

## Expected Behavior

- **Treat every person with dignity and respect.** Disagree all you want. Personal attacks are out of bounds.
- **Participate in good faith.** Contribute constructively to the communities you join.
- **Support the communities you're part of.** Show up, follow through, and help when you can.
- **Report harmful behavior.** If you see something that violates these standards, tell a patch admin or instance admin. Don't ignore it.

## Unacceptable Behavior

- Discrimination, harassment, or hate speech of any kind
- Threats, intimidation, or deliberate disruption
- Sharing others' private information without consent
- Sustained unwelcome contact after being asked to stop`
