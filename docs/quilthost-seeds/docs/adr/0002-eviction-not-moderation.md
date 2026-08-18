# Eviction, not moderation

Quilthost enforces the anti-discrimination baseline (patchwork docs/adr/025
makes it a contract term) without any privileged access to instance data.
The check runs on what any visitor can see — the lining is always public,
amendments to it are public and badged (patchwork docs/adr/037) — plus an
abuse-report channel. The only remedy is instance-granular: warn the
steward, then terminate hosting. Quilthost never reaches inside a quilt to
remove a patch, an event, or a person; inside the quilt, the instance
admin governs. The contract binds the steward's governance ("where your
quilt breaches the baseline, you moderate; if you won't, we stop hosting
you"), never individual members' speech.

Consequence, accepted: Quilthost is slower than a backdoor-wielding host at
responding to abuse, and members-only or private-patch harms are invisible
to it except through reports from inside the community. That is the same
trade Patchwork itself makes with per-document visibility.

The export has two tiers, both automated. **Ordinary endings**
(cancellation, non-payment, dissolution): full self-serve export through
the retention window, unconditional — this is the promise, and it is
absolute. **Eviction for cause**: export provided by default but at
Quilthost's stated discretion, plus the narrow named carve-out (no export
where providing it would itself be unlawful). The discretion is printed
at signup and on the seamrip guarantee page itself, so the guarantee
never claims more than it delivers. "Anti-lock-in within reason": the
reason is liability, and it is named, narrow, and disclosed rather than
buried. Industry note: even the ordinary-endings tier exceeds the norm —
mainstream hosts cut export access off with the account at termination —
which is why it is a written promise rather than an assumed courtesy.
