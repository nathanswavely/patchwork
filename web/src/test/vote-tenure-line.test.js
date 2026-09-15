/**
 * The vote panel says the tenure the gate is running (docs/adr/098).
 *
 * The frozen terms carry the number the patch configured; the gate enforces
 * the effective one, which on a patch younger than its own bar is none. The
 * panel used to read the frozen number, so five simulated members read
 * "voting requires 30 days' membership" beneath a button that took their
 * vote. The server now sends `tenure_days` and, for a member still inside
 * the window, `vote_eligible_at` — a date, which is the question they were
 * actually asking.
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

describe('VoteSection — the tenure line', () => {
  const src = source('components/VoteSection.svelte');

  it('takes the tenure in force from the server, never from the frozen terms', () => {
    expect(src).toMatch(/tenureDays = 0,/);
    expect(src).toMatch(/voteEligibleAt = '',/);
    expect(src).not.toMatch(/terms\?\.min_voting_tenure_days/);
  });

  it('names the day when one is given', () => {
    expect(src).toMatch(/you can vote here from \$\{eligibleDate\}/);
  });

  it('recites the rule only at someone the gate is actually refusing', () => {
    expect(src).toMatch(/if \(tenureDays > 0 && !canVote\)/);
  });
});

describe('ProposalDetail — passes both server answers down', () => {
  const src = source('pages/ProposalDetail.svelte');

  it('hands VoteSection tenure_days and vote_eligible_at', () => {
    expect(src).toMatch(/tenureDays=\{proposal\.tenure_days \|\| 0\}/);
    expect(src).toMatch(/voteEligibleAt=\{proposal\.vote_eligible_at \|\| ''\}/);
  });
});
