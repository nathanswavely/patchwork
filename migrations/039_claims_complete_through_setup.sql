-- Migration 039: claims complete through setup (docs/adr/039).
--
-- Unclaimed patches carry no governance: a directory listing nobody has
-- claimed cannot have consented to a lining, so any lining row sitting on
-- one is fabricated consent by definition — no member could have adopted or
-- amended it. Deleted unconditionally, no exceptions: patch setup re-adopts
-- the lining out loud regardless of what was here before.
DELETE FROM governance_docs WHERE kind = 'lining' AND node_id IN (SELECT id FROM nodes WHERE status != 'active');

-- A verified or approved claim is now a single-use, expiring (14 days) right
-- to enter patch setup, not an instant handoff. NULL until a claim is
-- approved; setup consumes the window by activating the node, not by
-- changing this column, so an approved-and-set-up claim still carries its
-- (now irrelevant) original expiry.
ALTER TABLE claim_requests ADD COLUMN setup_expires_at TEXT;
