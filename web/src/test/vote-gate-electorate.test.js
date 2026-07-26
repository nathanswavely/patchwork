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
