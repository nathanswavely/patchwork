-- A sign-in code beside the sign-in link.
--
-- A magic link finishes sign-in in whatever session opens it, which is the
-- browser the mail client hands the URL to. A client that cannot be handed
-- that URL — one with no browser of its own, or one whose session lives
-- somewhere the emailed link will never land — has no way through the front
-- door at all. The same email now also carries a short code the person can
-- read off the screen and type into the client that asked for it, so the
-- proof travels back by hand instead of by redirect. It proves exactly what
-- the link proves: control of the mailbox.
--
-- Both columns hang on the existing row rather than on a table of their own.
-- The link and the code are two ways to answer one request, so they share one
-- expiry, one `used` flag, and one fate: burning either burns both.

-- The sha256 hex of the six-digit code, stored the way `token` stores the
-- link's own hash (migration 022) — a read of the database file yields
-- nothing directly replayable. Nullable, and deliberately so: a row minted
-- before this migration has no code, and a NULL here is what makes it
-- permanently un-code-verifiable rather than verifiable against an empty
-- string. Verification requires the column to be non-NULL.
ALTER TABLE magic_links ADD COLUMN code_hash TEXT;

-- Wrong codes counted against this row. A code is six digits, so the row
-- must stop answering long before a guesser makes progress: at five the row
-- is marked used, which spends the emailed link along with it. Not a rate
-- limit — a limiter keeps the endpoint cheap, whereas this is what bounds
-- the guessing, and it lives on the row so it cannot be escaped by changing
-- address, network or process.
ALTER TABLE magic_links ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
