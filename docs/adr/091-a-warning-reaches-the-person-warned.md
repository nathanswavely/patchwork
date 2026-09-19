# ADR 091: A warning reaches the person warned, and never carries the reporter

Date: 2026-09-07. Status: **accepted**; implemented. PR #221.

## Context

The instance report queue offers five actions: dismiss, warn, remove content,
reset appearance, suspend user. Four of them did something. `warn` set the
report to `resolved` and sent one notification — to the *reporter*, saying
their report had been reviewed. The person whose content was reported was
never told anything. The menu entry had shipped, admins had been choosing it,
and it warned nobody.

This was found while fixing an unrelated bug and is worth recording, because
the failure is not a missing feature. It is a ladder with its bottom rung
sawn off. The other four actions are dismissal — do nothing — or removal and
suspension. If the mildest response is inert, an admin who wants to say "this
was noticed, don't do it again" has no way to say it, and the only responses
that *work* are the ones that take something away. A moderation system whose
proportionate option is a no-op pushes every real decision toward the severe
end, which is the opposite of what a graduated ladder is for.

Making it work is not simply a matter of sending a notification, because the
obvious content for one is exactly what must not be in it. A warning wants to
say what it is about. Three texts describe that, and all three are written for
moderators:

- `content_reports.reason` and `.details` — the reporter's own words.
- `content_reports.resolution_note` — the reviewing admin's note.

The note is the sharpest edge. Its field is labelled only "Optional note…",
it is displayed on exactly two moderator surfaces, and nothing has ever told
an admin it might be shown to the person reported. Notes written under that
understanding say things like *"repeat offender, same reporter as Tuesday"*.
Relaying them would publish, retroactively, text written in the belief it was
private — and would identify people who reported in good faith. The reporter's
own `reason` is no safer: it is free text, and "harassment by <name>" is a
reason somebody will type.

Reporting only works if reporting is safe. A warning that leaks its source
costs more than a warning that never arrives.

## Decision

**1. A warning is addressed to the account behind the reported content.**
A report names content; moderation lands on a person — the patch's owner, the
event's creator, or, for a report about an account, that account. This
resolution already existed inline in `suspend_user`; it is now
`reportedParty()` in `internal/handler/reports.go` and both actions call it.
The two must agree: a warning that reaches somebody a suspension would not is
a warning delivered to the wrong person.

**2. It says what was reported, and nothing about who reported it.** The
notification names the recipient's *own* patch or event, states that an admin
reviewed a report about it and issued a warning, says that nothing was changed
or removed, and links to the thing. Every fact in it is about property the
recipient already knows they have.

**3. It carries no text written by the reporter or the reviewing admin.**
Not the reason, not the details, not the resolution note. `resolution_note`
remains a moderator-facing field. If it is ever to be shown to the person
reported, its label and its help text have to say so *before* the first note
is written under the new rule, not after.

**4. The reporter's notification is unchanged.** They are told their report
was reviewed. They are not told the outcome — what was decided is the
instance's, and telling a reporter which lever was pulled invites both
score-keeping and retaliation.

**5. A warning is not mutable.** Like `account.suspended`, it is sent through
`CreateNotification` rather than the preference-registry path, so it has no
per-type switch. A person may not turn off being told they were warned. The
notification list's Moderation filter (same PR) exists so these are findable
without being silenceable — a browsing category, deliberately not a
preference category.

**6. It is on the record.** Audited as `admin.user_warn`, beside the
suspension it is the lesser form of.

## Consequences

The ladder has its bottom rung back. An admin can register that something was
noticed without taking anything away, and the person hears it.

The warning is deliberately vague about substance. It says *that* a report was
upheld, not *what* the person did wrong. For a repeat of something obvious
that is enough; for anything subtler it is not, and the honest answer there is
a conversation, not a notification. This is a real cost, accepted knowingly:
the alternative on offer was leaking the reporter.

A person can now learn that they were reported. That is inherent in being
warned and is not new information leakage of a kind the system otherwise
avoids — but it does mean an admin choosing `warn` is choosing to tell them,
where `dismiss` and the removal actions still say nothing.

`resolution_note` is now load-bearing as an internal field. A future change
that surfaces it has to relabel it first, and cannot be retroactive.

## Considered options

**Relay the resolution note.** The most useful warning, and the one that
retroactively publishes private text and can name the reporter. Rejected on
the reporter-safety ground above.

**Relay the reporter's `reason`.** Rejected for the same reason and one more:
it is the accusation in the accuser's words, delivered without review, to the
person accused.

**A separate "message to the person warned" field.** The honest way to say
more: an admin writes text knowing who reads it, in a field labelled as such,
beside the internal note rather than instead of it. Not built here — it is new
UI and a second decision about audience — but it is the forward path, and it
is why the note is left alone rather than repurposed.

**Delete `warn` from the menu.** Consistent, and the wrong direction. The
problem is that the mild rung does nothing, not that the mild rung exists.

**Email the warning.** Rejected. It matches suspension, which is in-app only,
and a moderation notice that arrives in a mailbox is harder to scope to the
instance it came from.
