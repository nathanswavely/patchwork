-- Edit rights on an event link
-- (docs/adr/2026-10-19-co-hosts-read-as-equals-and-the-owner-can-share-the-pen.md).
-- Granted and revoked only by the event's own patch. It does anything only
-- while the link is confirmed: a pending link is invisible everywhere, and
-- a grant on one waits for the other side's consent like the link does.
-- Whoever speaks for the linked patch may then edit every field the event
-- form edits except visibility; deleting, review and detach stay with the
-- owner, and an imported event stays read-only to everyone.
ALTER TABLE event_links ADD COLUMN can_edit INTEGER NOT NULL DEFAULT 0 CHECK (can_edit IN (0, 1));
