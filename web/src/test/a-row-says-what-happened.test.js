/**
 * Seven governance-simulation findings on the proposal and governance pages
 * (F-137, F-021, F-070, F-080, F-039, F-055, F-051).
 *
 * The lib helpers are exercised directly. Component wiring is asserted
 * against source text, since this project has no Svelte render library;
 * each change was also read on screen against a running app.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { isDirectRow } from '../lib/proposalRow.js';
import { readableVoteTerms, readableRules } from '../lib/governanceRules.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('F-137: an election is never a direct change', () => {
  const settled = { status: 'approved', approve_count: 0, reject_count: 0, abstain_count: 0 };

  it('reads an approved, ballotless proposal as a direct change', () => {
    expect(isDirectRow(settled)).toBe(true);
  });

  it('does not when the row is an election, by phase or by seats', () => {
    expect(isDirectRow({ ...settled, election_phase: 'closed' })).toBe(false);
    expect(isDirectRow({ ...settled, seats_contested: 3 })).toBe(false);
  });

  it('does not when anybody voted', () => {
    expect(isDirectRow({ ...settled, abstain_count: 1 })).toBe(false);
  });

  it('the list attributes an election to the calendar, as the detail page does', () => {
    const src = source('pages/ProposalList.svelte');
    expect(src).toContain("import { isDirectRow } from '../lib/proposalRow.js'");
    expect(src).not.toMatch(/function isDirectRow/);
    const election = src.indexOf('{#if proposal.election_phase}');
    const appliedBy = src.indexOf("isDirectRow(proposal) ? 'applied by' : 'by'");
    expect(election).toBeGreaterThan(-1);
    expect(election).toBeLessThan(appliedBy);
    expect(src).toContain('opened by the election calendar');
  });

  it('the detail page never calls an election a direct change either', () => {
    expect(source('pages/ProposalDetail.svelte')).toMatch(/let isDirectChange = \$derived\([^;]*&& !proposal\?\.election_phase\s*\);/);
  });
});

describe('F-021: a direct change says what happened', () => {
  // The heading is the caller's (it arrived with the settled-diff fix). The
  // editor's review step for a direct change is the caller this finding was
  // about: the rules change there now, so nothing is proposed.
  it('the rules editor heads a direct change "Rule changes", in both places it shows the diff', () => {
    const editor = source('pages/RulesProposalEditor.svelte');
    expect(editor.match(/<StructuredRulesDiff \{currentRules\} \{proposedRules\} heading=\{directChange \? 'Rule changes' : undefined\} \/>/g)).toHaveLength(2);
  });

  it('an applied change on its own page says the same', () => {
    expect(source('pages/ProposalDetail.svelte')).toMatch(/<StructuredRulesDiff[\s\S]*?heading=\{changeApplied \? 'Rule changes' : undefined\}/);
  });

  it('the profile glimpse badges an applied direct change "applied"', () => {
    const src = source('components/PatchProfileGlimpses.svelte');
    expect(src).toContain("if (isDirectRow(p)) return 'applied';");
    expect(src.indexOf("if (isDirectRow(p)) return 'applied';")).toBeLessThan(src.indexOf('return p.status;'));
  });
});

describe('F-070: a settled vote says how you voted', () => {
  const src = source('components/VoteSection.svelte');

  it('names the ballot once the window has closed', () => {
    expect(src).toContain("approve: 'You approved.', reject: 'You rejected.', abstain: 'You abstained.'");
    expect(src).toContain('{#if windowClosed && (myVoteWord || couldHaveVoted)}');
  });

  it('says "You did not vote" only on the server\'s word', () => {
    expect(src).toContain("{myVoteWord || 'You did not vote.'}");
    expect(source('pages/ProposalDetail.svelte')).toContain('couldHaveVoted={proposal.could_have_voted === true}');
  });
});

describe('F-080: the frozen terms are readable from the line that names them', () => {
  const terms = {
    decision_method: 'consensus',
    quorum_percent: 50,
    default_vote_duration_hours: 168,
    amendment_threshold: 'supermajority',
    amendment_auto_apply: false,
    min_voting_tenure_days: 30,
    subject_recusal: true,
    leadership_model: 'elected',
    admin_term_months: 12,
    succession_policy: 'election',
  };

  it('keeps only the rows that say how a vote runs', () => {
    const labels = readableVoteTerms(terms, 30).map((r) => r.label);
    expect(labels).toEqual([
      'How a proposal carries',
      'Quorum',
      'How long a vote stays open',
      'Before a new member may vote',
      'Somebody a proposal is about',
      'To amend a charter',
      'An amendment that carries',
    ]);
    // Every one of them is a row the rules page itself prints.
    const page = readableRules(terms).map((r) => r.label);
    for (const l of labels) expect(page).toContain(l);
  });

  it('states the tenure in force, not the configured number', () => {
    const tenure = (n) => readableVoteTerms(terms, n).find((r) => r.label === 'Before a new member may vote').value;
    expect(tenure(0)).toBe('Straight away');
    expect(tenure(12)).toBe('12 days');
  });

  it('the vote box labels them as this vote\'s terms and opens onto them', () => {
    const src = source('components/VoteSection.svelte');
    expect(src).not.toContain('Rules as of');
    expect(src).toContain("`This vote's terms, fixed ${when}`");
    expect(src).toContain('<summary>{termsLine}</summary>');
    expect(src).toContain('readableVoteTerms(terms, tenureDays)');
    expect(src).toContain("The patch's current rules may differ.");
  });
});

describe('F-039: the attestation form says it records leadership', () => {
  it('names leadership and points other decisions to Documents', () => {
    const src = source('components/AttestationRecords.svelte');
    expect(src).not.toContain("'Record a decision'");
    expect(src).not.toContain('>Record a decision</button>');
    expect(src).toContain('Record a leadership decision');
    expect(src).toContain('/patches/{slug}/governance/docs');
    const overview = source('components/GovernanceOverview.svelte');
    expect(overview).not.toContain('What the community decided is');
    expect(overview).toMatch(/Minutes and other decisions go in\s+<a href="\/patches\/\{slug\}\/governance\/docs"/);
  });
});

describe('F-055: a requester is not greeted as a stranger', () => {
  const src = source('pages/ProposalList.svelte');

  it('reads the pending set by name', () => {
    expect(src).toContain('getPendingMembershipSlugs().has(slug)');
  });

  it('says the request is waiting, ahead of the outsider prompt', () => {
    const waiting = src.indexOf('{:else if requestPending}');
    const outsider = src.indexOf('Follow or join this patch to get started.');
    expect(waiting).toBeGreaterThan(-1);
    expect(waiting).toBeLessThan(outsider);
    expect(src).toContain('Your request to join is waiting on an admin.');
  });
});

describe('F-051: the glimpses reload when standing changes', () => {
  it('keys the load effect on the viewer\'s standing and the counts it moves', () => {
    const src = source('components/PatchProfileGlimpses.svelte');
    expect(src).toMatch(/let standingKey = \$derived\(`\$\{membershipRole\}\|\$\{isAdmin\}\|\$\{node\?\.member_count/);
    const effect = src.slice(src.indexOf('$effect(() => {'));
    expect(effect.slice(0, effect.indexOf('});'))).toContain('void standingKey;');
  });
});
