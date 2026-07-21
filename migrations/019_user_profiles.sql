-- User profiles (docs/adr/006-profile-privacy-one-switch-membership-visibility.md).
--
-- memberships.visible is the one membership-visibility switch: it controls
-- both whether the membership appears on the person's profile and whether
-- the person appears in the patch's public member list. Default visible.
-- Hidden memberships are still seen by that patch's admins and members.
ALTER TABLE memberships ADD COLUMN visible INTEGER NOT NULL DEFAULT 1;

-- Profile links, same shape as nodes.links (011_node_links.sql):
-- [{"url": "https://...", "label": "Website"}, ...]
ALTER TABLE users ADD COLUMN links TEXT DEFAULT '[]';
