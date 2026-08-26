-- 057: an event carries the zone it happens in (docs/adr/045).
--
-- `starts_at` is a UTC instant and stays one: it is the sort key, the
-- index, and the wire format, and every comparison in the codebase is a
-- lexicographic string compare against it. What was missing is not a
-- different storage format but the fact the instant encodes — the wall
-- clock the organizer meant. A show at The Selvage starts at 8pm: 8pm on
-- the flyer, 8pm at the door, 8pm for the band loading in. We were
-- storing the encoding and reconstructing the fact against whatever zone
-- the reader's laptop happened to be set to.
--
-- Both columns are nullable because NULL means inherit, and inheritance
-- is resolved at read time: event → patch → instance → UTC. Storing the
-- patch's zone onto each event instead would freeze a copy that stops
-- tracking the patch, and would make the common case — an organizer
-- posting a show at their own venue — a field somebody has to fill.
ALTER TABLE events ADD COLUMN timezone TEXT;
ALTER TABLE nodes ADD COLUMN timezone TEXT;

-- No backfill, and none is needed. Existing `starts_at` values were
-- written by browsers standing in the community's own zone, so they are
-- already correct instants; setting geographic.timezone renders the whole
-- corpus right without rewriting a row. A quilt that sets nothing falls
-- back to UTC, which is exactly today's behaviour — so this migration
-- changes no rendering until an operator says where the quilt is.
