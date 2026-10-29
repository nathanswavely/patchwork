-- The patch a notification is about, so the list can name it.
--
-- A notification row stored only its link, so the one thing that differs
-- between most rows, which patch said it, was readable only when a title
-- happened to include the name. The in-app channel already holds the patch on
-- the event it delivers; this keeps it.
--
-- Nullable, because not every notification comes from a patch: a moderation
-- outcome, an account change, the bulletin, an instance admin's queue and a
-- followed patch on another quilt have none. The UI decides what to call the
-- sender by type; this column only supplies the name when there is one.
--
-- Existing rows are filled from their links, the same match the archived
-- check in internal/handler/notifications.go makes, plus an event's own patch
-- for the canonical /events/{id} shape. A row neither shape matches stays
-- NULL and shows no patch, which is what it showed before.
ALTER TABLE notifications ADD COLUMN node_id TEXT REFERENCES nodes(id) ON DELETE SET NULL;

UPDATE notifications SET node_id = (
    SELECT n.id FROM nodes n
     WHERE notifications.link = '/patches/' || n.slug
        OR notifications.link LIKE '/patches/' || n.slug || '/%'
        OR notifications.link LIKE '/patches/' || n.slug || '?%'
     LIMIT 1)
 WHERE link LIKE '/patches/%';

UPDATE notifications SET node_id = (
    SELECT e.node_id FROM events e
     WHERE notifications.link = '/events/' || e.id
        OR notifications.link LIKE '/events/' || e.id || '?%'
     LIMIT 1)
 WHERE node_id IS NULL AND link LIKE '/events/%';
