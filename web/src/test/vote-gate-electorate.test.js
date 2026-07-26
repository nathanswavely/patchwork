/**
 * Who may vote is the server's answer, not one the page infers.
 *
 * The trap this guards: the electorate is active admins and members past
 * `min_voting_tenure_days` (docs/adr/044), and tenure is invisible to the
 * client — the node payload carries a role, not a joined_at compared against
 * this patch's rules. A page that works `canVote` out from `membershipRole`
 * therefore offers the buttons to a member inside the tenure window, who is
 * then refused with a 403. The proposal payload's `can_vote` is the gate's own
 * answer; the page's job is to render it, not to reconstruct it.
 *
 * There is no Svelte render library in this project (see router.test.js,
 * patch-setup.test.js), so component wiring is asserted against source text.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('ProposalDetail — the vote gate', () => {
  const src = source('pages/ProposalDetail.svelte');

  it('takes canVote from the payload the server computed', () => {
    expect(src).toMatch(/canVote = \$derived\(.*proposal\?\.can_vote === true\)/);
  });

  it('never reconstructs the electorate from a role, which cannot see tenure', () => {
    const canVoteLine = src.split('\n').find((l) => l.includes('let canVote'));
    expect(canVoteLine).toBeTruthy();
    expect(canVoteLine).not.toMatch(/membershipRole|isMember/);
  });

  it('gates both vote surfaces on canVote', () => {
    // The section on the page...
    expect(src).toMatch(/<VoteSection[\s\S]*?\{canVote\}/);
    // ...and the sticky bar, which is a second door to the same act.
    expect(src).toMatch(/\{#if proposal && isVoting && canVote/);
  });

  it('keeps the sole-voter notice behind the same gate', () => {
    expect(src).toMatch(/soleVoter = \$derived\(isVoting && canVote/);
  });
});

describe('ProposalStatusBanner — the sentence above the buttons', () => {
  const src = source('components/ProposalStatusBanner.svelte');

  it('only says "cast your vote below" when there is a vote below', () => {
    expect(src).toMatch(/canVote \? ' Cast your vote below\.' : ''/);
    // The bare instruction must not survive anywhere unguarded.
    expect(src).not.toMatch(/\{timeLeft\}\. Cast your vote below\./);
  });

  it('takes canVote as a prop rather than assuming it', () => {
    expect(src).toMatch(/canVote = false,/);
  });
});

describe('VoteSection — the buttons themselves', () => {
  const src = source('components/VoteSection.svelte');

  it('renders the ballot only when the caller says this viewer may cast one', () => {
    expect(src).toMatch(/\{#if canVote &&/);
  });
});

/**
 * A vote is judged by the terms it opened with (docs/adr/047), and the section
 * that shows the arithmetic has to show the same ones the server used.
 *
 * The trap this guards: quorum and threshold were props, and GetProposal never
 * sent them, so every patch was told "No quorum required" and "Majority" no
 * matter what its rules said. Reading them off the payload's terms is what
 * makes the display and the outcome the same rules.
 */
describe('VoteSection — the terms it shows', () => {
  const src = source('components/VoteSection.svelte');

  it('derives quorum and threshold from the vote terms, not from bare props', () => {
    expect(src).toMatch(/quorumPercent = \$derived\(terms\?\.quorum_percent/);
    expect(src).toMatch(/terms\?\.amendment_threshold \|\| terms\?\.decision_method/);
    // The old prop names must not come back as inputs.
    expect(src).not.toMatch(/^\s*quorumPercent = 0,/m);
    expect(src).not.toMatch(/^\s*threshold = 'majority',/m);
  });

  it('divides quorum by the electorate, matching the server', () => {
    expect(src).toMatch(/totalVotes \/ electorateSize/);
    expect(src).not.toMatch(/memberCount/);
  });

  it('states the tenure requirement, which appears nowhere else in the UI', () => {
    expect(src).toMatch(/voting requires \$\{tenureDays\} days' membership/);
  });
});

describe('ProposalDetail — what it hands the vote section', () => {
  const src = source('pages/ProposalDetail.svelte');

  it('passes the electorate and the terms, not counts the payload never carried', () => {
    expect(src).toMatch(/electorateSize=\{proposal\.eligible_voters/);
    expect(src).toMatch(/terms=\{proposal\.voting_terms\}/);
    expect(src).not.toMatch(/proposal\.member_count/);
    expect(src).not.toMatch(/proposal\.quorum_percent/);
  });
});
