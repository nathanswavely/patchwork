-- Migration 042: repair notification links that point nowhere (issue #56).
-- (Originally merged as a second migration 041 — renumbered; the number
-- collided with 041_governance_config_leadership from a parallel branch.
-- Both are idempotent, so instances that already ran this under the old
-- name no-op through the re-run.)
--
-- Two shapes were built by hand in Go and never matched a registered SPA
-- route (see internal/weblink for the fix at the source):
--
--   /patches/{slug}/events/{id}      — no such route; the SPA fell through
--                                      to the home quilt. Events are
--                                      addressed globally: /events/{id}.
--   /patches/{slug}/governance/{id}  — a real route, but the PROPOSAL one.
--                                      A charter's id rendered there as a
--                                      proposal that doesn't exist.
--
-- The code fix only helps notifications sent from here on. Every row already
-- in the table still carries a dead link, so rewrite them in place.
--
-- Both rewrites search for their marker segment in substr(link, 10) — past
-- the fixed '/patches/' prefix — rather than in the whole link. A patch
-- slugged "events" or "governance" would otherwise have its own slug
-- mistaken for the marker and get spliced at the wrong offset. Slugs and ids
-- contain no '/', so the first match past the prefix is the right one.

-- Events: drop the patch scope entirely. instr() lands on the '/events/'
-- marker; +8 steps over it to the id.
UPDATE notifications
SET link = '/events/' || substr(link, instr(substr(link, 10), '/events/') + 17)
WHERE link LIKE '/patches/%/events/%';

-- Charters: insert the 'docs/' segment that separates a charter from a
-- proposal. Guarded on the trailing segment actually being a charter id, so
-- the far more numerous proposal links are left alone. Links that already
-- carry 'docs/' can't match either — their trailing segment is 'docs/{id}',
-- which is not an id in governance_docs.
UPDATE notifications
SET link = substr(link, 1, instr(substr(link, 10), '/governance/') + 20)
        || 'docs/'
        || substr(link, instr(substr(link, 10), '/governance/') + 21)
WHERE link LIKE '/patches/%/governance/%'
  AND substr(link, instr(substr(link, 10), '/governance/') + 21) IN (SELECT id FROM governance_docs);
