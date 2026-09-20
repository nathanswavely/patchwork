import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function shipped(path) {
  return readFileSync(resolve(__dirname, path), 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^[ \t]*\/\/.*$/gm, '');
}

const glimpses = shipped('../components/PatchProfileGlimpses.svelte');
const dashboard = shipped('../pages/Dashboard.svelte');

// F-106: a patch's public governance was three identically-named, undated
// rows out of seven. F-108: the front door kept asking for a vote already
// cast.

describe('a glimpse that admits it is one (F-106)', () => {
  // The events glimpse has reported its own remainder for as long as it has
  // been capped. The proposals list beside it took three and said nothing.
  it('says how many it is not showing', () => {
    expect(glimpses).toContain('let moreProposals = $derived(Math.max(0, proposalTotal - recentProposals.length));');
    expect(glimpses).toContain('{moreProposals} more');
  });

  // Read off the server's own total rather than a second fetch, the same
  // way moreEvents reads upcoming_event_count.
  it('reads the total from the endpoint it already called', () => {
    expect(glimpses).toContain('proposalTotal = proposalData.total ?? recentProposals.length;');
  });

  // "Council election / Council election / Council election" was one row
  // shown three times as far as a reader could tell.
  it('dates every row, so three contests are three contests', () => {
    expect(glimpses).toContain('{proposalWhen(proposal)}');
    expect(glimpses).toContain('return formatDay(p.created_at);');
  });

  // An open one has a fact the reader still has to act on (F-058).
  it('gives an open proposal its closing date instead', () => {
    expect(glimpses).toContain(
      "if (p.status === 'open' && p.voting_ends_at) return `closes ${formatDay(p.voting_ends_at)}`;"
    );
  });
});

describe('a front door that subtracts the vote you cast (F-108)', () => {
  it('reads what the viewer still owes, not what the patch has open', () => {
    expect(dashboard).toContain('data.awaiting_your_vote ?? 0');
    expect(dashboard).not.toContain('{ count: items.length, more: !!data.next_cursor }');
  });

  // The label has to mean the number. "Open proposals" over a count of
  // ballots owed is the same disagreement one layer down.
  it('says what the number counts', () => {
    expect(dashboard).toContain("{totalProposals === 1 ? 'proposal needs' : 'proposals need'} your vote");
    expect(dashboard).toContain('to vote on');
  });

  // awaiting_your_vote counts the whole patch, so the "+" that belongs on a
  // page-derived count would be a lie here.
  it('drops the more-than-a-page hedge it no longer needs', () => {
    expect(dashboard).not.toContain('proposalsMore');
  });
});
