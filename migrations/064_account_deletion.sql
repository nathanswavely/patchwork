-- Erase the person, keep the acts (docs/adr/086).
--
-- Self-serve account deletion does not remove the users row. Most foreign
-- keys into users(id) are RESTRICT — votes, proposals, attestations, notices
-- — because they are a community's record of what it decided, and a DELETE
-- either fails against them or, if the schema were loosened, punches holes in
-- a tally somebody is entitled to audit. So the row survives as a tombstone
-- with every identity column emptied, and this column is what says so.
--
-- Numbering: 064-066 were left open for three sibling branches in flight
-- alongside this one (they hold docs/adr/083-085). Gaps are already normal
-- here — migrations/006 is intentionally absent — and a gap is cheaper than
-- two branches claiming the same number.

ALTER TABLE users ADD COLUMN deleted_at TEXT;

-- Every read that has to tell a tombstone from an account does so by this
-- column, on tables joined per-row (authors of notices, proposals, votes), so
-- it is worth an index despite being NULL for almost every row.
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at) WHERE deleted_at IS NOT NULL;
