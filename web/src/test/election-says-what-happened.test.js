/**
 * What a contest says about itself (docs/adr/106).
 *
 * Epoch 4 of the governance simulation ran the first contest anybody actually
 * stood in, and the election surfaces went quiet at three moments a person
 * needed a sentence:
 *
 * - while nominations were open, the only number on the page was the voting
 *   countdown, a different deadline;
 * - when nominations closed, the Stand button vanished and nothing replaced
 *   it — three members arrived to stand and could not learn from the product
 *   that a window had ever existed;
 * - after saving a ballot, nothing at all, so all four voters went looking
 *   for proof elsewhere and three would have voted again.
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

describe('The nomination window is a date, and it leaves a mark', () => {
  const banner = source('components/ProposalStatusBanner.svelte');
  const panel = source('components/ElectionPanel.svelte');

  it('the banner names the day standing shuts while it is open', () => {
    expect(banner).toMatch(/nominationsCloseAt = null/);
    expect(banner).toMatch(
      /Nominations are open until \{formatDay\(nominationsCloseAt\)\}\. Voting starts then\./
    );
    // And still says something sensible for a contest with no stored date.
    expect(banner).toMatch(/Nominations are open\. Voting starts when they close\./);
  });

  it('the detail page hands it the date it needs', () => {
    const page = source('pages/ProposalDetail.svelte');
    expect(page).toMatch(/nominationsCloseAt=\{proposal\.nominations_close_at \|\| null\}/);
  });

  it('the panel says when standing closed, to whoever turns up after', () => {
    expect(panel).toMatch(/phase !== 'nominating' && proposal\.nominations_close_at/);
    expect(panel).toMatch(/Standing closed \{formatDay\(proposal\.nominations_close_at\)\}\./);
  });
});

describe('Saving a ballot says so', () => {
  const panel = source('components/ElectionPanel.svelte');

  it('knows whether this person has voted, from the server and not only from a save', () => {
    expect(panel).toMatch(/let ballotIn = \$derived\(saved \|\| candidates\.some\(\(c\) => c\.approved_by_me\)\)/);
  });

  it('confirms the save, and says the ballot can still be changed', () => {
    expect(panel).toMatch(/saved = true;/);
    expect(panel).toMatch(/Your ballot is in\. You can change it until voting closes\./);
    // One sentence, not two: the confirmation replaces the standing advice
    // rather than sitting above it, both offering to let you change your mind.
    expect(panel).toMatch(/\{#if ballotIn\}[\s\S]*?\{:else\}[\s\S]*?Approving nobody is the same as/);
  });

  it('agrees with itself about a one-seat contest', () => {
    expect(panel).toMatch(/The most approved candidate takes the seat\./);
    expect(panel).toMatch(/The \{seats\} most approved take the seats\./);
  });

  it('calls the button what it does once a ballot is in', () => {
    expect(panel).toMatch(/ballotIn \? 'Update my ballot' : 'Save my ballot'/);
  });

  it('withdraws the confirmation the moment the ballot is edited', () => {
    // Otherwise "your ballot is in" sits over a set of ticks that are not.
    expect(panel).toMatch(/approved = next;\s*\n\s*\/\/[\s\S]*?\n\s*saved = false;/);
  });
});
