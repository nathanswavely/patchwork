/**
 * My Patches (Settings → Patches): the list of every standing you hold.
 *
 * The rule it broke is docs/adr/042's — a rung renders only where it can
 * succeed. Every follower row wore a blue "Become a member", including the
 * community-submitted patches that take followers and nothing else, so the
 * click's whole answer was a 403 written for somebody who had not already
 * followed. The relationship row on the patch page had this right; this
 * list was the copy that drifted.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

const src = source('pages/UserSettingsPatches.svelte');

describe('My Patches: the member rung renders only where it can succeed', () => {
  it('withholds the rung on an unclaimed patch and on invite_only', () => {
    expect(src).toMatch(
      /if \(m\.node_status === 'unclaimed' \|\| m\.membership_policy === 'invite_only'\) return null/
    );
  });

  it('names the rung for what the click does — a request is not a joining', () => {
    expect(src).toContain("m.membership_policy === 'approval_required' ? 'Send request' : 'Become a member'");
  });

  it('reads the patch status the list is now served, not the membership status', () => {
    // Both fields are named `status` one object apart; the gate must not
    // read the membership's.
    expect(src).toContain('m.node_status');
  });

  it('wears the reason as state where the rung is absent', () => {
    expect(src).toContain('No one runs this patch yet. Until someone claims it, following is the only rung.');
    expect(src).toContain('This patch adds members by invitation. An admin has to invite you.');
    expect(src).toMatch(/\{#if note\}[\s\S]{0,200}state-badge/);
  });

  it('renders the rung behind its own guard, not unconditionally', () => {
    const following = src.slice(src.indexOf('<h3 class="section-heading">Following</h3>'));
    expect(following).toMatch(/\{#if rung\}[\s\S]{0,200}\{rung\}<\/button>/);
    expect(following).not.toMatch(/onclick=\{\(\) => handleBecomeMember/);
  });
});

describe('My Patches: joining runs the same ceremony as the patch page', () => {
  it('opens the join sheet rather than posting a silent request', () => {
    expect(src).toContain("import JoinSheet from '../components/JoinSheet.svelte'");
    expect(src).toMatch(/onclick=\{\(\) => \{ joinTarget = m; \}\}/);
    expect(src).toMatch(/<JoinSheet[\s\S]*membershipPolicy=\{joinTarget\?\.membership_policy \|\| 'open'\}/);
  });

  it('says what actually happened — a pending request is not a membership', () => {
    expect(src).toContain(
      "result.status === 'pending' ? 'Membership request sent' : 'You are now a member'"
    );
    expect(src).not.toContain("showToast('Joined as member'");
  });
});

describe('My Patches: a pending request is filed as one', () => {
  it('partitions on membership status, not role alone', () => {
    expect(src).toMatch(/adminPatches = \$derived\(patches\.filter\(m => m\.role === 'admin' && m\.status === 'active'\)\)/);
    expect(src).toMatch(/memberPatches = \$derived\(patches\.filter\(m => m\.role === 'member' && m\.status === 'active'\)\)/);
    expect(src).toMatch(/pendingPatches = \$derived\(patches\.filter\(m => m\.status === 'pending'\)\)/);
  });

  it('gives the request its own section and a way back out', () => {
    expect(src).toContain('<h3 class="section-heading">Requested</h3>');
    const requested = src.slice(
      src.indexOf('<h3 class="section-heading">Requested</h3>'),
      src.indexOf('<h3 class="section-heading">Following</h3>')
    );
    expect(requested).toContain('awaiting approval');
    expect(requested).toContain('label="Withdraw"');
    // Neither switch belongs on a standing you do not hold yet.
    expect(requested).not.toContain('visibilityToggle');
    expect(requested).not.toContain('contactToggle');
  });

  it('names each exit for the relationship it ends', () => {
    expect(src).toContain("'Request withdrawn'");
    expect(src).toContain("'Unfollowed patch'");
    expect(src).toContain("'Left patch'");
  });
});
