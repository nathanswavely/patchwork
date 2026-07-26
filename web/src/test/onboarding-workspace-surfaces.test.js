/**
 * Join sheet, unlock panel, and setup checklist (CONTEXT.md "Join sheet",
 * "Unlock panel", "Setup checklist", docs/adr/040). There is no Svelte
 * render library in this project (see router.test.js, scope-routing.test.js,
 * patch-setup.test.js), so component wiring is asserted against source text.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('JoinSheet', () => {
  const src = source('components/JoinSheet.svelte');

  it('carries no checkbox — joining informed is the agreement', () => {
    expect(src).not.toMatch(/type="checkbox"/);
  });

  it('states the membership policy in one sentence for both policies', () => {
    expect(src).toContain('Membership is open. Joining makes you a member.');
    expect(src).toContain('Membership is admin-approved. This sends a request.');
  });

  it('shows the lining state, linking amended lining to governance and the baseline to /lining', () => {
    expect(src).toMatch(/liningStatus === 'diverged'/);
    expect(src).toContain('This patch has amended the lining');
    expect(src).toMatch(/liningStatus === 'pristine' \|\| liningStatus === 'stale'/);
    expect(src).toContain("Starts from the quilt's");
    expect(src).toContain("goTo('/lining')");
  });

  it('fetches this patch\'s governance docs and lists only published, non-lining charters', () => {
    expect(src).toMatch(/api\(`nodes\/\$\{slug\}\/governance`\)/);
    expect(src).toMatch(/d\.kind !== 'lining' && d\.visibility === 'public'/);
  });

  it('omits the charters section when there are none (guarded by charters.length > 0)', () => {
    expect(src).toMatch(/\{#if !loadingCharters && charters\.length > 0\}/);
  });

  it('shows the optional intro message only for approval-required patches, capped at 500', () => {
    expect(src).toMatch(/\{#if isApproval\}[\s\S]*?join-message-input/);
    expect(src).toContain('maxlength="500"');
    expect(src).toContain('Introduce yourself to the admins (optional)');
  });

  // "Join" names joining the quilt; the patch rung is "Become a member"
  // everywhere, and the sheet's confirm names the act it commits (adr/042).
  it('labels the CTA Become a member vs Send request, and never fetches members-only content directly', () => {
    expect(src).toMatch(/\{isApproval \? 'Send request' : 'Become a member'\}/);
  });

  it('leaves the actual join call and post-join handling to the caller via onConfirm', () => {
    expect(src).not.toMatch(/method: 'POST'.*\/join/);
    expect(src).toMatch(/onConfirm\(isApproval && trimmed \? trimmed : undefined\)/);
  });
});

// The join sheet used to be wired twice, once in PatchProfile and once in
// PatchShell, and the two had drifted. PatchRelationship owns it now
// (docs/adr/042).
describe('PatchRelationship wires the join sheet for both surfaces', () => {
  const src = source('components/PatchRelationship.svelte');
  const profileSrc = source('pages/PatchProfile.svelte');
  const shellSrc = source('components/PatchShell.svelte');

  it('imports JoinSheet and mounts it', () => {
    expect(src).toContain("import JoinSheet from './JoinSheet.svelte'");
    expect(src).toMatch(/<JoinSheet[\s\S]*?onConfirm={handleJoin}/);
  });

  it('opens the sheet from the membership rung instead of calling the API directly', () => {
    expect(src).toMatch(/onclick={openJoinSheet}[\s\S]{0,120}Become a member/);
  });

  it('leaves the Follow button calling handleFollow directly — follows never see a sheet', () => {
    expect(src).toMatch(/onclick={handleFollow}[\s\S]{0,60}>Follow</);
  });

  it('handleJoin accepts an optional message and still performs the original API call and toasts', () => {
    expect(src).toMatch(/async function handleJoin\(message\)/);
    expect(src).toMatch(/body: message \? \{ message \} : undefined/);
    expect(src).toContain("'Membership request sent'");
  });

  it('is the only implementation — the profile and the shell mount it rather than repeat it', () => {
    expect(profileSrc).toContain('<PatchRelationship');
    expect(shellSrc).toContain('<PatchRelationship');
    expect(profileSrc).not.toContain('<JoinSheet');
    expect(shellSrc).not.toContain('<JoinSheet');
    for (const s of [profileSrc, shellSrc]) {
      expect(s).not.toMatch(/nodes\/\$\{slug\}\/leave/);
    }
  });
});

describe('PatchSettingsMembers shows the join message on pending requests', () => {
  const src = source('pages/PatchSettingsMembers.svelte');

  it('renders join_message as a quoted note, only when present', () => {
    expect(src).toMatch(/\{#if member\.join_message\}/);
    expect(src).toMatch(/class="join-message"/);
  });
});

describe('UnlockPanel', () => {
  const src = source('components/UnlockPanel.svelte');

  it('reads the patch context rather than requiring props', () => {
    expect(src).toContain("getContext('patch')");
  });

  it('gates on membership role exactly "member" (never admin or follower)', () => {
    expect(src).toMatch(/membership\?\.role === 'member'/);
  });

  it('gates on a 30-day join window', () => {
    expect(src).toMatch(/const THIRTY_DAYS_MS = 30 \* 24 \* 60 \* 60 \* 1000/);
    expect(src).toMatch(/ms <= THIRTY_DAYS_MS/);
  });

  it('waits on auth and memberships resolving before deciding visibility (no flash)', () => {
    expect(src).toMatch(/isAuthChecked\(\)/);
    expect(src).toMatch(/isMembershipsLoaded\(\)/);
  });

  it('dismiss persists per user and patch', () => {
    expect(src).toContain("import { isUnlockPanelDismissed, dismissUnlockPanel } from '../lib/onboarding.js'");
    expect(src).toMatch(/dismissUnlockPanel\(getUser\(\)\?\.id, node\?\.id\)/);
  });

  it('links to governance docs, proposals, and the member list', () => {
    expect(src).toContain('/governance/docs`');
    expect(src).toContain('/governance/proposals`');
    expect(src).toContain('/members`');
  });
});

describe('SetupChecklist', () => {
  const src = source('components/SetupChecklist.svelte');

  it('is admin-only and never shown for unclaimed patches', () => {
    expect(src).toMatch(/isAdmin && !isUnclaimed/);
  });

  it('derives tile, tags, and whereabouts from the node payload', () => {
    expect(src).toMatch(/const hasTile = node\.appearance != null/);
    expect(src).toMatch(/const hasTags = Array\.isArray\(node\.tags\) && node\.tags\.length > 0/);
    expect(src).toMatch(/hasMapLocation\(node\.latitude, node\.longitude\)/);
  });

  it('marks whereabouts and governance as skippable/optional in copy', () => {
    expect(src).toContain('not every patch is a place');
    expect(src).toContain('a band never needs this');
  });

  it('derives the first-event item from the events endpoint', () => {
    expect(src).toMatch(/api\(`events\?node_slug=\$\{encodeURIComponent\(slug\)\}&limit=1`\)/);
  });

  it('derives governance decided from a non-lining doc, OR the localStorage visited fallback', () => {
    expect(src).toMatch(/hasCharterBeyondLining \|\| isGovernanceHubVisited\(userId, node\.id\)/);
    expect(src).toMatch(/docs\.some\(\(d\) => d\.kind !== 'lining'\)/);
  });

  it('the share item copies the patch link and marks done only once clicked', () => {
    expect(src).toMatch(/navigator\.clipboard\.writeText\(url\)/);
    expect(src).toMatch(/markPatchLinkShared\(userId, node\?\.id\)/);
  });

  it('collapses entirely once every item is done', () => {
    expect(src).toMatch(/let allDone = \$derived\(checksLoaded && items\.length > 0 && items\.every\(\(i\) => i\.done\)\)/);
    expect(src).toMatch(/!allDone/);
  });

  it('dismiss persists per admin and patch, and never blocks anything (no modal/backdrop)', () => {
    expect(src).toMatch(/dismissSetupChecklist\(userId, node\?\.id\)/);
    expect(src).not.toContain('modal-backdrop');
  });
});

describe('GovernanceOverview marks the governance-visited fallback for the setup checklist', () => {
  const src = source('components/GovernanceOverview.svelte');

  it('marks visited for admins only', () => {
    expect(src).toContain("import { markGovernanceHubVisited } from '../lib/onboarding.js'");
    expect(src).toMatch(/if \(isAdmin && nodeId\) markGovernanceHubVisited\(getUser\(\)\?\.id, nodeId\)/);
  });
});

describe('GovernanceOverview hosts the unlock panel and setup checklist', () => {
  const src = source('components/GovernanceOverview.svelte');
  const shellSrc = source('components/PatchShell.svelte');

  it('imports and renders both inside the overview pane', () => {
    expect(src).toContain("import UnlockPanel from './UnlockPanel.svelte'");
    expect(src).toContain("import SetupChecklist from './SetupChecklist.svelte'");
    expect(src).toContain('<UnlockPanel');
    expect(src).toContain('<SetupChecklist');
  });

  it('PatchShell no longer mounts them shell-wide', () => {
    expect(shellSrc).not.toContain('<UnlockPanel');
    expect(shellSrc).not.toContain('<SetupChecklist');
  });
});
