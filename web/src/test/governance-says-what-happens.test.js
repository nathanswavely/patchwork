/**
 * Governance pages say what the server will do, and read what it sends
 * (#409).
 *
 * Six places where the web client and the governance handlers had drifted
 * apart: the history page read diff fields the server never sends, an
 * approval was credited to the person who asked for it, two editors told
 * people a vote would open where none does, the nominee picker stopped at
 * page one, and the proposal form ignored a patch that decides at meetings.
 *
 * The two helpers are exercised directly. The pages are asserted against
 * source text, because there is no Svelte render library in this project.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { textsFromDiff } from '../lib/governanceDiff.js';

vi.mock('../lib/api.js', () => ({ api: vi.fn() }));
import { api } from '../lib/api.js';
import { allMembers } from '../lib/patchMembers.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('version history renders the diff the server sends', () => {
  // The shape GetGovernanceDiff encodes: governance.DiffResult.
  const payload = {
    from_sha: 'aaa',
    to_sha: 'bbb',
    filename: 'charter.md',
    hunks: [
      { type: 'equal', text: '# Charter\n\nMembers meet ' },
      { type: 'delete', text: 'monthly' },
      { type: 'insert', text: 'weekly' },
      { type: 'equal', text: '.\n' },
      { type: 'insert', text: 'Dues are optional.\n' },
    ],
    unified: '@@ -1,3 +1,4 @@ ...',
  };

  it('rebuilds both versions from the hunks', () => {
    expect(textsFromDiff(payload)).toEqual({
      oldText: '# Charter\n\nMembers meet monthly.\n',
      newText: '# Charter\n\nMembers meet weekly.\nDues are optional.\n',
    });
  });

  it('returns null without hunks, so the page can fall back to the patch text', () => {
    expect(textsFromDiff(null)).toBeNull();
    expect(textsFromDiff({ unified: 'x', hunks: null })).toBeNull();
  });

  it('never reads the fields the server does not send', () => {
    const page = source('pages/GovernanceVersionHistory.svelte');
    expect(page).not.toMatch(/old_content|new_content/);
    expect(page).toMatch(/diffTexts = \$derived\(textsFromDiff\(diffData\)\)/);
    expect(page).toMatch(/compareTexts = \$derived\(textsFromDiff\(compareDiff\)\)/);
    expect(page).toMatch(/oldText=\{diffTexts\.oldText\}/);
    expect(page).toMatch(/oldText=\{compareTexts\.oldText\}/);
  });
});

describe('an approved request is not credited to its author', () => {
  const detail = source('pages/ProposalDetail.svelte');

  it('calls a proposal a direct change only when it was born applied', () => {
    // settleApplied stamps a direct change with its own created_at; an
    // approval is stamped when the maintainer decides.
    expect(detail).toMatch(
      /isDirectChange = \$derived\(\s*effectiveState === 'in_effect' &&\s*!!proposal\?\.applied_at &&\s*proposal\.applied_at === proposal\.created_at\s*\)/
    );
    expect(detail).not.toMatch(/isDirectChange = \$derived\([^)]*voters/);
  });

  it('still names the author only on a direct change', () => {
    expect(detail).toMatch(/\{isDirectChange \? 'Applied by' : 'Proposed by'\}/);
  });
});

describe('the rules editor says where a change goes', () => {
  const editor = source('pages/RulesProposalEditor.svelte');

  it('sends a member on an admin-decides patch to the maintainer, not a vote', () => {
    expect(editor).toMatch(/toMaintainer = \$derived\(\s*!directChange && !refusedHere && currentRules\?\.decision_method === 'admin'\s*\)/);
    expect(editor).toMatch(/This goes to this patch's maintainer, who decides whether to apply it/);
    expect(editor).toMatch(/\{:else if toMaintainer\}\s*\{submitting \? 'Sending\.\.\.' : 'Send to the maintainer'\}/);
  });

  it('opens a vote only on the voting path', () => {
    // The vote sentence is the last branch: direct, refused and
    // to-the-maintainer all come first.
    const vote = editor.indexOf('opens a vote${voteLength}');
    expect(vote).toBeGreaterThan(editor.indexOf(': toMaintainer'));
  });

  it('does not offer a member a rules proposal the server refuses on a patch that decides elsewhere', () => {
    expect(editor).toMatch(/refusedHere = \$derived\(!directChange && currentRules\?\.proposal_venue === 'elsewhere'\)/);
    expect(editor).toMatch(/disabled=\{submitting \|\| !title\.trim\(\) \|\| refusedHere\}/);
  });
});

describe('the nominee pickers page through every member', () => {
  beforeEach(() => api.mockReset());

  it('follows next_cursor with after=, which is what the server reads', async () => {
    api
      .mockResolvedValueOnce({ items: [{ user_id: 'a', role: 'member' }], next_cursor: 'c1' })
      .mockResolvedValueOnce({ items: [{ user_id: 'b', role: 'member' }], next_cursor: '' });
    const got = await allMembers('gallery-row', { role: 'member' });
    expect(got.map((m) => m.user_id)).toEqual(['a', 'b']);
    expect(api.mock.calls[0][0]).toBe('nodes/gallery-row/members?limit=100&role=member');
    expect(api.mock.calls[1][0]).toBe('nodes/gallery-row/members?limit=100&role=member&after=c1');
    for (const [path] of api.mock.calls) expect(path).not.toMatch(/cursor=/);
  });

  it('stops on a cursor that does not move', async () => {
    api.mockResolvedValue({ items: [{ user_id: 'a' }], next_cursor: 'same' });
    await allMembers('x');
    expect(api).toHaveBeenCalledTimes(2);
  });

  it('is what the proposal form and the election picker use', () => {
    const form = source('pages/ProposalForm.svelte');
    const detail = source('pages/ProposalDetail.svelte');
    expect(form).toMatch(/await allMembers\(slug, \{ role: 'member' \}\)/);
    expect(form).not.toMatch(/api\(`nodes\/\$\{slug\}\/members`\)/);
    expect(detail).toMatch(/await allMembers\(slug\)/);
    expect(detail).not.toMatch(/cursor=\$\{/);
  });
});

describe('the proposal form follows CreateProposal', () => {
  const form = source('pages/ProposalForm.svelte');

  it('reads the venue off the node payload', () => {
    expect(form).toMatch(/decidesElsewhere = \$derived\(patch\.value\.node\?\.governance_config\?\.proposal_venue === 'elsewhere'\)/);
  });

  it('opens a proposal for discussion on a patch that decides elsewhere, with no window', () => {
    expect(form).toMatch(/forDiscussion = \$derived\(decidesElsewhere && !directChange\)/);
    expect(form).toMatch(/asksDuration = \$derived\(!forDiscussion && /);
    expect(form).toMatch(/\{:else if forDiscussion\}\s*\{submitting \? 'Opening\.\.\.' : 'Open for discussion'\}/);
    expect(form).toMatch(/Yours opens here for discussion, with no vote/);
  });

  it('keeps an advisory vote and the maintainer request off a patch that decides elsewhere', () => {
    expect(form).toMatch(/advisoryVote = \$derived\(adminDecides && isPatchAdmin && putToVote && !decidesElsewhere\)/);
    expect(form).toMatch(/toMaintainer = \$derived\(adminDecides && !isPatchAdmin && !decidesElsewhere\)/);
  });

  it("starts the window at the patch's default rather than a hard-coded 72 hours", () => {
    expect(form).toMatch(/defaultDuration = \$derived\(patch\.value\.node\?\.governance_config\?\.default_vote_duration_hours \|\| 72\)/);
    expect(form).toMatch(/if \(!durationTouched\) durationHours = defaultDuration/);
    expect(form).not.toMatch(/Tip: 72 hours/);
  });
});

describe('the document door says what it opens', () => {
  for (const page of ['pages/GovernanceList.svelte', 'pages/GovernanceDetail.svelte']) {
    it(`${page} says Edit where the change applies directly`, () => {
      const src = source(page);
      expect(src).toMatch(
        /directChange = \$derived\(\s*membershipRole === 'admin' && patch\.value\.node\?\.governance_config\?\.decision_method === 'admin'\s*\)/
      );
      expect(src).toMatch(/\{directChange \? 'Edit' : 'Propose change'\}/);
    });
  }
});
