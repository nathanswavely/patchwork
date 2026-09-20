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

/**
 * A nomination you can make, and take back (docs/adr/107).
 *
 * Four surfaces promised "put someone forward" and none had it; the one
 * button posted an empty body, so it stood you. A member who came to
 * nominate a colleague put herself on a three-seat ballot by accident, could
 * not get off, and wrote a comment asking her neighbours not to vote for her
 * — which four of them read and acted on.
 */
describe('Putting somebody forward, and taking your own name back', () => {
  const panel = source('components/ElectionPanel.svelte');
  const page = source('pages/ProposalDetail.svelte');

  it('sends the chosen member rather than an empty body', () => {
    expect(panel).toMatch(/body: \{ user_id: nomineeId \}/);
    // Standing yourself is still its own act, with its own empty body.
    expect(panel).toMatch(/method: 'POST', body: \{\} \}/);
  });

  it('never offers you your own name in the put-somebody-forward list', () => {
    expect(panel).toMatch(/m\.user_id !== me\?\.id && !candidates\.some\(\(c\) => c\.user_id === m\.user_id\)/);
  });

  it('has a way off the slate, and it is your own name only', () => {
    expect(panel).toMatch(/proposals\/\$\{proposal\.id\}\/candidates\/me`, \{ method: 'DELETE' \}/);
    expect(panel).toMatch(/\{busy \? 'Withdrawing…' : 'Withdraw'\}/);
  });

  it('says what putting somebody forward does to them', () => {
    expect(panel).toMatch(
      /They go on the ballot straight away, and can withdraw themselves\s*\n?\s*until nominations close\./
    );
  });

  it('pages the member list, so the twenty-first member can be nominated', () => {
    // docs/adr/095 pages that endpoint; a picker that ignores next_cursor
    // makes everyone below the first page un-nominatable.
    expect(page).toMatch(/next_cursor/);
    expect(page).toMatch(/limit=100/);
  });

  it('loads the list only while it can be used', () => {
    expect(page).toMatch(/proposal\?\.election_phase === 'nominating' && canNominate/);
  });
});

/**
 * A chair nobody wins keeps the council's clock (docs/adr/108).
 *
 * An empty chair's term end never moved, so it was overdue on every pass and
 * the calendar re-contested it after each breather for ever: six contests 56
 * days apart under a page saying "Each term runs 12 months". And while the
 * breather ran, the page said the next contest was due now, every day, for
 * four weeks.
 */
describe('The council page tells the truth about its own calendar', () => {
  const overview = source('components/GovernanceOverview.svelte');

  it('has a sentence for a vacant chair nobody can nominate into', () => {
    expect(overview).toMatch(/seat\.fill === 'contest_fills'/);
    expect(overview).toMatch(/Vacant, and this patch has no admins to put a name forward\./);
    expect(overview).toMatch(/The election that fills it opens \$\{formatDay\(seat\.contest_opens\)\}/);
  });

  it('stops telling an admin-less patch to ask an admin', () => {
    // The answer now lives on each empty chair rather than in a general
    // sentence underneath them, because a chair can name its own date and
    // the general sentence could not — it addressed a single vacancy as
    // "these seats". `fill` is 'contest_fills' exactly when the patch has
    // no admins, so the row carries the whole story and councilAction
    // stands down rather than saying it twice.
    expect(overview).toMatch(/Vacant, and this patch has no admins to put a name forward\./);
    expect(overview).toContain("if (vacantSeats.every((s) => s.fill === 'contest_fills')) return '';");
    // And the sentence that sends somebody to an admin who does not exist
    // is still reachable only where one does.
    const askAn = overview.indexOf('Ask one to nominate you');
    const adminless = overview.indexOf("s.fill === 'contest_fills'");
    expect(askAn).toBeGreaterThan(adminless);
  });
});

/**
 * Whose contest it is, and what it says when it ends (docs/adr/109).
 *
 * A contest the calendar opened wore the longest-standing admin's name on a
 * public page — "Proposed by Priya Natarajan" for something a timer started
 * while she was nine months away. A settled election's banner read "Approved.
 * This change is now in effect." over a council. And the candidate names on
 * the ballot were inside the checkbox's own label, so tapping a name to find
 * out who somebody was cast a vote for them.
 */
describe('An election nobody proposed', () => {
  const page = source('pages/ProposalDetail.svelte');
  const banner = source('components/ProposalStatusBanner.svelte');
  const panel = source('components/ElectionPanel.svelte');

  it('says the calendar opened it, not a member', () => {
    expect(page).toMatch(/\{#if proposal\.election_phase\}/);
    expect(page).toMatch(/Opened by this patch&rsquo;s election calendar/);
    // Keyed on the election, not the author id, so contests raised before the
    // calendar started signing them read right too.
    expect(page).toMatch(/\{:else\}[\s\S]*?'Applied by' : 'Proposed by'/);
  });

  it('has a banner for a settled election, not an amendment’s', () => {
    expect(banner).toMatch(
      /effectiveState === 'in_effect' \|\| effectiveState === 'passed'\) && electionPhase/
    );
    expect(banner).toMatch(/This election has closed and the council below is seated\./);
  });

  it('gives a candidate a name to read and a box to press', () => {
    // The name is a link to the person, which is what it is everywhere else.
    expect(panel).toMatch(/<a class="who" href=\{`\/users\/\$\{c\.username\}`\}>/);
    // And the label wraps the checkbox alone.
    expect(panel).toMatch(/<label class="tick"[\s\S]*?<input type="checkbox"[\s\S]*?<\/label>/);
    expect(panel).not.toMatch(/<label>\s*\n\s*<input type="checkbox"/);
  });
});
