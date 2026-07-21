-- Quilt identity (docs/adr/014-quilt-identity-in-the-database.md).
--
-- instance_settings holds community-editable identity overrides (name,
-- description, chosen default icon key). A row here overrides the
-- patchwork.yaml value; yaml stays the bootstrap default. Deployment
-- concerns (domain, ports, SMTP, federation) never live here.
CREATE TABLE instance_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- The single uploaded quilt icon — the one bounded exception to ADR 007
-- (media references, not bytes): exactly one row, PNG or JPEG, <=512KB,
-- validated square 64-1024px at upload. Not part of the seamrip boundary:
-- instance identity does not travel; a fork re-brands.
CREATE TABLE instance_icon (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    mime TEXT NOT NULL,
    data BLOB NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
