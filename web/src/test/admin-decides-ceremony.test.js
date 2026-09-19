/**
 * The maintainer decides, and may consult (docs/adr/092).
 *
 * On an admin-decides patch the maintainer decides every proposal: an admin's
 * is a direct change unless they ask the members first, a member's waits on
 * them, and any vote held is advisory. docs/adr/041 already said the UI never
 * says "propose", "submit" or "vote" for a direct change — the rules editor
 * honoured it and the general proposal form did not, which is how a sole
 * admin got a voting-duration picker and a "Submit Proposal" button for a
 * change that applied the instant they clicked.
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

describe('ProposalForm — ceremony follows the decision method', () => {
  const src = source('pages/ProposalForm.svelte');

  it('reads the decision method off the node payload, and the role off membershipRole', () => {
    expect(src).toMatch(/decisionMethod = \$derived\(patch\.value\.node\?\.governance_config\?\.decision_method/);
    expect(src).toMatch(/isPatchAdmin = \$derived\(membershipRole === 'admin'\)/);
    // Not isAdmin: the node payload sets it for instance admins too.
    expect(src).not.toMatch(/directChange = \$derived\(.*\bisAdmin\b/);
  });

  it('offers an admin the choice between applying now and asking the members', () => {
    expect(src).toMatch(/\{#if adminDecides && isPatchAdmin\}/);
    expect(src).toMatch(/Apply it now/);
    expect(src).toMatch(/Ask the members first/);
    expect(src).toMatch(/bind:group=\{putToVote\}/);
  });

  it('tells a member their proposal goes to the maintainer', () => {
    expect(src).toMatch(/\{:else if toMaintainer\}/);
    expect(src).toMatch(/maintainer decides proposals\. Yours goes to them/);
  });

  it('asks for a voting duration only where a vote will run', () => {
    expect(src).toMatch(/asksDuration = \$derived\(!adminDecides \|\| advisoryVote\)/);
    expect(src).toMatch(/\{#if asksDuration\}/);
    expect(src).toMatch(/if \(asksDuration\) payload\.duration_hours = durationHours/);
  });

  it('never says "submit" for a direct change', () => {
    expect(src).toMatch(/\{#if directChange\}\s*\{submitting \? 'Applying\.\.\.' : 'Apply change'\}/);
    expect(src).toMatch(/'Open advisory vote'/);
    expect(src).toMatch(/'Send to the maintainer'/);
  });

  it('sends put_to_vote only from an admin on an admin-decides patch', () => {
    expect(src).toMatch(/if \(adminDecides && isPatchAdmin\) payload\.put_to_vote = putToVote/);
  });
});

describe('AmendmentEditor — same derivation as the rules editor', () => {
  const src = source('pages/AmendmentEditor.svelte');

  it('derives directChange from the patch role and the decision method', () => {
    expect(src).toMatch(/directChange = \$derived\(\s*membershipRole === 'admin' && patch\.value\.node\?\.governance_config\?\.decision_method === 'admin'/);
  });

  it('labels the buttons as applying, not submitting, for a direct change', () => {
    expect(src).toMatch(/directChange \? 'Review & apply' : 'Review & submit'/);
    expect(src).toMatch(/\{#if directChange\}\s*\{submitting \? 'Applying\.\.\.' : 'Apply change'\}/);
    expect(src).toMatch(/directChange \? 'Change applied' : 'Proposal created'/);
  });
});

describe('ProposalStatusBanner — advisory votes and the maintainer\'s verbs', () => {
  const src = source('components/ProposalStatusBanner.svelte');

  it('takes advisory, canDecide and declinedBy from the server', () => {
    expect(src).toMatch(/advisory = false,/);
    expect(src).toMatch(/canDecide = false,/);
    expect(src).toMatch(/declinedBy = '',/);
  });

  it('says an advisory vote is advisory before anything else', () => {
    expect(src).toMatch(/\{:else if effectiveState === 'voting' && advisory\}/);
    expect(src).toMatch(/'Advisory vote\. '/);
    expect(src).toMatch(/'The maintainer decides\.'/);
    // The advisory branch sits before the plain voting branch, or it never
    // renders.
    expect(src.indexOf("effectiveState === 'voting' && advisory")).toBeLessThan(
      src.indexOf("{:else if effectiveState === 'voting'}")
    );
  });

  it('renders a waiting proposal with the decision buttons gated on canDecide', () => {
    expect(src).toMatch(/\{:else if effectiveState === 'awaiting_admin'\}/);
    expect(src).toMatch(/Waiting on the maintainer\./);
    expect(src).toMatch(/\{#if canDecide\}\s*<button[^>]*onclick=\{\(\) => handleDecide\('approve'\)\}/);
    expect(src).toMatch(/onConfirm=\{\(\) => handleDecide\('decline'\)\}/);
  });

  it('offers to ask the members only once', () => {
    expect(src).toMatch(/mayAskMembers = \$derived\(canDecide && !hasBallots && !votingEndsAt\)/);
    expect(src).toMatch(/\{#if mayAskMembers\}/);
  });

  it('posts decisions to the decide endpoint and the ask to open-vote', () => {
    expect(src).toMatch(/proposals\/\$\{proposalId\}\/decide/);
    expect(src).toMatch(/proposals\/\$\{proposalId\}\/open-vote/);
  });

  it('words a decline as one person\'s decision, never a failed vote', () => {
    expect(src).toMatch(/\{#if declinedBy\}\s*Declined by \{declinedBy\}\./);
  });
});

describe('ProposalDetail — the tally is advice where the server says so', () => {
  const src = source('pages/ProposalDetail.svelte');

  it('reads advisory and can_decide from the payload', () => {
    expect(src).toMatch(/advisory = \$derived\(proposal\?\.advisory === true\)/);
    expect(src).toMatch(/canDecide = \$derived\(proposal\?\.can_decide === true\)/);
  });

  it('turns the sole-voter notice off on an advisory vote', () => {
    expect(src).toMatch(/soleVoter = \$derived\(isVoting && canVote && !advisory/);
  });

  it('shows a tally on a waiting proposal only once the members were asked', () => {
    expect(src).toMatch(/\(effectiveState === 'awaiting_admin' && hasBallots\)/);
  });

  it('passes the maintainer fields through to the banner and vote section', () => {
    expect(src).toMatch(/<ProposalStatusBanner[\s\S]*?\{advisory\}[\s\S]*?\{canDecide\}[\s\S]*?declinedBy=\{proposal\.declined_by/);
    expect(src).toMatch(/<VoteSection[\s\S]*?\{advisory\}[\s\S]*?proposalType=\{proposal\.proposal_type/);
  });
});

describe('VoteSection — no quorum on advice', () => {
  const src = source('components/VoteSection.svelte');

  it('explains an advisory tally as advice', () => {
    expect(src).toMatch(/if \(advisory\) return explanations\.admin/);
    expect(src).toMatch(/\{#if advisory\}\s*<span class="quorum-none muted">No quorum\. This vote advises; it does not decide\./);
  });
});

describe('ProposalList — a waiting row says what it waits for', () => {
  const src = source('pages/ProposalList.svelte');

  it('labels a waiting proposal instead of a clock that is not running', () => {
    expect(src).toMatch(/\{#if proposal\.state === 'awaiting_admin'\}/);
    expect(src).toMatch(/waiting on the maintainer/);
  });

  it('calls an admin\'s new proposal a change on an admin-decides patch', () => {
    expect(src).toMatch(/newLabel = \$derived\(adminDecides && membershipRole === 'admin' \? 'New change' : 'New Proposal'\)/);
  });
});
