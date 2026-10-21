import { describe, it, expect } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { resolve, join, relative } from 'node:path';

function shipped(path) {
  return readFileSync(path, 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^[ \t]*\/\/.*$/gm, '');
}

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) {
      if (name === 'test' || name === 'vendor') continue;
      walk(p, out);
    } else if (/\.(svelte|js)$/.test(name)) {
      out.push(p);
    }
  }
  return out;
}

const src = resolve(__dirname, '..');
const proposalDetail = shipped(resolve(src, 'pages/ProposalDetail.svelte'));
const nodesGo = readFileSync(resolve(__dirname, '../../../internal/handler/nodes.go'), 'utf8');

// #367: the election nominee picker sent its page token as `cursor`, the
// server reads `after`, and so every page it asked for was page one.
describe('the nominee picker reaches the hundred-and-first member (#367)', () => {
  it('the server pages on `after`', () => {
    expect(nodesGo).toContain('after := r.URL.Query().Get("after")');
  });

  it('sends the page token back as `after`', () => {
    expect(proposalDetail).toContain('`?limit=100&after=${encodeURIComponent(after)}`');
    expect(proposalDetail).toContain("after = data.next_cursor || '';");
  });

  // The server has one name for the page token. A client that spells it
  // any other way gets page one forever and no error to say so.
  it('no client sends a page token as `cursor=`', () => {
    const offenders = walk(src)
      .filter((p) => /[?&]cursor=/.test(shipped(p)))
      .map((p) => relative(src, p));
    expect(offenders).toEqual([]);
  });
});
