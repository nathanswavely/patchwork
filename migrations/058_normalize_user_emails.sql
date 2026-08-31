-- Canonicalize users.email to trimmed lowercase.
--
-- Sign-in stored whatever capitalization was typed at signup, and looked
-- accounts up with an exact `WHERE email = ?` under SQLite's BINARY
-- collation. So an account stored as 'Bob@Example.com' was reachable only
-- by typing that exact capitalization; 'bob@example.com' missed it and fell
-- through to the "new email" branch, offering to create a second account.
--
-- This migration is the other half of NormalizeEmail (internal/auth/email.go)
-- and must ship with it. Normalizing the lookup alone would strand every
-- mixed-case row that already exists — they would stop matching anything.
--
-- Collisions: `users.email` is UNIQUE, but case-sensitively so, which means
-- two rows differing only in capitalization can already exist. Canonicalizing
-- both is impossible, and picking a winner is worse than impossible — the
-- loser's owner would type their own address, have it normalized onto the
-- winner's row, and be signed into someone else's account. There is no
-- silent resolution that is not an account takeover, so this refuses to run
-- and hands the choice to the operator. Startup fails loudly with the
-- remedy; the alternative fails quietly with somebody's account.
--
-- The reference instance (lancasterpatchwork.org, 7 users) had zero
-- non-canonical rows and zero colliding pairs when this was written, so the
-- guard is expected to be dormant everywhere. It exists because "expected"
-- is not "guaranteed" on a self-hosted platform.

CREATE TRIGGER _058_email_case_collision
BEFORE UPDATE OF email ON users
FOR EACH ROW
WHEN EXISTS (
  SELECT 1 FROM users other
  WHERE other.id <> NEW.id AND other.email = NEW.email
)
BEGIN
  SELECT RAISE(ABORT,
    'migration 058: two accounts hold the same email address differing only in capitalization, so it cannot be canonicalized without merging one person''s account into another''s. Find them with: SELECT id, username, email FROM users WHERE lower(trim(email)) IN (SELECT lower(trim(email)) FROM users WHERE email IS NOT NULL AND email <> '''' GROUP BY 1 HAVING count(*) > 1); then decide which account keeps the address and clear or change the other (UPDATE users SET email = NULL WHERE id = ...) before starting again.');
END;

UPDATE users
SET email = lower(trim(email))
WHERE email IS NOT NULL
  AND email <> lower(trim(email));

DROP TRIGGER _058_email_case_collision;
