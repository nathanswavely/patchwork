import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  summarizeRuleChanges,
  changedRuleFields,
  ruleLabel,
  formatRuleValue,
} from '../lib/rulesDiff.js';

function shipped(path) {
  return readFileSync(resolve(__dirname, path), 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^[ \t]*\/\/.*$/gm, '');
}

const editor = shipped('../pages/RulesProposalEditor.svelte');
const diff = shipped('../components/StructuredRulesDiff.svelte');

// A patch's whole rules file, as the endpoint sends it. Eighteen fields,
// which is the number that mattered: the old generator inspected five.
function rules(overrides = {}) {
  return {
    decision_method: 'majority',
    quorum_percent: 20,
    default_vote_duration_hours: 72,
    amendment_threshold: 'supermajority',
    amendment_auto_apply: false,
    succession_policy: 'longest_tenure',
    min_voting_tenure_days: 0,
    subject_recusal: false,
    leadership_venue: 'patchwork',
    proposal_venue: 'patchwork',
    leadership_model: 'maintainer',
    membership_policy: 'open',
    admin_term_months: 12,
    nomination_days: 7,
    max_admins: 0,
    inactivity_days: 90,
    follower_permissions: { events: true, proposals: true, charters: false, members: true },
    ...overrides,
  };
}

// F-038, reproduced in a browser on 2026-10-19: the prefilled Summary of
// changes described a fraction of what the diff beside it listed, and that
// string is what the governance Record keeps forever.
describe('the prefilled summary names the whole change (F-038)', () => {
  it('names a single change with both of its values', () => {
    const summary = summarizeRuleChanges(
      rules(),
      rules({ membership_policy: 'approval_required' }),
    );
    expect(summary).toBe('Membership Policy: Open → Approval required');
  });

  // The first reproduction: three settings changed, and the summary read
  // "Approval-required membership" — the name of one of them, in a position
  // that reads as the whole.
  it('leads with the count when more than one rule moved', () => {
    const summary = summarizeRuleChanges(
      rules(),
      rules({
        leadership_venue: 'elsewhere',
        proposal_venue: 'elsewhere',
        membership_policy: 'approval_required',
      }),
    );
    expect(summary).toBe(
      '3 governance rules changed: Where admins are chosen, Where proposals are decided, Membership Policy',
    );
  });

  // The second reproduction: both venues moved off Patchwork, which is
  // about as consequential as a patch's rules get, and the summary read
  // "Update governance rules" over a diff naming both.
  it('never says "Update governance rules" over a diff with named changes', () => {
    const current = rules();
    const proposed = rules({ leadership_venue: 'elsewhere', proposal_venue: 'elsewhere' });
    expect(changedRuleFields(current, proposed)).toHaveLength(2);
    const summary = summarizeRuleChanges(current, proposed);
    expect(summary).not.toContain('Update governance rules');
    expect(summary).toBe(
      '2 governance rules changed: Where admins are chosen, Where proposals are decided',
    );
  });

  // The fields the old generator could not see, one at a time. Each one is
  // a real change somebody can make in this form, and none of them may
  // leave the record saying nothing moved.
  it('sees every field the diff can show', () => {
    const cases = {
      amendment_threshold: 'consensus',
      amendment_auto_apply: true,
      succession_policy: 'freeze',
      min_voting_tenure_days: 30,
      subject_recusal: true,
      leadership_venue: 'elsewhere',
      proposal_venue: 'elsewhere',
      admin_term_months: 24,
      nomination_days: 14,
      max_admins: 5,
      inactivity_days: 180,
      follower_permissions: { events: true, proposals: false, charters: false, members: true },
    };
    for (const [key, value] of Object.entries(cases)) {
      const summary = summarizeRuleChanges(rules(), rules({ [key]: value }));
      expect(summary, key).not.toBe('Update governance rules');
      expect(summary, key).toContain(ruleLabel(key));
    }
  });

  // A rules file is a JSON document the server owns. A field added to it
  // that nobody has labelled here still has to show up in the record.
  it('names an unlabelled field by its own name rather than ignoring it', () => {
    const summary = summarizeRuleChanges(
      rules({ quiet_hours: 'none' }),
      rules({ quiet_hours: 'nights' }),
    );
    expect(summary).toBe('Quiet hours: none → nights');
  });

  it('counts the ones it has no room to name', () => {
    const summary = summarizeRuleChanges(
      rules(),
      rules({
        decision_method: 'consensus',
        quorum_percent: 50,
        default_vote_duration_hours: 168,
        amendment_threshold: 'consensus',
        amendment_auto_apply: true,
        succession_policy: 'freeze',
        min_voting_tenure_days: 30,
      }),
    );
    expect(summary).toContain('7 governance rules changed:');
    expect(summary).toContain('and 2 more');
  });

  it('falls back only when nothing moved', () => {
    expect(summarizeRuleChanges(rules(), rules())).toBe('Update governance rules');
    expect(summarizeRuleChanges(null, rules())).toBe('Update governance rules');
  });
});

describe('the summary and the diff read from one list', () => {
  // The defect was two lists: the diff walked every key, the summary
  // inspected five by hand. Either one can be wrong; only two can disagree,
  // and a reviewer reads them side by side.
  it('the editor prefills from the shared module and nowhere else', () => {
    expect(editor).toContain("import { summarizeRuleChanges } from '../lib/rulesDiff.js';");
    expect(editor).toContain('const suggestion = summarizeRuleChanges(currentRules, proposedRules);');
    expect(editor).not.toContain('function generateTitle');
    expect(editor).not.toContain('TITLE_VALUES');
  });

  it('the diff renders from the shared module and nowhere else', () => {
    expect(diff).toContain("from '../lib/rulesDiff.js';");
    expect(diff).toContain('changedRuleFields(currentRules, proposedRules)');
    expect(diff).not.toContain('const LABELS =');
    expect(diff).not.toContain('function formatValue');
  });

  // Submitted as change_summary, which is the commit message on the
  // amendment branch (internal/handler/proposals.go).
  it('submits the summary as the record entry', () => {
    expect(editor).toContain('change_summary: title.trim(),');
  });

  // Prefill, not overwrite: a sentence the author typed survives a trip
  // back to the editor, and a suggestion nobody touched is refreshed
  // against what the diff now shows.
  it('refreshes its own suggestion and keeps the author’s', () => {
    expect(editor).toContain('if (!title || title === suggestedTitle) {');
    expect(editor).toContain('suggestedTitle = suggestion;');
  });
});

describe('the words a change is named in', () => {
  it('reads a boolean rule as Yes or No rather than true', () => {
    expect(formatRuleValue('subject_recusal', true)).toBe('Yes');
    expect(formatRuleValue('subject_recusal', false)).toBe('No');
  });

  it('keeps the diff’s own wording for a venue', () => {
    expect(formatRuleValue('proposal_venue', 'patchwork')).toBe('In Patchwork');
    expect(formatRuleValue('proposal_venue', 'elsewhere')).toBe('Somewhere else, recorded here');
  });
});
