# Lifecycle and money: card upfront, identical tiers, never-hostage suspension

Signup takes a card (Stripe Checkout) with the first 30 days free —
generosity lives in the free month, not a cardless trial, because a
cardless trial on a platform that sends email and hosts public content
is a spam-farm invitation. Tiers are self-selected with no verification
— Solidarity $6 / Community $12 / Supporter $24 — and the product is
identical at every tier: feature-gating would violate "same binary,
always" (the features are Patchwork's, not Quilthost's to gate), and
per-member pricing punishes the growth the platform exists for. A quilt
that outgrows the standard container is an ops conversation, not a tier.

Instance lifecycle: provisioning → active → past_due → suspended →
terminated → retained → deleted. Past due (days 0–14): Stripe retries,
console nags, the quilt stays fully up — a lapsed card never takes a
community offline while the machine retries. Suspended (day 14):
container stopped, edge serves a plain "this quilt is resting" page;
**the export stays self-serve during suspension** — a suspended
community must be able to leave, or suspension is a hostage state (the
industry failure deliberately not copied). Terminated (after 60 days
suspended, or on voluntary cancellation): container and route gone,
90-day retention window begins (ADR 0002), export still self-serve.
Deleted: volume and backups actually erased, steward notified first.
