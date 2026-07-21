-- Replace nodes.theme (palette key only) with nodes.appearance — a JSON
-- object holding the patch's full chosen tile appearance:
--   {"palette": "anthem", "block": "pinwheel", "rotation": 90}
-- NULL means unset: the tile is hash-assigned from the patch ID.
-- See docs/adr/004-tile-appearance-storage-and-registry.md.

ALTER TABLE nodes ADD COLUMN appearance TEXT DEFAULT NULL;

UPDATE nodes SET appearance = json_object('palette', theme) WHERE theme IS NOT NULL;

ALTER TABLE nodes DROP COLUMN theme;
