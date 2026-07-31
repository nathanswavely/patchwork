-- Amendments adopted elsewhere (docs/adr/053).
--
-- docs/adr/052 asked the venue question twice — once of leadership, once of
-- proposals — and only the leadership half shipped. This is the other half:
-- a record that a meeting adopted a text, and the text it adopted.
--
-- Why its own table rather than a `kind` on `attestations`. Migration 048
-- expected the opposite ("a 'proposal' kind lands when proposal attestation
-- does"), and working the feature through changed the answer. A leadership
-- attestation is about people: it carries names, a term end, and a supersede
-- chain, and its effect lands on memberships. This one is about a document:
-- it carries a filename and a body, has no names and no term, and its effect
-- lands on a charter. Sharing one table would mean a row whose every other
-- column is NULL and a discriminator branching at every read. Two tables,
-- each with columns that all mean something.
--
-- Nothing here supersedes anything, and that is docs/adr/053's decision
-- rather than an omission: an attestation asserts the whole current text and
-- checks no base, so every record replaces and the chain is the document's
-- git history, which already exists and is already the thing members read.
CREATE TABLE amendment_attestations (
    id      TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    -- The charter this adopted. Nullable because a document can be deleted
    -- and the record of what the community adopted must outlive it.
    doc_id  TEXT REFERENCES governance_docs(id) ON DELETE SET NULL,
    -- The git filename, which is the durable identity (docs/adr/011:
    -- filename = governanceFilename(title)). Kept alongside doc_id so a
    -- record still says which text it was about after the row is gone.
    target_doc TEXT NOT NULL,
    doc_title  TEXT NOT NULL,
    -- When the community actually decided, which is not when it was typed
    -- in. The gap between the two is ordinary and worth showing.
    decided_at TEXT NOT NULL,
    summary    TEXT NOT NULL DEFAULT '',
    -- The whole text as adopted, stored here as well as written to the
    -- charter and to git. Duplication on purpose, and the same call
    -- proposals already make with proposed_body: the charter holds only the
    -- latest text, so without this a record of what a meeting adopted last
    -- March becomes a claim with nothing behind it the moment the next
    -- meeting adopts something else. It also means the record travels whole
    -- through seamrip, where git repos do not.
    adopted_body TEXT NOT NULL,
    git_sha      TEXT NOT NULL DEFAULT '',
    -- Who asserted it. Patchwork cannot check an attestation and does not
    -- pretend to; it records who said what, and when, in public.
    recorded_by TEXT NOT NULL REFERENCES users(id),
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_amendment_attestations_node ON amendment_attestations(node_id);
-- The in-flight notice looks up by document: an open amendment proposal asks
-- whether the ground moved under it since it was drafted (docs/adr/053).
CREATE INDEX idx_amendment_attestations_doc ON amendment_attestations(node_id, target_doc);
