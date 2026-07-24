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
	legacyLiningOriginal,
	legacyLiningHumanized,
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
// No top-level heading: the title lives in the governance_docs row
// (DefaultLiningTitle), and git file content must equal the DB body verbatim.
const liningV1 = `This patch, like every patch on the quilt, starts by agreeing to the lining. If a patch changes this lining, the change is recorded and public. Its members operate with autonomy as long as they maintain the following commitment:

## Keep each other safe

This is an antifascist space. Nobody is harmed, excluded, or diminished for who they are: not for race, ethnicity, gender, sexuality, disability, age, religion, immigration status, or class. We regulate actions, not identity. Tolerance is a social contract, and whoever organizes to end it for others isn't owed a place in it. Any organizing that ranks people by any of those lines, scapegoats minorities, or promotes xenophobia is prohibited. No fascist or white-supremacist organizing, recruiting, or symbols, no matter how politely they're dressed. A space that tolerates hate turns into a space for hate.

## Good faith

We accept that argument and debate are an essential part of community building, but personal attacks are prohibited. We do our best to follow through on our word, and if we can't, we communicate clearly. We leave people and places a little better than we found them, and we do the good that's ours to do.

## Don't look away

If someone is being harmed, we say something, and if the patch itself is the problem, we escalate. Reporting isn't snitching, and we don't assume it's someone else's problem.

## Prohibited behavior

Harassment, threats, or hate speech. Outing people, or passing along what they told you in confidence. Contact that keeps coming after someone says stop. Any of these can result in user removal, and a patch that shelters such users will risk removal as well.`

// legacyLiningOriginal is the first DefaultLiningBody ever shipped — the
// text patches created on early instances carry verbatim. Captured
// byte-exact from the shipped constant; never edit it, only ever delete it
// once no live instance can still carry it.
const legacyLiningOriginal = `This is the lining — the baseline that holds this patchwork together. Every patch on this quilt agrees to these standards. Individual patches can build on top of them, but they can't override them.

## Our Commitment

This patchwork is antifascist by design. We are dedicated to providing a welcoming, inclusive, and safe space for everyone — regardless of race, ethnicity, gender identity, sexual orientation, disability, age, religion, immigration status, or socioeconomic background.

## Expected Behavior

- **Treat every person with dignity and respect.** Disagreements are natural; personal attacks are not.
- **Participate in good faith.** Contribute constructively to the communities you join.
- **Support the communities you're part of.** Show up, follow through, and help when you can.
- **Report harmful behavior.** If you see something that violates these standards, tell a patch admin or instance admin. Don't ignore it.

## Unacceptable Behavior

- Discrimination, harassment, or hate speech of any kind
- Threats, intimidation, or deliberate disruption
- Sharing others' private information without consent
- Sustained unwelcome contact after being asked to stop
- Using this infrastructure to organize against the humanity of other people

## Enforcement

Patch admins may warn, temporarily suspend, or permanently remove members who violate these standards. Instance admins may take action on patches that fail to enforce these standards.

## The Right to Seamrip

Any group can take the tools, the governance docs, and the data, and start their own quilt. This isn't destructive — it's the immune system. The threat of seamripping keeps the organization honest.

## Amendments

These standards can be amended through the governance process. Proposed changes require community review and a supermajority vote at the instance level.
`

// legacyLiningHumanized is DefaultLiningBody after the copy pass that
// retired em dashes and trimmed the text — the last default shipped before
// the lineage existed. Same rules: byte-exact, never edit.
const legacyLiningHumanized = `This is the lining, the baseline that holds this patchwork together. Every patch on this quilt agrees to these standards. Individual patches can build on top of them, but they can't override them.

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
- Sustained unwelcome contact after being asked to stop
- Using this infrastructure to organize against the humanity of other people

## Enforcement

Patch admins may warn, temporarily suspend, or permanently remove members who violate these standards. Instance admins may take action on patches that fail to enforce these standards.

## The Right to Seamrip

Any group can take the tools, the governance docs, and the data, and start their own quilt. That's the immune system working. The threat of seamripping keeps the organization honest.

## Amendments

These standards can be amended through the governance process. Proposed changes require community review and a supermajority vote at the instance level.
`
