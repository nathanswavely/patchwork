/**
 * Going back to editing keeps the change.
 *
 * Reproduced in a browser on 2026-10-19 on `wallflower-mural-project`: in the
 * rules editor, set both venues to "Somewhere else, recorded here" and
 * Membership Policy to "Approval required", click "Review & apply", then click
 * "Back to editing". Every select was back at its stored value, the Changes
 * section was gone, and the button that had just been pressed was disabled
 * again, because `hasChanges` was false. The author's work was gone with no
 * warning, on the one screen in the product whose whole job is composing a
 * governed change.
 *
 * The page renders `StructuredRulesEditor` on its 'editing' step only, so the
 * step flip destroys the form and builds a new one. That instance seeded its
 * state from `currentRules` and then notified the parent of what it had
 * seeded, overwriting the pending change the parent was still holding.
 *
 * There is no Svelte render library in this project, so what these assert is
 * the wiring: which object the form opens on, and that it does not fight an
 * edit in progress. The behaviour itself was confirmed in a browser against a
 * running app, which is the only place it could be.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

const editor = source('components/StructuredRulesEditor.svelte');
const page = source('pages/RulesProposalEditor.svelte');

describe('the rules form opens on the change in progress', () => {
  it('takes the pending change as a prop, not just the stored rules', () => {
    expect(editor).toMatch(/proposedRules = null/);
    expect(editor).toMatch(/const incoming = proposedRules \|\| currentRules;/);
  });

  it('seeds every field it edits from that object', () => {
    // The whole form, field by field: one left reading `currentRules` would
    // reset exactly one setting on the way back, which is worse than all of
    // them, because nobody would notice.
    for (const field of [
      'decision_method',
      'quorum_percent',
      'default_vote_duration_hours',
      'amendment_threshold',
      'amendment_auto_apply',
      'succession_policy',
      'min_voting_tenure_days',
      'subject_recusal',
      'leadership_venue',
      'proposal_venue',
      'membership_policy',
      'follower_permissions',
    ]) {
      expect(editor).toMatch(new RegExp(`incoming\.${field}`));
    }
  });

  it('does not re-seed from the echo of its own notification', () => {
    // The parent hands straight back what the form emits. Re-seeding from
    // that normalizes a value somebody is still typing: emptying the Quorum
    // field emits no `quorum_percent` at all, and reading it back as `?? 0`
    // snapped the field to 0 under the cursor (seen in a browser while
    // fixing this). So the guard is what was sent, not what was last shown.
    expect(editor).toMatch(/let emitted = ''/);
    expect(editor).toMatch(/if \(JSON\.stringify\(incoming\) === emitted\) return;/);
    expect(editor).toMatch(/emitted = JSON\.stringify\(rules\);/);
    // And every notification goes through the one place that records it.
    expect(editor).not.toMatch(/onSave\(buildRules\(\)\)/);
  });

  it('is handed the pending change by the page', () => {
    expect(page).toMatch(/<StructuredRulesEditor[\s\S]*?proposedRules=\{proposedRules\}[\s\S]*?\/>/);
  });

  it('leaves the pending change alone when the step flips', () => {
    // "Back to editing" sets the step and nothing else.
    expect(page).toMatch(/onclick=\{\(\) => \{ step = 'editing'; \}\}/);
    // And the stored rules are only ever copied over the pending change
    // deliberately: by loading the page, and by Discard.
    const copies = page.match(/proposedRules = JSON\.parse\(JSON\.stringify\(currentRules\)\)/g) || [];
    expect(copies).toHaveLength(2);
  });
});
