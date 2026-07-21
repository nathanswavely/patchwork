-- 029: events on unclaimed patches (docs/adr/026).
-- Anyone may submit an event to a patch they don't run; it waits in
-- pending_review for whoever owns the calendar (instance admin for
-- unclaimed patches, patch admins for active ones). Trusted contributors
-- (an instance-level users flag, granted explicitly by the instance
-- admin) skip review on unclaimed patches only.

ALTER TABLE events ADD COLUMN status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE users ADD COLUMN trusted_contributor INTEGER NOT NULL DEFAULT 0;

-- Per-patch choice: whether an active patch accepts event suggestions
-- from non-members. Patch-admin owned, default on; irrelevant for
-- unclaimed patches (their door is always the instance admin's queue).
ALTER TABLE nodes ADD COLUMN accept_event_suggestions INTEGER NOT NULL DEFAULT 1;

CREATE INDEX idx_events_status ON events(status);
