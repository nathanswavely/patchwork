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
    expect(src.replace(/\s+/g, ' ')).toContain('.withdrawn, .lapsed,');
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
    expect(src).toMatch(/status === 'withdrawn' \|\| status === 'lapsed'/);
    expect(src).toMatch(/return 'status-withdrawn'/);
  });
});

describe('PatchProfileGlimpses — the front page never prints REJECTED over an absence', () => {
  const src = source('components/PatchProfileGlimpses.svelte');

  it('reads the state before the status, the way the proposals list does', () => {
    expect(src).toMatch(/function outcomeWord\(p\)/);
    expect(src).toMatch(/if \(p\.state === 'lapsed'\) return 'lapsed'/);
    expect(src).toMatch(/if \(p\.state === 'unsettled'\) return 'unsettled'/);
  });

  it('prints that word in the pill rather than the raw column', () => {
    expect(src).toMatch(/\{@const outcome = outcomeWord\(proposal\)\}/);
    expect(src).toMatch(/>\{outcome\}<\/span>/);
    expect(src).not.toMatch(/>\{proposal\.status\}<\/span>/);
  });

  it('keeps error red for a real rejection only', () => {
    expect(src).toMatch(/class:status-rejected=\{outcome === 'rejected'\}/);
    expect(src).not.toMatch(/class:status-rejected=\{proposal\.status === 'rejected'\}/);
  });
});

describe('the Record gives a lapse its own sentence', () => {
  const src = source('pages/GovernanceRecord.svelte');

  it('never says "did not carry" about a vote nobody decided', () => {
    expect(src).toMatch(/if \(e\.outcome === 'lapsed'\)/);
    expect(src).toMatch(
      /return 'Put to a vote\. Nobody decided it either way; the proposal lapsed\.';/
    );
    const lapsed = src.indexOf("e.outcome === 'lapsed'");
    const didNotCarry = src.indexOf('Put to a vote and did not carry.');
    expect(lapsed).toBeGreaterThan(-1);
    expect(lapsed).toBeLessThan(didNotCarry);
  });

  it('mutes the lapsed entry alongside the other absences', () => {
    expect(src).toMatch(/e\.outcome === 'unsettled' \|\| e\.outcome === 'failed' \|\| e\.outcome === 'lapsed'/);
  });
});

describe('an unsettled election is a lapse in election shape (docs/adr/051)', () => {
  const banner = source('components/ProposalStatusBanner.svelte');
  const panel = source('components/ElectionPanel.svelte');
  const list = source('pages/ProposalList.svelte');

  it('the banner says nobody was seated rather than reciting a 0-0 tally', () => {
    const unsettled = banner.indexOf("effectiveState === 'unsettled'");
    const rejected = banner.indexOf("effectiveState === 'rejected'");
    expect(unsettled).toBeGreaterThan(-1);
    expect(unsettled).toBeLessThan(rejected);
    expect(banner).toMatch(
      /This election settled nothing; nobody was seated, and the seats it was for are unchanged\./
    );
    // Not "the council continues": this banner outlives the council it would
    // be describing, and on a patch with no admins it was a comfortable lie
    // (docs/adr/106).
    expect(banner).not.toMatch(/The council continues until a successor is elected/);
  });

  it('the banner styles it muted, like withdrawn and lapsed', () => {
    expect(banner.replace(/\s+/g, ' ')).toContain('.withdrawn, .lapsed, .unsettled {');
  });

  it('the election panel tags nobody seated on a contest that seated nobody', () => {
    expect(panel).toMatch(/let settledNothing = \$derived\(proposal\?\.state === 'unsettled'\)/);
    // The guard lives in one function now rather than inline in two places,
    // so the class and the tag cannot drift apart. Still the same rule: a
    // contest that settled nothing seats nobody, whatever the tally says.
    expect(panel).toMatch(/if \(phase !== 'closed' \|\| settledNothing\) return false;/);
    expect(panel).toContain('class:seated={isSeated(c, i)}');
    expect(panel).toContain('{#if isSeated(c, i)}');
  });

  it('the list row says "unsettled" and mutes it', () => {
    expect(list).toMatch(/if \(p\.state === 'unsettled'\) return 'unsettled'/);
    expect(list).toMatch(/status === 'withdrawn' \|\| status === 'lapsed' \|\| status === 'unsettled'\) return 'status-withdrawn'/);
  });
});

describe('VoteSection — "not yet" is a promise a closed window cannot keep', () => {
  const src = source('components/VoteSection.svelte');

  it('knows whether more votes can still arrive', () => {
    expect(src).toMatch(/let windowClosed = \$derived\.by\(\(\) => \{/);
    expect(src).toMatch(/if \(propState !== 'voting' && propState !== 'open'\) return true;/);
  });

  it('drops the "yet" once the window has closed', () => {
    expect(src).toMatch(
      /\{windowClosed \? 'Quorum not met' : 'Quorum not yet met'\} \(\{totalVotes\} of \{quorumNeeded\} needed\)/
    );
  });
});
