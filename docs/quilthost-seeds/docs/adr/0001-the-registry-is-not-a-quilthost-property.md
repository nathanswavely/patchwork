# The registry is not a Quilthost property

Quilthost is the hosting arm only. The public registry of quilts (patchwork
docs/adr/025) is an independent editorial artifact: submission-based, open
to any quilt that meets the published criteria (anti-discrimination
baseline as the floor), and **Quilthost-hosted quilts submit through the
same door as self-hosted ones** — no automatic listing, no fast lane, no
customer-only directory. The console may offer a "submit this quilt to the
registry" convenience, but it files the same submission anyone can file.

Rejected: the registry as a Quilthost product feature (a directory of
hosted quilts). Commercially attractive, but it turns discovery into a
perk of paying — an asterisk on the anti-lock-in promise — and it
contradicts patchwork ADR 025, which keeps the registry format open
precisely so no operator's list is "the" list.
