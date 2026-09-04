-- Migration 062: the contact card — reachable patch by patch (docs/adr/080).
--
-- A person keeps one contact card on their account: a phone number, an
-- email address to be reached at (not the sign-in address), and a short
-- note ("Signal preferred, text before calling"). None of it is public.
-- It is shown to the admins and members of exactly the patches whose
-- membership the person has switched contact sharing on for, in that
-- patch's Members room — never on the profile, never in the public member
-- list, never in the federated actor.
--
-- memberships.share_contact is that per-membership switch. It is a
-- second axis, not a second visibility switch (docs/adr/006 keeps one):
-- visibility governs whether a membership is *known* publicly, sharing
-- governs whether the people already in the room can *reach* you. It
-- defaults off for every patch — nobody's number is shared by a migration.
-- Meaningful only on member/admin rows; a follower is not in the room.

ALTER TABLE users ADD COLUMN contact_phone TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN contact_email TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN contact_note  TEXT NOT NULL DEFAULT '';

ALTER TABLE memberships ADD COLUMN share_contact INTEGER NOT NULL DEFAULT 0;
