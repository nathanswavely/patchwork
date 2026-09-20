import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// The shipped text only, with the comments taken out.
//
// These files explain their copy in comments that quote the wording they
// replaced, which is the whole value of the comment and would make every
// "no longer says X" assertion below pass forever on a revert. Strip
// whole-line `//`, block comments and markup comments, and assert against
// what a reader can actually reach.
function shipped(path) {
  return readFileSync(resolve(__dirname, path), 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^[ 	]*\/\/.*$/gm, '');
}

const overview = shipped('../components/GovernanceOverview.svelte');
const voteSection = shipped('../components/VoteSection.svelte');
const templateDrawer = shipped('../components/TemplatePreviewDrawer.svelte');

// F-115 and F-129: three places where the product stated a rule it does not
// run, each one read in the same sitting as the rule it contradicts.

describe('a seat, and what takes it away (F-115)', () => {
  // "Nobody loses their seat when a term ends" is true of terms and reads as
  // an absolute, which is how it landed as a denial of the warning sitting in
  // the same admin's bell. The qualifier now carries the sentence.
  it('does not state an absolute about losing a seat', () => {
    expect(overview).not.toContain('Nobody loses their seat');
  });

  it('says a term ending is what takes nobody away, and names the thing that does', () => {
    expect(overview).toContain("A term ending takes nobody's seat away");
    expect(overview).toContain('Inactivity is the only thing that empties a held');
  });

  // Only where the patch runs the rule. A patch with inactivity switched off
  // must not be warned about a sweep that will never touch it.
  it('withholds the inactivity sentence where the patch does not run it', () => {
    expect(overview).toContain('{#if inactivityDays > 0}');
    expect(overview).toContain("let inactivityDays = $derived(Number(rules?.inactivity_days) || 0);");
  });

  // "may be asked to step down" described a conversation. The sweep warns at
  // the patch's own number and empties the chair at twice it, asking nobody.
  it('describes the sweep as it behaves, with both of its days', () => {
    expect(overview).not.toContain('may be asked to step down');
    expect(overview).toContain('is warned, and the seat is declared vacant at ${rules.inactivity_days * 2} days');
  });
});

describe('consensus, above a button labelled Reject (F-129)', () => {
  // The ballot printed "no reject votes allowed" directly above Reject. One
  // reject is exactly how a member blocks a consensus proposal.
  it('does not forbid the act the button performs', () => {
    expect(voteSection).not.toContain('no reject votes allowed');
    expect(voteSection).toContain('one reject blocks it');
  });

  // And the hub two clicks away described a threshold the code does not run:
  // consensus here is rejectCount == 0 with at least one approval, so a
  // proposal nobody objects to carries.
  it('states the rule the resolver runs, in both places that describe it', () => {
    for (const src of [overview, templateDrawer]) {
      expect(src).not.toMatch(/nearly everyone/);
      expect(src).toMatch(/one reject blocks a proposal/i);
    }
  });
});

describe('a count the page will stand behind (F-130)', () => {
  // The server withholds the proposal counts wherever the proposals list
  // would refuse the same viewer. The stat row has to read the flag, because
  // a withheld count arrives as 0 and "0 open proposals" over a patch running
  // one is the false-empty admins_withheld already exists to avoid.
  it('reads the withheld flag rather than the number', () => {
    expect(overview).toContain('overview.proposals_withheld');
  });

  it('offers no link to the page that would refuse the reader', () => {
    const stat = overview.slice(overview.indexOf('{#if overview.proposals_withheld}'));
    const closed = stat.slice(0, stat.indexOf('{:else}'));
    expect(closed).not.toContain('href');
    expect(closed).toContain('Proposals are members-only');
  });
});
