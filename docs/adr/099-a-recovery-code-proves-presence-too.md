# 099. A recovery code proves presence too

**Status:** accepted, 2026-09-15

## Context

docs/adr/017 put a step-up gate on the irreversible: a fresh WebAuthn
assertion, good for five minutes, in front of instance wipe, instance
export, and promotion to instance admin. It considered password or email
re-entry and rejected both — Patchwork has no passwords, and email re-auth
is unavailable on the SMTP-less deployments the project explicitly
supports. It then said this locked nobody out, "since enrollment needs
only the session they already hold."

That sentence is wrong, and epoch 1 of the governance simulation
(docs/adr/096) found the person it is wrong about. Omar co-chairs a
neighbourhood coalition that decides everything at a monthly meeting and
wants Patchwork only to write down what was decided. His patch declares
`leadership_venue: elsewhere`, so the *only* way it can make an admin is
a leadership attestation (docs/adr/052) — and attestations are step-up
gated. His office desktop has no platform authenticator and he owns no
security key, so `navigator.credentials.create()` fails with
`NotSupportedError`. He cannot enrol. He therefore cannot attest. He
therefore cannot record who his meeting elected, which is the single
thing his community came here to do. The app's advice at that moment was
"Use an email link instead", which grants no step-up window at all and
which his instance, having no SMTP, could not have sent him either.

Two things had drifted since 017. The gate spread from three
instance-level acts, performed rarely by someone administering a server,
to a patch-level act a volunteer secretary performs every month. And the
one factor it accepts was assumed universal when it is a property of the
device in front of you.

Patchwork already has an answer to "prove who you are with no delivery
channel and no working authenticator": recovery codes (docs/adr/020),
twelve characters from a paper-safe alphabet, hashed, single-use, issued
ten at a time and kept offline by the person. That is the same shape of
proof step-up is asking for.

## Decision

**A recovery code opens the step-up window, on one condition: it was
issued before the session presenting it began.**

The condition is the whole security argument. Generating a batch needs
only a session, so without it a stolen cookie could mint its own second
factor and use it in the same breath — an attacker holding a passkey-less
admin's session would go from "everything except the irreversible" to
"including the irreversible", which is precisely the boundary 017 drew.
A batch that predates the session is something the person had beforehand
and kept, which is what "something you have" means. `sessions.created_at`
and `recovery_codes.created_at` both exist and are written in the same
format, so the rule is one comparison and needs no new state.

**Redeeming a recovery code to sign in opens the window on arrival.**
That redemption is itself a proof of possession, performed seconds
earlier, and without it the rule above would trap the very person it
exists for: someone with no passkey who makes their first batch would
hold ten codes that all postdate the session they are sitting in.

Together these give a route that works on any device, with no mail
server: make codes, sign out, sign back in with one — you arrive
confirmed — and from then on the rest of the batch predates your session
and confirms directly. The cost is one code per confirmation, which is a
real cost and is meant to be: it keeps the gate expensive enough to stay
meaningful, and running low is something the page says out loud.

**The failure modes are distinguishable.** Sign-in redemption folds every
failure into one message so it cannot be used to probe which usernames
exist; step-up does not, because the caller is already authenticated as
the account in question and learns nothing about anybody else. "You have
no codes", "your codes are newer than this sign-in" and "that code is
wrong" need different actions from the person, and a single message would
leave someone retyping a code that can never work.

## Consequences

- A passkey remains the better factor and the default: it is phishing
  resistant, it is not consumed, and nothing about this decision changes
  what the gate asks for first. This is the floor, not the replacement.
- Ten codes is ten confirmations, minus any spent signing in. A person
  who governs an elsewhere-venue patch monthly will exhaust a batch in
  under a year, and the prompt warns at two remaining. If that proves
  too thin in practice the answer is a larger batch or a longer window,
  not a weaker proof.
- The rule has one sharp edge worth stating: codes generated during the
  current session are refused, which reads as arbitrary until it is
  explained. Both the prompt and Settings → Security explain it, in one
  sentence, at the moment it matters.
- **This closes the fallback and leaves a question open.** Whether
  recording an attestation should carry a step-up gate *at all* is a
  separate decision, not made here. 017's own reasoning argues against
  it — "re-prompting for routine moderation trains admins to approve
  prompts reflexively, which is how step-up auth stops working", and
  monthly minute-taking is routine — but an attestation grants admin
  rights on a patch, which is not nothing. That belongs in its own ADR
  with docs/adr/052 in front of it.
- docs/adr/017's claim that the gate locks nobody out is amended rather
  than deleted: it is true now, and it was not before.
