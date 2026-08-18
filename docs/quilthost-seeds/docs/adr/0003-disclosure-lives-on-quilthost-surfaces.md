# Disclosure lives on Quilthost's surfaces, not inside the instance

Hosted quilts run the unmodified public binary, so there is no Quilthost
surface inside an instance — no login interstitial, no injected banner —
and building one would fork the product. The "you are trading control
for convenience" disclosure therefore lives on the three surfaces that
exist: (1) **signup**, a plain-language panel stating that Patchwork is
designed to be self-hosted, exactly what hosting trades away and what
the steward keeps, closed with a one-time explicit acknowledgment — the
liability moment; (2) **the console**, a standing "Self-host this quilt"
page holding the exit runbook one click from the dashboard — the durable
reminder is the visible presence of the door, not a nag; (3) **the
Label**, Patchwork's own member-facing disclosure page (patchwork
docs/adr/023) — Quilthost hands every new steward prewritten Label text at
setup ("hosted by Quilthost, $N/mo, seamrip export available to admins at
any time"), but never writes it for them: the Label is steward-stated by
design, and Quilthost has no write path into instances anyway (ADR 0002).

Rejected: making Label disclosure a contract term. Transparency as
obligation reads well, but it would have Quilthost policing the content of
a community's own governance document — exactly what eviction-not-
moderation forswears.
