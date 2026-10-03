import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { readableRules, wordFor, SUCCESSION_OPTIONS } from '../lib/governanceRules.js';

function shipped(path) {
  return readFileSync(resolve(__dirname, path), 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^[ \t]*\/\/.*$/gm, '');
}

const page = shipped('../pages/GovernanceRules.svelte');
const shell = shipped('../components/GovernanceShell.svelte');
const overview = shipped('../components/GovernanceOverview.svelte');
const editor = shipped('../pages/RulesProposalEditor.svelte');
const app = shipped('../App.svelte');

// F-117. "I did not want to propose anything, I wanted to *look*. I
// genuinely hesitated for a minute before clicking it, because on this site
// proposing a thing seems to start a vote and I did not want to start a
// vote by accident just to read our own settings."

describe('a patch can be read without being changed', () => {
  it('has a URL of its own, after the more specific propose route', () => {
    const propose = app.indexOf("addRoute('/patches/:slug/governance/rules/propose'");
    const read = app.indexOf("addRoute('/patches/:slug/governance/rules'");
    expect(propose).toBeGreaterThan(-1);
    expect(read).toBeGreaterThan(propose);
  });

  it('is a section of Governance, not a door inside a form', () => {
    expect(shell).toContain("{ label: 'Rules', href: `/patches/${slug}/governance/rules` }");
  });

  it('says on the page that reading it changes nothing', () => {
    expect(page).toContain('Nothing on this');
    expect(page).toContain('page changes anything.');
  });

  // The overview summarises four fields of twenty in prose, and its only
  // link was an offer to change them.
  it('the overview offers a way in that is not an offer to change', () => {
    expect(overview).toContain("See all of this patch's rules");
    expect(overview).toContain('href="/patches/{slug}/governance/rules"');
  });
});

describe('what pressing the button will do (F-117)', () => {
  it('the review screen says which of the two acts it is', () => {
    expect(editor).toContain('let submitConsequence = $derived(');
    expect(editor).toContain('Saving changes these rules now. Nobody votes on it.');
    expect(editor).toContain('These rules do not change unless it carries.');
    expect(editor).toContain('{submitConsequence}');
  });

  // "A vote" and "a vote that runs for two weeks" are different things to
  // agree to, so the sentence carries the patch's own duration.
  it('names the patch\'s own voting period', () => {
    expect(editor).toContain('currentRules?.default_vote_duration_hours');
  });

  it('the rules page says the same thing before anyone opens the form', () => {
    expect(page).toContain('Your change takes effect when you save it.');
    expect(page).toContain('decided by a vote.');
  });
});

describe('one vocabulary, read and write (F-117)', () => {
  it('renders a maintainer patch without inviting it to set a term', () => {
    const lines = readableRules({
      decision_method: 'majority', quorum_percent: 0, default_vote_duration_hours: 72,
      amendment_threshold: 'majority', amendment_auto_apply: true,
      leadership_model: 'maintainer', succession_policy: 'longest_tenure',
    });
    const labels = lines.map((l) => l.label);
    expect(labels).toContain('How a proposal carries');
    expect(labels).not.toContain('How long a term runs');
    expect(lines.find((l) => l.label === 'How admins are made').value).toBe('One maintainer');
  });

  // A patch that decides elsewhere runs none of the ballot rules, so
  // printing them would narrate a mechanic it does not have (docs/adr/049).
  it('leaves out the ballot rules where there is no ballot', () => {
    const labels = readableRules({
      proposal_venue: 'elsewhere', leadership_model: 'maintainer',
    }).map((l) => l.label);
    expect(labels).not.toContain('Quorum');
    expect(labels).not.toContain('How a proposal carries');
    expect(labels).toContain('Where proposals are decided');
  });

  // The two numbers the inactivity sweep actually runs on, the same pair
  // the council page and the bell now state (F-115).
  it('states both inactivity days rather than one', () => {
    const line = readableRules({ leadership_model: 'maintainer', inactivity_days: 30 })
      .find((l) => l.label === 'An admin who stops taking part');
    expect(line.value).toContain('30 days');
    expect(line.value).toContain('60 days');
  });

  it('names the follower permissions the notice is about', () => {
    const line = readableRules(
      { leadership_model: 'maintainer' },
      { events: true, proposals: false, charters: false, members: true }
    ).find((l) => l.label === 'What a follower can see');
    expect(line.value).toBe('events, the member list');
  });

  it('says so plainly when a follower can see nothing', () => {
    const line = readableRules(
      { leadership_model: 'maintainer' },
      { events: false, proposals: false, charters: false, members: false }
    ).find((l) => l.label === 'What a follower can see');
    expect(line.value).toBe('Nothing beyond the patch page');
  });

  // Found in the browser: an admin-decided patch stores 0 hours and the
  // page read "How long a vote stays open: 0". wordFor cannot treat 0 as
  // absence, because 0 is a real answer for tenure ("Immediate"), so the
  // caller decides.
  it('omits a voting period the patch has not set rather than printing zero', () => {
    const labels = readableRules({
      decision_method: 'admin', default_vote_duration_hours: 0,
      leadership_model: 'maintainer',
    }).map((l) => l.label);
    expect(labels).not.toContain('How long a vote stays open');
  });

  it('still shows one the patch has set', () => {
    const line = readableRules({
      decision_method: 'majority', default_vote_duration_hours: 336,
      leadership_model: 'maintainer',
    }).find((l) => l.label === 'How long a vote stays open');
    expect(line.value).toBe('2 weeks');
  });

  // Zero still means "Immediate" for tenure, which is why the fix belongs
  // at the call site and not in wordFor.
  it('keeps zero meaningful where zero is an answer', () => {
    const line = readableRules({
      decision_method: 'majority', default_vote_duration_hours: 72,
      min_voting_tenure_days: 0, leadership_model: 'maintainer',
    }).find((l) => l.label === 'Before a new member may vote');
    expect(line.value).toBe('Straight away');
  });

  // A stored value the options do not know is shown, not swallowed — the
  // same rule the editor follows when it round-trips one.
  it('shows a policy it has no word for rather than nothing', () => {
    expect(wordFor(SUCCESSION_OPTIONS, 'something_new')).toBe('something_new');
    expect(wordFor(SUCCESSION_OPTIONS, '')).toBe('Not set');
  });
});
