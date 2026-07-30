-- Elections: seats, candidates, and approval ballots (docs/adr/051).
--
-- "The community elects admins for fixed terms. Regular elections ensure power
-- rotates" is what an `elected` patch tells its members. Until now the only
-- way that could be true was an attestation — a council elected somewhere else
-- and recorded here (docs/adr/052). This is the contest Patchwork runs itself.

-- A seat is a governed admin position that outlives whoever fills it
-- (docs/adr/051). It earns being an entity here and not before: an attestation
-- carries its own term end and its own names, so a seats table alongside it
-- would have been a second home for what the record already said. An election
-- fills seats without any record to hang them on, and a seat can now be
-- occupied or vacant independently of who was last elected to it.
--
-- term_ends_at lives here rather than on the patch, which is what makes
-- staggering a policy rather than machinery: aligned seats share a date,
-- staggered seats differ, and nothing above this line has to change for a
-- patch to spread its cohort.
CREATE TABLE seats (
    id           TEXT PRIMARY KEY,
    node_id      TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    -- NULL is a vacant seat: it exists, it will be contested, nobody holds it.
    holder_id    TEXT REFERENCES users(id) ON DELETE SET NULL,
    term_ends_at TEXT,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_seats_node ON seats(node_id);
CREATE INDEX idx_seats_holder ON seats(holder_id);

-- Who is standing. An election is a proposal that carries candidates
-- (docs/adr/051) rather than a new proposal_type: the type column's CHECK
-- cannot be altered without rebuilding a table three others hold foreign keys
-- into, and the presence of candidates is what the code branches on in any
-- case. An election is a 'membership' proposal with rows here; a meritocratic
-- nomination is one with target_user_id and none.
--
-- Candidacy is a member act with no tenure condition (docs/adr/044's doctrine:
-- tenure asks whether someone may *decide*, and a candidate is being decided
-- about). Enforced in the handler, where the membership is visible.
CREATE TABLE election_candidates (
    id          TEXT PRIMARY KEY,
    proposal_id TEXT NOT NULL REFERENCES proposals(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(proposal_id, user_id)
);
CREATE INDEX idx_election_candidates_proposal ON election_candidates(proposal_id);

-- Approval voting (docs/adr/051): a ballot is the set of candidates one person
-- approves, so it is rows rather than a value. The `votes` table cannot hold
-- this — it is UNIQUE(proposal_id, user_id) over three values, which says yes
-- or no to a single question and nothing else — and it is left untouched so
-- that nothing which currently tallies changes behaviour.
--
-- Ranked-choice was rejected: no vote-splitting pathology at community scale,
-- no tie-break doctrine to invent, and a tally members can verify by reading
-- the page. UNIQUE(proposal_id, voter_id, candidate_id) makes approving twice
-- a no-op rather than a double count.
CREATE TABLE election_ballots (
    id           TEXT PRIMARY KEY,
    proposal_id  TEXT NOT NULL REFERENCES proposals(id) ON DELETE CASCADE,
    voter_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    candidate_id TEXT NOT NULL REFERENCES election_candidates(id) ON DELETE CASCADE,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(proposal_id, voter_id, candidate_id)
);
CREATE INDEX idx_election_ballots_proposal ON election_ballots(proposal_id);

-- How many seats this election fills. Zero on every proposal that is not an
-- election, which is all of them today.
ALTER TABLE proposals ADD COLUMN seats_contested INTEGER NOT NULL DEFAULT 0;

-- When nominations close and voting opens.
--
-- This is the one place a proposal is not born voting (docs/adr/048, amended
-- by 051): an election opens for nominations first, so that the slate is not
-- whoever the person calling it chose. voting_ends_at therefore runs from
-- *this* moment rather than from creation, and docs/adr/047's voting-terms
-- photograph is taken here too — the terms must be fixed when the vote starts,
-- not when the nomination period was opened.
--
-- NULL on every other proposal, which is what "born voting" means for them.
ALTER TABLE proposals ADD COLUMN nominations_close_at TEXT;
