-- A post says whether AI made it
-- (docs/adr/2026-10-20-a-post-says-whether-ai-made-it.md).
--
-- One answer per published text and one per image, with three values and
-- NULL. NULL is "not asked, or not stated": every row that exists today, and
-- everything written elsewhere (event sources, federation), which nobody here
-- can answer for. Nobody is labelled retroactively.
--
--   none       no AI
--   assisted   AI helped (editing, cleanup, translation); recorded, not shown
--   generated  AI made this; shown as a tag
--
-- Images are asked separately because a flyer's artwork and its description
-- are often made by different people and tools.
ALTER TABLE events ADD COLUMN ai_use TEXT CHECK (ai_use IN ('none', 'assisted', 'generated'));
ALTER TABLE events ADD COLUMN image_ai TEXT CHECK (image_ai IN ('none', 'assisted', 'generated'));
ALTER TABLE nodes ADD COLUMN ai_use TEXT CHECK (ai_use IN ('none', 'assisted', 'generated'));
ALTER TABLE nodes ADD COLUMN image_ai TEXT CHECK (image_ai IN ('none', 'assisted', 'generated'));
ALTER TABLE proposals ADD COLUMN ai_use TEXT CHECK (ai_use IN ('none', 'assisted', 'generated'));
ALTER TABLE notices ADD COLUMN ai_use TEXT CHECK (ai_use IN ('none', 'assisted', 'generated'));

-- Whether a patch's work relies heavily on AI-generated content: a newsletter
-- a model writes, generated music, listings a model writes. NULL is not
-- stated, which is also what an unclaimed patch always says (docs/adr/039).
-- A 1 is a public badge, never a mark on the quilt.
ALTER TABLE nodes ADD COLUMN ai_work INTEGER CHECK (ai_work IN (0, 1));

-- The per-person discovery filter for declared patches, the twin of
-- hide_amended_linings. There is no instance-wide one: an instance that wants
-- to exclude labelled AI work is writing a different policy.
ALTER TABLE users ADD COLUMN hide_ai_work INTEGER NOT NULL DEFAULT 0;
