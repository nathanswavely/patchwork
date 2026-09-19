-- 074_suggested_tags.sql
-- A patch may ask for a word. See docs/adr/114.
--
-- tags.status: 'approved' is the vocabulary a patch can pick from and the
-- public tag list. 'pending' is a suggested tag: proposed by a patch admin,
-- worn provisionally by the patch that proposed it, invisible to everyone
-- else until an instance admin approves it. 'rejected' is a word an admin
-- declined; the row stays so the name is spent and the same suggestion does
-- not come back every week, and so an admin's own Add tag form can resurrect
-- it rather than colliding with a UNIQUE constraint on a tag that is not
-- visible anywhere.
--
-- Defaulting to 'approved' is what makes pre-existing rows, and seamrip
-- archives written before this migration, land as ordinary vocabulary. That
-- is the same move migration 026 made for node_tags.position.
ALTER TABLE tags ADD COLUMN status TEXT NOT NULL DEFAULT 'approved';

-- Who coined the word: the first person to suggest it. A second patch
-- proposing the same word attaches to this row rather than coining again,
-- so this is deliberately the coiner and not "every suggestor".
--
-- No ON DELETE CASCADE, and none would fire anyway: docs/adr/086 keeps a
-- deleted account as a tombstone row, so this reference always resolves. A
-- coined word is a vocabulary act rather than the person alone, so it
-- outlives the account and the queue shows "Deleted account". Every read of
-- this column goes through displayNameExpr/usernameExpr.
ALTER TABLE tags ADD COLUMN suggested_by TEXT REFERENCES users(id);

ALTER TABLE tags ADD COLUMN decided_by TEXT REFERENCES users(id);
ALTER TABLE tags ADD COLUMN decided_at TEXT;

CREATE INDEX idx_tags_status ON tags(status);

-- The name column is declared UNIQUE with SQLite's default BINARY collation,
-- so 'Music' and 'music' have always been two different rows. Only the admin
-- page's client code lowercased, which stopped mattering the moment names
-- could arrive from anybody. Normalization now happens server-side on every
-- write path; this index is the constraint that makes that promise binding
-- rather than a convention every future caller has to remember.
--
-- A column constraint cannot be altered in place, so this is an additional,
-- stricter index rather than a replacement, and the original UNIQUE stays.
-- It fails loudly on a database that already holds a case-collision: all 49
-- tags on the reference instance are lowercase, so there is nothing to
-- reconcile there, but an instance that created 'Music' through the API
-- must merge it by hand before upgrading.
CREATE UNIQUE INDEX idx_tags_name_nocase ON tags(name COLLATE NOCASE);
