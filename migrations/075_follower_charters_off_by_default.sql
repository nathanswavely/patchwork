-- docs/adr/116: a follower no longer reads a patch's members-only charters
-- unless the patch says so.
--
-- `follower_permissions.charters` shipped true in migration 012's column
-- default, which ALTER TABLE ADD COLUMN wrote into every row that already
-- existed, and DefaultRules and three of the four governance templates
-- carried the same true forward. So every patch granted it, and none decided
-- to. At the time the key also gated *published* charters, which made an
-- on-by-default sensible; docs/adr/036 narrowed it to the members-only shelf
-- and nothing revisited the default. Following is frictionless and needs
-- nobody's approval, so on an invite-only patch this made the private shelf
-- readable by anyone who clicked Follow.
--
-- This closes it for the rows. The column is a cache of governance-rules.json
-- in each patch's git repo (governance.SyncRulesToDB), so the repo is closed
-- too, at startup, by handler.CloseFollowerChartersDefault — which also tells
-- each patch's admins, because this is Patchwork editing their rules and the
-- record should say so. Doing it here alone would be undone by the next rules
-- amendment.
--
-- Only the charters key moves. events, proposals and members govern what a
-- follower sees of a patch's public life and are left exactly as each patch
-- has them.
--
-- Migration 012's column DEFAULT still says charters:true and is deliberately
-- left alone: changing it needs a rebuild of `nodes`, and no insert path
-- relies on it — CreateNode, seed and import all write the column explicitly.
-- CloseFollowerChartersDefault is the backstop if one ever does.

UPDATE nodes
SET follower_permissions = json_set(
      CASE
        WHEN follower_permissions IS NULL OR follower_permissions = '' THEN '{}'
        ELSE follower_permissions
      END,
      '$.charters', json('false')
    )
WHERE json_valid(COALESCE(NULLIF(follower_permissions, ''), '{}'))
  AND json_extract(COALESCE(NULLIF(follower_permissions, ''), '{}'), '$.charters') = 1;
