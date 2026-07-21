# ADR 028: Legal documents ship with defaults and live in instance settings

Date: 2026-07-20. Status: accepted.

## Context

A live deployment (lancasterpatchwork.org) collects email addresses, IP
addresses, profiles, and membership records, and speaks ActivityPub to
servers in other jurisdictions. That makes a posted privacy policy a
practical legal requirement for any US deployment (CalOPPA applies to any
site collecting personal information from California residents), and a
user agreement the operator's main protection as a host of user-generated
content (moderation authority, age minimum, copyright complaints,
no-warranty).

Patchwork is white-label: every quilt that seamrips and deploys has the
same gap, run by stewards who are organizers, not lawyers. A hardcoded
Lancaster-only page would leave every other deployment bare; requiring
each deployment to author its own would mean most launch with nothing.

Two prior decisions shape the answer. ADR 014 split configuration by who
changes it: community-editable state lives in `instance_settings`, not
yaml. ADR 023 (the Label) established that a deployment's
self-description is public, readable logged out, and honest about its own
provenance.

## Decision

**Defaults ship in the binary.** A default privacy policy and user
agreement are Go string constants (`internal/handler/legal_defaults.go`),
served from first boot at `GET /api/v1/legal/{privacy|terms}` and
rendered at `/privacy` and `/terms`. The defaults describe what the
software actually does — every claim is checked against the codebase, and
a feature change that falsifies a sentence must update that sentence in
the same PR. Each default opens by saying it *is* the default, so a
reader always knows whether the stewards wrote what they're reading.

**Overrides are instance settings.** An admin can replace either document
wholesale from the admin panel (Legal tab); the custom markdown is stored
under `legal_privacy` / `legal_terms` in `instance_settings` (ADR 014
pattern — no new table, no migration). Reset means deleting the key:
default vs. custom is a presence check, never a diff. There is no partial
override — a document is either the shipped default or entirely the
stewards' words, so responsibility for the text is never ambiguous.

**Templates track identity at serve time.** `{quilt_name}` is substituted
with the effective instance name on every request, so an ADR 014 rename
never strands a stale name inside a legal document.

**Consent is at account creation.** Both account-creating forms (invite
landing and signup complete) carry "By creating an account you agree…"
links. The documents are linked from the shell footer alongside the Label
regardless of whether a Label is published — a legal document, unlike a
Label, always exists.

**Legal documents do not travel in the seamrip.** Like the quilt icon
(migration 021), they describe *this* deployment and its operator; a fork
has a different operator with different obligations. The fork boots with
the shipped defaults.

## Consequences

- Every deployment, including a fresh seamrip, has a truthful privacy
  policy and user agreement from first boot with zero admin effort.
- The defaults are part of the software's honesty surface: changing what
  the software collects or federates now carries a documentation duty in
  `legal_defaults.go`, enforced by review rather than tooling.
- A customized document goes stale silently if the software changes
  underneath it; the admin editor keeps the current shipped default
  visible ("restore default") to make drift recoverable.
- No DMCA designated-agent registration or jurisdiction-specific text is
  included: those are per-operator acts the software cannot perform.
  Operators wanting §512 safe harbor must register an agent with the US
  Copyright Office themselves and may customize the terms accordingly.
