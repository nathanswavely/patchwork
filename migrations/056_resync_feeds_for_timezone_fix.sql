-- 056: make every attached feed re-read itself once (docs/adr/065).
--
-- Feed times that carried no zone were read as UTC, so a venue on
-- Eastern time had every such event stored four or five hours early and
-- every all-day event stored on the wrong day. The parsers now read
-- those in the instance's zone — but a parser fix alone repairs nothing
-- already in the events table, because a feed only reparses when it
-- comes back changed. Conditional GET is doing its job: unchanged
-- calendar, one 304, no reconcile, wrong times left standing.
--
-- So drop the conditional-GET state. The next sync fetches in full,
-- reparses, and the reconciler writes the corrected starts_at onto the
-- rows it already has.
UPDATE event_sources SET etag = NULL, last_modified = NULL;
UPDATE aggregators SET etag = NULL, last_modified = NULL;

-- And make that one sync quiet. A non-recurring event keeps its
-- identity across the fix (same UID, no occurrence) and is UPDATEd in
-- place, but an occurrence of a RECURRING event is identified by its
-- start instant — which is precisely what changes here. Those arrive as
-- new keys, and a sync that has succeeded before announces new events
-- to every follower. Nulling last_success_at makes this one pass adopt
-- the corrected calendar the way a first sync does: silently. It is set
-- again by the very next successful fetch, so exactly one sync is
-- quiet.
UPDATE event_sources SET last_success_at = NULL;
UPDATE aggregators SET last_success_at = NULL;

-- The cached listings a crosswalk entry reads from were parsed under the
-- old rule too, and are deliberately left in place: they are replaced
-- wholesale on the aggregator's next successful fetch, and a crosswalk
-- entry that syncs before its aggregator finds the old times for one
-- cycle. Emptying the cache instead would hand that entry a feed
-- carrying nothing, and absence is how this reconciler spells removal —
-- it would delete every future event it owns and re-create them minutes
-- later under new ids, losing their event links and RSVPs on the way.
