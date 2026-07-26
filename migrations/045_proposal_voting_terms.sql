-- A vote keeps the terms it opened with (docs/adr/047).
--
-- The rules that decide a vote — who may vote, what quorum it needs, what
-- threshold carries it — were read live at resolution, so a rules edit
-- mid-vote judged a contest people had already cast ballots in. The proposal
-- now carries the governance_config in force when voting opened, and
-- resolution reads that copy instead of the node's current one.
--
-- NULL means "no photograph" and falls back to the node's live config: that is
-- what every proposal resolved before this migration ran under, and it keeps
-- an unmigrated or hand-inserted row working rather than silently ineligible.
ALTER TABLE proposals ADD COLUMN voting_terms TEXT;

-- Backfill the votes that are still running. A resolved proposal is done and
-- its terms no longer decide anything, so it keeps NULL rather than being
-- stamped with a config it may never have run under.
UPDATE proposals
SET voting_terms = (SELECT governance_config FROM nodes WHERE nodes.id = proposals.node_id)
WHERE status = 'open'
  AND (SELECT governance_config FROM nodes WHERE nodes.id = proposals.node_id) IS NOT NULL;
