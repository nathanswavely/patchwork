import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function shipped(path) {
  return readFileSync(resolve(__dirname, path), 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^[ \t]*\/\/.*$/gm, '');
}

const page = shipped('../pages/ProposalDetail.svelte');
const bar = shipped('../components/StickyVoteBar.svelte');

// F-049 and F-050, from epoch 1. Both are what the proposal page does in
// the seconds after somebody votes.

describe('the page agrees with itself after a vote (F-049)', () => {
  // "The 'Show voters' button still says (2) after my vote went in."
  // "'Hide voters (1), Ivo Petran approve' — one name, and it isn't mine,
  // under a count that says two. The count says yes. The voters list says
  // no."
  it('asks the server what it holds, rather than only moving the counts', () => {
    expect(page).toContain('refreshAfterVote();');
    expect(page).toContain('async function refreshAfterVote()');
    expect(page).toContain('proposal = await api(`proposals/${proposalId}`);');
  });

  // The counts still move at once: the number should move under the finger
  // that pressed the button.
  it('keeps the optimistic count so the number moves immediately', () => {
    expect(page).toMatch(/proposal\.approve_count = \(proposal\.approve_count \|\| 0\)/);
  });

  // loadProposal() sets loading = true and swaps the page for a skeleton.
  // Blanking the proposal somebody just voted on is worse than the stale
  // list was.
  it('refreshes without blanking the page', () => {
    const fn = page.slice(page.indexOf('async function refreshAfterVote()'));
    const body = fn.slice(0, fn.indexOf('\n  }'));
    expect(body).not.toContain('loading = true');
  });

  // A failed refresh must not undo a vote that landed.
  it('leaves the counts standing if the refresh fails', () => {
    const fn = page.slice(page.indexOf('async function refreshAfterVote()'));
    expect(fn.slice(0, 400)).toContain('catch');
    expect(fn.slice(0, 400)).not.toContain('proposal = null');
  });
});

describe('the sticky bar does not cover the thing it duplicates (F-050)', () => {
  // The bar is position: fixed; bottom: 0 at every width. Its compensating
  // padding was inside the 640px query, so on a desktop window it sat on
  // the end of the page — where the real buttons and the rule line are.
  it('makes room for the bar at every width, not only on phones', () => {
    const rule = page.indexOf('.proposal-page {\n    padding-bottom: 100px;\n  }');
    expect(rule, 'the page-level padding rule is missing').toBeGreaterThan(-1);
    const mq = page.indexOf('@media (max-width: 640px)');
    expect(rule).toBeLessThan(mq);
  });

  // "That bottom bar says Approve even on the rules proposal where I've
  // already voted" — the panel two inches above said Approved.
  it('says what the panel says once a vote is cast', () => {
    expect(bar).toContain("{userVote === 'approve' ? 'Approved' : 'Approve'}");
    expect(bar).toContain("{userVote === 'reject' ? 'Rejected' : 'Reject'}");
    expect(bar).toContain("{userVote === 'abstain' ? 'Abstained' : 'Abstain'}");
  });

  // The class is what colours it; it was also the only thing marking the
  // cast vote, which a label is for.
  it('keeps the active class as well, for the colour', () => {
    expect(bar).toContain("class:active={userVote === 'approve'}");
  });
});
