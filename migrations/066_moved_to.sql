-- 066: the moved-to pointer, on a patch and on a person (docs/adr/090).
--
-- ADR 012's third egress affordance. A fork is only a safety valve if the
-- people who need it can find it, and after a seamrip the old instance is
-- still where every link, flyer and search result points. This column is
-- what lets the old home say where the new one is, set by the community
-- rather than by whoever runs the server.
--
-- Two columns rather than one instance-level pointer, because the two
-- moves are different: a patch can move without the quilt around it, and a
-- person can move without their patches. Nullable TEXT, holding an http or
-- https URL that is checked at every write path the way events.event_url
-- is (docs/adr/079), since both end up as an href on somebody's page.
--
-- Numbering: 064 is account deletion on main, 065 is claimed by an open PR,
-- so this branch was assigned 066 up front rather than reading the highest
-- number on disk (CLAUDE.md, "Claiming a number").
ALTER TABLE nodes ADD COLUMN moved_to TEXT;
ALTER TABLE users ADD COLUMN moved_to TEXT;
