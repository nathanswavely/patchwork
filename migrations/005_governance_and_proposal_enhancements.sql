-- 005_governance_and_proposal_enhancements.sql
-- Add governance_docs table and enhance proposals with type and duration.

CREATE TABLE governance_docs (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_governance_docs_node_id ON governance_docs(node_id);

ALTER TABLE proposals ADD COLUMN proposal_type TEXT NOT NULL DEFAULT 'other' CHECK (proposal_type IN ('amendment', 'membership', 'action', 'other'));
ALTER TABLE proposals ADD COLUMN duration_hours INTEGER NOT NULL DEFAULT 72;
