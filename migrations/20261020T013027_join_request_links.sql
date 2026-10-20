-- The bell's join-request line opens the queue (F-043).
--
-- MembershipRequest notifications linked to /patches/{slug}/members?status=pending,
-- the ordinary roster with a filter the page never read, so an admin sent to
-- answer a request landed on a list with only their own name on it. The queue
-- is at /patches/{slug}/settings/members, which is where the Dashboard already
-- linked and where internal/weblink.PatchJoinRequests now points.
--
-- The code fix only helps notifications sent from here on, so rewrite the rows
-- already in people's bells. The suffix is replaced by length rather than by a
-- search, so a patch slugged "members" is not spliced at its own slug.
-- '/members?status=pending' is 23 characters. In LIKE only % and _ are special,
-- and the pattern holds neither outside the leading wildcard.
UPDATE notifications
SET link = substr(link, 1, length(link) - 23) || '/settings/members'
WHERE link LIKE '/patches/%/members?status=pending';
