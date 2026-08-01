-- An image for a patch and for an event (docs/adr/007, first slice).
--
-- ADR 007 decided the binary never touches media bytes: SQLite stores a
-- reference, the bytes live in a bucket the patch pays for, and uploads go
-- browser-to-bucket through a presigned URL. All of that still stands. None of
-- it is here.
--
-- This is the reference half without the upload half. A patch pastes a URL to
-- a flyer it already hosts, the way `users.avatar_url` has worked since
-- migration 001 — the only media model this codebase has ever had, and the one
-- with no infrastructure behind it. When ADR 007's presigned flow lands it
-- becomes a way to *produce* one of these URLs, not a different column.
--
-- An arts-scene instance with no flyers and no show photos is the gap that
-- makes this worth shipping ahead of the bucket.
ALTER TABLE nodes ADD COLUMN image_url TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN image_url TEXT NOT NULL DEFAULT '';

-- Alt text, required whenever a URL is set (enforced in the handler).
--
-- ADR 007 made this the a11y baseline and gave it a second job: it is what
-- survives when the bytes go. A remote image is one expired link, one deleted
-- Squarespace page, or one blocked host away from being nothing at all, and a
-- caption is the difference between a broken frame and "flyer for the March
-- 14th show at The Selvage".
ALTER TABLE nodes ADD COLUMN image_alt TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN image_alt TEXT NOT NULL DEFAULT '';
