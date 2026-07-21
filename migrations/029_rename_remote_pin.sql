-- "Pin" retired as the UI word for events (docs/adr/027). The notification
-- type key follows the backend-speaks-generic convention: remote.event.
-- Links carried a "?pin=" dedup tail; rewrite it too, so redeliveries of a
-- pre-rename event still dedup exactly against the migrated rows.
UPDATE notifications SET type = 'remote.event' WHERE type = 'remote.pin';
UPDATE notifications
   SET link = REPLACE(link, '?pin=', '?event=')
 WHERE type = 'remote.event' AND link LIKE '%?pin=%';
