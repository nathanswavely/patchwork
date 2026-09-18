-- Trust has a scope, and a suggestion carries its calendar.
-- See docs/adr/2026-09-18-trust-has-a-scope-and-a-suggestion-carries-its-calendar.md
--
-- ADR 026's trusted-contributor grant was quilt-wide or nothing: a person who
-- suggested one listing and wanted to keep its calendar had to be handed every
-- unclaimed calendar on the instance, so in practice they were handed none and
-- every event they added queued again for the same admin who had just approved
-- the patch. This migration gives the grant a second scope — one patch — and
-- gives a person a way to ask for either.

-- The per-patch grant (decision 2). The same standing ADR 026 describes, on
-- one unclaimed patch and nothing else: posting an event directly, editing
-- your own event, standing in the link handshake (docs/adr/057), CSV upload,
-- and event sources (decision 5). It does not reach the one gate that is
-- about no patch — deriving a verification domain from a new suggestion's
-- website — because a suggestion is not the patch it names.
--
-- Deliberately NOT a membership row: an unclaimed patch admits nobody, and a
-- grant that looked like membership would show up in counts, listings, the
-- electorate and the thread graph. It is a grant, and it lives in its own
-- table so nothing that reads memberships can mistake it for one.
--
-- node_id cascades: the grant is about that patch and nothing else, so it
-- goes with it. user_id does not, because docs/adr/086 keeps a deleted
-- account as a tombstone and CASCADE would never fire anyway; the purge list
-- in internal/handler/deletion_boundary.go deletes these by hand.
--
-- granted_by is the instance admin who gave it, kept as provenance the way
-- tags.suggested_by is. `source` records how: 'suggestion' is the checked
-- line at patch-suggestion approval (decision 3), 'request' is an answered
-- trust request (decision 7), 'admin' is an instance admin granting outright.
CREATE TABLE node_trusted_contributors (
  user_id    TEXT NOT NULL REFERENCES users(id),
  node_id    TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  granted_by TEXT NOT NULL REFERENCES users(id),
  granted_at TEXT NOT NULL,
  source     TEXT NOT NULL CHECK (source IN ('suggestion', 'request', 'admin')),
  PRIMARY KEY (user_id, node_id)
);

CREATE INDEX idx_node_trusted_contributors_node ON node_trusted_contributors(node_id);

-- A trust request (decision 7): the ask, made from the one place the review
-- cost is being paid — the event form, when the event is about to queue on an
-- unclaimed patch.
--
-- `scope` is what was asked for: 'all' is the whole quilt, 'patches' is the
-- named set in trust_request_nodes. `granted_scope` is what the admin
-- actually gave, which may be wider or narrower than the ask; the two are
-- kept as two facts so the audit line can say both.
--
-- `status`: 'pending' until answered. 'moot' is the request that answered
-- itself — every patch it named was claimed before an admin got to it, so
-- there is nothing left to grant and nobody declined anything.
--
-- `note` is the admin's reason, which the requester is told. One open request
-- per person is enforced in the handler rather than by a partial unique
-- index, because 'declined' is not spent forever: a person is not a word, and
-- they may ask again after a cooldown.
CREATE TABLE trust_requests (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL REFERENCES users(id),
  scope         TEXT NOT NULL CHECK (scope IN ('all', 'patches')),
  message       TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'declined', 'moot')) DEFAULT 'pending',
  note          TEXT NOT NULL DEFAULT '',
  granted_scope TEXT,
  decided_by    TEXT REFERENCES users(id),
  decided_at    TEXT,
  created_at    TEXT NOT NULL
);

-- The open-request lookup: "does this person already have one waiting", and
-- "how long since the last decline".
CREATE INDEX idx_trust_requests_user_status ON trust_requests(user_id, status);

-- The patches a request names. Cascades on both sides: a claimed patch drops
-- off the request (decision 7, carried out in activateClaimedNode), and a
-- request that goes takes its rows with it.
CREATE TABLE trust_request_nodes (
  request_id TEXT NOT NULL REFERENCES trust_requests(id) ON DELETE CASCADE,
  node_id    TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  PRIMARY KEY (request_id, node_id)
);

CREATE INDEX idx_trust_request_nodes_node ON trust_request_nodes(node_id);

-- A suggestion may carry a feed (decision 6). The URL rides on the listing
-- row until the listing publishes — at submission for a quilt-wide trusted
-- suggester, at approval for everyone else, where the admin sees the feed and
-- its upcoming count and may uncheck it. It is not an event_sources row yet,
-- because nothing should sync a feed onto a patch nobody has approved.
--
-- suggested_feed_count is what the detection fetch found at submission time:
-- the number the reviewing admin reads before deciding. It is a snapshot of
-- one fetch, never kept current — the moment the source attaches, the source
-- is authoritative and this pair stops being read.
ALTER TABLE nodes ADD COLUMN suggested_feed_url TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN suggested_feed_count INTEGER NOT NULL DEFAULT 0;
