-- The native apps this quilt vouches for.
-- docs/adr/2026-09-20-an-instance-vouches-for-an-app.md
--
-- A passkey belongs to a domain, and a phone will only hand one to an app
-- when the domain says, in a file it publishes itself, that the app is
-- allowed to hold it. Apple reads /.well-known/apple-app-site-association,
-- Android reads /.well-known/assetlinks.json, and the only fact either file
-- carries is an app identifier this instance decided to name.
--
-- So the identifiers live in a table an instance admin fills in, and the
-- two files are rendered from the rows. Empty is the default and the
-- ordinary state: an instance that has listed nothing serves neither file,
-- and nothing about anybody's app store appears on a quilt that did not ask
-- for it. Nothing is seeded, here or anywhere else in the project — the
-- reference deployment's own apps would be somebody else's identifiers on
-- every fork that ever ran this migration.
--
-- The table stays behind in a seamrip (internal/seamrip names it with the
-- reason): a fork answers at a different domain, so it publishes different
-- files, and the apps it vouches for are its own decision to make.

CREATE TABLE native_apps (
    -- UUIDv7, minted by auth.NewUUIDv7 like every other id here.
    id           TEXT PRIMARY KEY,
    platform     TEXT NOT NULL CHECK (platform IN ('apple', 'android')),
    -- apple: "TEAMID.bundle.id" — the ten-character Team ID, a dot, the
    -- bundle identifier. android: the package name.
    identifier   TEXT NOT NULL,
    -- android only: a JSON array of SHA-256 signing-certificate
    -- fingerprints, uppercase hex pairs joined by colons
    -- ("AA:BB:…", 32 pairs). Apple rows carry '[]'. It is a JSON column
    -- rather than a child table because a fingerprint has no identity of
    -- its own: it is one of the app's, it is rewritten whole when the app
    -- is re-listed, and nothing ever joins to one.
    fingerprints TEXT NOT NULL DEFAULT '[]',
    -- What the admin calls it, so the list reads like their apps rather
    -- than like a column of identifiers. Sanitised and capped the way a
    -- passkey nickname is (auth.SanitizeCredentialName is the model).
    label        TEXT NOT NULL DEFAULT '',
    -- Who listed it. A reference rather than a cascade: docs/adr/086 keeps
    -- a deleted account as a tombstone, so the row survives its author.
    added_by     TEXT REFERENCES users(id),
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    -- One row per app. Listing an app twice is not a second grant, it is
    -- the same grant typed again, and the add path answers 409 rather than
    -- quietly writing a duplicate into a published file.
    UNIQUE (platform, identifier)
);
