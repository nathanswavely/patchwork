/**
 * What a rule costs this patch today (docs/adr/104).
 *
 * The rules editor asked for a quorum as a bare percentage and a tenure bar
 * as a bare number of days, and said nothing about what either came to for
 * the patch being edited. A simulated eight-person co-op took the Formal
 * defaults, ran three proposals in its first month and carried none; its
 * founder had no way to learn that 50% meant four of her eight members had
 * to act inside a fortnight, or that "Full consensus" let one reject defeat
 * what the other seven approved.
 *
 * The sentences are only worth printing if they are the arithmetic the
 * server runs, so what these assert is the mirroring: the quorum ceiling
 * (`votesNeededForQuorum`) and the young-patch rule (docs/adr/098's
 * `effectiveTenureDays`).
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

describe('StructuredRulesEditor — the numbers, for this patch', () => {
  const src = source('components/StructuredRulesEditor.svelte');

  it('takes the facts from the server rather than inventing them', () => {
    expect(src).toMatch(/electorate = null/);
    expect(src).toMatch(/electorate\?\.members/);
    expect(src).toMatch(/electorate\?\.tenure_days/);
    expect(src).toMatch(/electorate\?\.patch_age_days/);
  });

  // votesNeededForQuorum: the ceiling, capped at the electorate.
  it('needs the same number of ballots the resolver will need', () => {
    expect(src).toMatch(/Math\.ceil\(\(canVoteToday \* quorumPercent\) \/ 100\)/);
    expect(src).toMatch(/Math\.min\(canVoteToday,/);
  });

  // docs/adr/098: a patch younger than its own bar has no bar.
  it('knows a young patch has no tenure bar yet', () => {
    expect(src).toMatch(/minVotingTenureDays > 0 && patchAgeDays >= minVotingTenureDays/);
    expect(src).toMatch(/tenureInForce \? tenures\.filter\(\(d\) => d >= minVotingTenureDays\)\.length : memberCount/);
    expect(src).toMatch(/Not in force yet: this patch \$\{age\}, younger than the wait it asks for/);
  });

  it('says what a quorum asks of the people who can vote today', () => {
    expect(src).toMatch(
      /\$\{votesNeeded\} of the \$\{canVoteToday\} people who can vote today must cast a ballot/
    );
    // "1 of the 1 person" is arithmetic, not a sentence.
    expect(src).toMatch(/One person can vote here today, so that single ballot is the whole quorum\./);
    // Abstaining discharges the obligation and counts toward quorum, and the
    // only place anybody learned that was a simulated founder's own notice.
    expect(src).toMatch(/An abstention counts toward it\./);
    expect(src).toMatch(/No quorum: however few people vote, the result stands\./);
  });

  it('says what consensus does before somebody chooses it', () => {
    expect(src).toMatch(/One reject defeats a proposal, however many approve it\./);
    expect(src).toMatch(/Two thirds of the ballots cast must approve\./);
  });

  it('puts each sentence beside the control it is about', () => {
    expect(src).toMatch(/\{#if decisionHint\}/);
    expect(src).toMatch(/\{#if quorumHint\}/);
    expect(src).toMatch(/\{#if tenureHint\}/);
  });
});

describe('RulesProposalEditor — loading the facts', () => {
  const src = source('pages/RulesProposalEditor.svelte');

  it('asks for them separately from the rules', () => {
    // The rules payload is spread back into the submission, so a fact about
    // the patch may not travel in it.
    expect(src).toMatch(/governance\/electorate/);
    expect(src).toMatch(/electorate=\{electorate\}/);
  });

  it('still lets somebody change their rules if the facts do not load', () => {
    expect(src).toMatch(/catch \{\s*electorate = null;\s*\}/);
  });
});
