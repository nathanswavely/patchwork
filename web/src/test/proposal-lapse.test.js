/**
 * A window that closes settles something (docs/adr/097).
 *
 * A proposal whose voting window ran out under quorum lapses: status
 * 'rejected' (the schema's only terminal "no"), state 'lapsed'. Nobody
 * decided it, so every surface says "not decided" rather than "rejected" —
 * the banner, the list row, and the badge. Before this, such a proposal
 * stayed 'open' forever with a vote endpoint that said "voting period has
 * ended", and nothing in the UI had a word for it.
 *
 * There is no Svelte render library in this project, so component wiring is
 * asserted against source text.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('ProposalStatusBanner — a lapsed vote is not a failed one', () => {
  const src = source('components/ProposalStatusBanner.svelte');

  it('has a lapsed branch ahead of rejected, worded as not decided', () => {
    const lapsed = src.indexOf("effectiveState === 'lapsed'");
    const rejected = src.indexOf("effectiveState === 'rejected'");
    expect(lapsed).toBeGreaterThan(-1);
    expect(lapsed).toBeLessThan(rejected);
    expect(src).toMatch(/lapsed and was not decided/);
  });

  it('styles lapsed like withdrawn, not like rejected', () => {
    expect(src).toMatch(/\.withdrawn,\s*\n\s*\.lapsed \{/);
  });
});

describe('ProposalDetail — the tally of a lapsed vote is still shown', () => {
  const src = source('pages/ProposalDetail.svelte');

  it('lists lapsed among the settled states whose tally is worth showing', () => {
    expect(src).toMatch(/\['approved', 'in_effect', 'rejected', 'lapsed', 'passed'\]\.includes\(effectiveState\)/);
  });
});

describe('ProposalList — the row says what happened, not what the column holds', () => {
  const src = source('pages/ProposalList.svelte');

  it('names a lapsed row "lapsed" in both the meta line and the badge', () => {
    expect(src).toMatch(/function rowStatus\(p\)/);
    expect(src).toMatch(/if \(p\.state === 'lapsed'\) return 'lapsed'/);
    expect(src).toMatch(/<span class="muted">\{rowStatus\(proposal\)\}<\/span>/);
    expect(src).toMatch(/<span class="badge \{statusClass\(rowStatus\(proposal\)\)\}">\{rowStatus\(proposal\)\}<\/span>/);
  });

  it('keeps the direct-change row green and mutes the lapsed one', () => {
    expect(src).toMatch(/status === 'approved' \|\| status === 'applied'\) return 'status-approved'/);
    expect(src).toMatch(/status === 'withdrawn' \|\| status === 'lapsed'\) return 'status-withdrawn'/);
  });
});
