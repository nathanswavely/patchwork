-- Tags carry their motif, and a patch's tags have an order.
-- See docs/adr/021-tags-are-the-only-classification.md.
--
-- tags.motif: optional motif slug (frontend registry key) shown for patches
-- whose tags include this one and who chose no explicit motif. NULL means
-- the tag contributes no motif. Unknown slugs fall through on the frontend
-- (ADR 004 rule), so no slug validation happens here.
--
-- node_tags.position: the patch admin's priority order (0 = first). The
-- first motif-bearing tag wins motif derivation; previously "first" was
-- whatever order an ORDER BY-less join returned, which could flip a patch's
-- derived motif between page loads. Nullable so seamrip archives from
-- before this migration still import (missing keys arrive as NULL); NULL
-- sorts after every explicit position, tie-broken by tag name.

ALTER TABLE tags ADD COLUMN motif TEXT DEFAULT NULL;
ALTER TABLE node_tags ADD COLUMN position INTEGER;

-- Backfill: the previously hardcoded TAG_MOTIFS map from
-- web/src/lib/patchIcons.js, moved into vocabulary data. This map dies in
-- the frontend with this migration.
UPDATE tags SET motif = CASE name
    WHEN 'visual-arts' THEN 'palette'
    WHEN 'gallery'     THEN 'images'
    WHEN 'music'       THEN 'musicNotes'
    WHEN 'venue'       THEN 'buildings'
    WHEN 'theater'     THEN 'maskHappy'
    WHEN 'dance'       THEN 'sneaker'
    WHEN 'film'        THEN 'filmSlate'
    WHEN 'literary'    THEN 'bookOpen'
    WHEN 'food'        THEN 'forkKnife'
    WHEN 'craft'       THEN 'scissors'
    WHEN 'community'   THEN 'usersThree'
    WHEN 'education'   THEN 'gradCap'
    WHEN 'sports'      THEN 'soccerBall'
    WHEN 'tech'        THEN 'code'
    WHEN 'wellness'    THEN 'heartbeat'
    WHEN 'ceramics'    THEN 'paintBrush'
    WHEN 'printmaking' THEN 'stamp'
    WHEN 'radio'       THEN 'radio'
    ELSE motif
END;

-- Deterministic order for pre-existing rows: alphabetical by tag name, per
-- node. Arbitrary but stable; patch admins re-order via the tag picker.
UPDATE node_tags SET position = (
    SELECT COUNT(*)
    FROM node_tags nt2
    JOIN tags t2 ON t2.id = nt2.tag_id
    JOIN tags t1 ON t1.id = node_tags.tag_id
    WHERE nt2.node_id = node_tags.node_id AND t2.name < t1.name
);
