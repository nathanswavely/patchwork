-- 044_drop_instance_icon.sql
-- The quilt icon is drafted, not uploaded (docs/adr/042), so nothing reads
-- the uploaded-icon table or the old default-block choice any more. Drop
-- both rather than leave a dead 512KB blob and a stale key behind.
--
-- This is destructive and one-way: an instance that had uploaded an icon
-- loses those bytes here. It has already stopped serving them — the
-- endpoint renders the drafted design, or a starter block assigned from
-- the quilt's name, from the moment the new binary boots. An operator who
-- wants the old image back takes it from a backup taken before this
-- migration ran; it was never part of a seamrip export either way
-- (docs/adr/002, docs/adr/014).

DROP TABLE IF EXISTS instance_icon;

DELETE FROM instance_settings WHERE key = 'icon_default';
