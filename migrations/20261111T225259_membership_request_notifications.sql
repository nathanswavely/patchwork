-- A request notification goes when the request does, and an invitation names
-- who made it.
--
-- notifications.membership_id ties a membership.request row to the request it
-- asks about. Approving, rejecting and the requester's own withdrawal delete
-- every notification tied to that membership, so the bell stops counting a
-- request nobody can act on any more (F-045). Before this a request's line
-- survived its answer and the unread badge counted it, so an admin read three
-- lines, a badge saying two, and a queue saying none. Nullable: almost every
-- other notification is about something else, and is left alone.
--
-- ON DELETE CASCADE because a membership row that is deleted (a declined or
-- rescinded invitation, an erased account) leaves no request to answer.
ALTER TABLE notifications ADD COLUMN membership_id TEXT REFERENCES memberships(id) ON DELETE CASCADE;
CREATE INDEX idx_notifications_membership ON notifications(membership_id) WHERE membership_id IS NOT NULL;

-- Rows written before the column carry no tie, so the ones already answered
-- are found by date instead: a request line older than every request still
-- open on its patch was answered, because the notification is written just
-- after the membership row it announces. A line on a patch with no open
-- request at all is answered too. The rest stay, untied, until the 90-day
-- sweep: they may be about a request that is still waiting.
DELETE FROM notifications
 WHERE type = 'membership.request'
   AND node_id IS NOT NULL
   AND created_at < COALESCE(
       (SELECT MIN(m.joined_at) FROM memberships m
         WHERE m.node_id = notifications.node_id AND m.status = 'pending'),
       '9999');

-- Who issued an outstanding invitation (docs/adr/098), so the person invited
-- and the patch's admins can both read it beside the invitation (F-044).
-- Set when an admin invites and cleared when the invitation is accepted:
-- the column describes the invitation, and once it is answered there is none.
-- The audit log keeps membership.invite with its actor either way.
ALTER TABLE memberships ADD COLUMN invited_by TEXT REFERENCES users(id);
