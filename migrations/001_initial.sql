-- 001_initial.sql
-- Core schema for Patchwork. All IDs are UUIDv7 TEXT. All timestamps are ISO 8601 TEXT.

CREATE TABLE users (
    id          TEXT PRIMARY KEY,
    email       TEXT UNIQUE,
    username    TEXT UNIQUE NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    bio         TEXT NOT NULL DEFAULT '',
    avatar_url  TEXT NOT NULL DEFAULT '',
    role        TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('member', 'admin', 'moderator')),
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE credentials (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id   BLOB NOT NULL UNIQUE,
    public_key      BLOB NOT NULL,
    attestation_type TEXT NOT NULL DEFAULT '',
    aaguid          BLOB,
    sign_count      INTEGER NOT NULL DEFAULT 0,
    name            TEXT NOT NULL DEFAULT 'Passkey',
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_credentials_user_id ON credentials(user_id);

CREATE TABLE magic_links (
    id          TEXT PRIMARY KEY,
    email       TEXT NOT NULL,
    token       TEXT NOT NULL UNIQUE,
    expires_at  TEXT NOT NULL,
    used        INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_magic_links_token ON magic_links(token);

CREATE TABLE invite_links (
    id          TEXT PRIMARY KEY,
    created_by  TEXT NOT NULL REFERENCES users(id),
    token       TEXT NOT NULL UNIQUE,
    max_uses    INTEGER NOT NULL DEFAULT 1,
    use_count   INTEGER NOT NULL DEFAULT 0,
    expires_at  TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_invite_links_token ON invite_links(token);

CREATE TABLE nodes (
    id          TEXT PRIMARY KEY,
    parent_id   TEXT REFERENCES nodes(id) ON DELETE SET NULL,
    owner_id    TEXT NOT NULL REFERENCES users(id),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    node_type   TEXT NOT NULL DEFAULT 'leaf' CHECK (node_type IN ('container', 'leaf')),
    latitude    REAL,
    longitude   REAL,
    address     TEXT NOT NULL DEFAULT '',
    website     TEXT NOT NULL DEFAULT '',
    visibility  TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private', 'unlisted')),
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_nodes_parent_id ON nodes(parent_id);
CREATE INDEX idx_nodes_owner_id ON nodes(owner_id);
CREATE INDEX idx_nodes_slug ON nodes(slug);

CREATE TABLE edges (
    id          TEXT PRIMARY KEY,
    source_id   TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    target_id   TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    edge_type   TEXT NOT NULL DEFAULT 'connection' CHECK (edge_type IN ('connection', 'collaboration', 'membership')),
    weight      REAL NOT NULL DEFAULT 1.0,
    metadata    TEXT NOT NULL DEFAULT '{}',
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(source_id, target_id, edge_type)
);
CREATE INDEX idx_edges_source_id ON edges(source_id);
CREATE INDEX idx_edges_target_id ON edges(target_id);

CREATE TABLE events (
    id          TEXT PRIMARY KEY,
    node_id     TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    created_by  TEXT NOT NULL REFERENCES users(id),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    location    TEXT NOT NULL DEFAULT '',
    latitude    REAL,
    longitude   REAL,
    starts_at   TEXT NOT NULL,
    ends_at     TEXT,
    recurrence  TEXT NOT NULL DEFAULT '',
    visibility  TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private', 'unlisted')),
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_events_node_id ON events(node_id);
CREATE INDEX idx_events_starts_at ON events(starts_at);

CREATE TABLE memberships (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    node_id     TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('member', 'admin', 'moderator')),
    joined_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(user_id, node_id)
);
CREATE INDEX idx_memberships_user_id ON memberships(user_id);
CREATE INDEX idx_memberships_node_id ON memberships(node_id);

CREATE TABLE proposals (
    id          TEXT PRIMARY KEY,
    node_id     TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    author_id   TEXT NOT NULL REFERENCES users(id),
    title       TEXT NOT NULL,
    body        TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'approved', 'rejected', 'withdrawn')),
    voting_ends_at TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_proposals_node_id ON proposals(node_id);

CREATE TABLE votes (
    id          TEXT PRIMARY KEY,
    proposal_id TEXT NOT NULL REFERENCES proposals(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id),
    value       TEXT NOT NULL CHECK (value IN ('approve', 'reject', 'abstain')),
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(proposal_id, user_id)
);
CREATE INDEX idx_votes_proposal_id ON votes(proposal_id);

CREATE TABLE audit_log (
    id          TEXT PRIMARY KEY,
    user_id     TEXT REFERENCES users(id) ON DELETE SET NULL,
    action      TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    metadata    TEXT NOT NULL DEFAULT '{}',
    ip_address  TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_audit_log_user_id ON audit_log(user_id);
CREATE INDEX idx_audit_log_entity ON audit_log(entity_type, entity_id);
CREATE INDEX idx_audit_log_created_at ON audit_log(created_at);

CREATE TABLE content_reports (
    id              TEXT PRIMARY KEY,
    reporter_id     TEXT NOT NULL REFERENCES users(id),
    entity_type     TEXT NOT NULL,
    entity_id       TEXT NOT NULL,
    reason          TEXT NOT NULL,
    details         TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'reviewed', 'resolved', 'dismissed')),
    reviewed_by     TEXT REFERENCES users(id),
    resolution_note TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_content_reports_status ON content_reports(status);
CREATE INDEX idx_content_reports_entity ON content_reports(entity_type, entity_id);
