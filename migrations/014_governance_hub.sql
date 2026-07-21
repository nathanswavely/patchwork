-- Unified Governance Hub: comments, reactions, and proposal revisions.
-- Comments are stored in SQLite (process/discussion), not git (documents).

-- Threaded comments on proposals
CREATE TABLE proposal_comments (
  id TEXT PRIMARY KEY,
  proposal_id TEXT NOT NULL REFERENCES proposals(id) ON DELETE CASCADE,
  parent_id TEXT REFERENCES proposal_comments(id) ON DELETE CASCADE,
  author_id TEXT NOT NULL REFERENCES users(id),
  body TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  ap_id TEXT
);
CREATE INDEX idx_proposal_comments_proposal ON proposal_comments(proposal_id);
CREATE INDEX idx_proposal_comments_parent ON proposal_comments(parent_id);

-- Emoji reactions on comments
CREATE TABLE comment_reactions (
  id TEXT PRIMARY KEY,
  comment_id TEXT NOT NULL REFERENCES proposal_comments(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id),
  emoji TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  UNIQUE(comment_id, user_id, emoji)
);
CREATE INDEX idx_comment_reactions_comment ON comment_reactions(comment_id);

-- Proposal revisions (versioned edits by the author)
CREATE TABLE proposal_revisions (
  id TEXT PRIMARY KEY,
  proposal_id TEXT NOT NULL REFERENCES proposals(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  proposed_body TEXT,
  revision_number INTEGER NOT NULL,
  author_id TEXT NOT NULL REFERENCES users(id),
  change_note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_proposal_revisions ON proposal_revisions(proposal_id, revision_number);

-- Track governance setup completion per node
ALTER TABLE nodes ADD COLUMN governance_setup_complete BOOLEAN DEFAULT FALSE;
