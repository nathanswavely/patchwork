-- Migration 068: contact items, shared one patch at a time (docs/adr/083).
--
-- Supersedes the shape migration 062 gave the contact card. That card was
-- three columns on `users` and one boolean per membership, so every patch a
-- person shared with received byte-identical fields: there was no way to give
-- the band a phone number and the volunteer crew an email address. The `note`
-- column was already absorbing the pressure — people typed "+1 717 555 0100,
-- Signal only" into `contact_phone` because a channel had nowhere else to go.
--
-- A contact item carries its own kind, so a channel is a fact about the item
-- rather than the column it happens to sit in, and a surface can render tel:
-- and mailto: without guessing. `label` is the person's own word for it
-- ("work", "Signal only"); `position` is their ordering.
--
-- contact_item_shares is the whole disclosure model: one row means one item is
-- shown to one patch's active admins and members. There is deliberately no
-- "share with everything" — no column here holds a rule, because a rule would
-- be evaluated later, and later is when somebody else's action (an admin
-- promoting a follower, an admin opening a patch's door) would decide what a
-- person discloses. Every row is an act by the item's owner.
--
-- A share is deleted when the membership behind it ends — leaving, a ban, or
-- demotion to follower. Rejoining starts shared with nothing, because keeping
-- rows dormant would make *rejoining* disclose a phone number without a
-- decision. Enforced in the handlers, since a share is keyed to the patch
-- rather than to the membership row (membership rows persist as
-- status = 'left').
--
-- The old columns are NOT dropped here. Nothing will read them once the
-- handlers move, and they carry the only copy of pre-068 data until the Go
-- backfill has run everywhere; retiring them is a later migration, once no
-- deployment can still be mid-upgrade.

CREATE TABLE contact_items (
  id         TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL CHECK (kind IN ('phone', 'email', 'handle', 'note')),
  value      TEXT NOT NULL,
  label      TEXT NOT NULL DEFAULT '',
  position   INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_contact_items_user ON contact_items(user_id, position, id);

CREATE TABLE contact_item_shares (
  item_id    TEXT NOT NULL REFERENCES contact_items(id) ON DELETE CASCADE,
  node_id    TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  PRIMARY KEY (item_id, node_id)
);
-- The Members room asks "which items does this patch see", so node_id leads.
CREATE INDEX idx_contact_item_shares_node ON contact_item_shares(node_id, item_id);
