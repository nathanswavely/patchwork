-- A ballot a voter can use: a candidate may say why they are standing, and a
-- voter may take part without approving anybody.
--
-- F-095. Standing took no statement, the candidate row carried no bio and no
-- link, and the view had no field for either, so everything a voter knew
-- about a candidate had to be found on another page or guessed. The man who
-- had run the bar rota for three years and was standing for the board:
-- "Nowhere to say why... The ballot is apparently going to be a list of names
-- and nothing else. Ask me for a few lines when I press the button." One
-- voter left the ballot for the Members tab to find his one line; three more
-- cast theirs on the strength of a comment behind a Discussion tab. "Don't
-- make the crucial fact live behind a tab called 'Discussion' that I only
-- opened by accident."
--
-- F-092. An approval ballot with nobody on it leaves no rows, so it cannot be
-- told apart from not voting, and the panel said so in as many words:
-- "Approving nobody is the same as not voting." True, and the opposite of
-- what a member organising for quorum needs on a patch that had failed six
-- contests for turnout. Two of them read it side by side with a notice
-- asking everyone to turn up: "She is telling eight people to turn up and
-- tick nothing to make quorum; the page says that does nothing. One of them
-- is wrong and I do not know which, and it is the whole thing that decides
-- whether this seventh election works."
--
-- An ordinary proposal has had this since it existed — `votes.value` takes
-- 'abstain' and the rules editor says "An abstention counts toward it" — so
-- this is the election catching up with the vote beside it rather than a new
-- idea.

ALTER TABLE election_candidates ADD COLUMN statement TEXT NOT NULL DEFAULT '';

-- Its own table rather than a nullable candidate_id on election_ballots.
-- That column is NOT NULL, so making a ballot able to point at nobody means
-- rebuilding the table, and "a ballot row for no candidate" is a muddier
-- thing to read in six months than a row that says what it is. Nothing
-- references election_ballots, so the rebuild would have been safe; the
-- reason not to is the meaning, not the risk.
--
-- One per voter per contest, and the two states are exclusive: approving
-- somebody clears an abstention and abstaining clears the approvals, both in
-- the handler, because "I approve Sam and also approve nobody" is not a
-- thing a person can mean.
CREATE TABLE election_abstentions (
    id          TEXT PRIMARY KEY,
    proposal_id TEXT NOT NULL REFERENCES proposals(id) ON DELETE CASCADE,
    voter_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(proposal_id, voter_id)
);

CREATE INDEX idx_election_abstentions_proposal ON election_abstentions(proposal_id);
