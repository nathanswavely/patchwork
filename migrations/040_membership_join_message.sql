-- Migration 040: membership join message — the join sheet's intro (docs/adr/040).
--
-- On approval-required patches, a person requesting membership may attach
-- one optional note to the admins reviewing the request. It is a note to
-- the door, not a record of consent: nulled once the request is resolved,
-- never shown outside that patch's admins, and never set at all for
-- followers or joins that activate immediately (docs/adr/040 — no
-- per-surface consent records).

ALTER TABLE memberships ADD COLUMN join_message TEXT;
