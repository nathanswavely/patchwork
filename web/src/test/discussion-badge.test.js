import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function shipped(path) {
  return readFileSync(resolve(__dirname, path), 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^[ \t]*\/\/.*$/gm, '');
}

const detail = shipped('../pages/ProposalDetail.svelte');
const thread = shipped('../components/CommentThread.svelte');
const proposalsGo = readFileSync(resolve(__dirname, '../../../internal/handler/proposals.go'), 'utf8');

// #368: the badge read a `comment_count` the proposal payload never sent.
describe('the Discussion tab counts its discussion (#368)', () => {
  it('the proposal payload sends comment_count', () => {
    expect(proposalsGo).toContain('result["comment_count"] = commentCount');
  });

  it("the badge reads the server's count until the thread has loaded", () => {
    expect(detail).toContain("count: threadCount ?? proposal?.comment_count ?? 0");
  });

  it('the thread reports what it holds, replies included, each time it loads', () => {
    expect(thread).toContain('oncount?.(countComments(comments));');
    expect(thread).toContain('return (list || []).reduce((n, c) => n + 1 + countComments(c.replies), 0);');
    expect(detail).toContain('oncount={(n) => (threadCount = n)}');
  });

  // Moving to another proposal must not carry the last one's count over.
  it('forgets the thread count when another proposal loads', () => {
    expect(detail).toMatch(/async function loadProposal\(\) \{[\s\S]*?threadCount = null;/);
  });
});
