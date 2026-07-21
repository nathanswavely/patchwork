-- 022_hash_session_tokens.sql
-- Session tokens are now stored as hex-encoded SHA-256 hashes, matching
-- invite_links, magic_links, and signup_tokens. Existing rows hold raw
-- tokens that will never match a hashed lookup, so they are dropped rather
-- than left as dead weight. Cost: every signed-in user logs in once more.

DELETE FROM sessions;
