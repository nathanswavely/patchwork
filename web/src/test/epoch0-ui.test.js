/**
 * Epoch 0 of the governance simulation, frontend findings.
 *
 * Each describe is one finding from a persona's journal, reproduced by the
 * auditor. There is no Svelte render library in this project, so component
 * wiring is asserted against source text; the behaviour was checked in a
 * browser against the simulation instance.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { timeLeft, timeLeftPhrase, timeLeftShort } from '../lib/datetime.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('F-008 — the create form asks who can join', () => {
  const src = source('pages/PatchForm.svelte');

  // F-008's answer was to start unset, so that nobody got a policy they had
  // not chosen. That fixed the right bug the wrong way round: unset weights
  // the three options equally and the first card reads as the ordinary pick,
  // while the template list below is preselected at Minimal — whose rules say
  // invite_only. The form pre-answered one question and left the other
  // looking like a choice between equals, and on the reference instance a
  // five-minute-old account took the first card and got an open patch.
  //
  // So the default is back, pointing the other way. Still asked, still
  // required, still one click to change; what moved is which way it fails
  // when nobody engages, and closed is the safe direction.
  it('defaults to the closed option rather than leaving the question unweighted', () => {
    expect(src).toMatch(/let membershipPolicy = \$state\('invite_only'\)/);
    expect(src).toMatch(/if \(!membershipPolicy\) return 'Choose a membership policy'/);
  });

  it('offers the three policies the API accepts, closed first, each with one plain line', () => {
    expect(src).toMatch(/id: 'invite_only', name: 'Invite only', desc: '[^']+'/);
    expect(src).toMatch(/id: 'approval_required', name: 'Approval required', desc: '[^']+'/);
    expect(src).toMatch(/id: 'open', name: 'Open', desc: '[^']+'/);
    // Order is the point, not just presence: the first card is the one a
    // hurried reader takes.
    expect(src.indexOf("id: 'invite_only'")).toBeLessThan(src.indexOf("id: 'approval_required'"));
    expect(src.indexOf("id: 'approval_required'")).toBeLessThan(src.indexOf("id: 'open'"));
    // Not [^>]* — the onchange handler in this tag contains an arrow.
    expect(src).toMatch(/<input type="radio" name="membership_policy" value=\{p\.id\} bind:group=\{membershipPolicy\}[\s\S]*?required/);
  });

  // Joining and following are different relationships and the form never said
  // so, which is how Open gets picked by somebody reasoning that people need
  // it to see the patch — the one thing this setting does not govern.
  it('contrasts joining with following, which the setting does not touch', () => {
    expect(src).toMatch(/Members vote on proposals/);
    expect(src).toMatch(/Following is separate and always open/);
    // No em dashes in copy a reader sees. Scoped to the paragraph, not the
    // surrounding markup: code comments are not copy and keep house style.
    const hint = src.match(/<p class="field-hint muted">\s*Members vote[\s\S]*?<\/p>/);
    expect(hint, 'who-can-join hint paragraph not found').toBeTruthy();
    expect(hint[0]).not.toContain('—');
  });

  it('sits ahead of the template picker and sends the choice in the create payload', () => {
    expect(src.indexOf('<legend>Membership Policy')).toBeLessThan(src.indexOf('Governance Template'));
    expect(src).toMatch(/membership_policy: membershipPolicy,\n\s*template,/);
  });

  // This used to assert the opposite — that setup skipped the question,
  // on the reasoning that a claimed listing already carried a policy. It
  // did, and nobody had chosen it: every listing is written 'open', the
  // value is inert while the patch is unclaimed, and a claim made it the
  // live door. The first real claim on Lancaster admitted anyone. Setup is
  // the creation moment (docs/adr/039), so it asks the creation question.
  it('asks in setup mode too, seeded from the template', () => {
    expect(src).not.toMatch(/\{#if mode !== 'setup'\}\s*\n\s*<fieldset class="field policy-field">/);
    expect(src).toMatch(/let policySeeded = \$derived\(mode === 'setup' && !policyAnswered/);
    expect(src).toMatch(/if \(policySeeded\) membershipPolicy = seededPolicy/);
    // Answering it yourself ends the seeding — the template stops moving
    // an answer the claimant has already given.
    expect(src).toMatch(/onchange=\{\(\) => policyAnswered = true\}/);
  });
});

describe('F-011 — a press on Create Patch is not swallowed by the suggested placement', () => {
  const src = source('pages/PatchForm.svelte');

  it('waits for the pointer to come up before revealing the picker', () => {
    expect(src).toMatch(/window\.addEventListener\('pointerdown', down, true\)/);
    expect(src).toMatch(/if \(pointerHeld\) await pointerReleased\(\);\s*\n\s*showPicker = true;/);
    // A tick after pointerup, so the click has dispatched against the
    // layout it started on.
    expect(src).toMatch(/window\.addEventListener\('pointerup', done\)/);
    expect(src).toMatch(/setTimeout\(resolve, 0\)/);
  });

  it('keeps the confirm step: the form still only sends a placed marker', () => {
    expect(src).toMatch(/latitude: placed \? latitude : undefined/);
    expect(src).toMatch(/function confirmPlacement\(lat, lng\)/);
  });
});

describe('F-016 — creating a patch lands on the patch, not on /welcome', () => {
  const src = source('pages/PatchForm.svelte');

  it('refreshes the memberships store after the POST and before navigating', () => {
    expect(src).toMatch(/import \{ loadMemberships \} from '\.\.\/stores\/memberships\.svelte\.js'/);
    const post = src.indexOf("await api('nodes', { method: 'POST', body })");
    const reload = src.indexOf('await loadMemberships();', post);
    const nav = src.indexOf('navigate(`/patches/${result.slug}`)', post);
    expect(post).toBeGreaterThan(-1);
    expect(reload).toBeGreaterThan(post);
    expect(nav).toBeGreaterThan(reload);
  });

  it('does the same on the setup path, whose claimant becomes admin too', () => {
    const setup = src.indexOf('await api(`claims/${claimId}/setup`');
    const reload = src.indexOf('await loadMemberships();', setup);
    const nav = src.indexOf('navigate(`/patches/${setupSlug}`);\n      submitting = false;', setup);
    expect(reload).toBeGreaterThan(setup);
    expect(nav).toBeGreaterThan(reload);
  });
});

describe('F-017 — dashboard counts are counts, and the attention list is the person\'s own', () => {
  const src = source('pages/Dashboard.svelte');

  it('reads member_count for pending requests instead of the length of a one-row page', () => {
    expect(src).toMatch(/typeof data\.member_count === 'number' \? data\.member_count : items\.length/);
    expect(src).not.toMatch(/if \(items\.length > 0\) pending\[m\.node_slug\] = items\.length/);
  });

  it('asks for a real page of proposals and says 20+ when more follow', () => {
    expect(src).toMatch(/const PAGE = 20;/);
    expect(src).toMatch(/proposals\?status=open&limit=\$\{PAGE\}/);
    expect(src).toMatch(/\{ count: items\.length, more: !!data\.next_cursor \}/);
    expect(src).toMatch(/function countLabel\(count, more\)/);
    expect(src).toMatch(/countLabel\(totalProposals, proposalsMore\)/);
  });

  it('scopes upcoming events to the person\'s own patches', () => {
    expect(src).toMatch(/events\?scope=my&from=/);
    expect(src).toMatch(/countLabel\(upcomingEvents\.length, upcomingMore\)/);
  });
});

describe('F-022 — the rules editor knows every succession policy a template ships', () => {
  const src = source('components/StructuredRulesEditor.svelte');
  // The options moved to a module the read-only rules page shares with the
  // editor, so one setting cannot end up with two names (F-117). The
  // property this holds is unchanged: every policy a template can store has
  // an option here, and each says in plain words what actually happens.
  const vocab = source('lib/governanceRules.js');

  it('has options for election and nomination, each with a plain hint', () => {
    expect(vocab).toMatch(/value: 'election', label: 'Election',\s*\n\s*hint: '[^']+'/);
    expect(vocab).toMatch(/value: 'nomination', label: 'Nomination',\s*\n\s*hint: '[^']+'/);
    expect(src).toMatch(/<p class="venue-hint muted">\{successionHint\}<\/p>/);
  });

  it('reads them from the shared module rather than keeping a second copy', () => {
    expect(src).toMatch(/from '\.\.\/lib\/governanceRules\.js'/);
    expect(src).not.toMatch(/const SUCCESSION_OPTIONS = \[/);
  });

  it('round-trips a stored value it has no option for rather than resetting it', () => {
    expect(src).toMatch(/\[\.\.\.SUCCESSION_OPTIONS, \{ value: successionPolicy, label: successionPolicy, hint: '' \}\]/);
    expect(src).toMatch(/\{#each successionOptions as opt \(opt\.value\)\}/);
    // Everything loaded is spread back before the edited fields land.
    expect(src).toMatch(/\.\.\.\(currentRules \|\| \{\}\),\s*\n\s*decision_method: decisionMethod,/);
  });
});

describe('F-023 — the event form heading follows who is posting', () => {
  const src = source('pages/EventForm.svelte');

  it('says "Suggest" and "reviewed" only when the pick will be held for review', () => {
    expect(src).toMatch(/\{:else if lockSlug && willReview\}\s*\n\s*<h1>Suggest an <VocabLabel term="event" \/><\/h1>/);
    expect(src).toMatch(/It will be reviewed before it appears\./);
    expect(src).not.toMatch(/<h1>\{!isEdit && lockSlug \? 'Suggest an'/);
  });

  it('tells a member or admin their event appears when saved', () => {
    expect(src).toMatch(/\{:else if lockSlug\}\s*\n\s*<h1>Create <VocabLabel term="event" \/><\/h1>/);
    expect(src).toMatch(/It appears as soon as you save it\./);
  });
});

describe('F-025 — Escape closes the template preview drawer', () => {
  const src = source('pages/PatchForm.svelte');

  it('listens on the window and clears previewTemplate', () => {
    expect(src).toMatch(/onkeydown=\{\(e\) => \{ if \(e\.key === 'Escape' && previewTemplate\) previewTemplate = ''; \}\}/);
  });
});

describe('F-036 — a 14-day window says 14 days on its first day', () => {
  const now = new Date('2026-09-14T12:00:00Z');
  const at = (ms) => new Date(now.getTime() + ms).toISOString();
  const DAY = 86_400_000;
  const HOUR = 3_600_000;

  it('rounds days up when more than a day remains', () => {
    expect(timeLeftPhrase(timeLeft(at(14 * DAY), now))).toBe('14 days');
    expect(timeLeftPhrase(timeLeft(at(14 * DAY - HOUR), now))).toBe('14 days');
    expect(timeLeftPhrase(timeLeft(at(DAY + 1), now))).toBe('2 days');
    expect(timeLeftPhrase(timeLeft(at(DAY), now))).toBe('1 day');
  });

  it('keeps hours under a day, and minutes under an hour', () => {
    expect(timeLeftPhrase(timeLeft(at(DAY - 1), now))).toBe('23 hours');
    expect(timeLeftPhrase(timeLeft(at(5 * HOUR + 40 * 60_000), now))).toBe('5 hours');
    expect(timeLeftPhrase(timeLeft(at(59 * 60_000), now))).toBe('59 minutes');
    expect(timeLeftPhrase(timeLeft(at(1), now))).toBe('1 minute');
    expect(timeLeftShort(timeLeft(at(14 * DAY - HOUR), now))).toBe('14d');
    expect(timeLeftShort(timeLeft(at(5 * HOUR), now))).toBe('5h');
  });

  it('reports an ended window and no window', () => {
    expect(timeLeft(at(0), now)).toEqual({ ended: true, unit: null, n: 0 });
    expect(timeLeft(null, now)).toBeNull();
    expect(timeLeftPhrase(timeLeft(at(-1), now))).toBe('');
  });

  it('is the one formatter every surface uses', () => {
    for (const rel of [
      'components/ProposalStatusBanner.svelte',
      'components/VoteSection.svelte',
      'components/StickyVoteBar.svelte',
      'pages/ProposalList.svelte',
    ]) {
      const src = source(rel);
      expect(src, rel).toMatch(/from '\.\.\/lib\/datetime\.js'/);
      expect(src, rel).not.toMatch(/Math\.floor\(ms \/ 86400000\)/);
      expect(src, rel).not.toMatch(/Math\.floor\(hours \/ 24\)/);
    }
  });
});
