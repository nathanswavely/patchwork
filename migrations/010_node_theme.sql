-- Add theme column to nodes for quilt block sub-theme selection.
-- Nullable: when NULL, the frontend assigns one deterministically from the node ID hash.
ALTER TABLE nodes ADD COLUMN theme TEXT DEFAULT NULL;
