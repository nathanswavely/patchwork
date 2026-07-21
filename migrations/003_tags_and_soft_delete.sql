-- 003_tags_and_soft_delete.sql
-- Add tags system and soft-delete support for nodes.

CREATE TABLE tags (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE node_tags (
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    tag_id  TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (node_id, tag_id)
);
CREATE INDEX idx_node_tags_tag_id ON node_tags(tag_id);

ALTER TABLE nodes ADD COLUMN status TEXT NOT NULL DEFAULT 'active';
