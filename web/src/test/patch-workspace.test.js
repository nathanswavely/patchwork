/**
 * Issue #6 — unclaimed patches must have a usable admin workspace.
 *
 * Before the fix, PatchShell returned an empty tab list for unclaimed patches
 * (zero navigation), and Patch Settings hardcoded Members/Notifications, which
 * are meaningless for a patch with no membership. The tab/section subset now
 * lives in pure helpers so it can be asserted directly.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { workspaceTabs, patchSettingsSections } from '../lib/patchWorkspace.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

const ids = (tabs) => tabs.map((t) => t.id);

describe('#6: workspace tabs for unclaimed patches', () => {
  it('an instance admin gets Events and Settings — never zero tabs', () => {
    const tabs = workspaceTabs({ isUnclaimed: true, isAdmin: true });
    expect(ids(tabs)).toEqual(['events', 'settings']);
  });

  it('a non-admin viewer gets the Events tab but not Settings', () => {
    const tabs = workspaceTabs({ isUnclaimed: true, isAdmin: false, membershipRole: 'follower' });
    expect(ids(tabs)).toEqual(['events']);
  });

  it('never exposes Governance or Members on an unclaimed patch', () => {
    const tabs = workspaceTabs({ isUnclaimed: true, isAdmin: true });
    expect(ids(tabs)).not.toContain('governance');
    expect(ids(tabs)).not.toContain('members');
  });
});

describe('#6: workspace tabs for claimed patches are unchanged', () => {
  it('an admin gets the full row', () => {
    const tabs = workspaceTabs({ isUnclaimed: false, isAdmin: true, membershipRole: 'admin' });
    expect(ids(tabs)).toEqual(['governance', 'members', 'events', 'settings']);
  });

  it('a follower gets governance plus its permitted tabs, no settings', () => {
    const tabs = workspaceTabs({
      isUnclaimed: false,
      isAdmin: false,
      membershipRole: 'follower',
      followerPermissions: { members: false, events: true },
    });
    expect(ids(tabs)).toEqual(['governance', 'events']);
  });

  it('a plain member gets governance, members, events', () => {
    const tabs = workspaceTabs({ isUnclaimed: false, isAdmin: false, membershipRole: 'member' });
    expect(ids(tabs)).toEqual(['governance', 'members', 'events']);
  });
});

describe('#6: settings sections filter on claim state', () => {
  it('unclaimed patches show Info, Appearance, Sources, Verification, Danger', () => {
    const secs = patchSettingsSections({ isUnclaimed: true });
    // Event sources appear here too: the instance admin holds unclaimed
    // calendars in trust and may attach feeds (docs/adr/031).
    expect(secs.map((s) => s.id)).toEqual(['info', 'appearance', 'sources', 'verification', 'danger']);
  });

  it('unclaimed patches drop Members and Notifications', () => {
    const secs = patchSettingsSections({ isUnclaimed: true }).map((s) => s.id);
    expect(secs).not.toContain('members');
    expect(secs).not.toContain('notifications');
  });

  it('claimed patches keep the full section list without Verification', () => {
    const secs = patchSettingsSections({ isUnclaimed: false }).map((s) => s.id);
    expect(secs).toEqual(['info', 'appearance', 'members', 'sources', 'notifications', 'danger']);
    expect(secs).not.toContain('verification');
  });
});

describe('#6: shells are wired to the helpers', () => {
  it('PatchShell derives tabs from workspaceTabs, not an empty unclaimed list', () => {
    const src = source('components/PatchShell.svelte');
    expect(src).toContain('workspaceTabs(');
    // The old empty-tab shortcut must be gone.
    expect(src).not.toContain('if (isUnclaimed) return [];');
  });

  it('PatchSettings derives sections from patchSettingsSections', () => {
    const src = source('pages/PatchSettings.svelte');
    expect(src).toContain('patchSettingsSections(');
  });

  it('the unclaimed profile Manage entry lands on events, not governance', () => {
    const src = source('pages/PatchProfile.svelte');
    expect(src).toMatch(/isUnclaimed[\s\S]*?\/patches\/\{slug\}\/events/);
  });
});
