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
    expect(ids(tabs)).toEqual(['governance', 'members', 'events', 'noticeboard', 'settings']);
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

  // docs/adr/2026-09-19-an-event-says-who-it-is-for-within-what-the-patch-allows.md:
  // follower_permissions.events is now an enforced read ceiling in Go, not a
  // tab-hiding switch, so it never removes the Events tab any more — a
  // follower always gets it, and the list it shows is whatever the server
  // already answers for them (that patch's public events, when the switch
  // is off).
  it('a follower keeps the Events tab even with follower_permissions.events off', () => {
    const tabs = workspaceTabs({
      isUnclaimed: false,
      isAdmin: false,
      membershipRole: 'follower',
      followerPermissions: { events: false },
    });
    expect(ids(tabs)).toContain('events');
  });

  it('a plain member gets governance, members, events', () => {
    const tabs = workspaceTabs({ isUnclaimed: false, isAdmin: false, membershipRole: 'member' });
    expect(ids(tabs)).toEqual(['governance', 'members', 'events', 'noticeboard']);
  });

  // The noticeboard is the room's (docs/adr/081): an instance admin with no
  // role here has isAdmin set by the node payload and still gets no tab.
  it('an instance admin with no role gets settings but not the noticeboard', () => {
    const tabs = workspaceTabs({ isUnclaimed: false, isAdmin: true, membershipRole: '' });
    expect(ids(tabs)).toEqual(['governance', 'members', 'events', 'settings']);
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
    expect(secs).toEqual(['info', 'appearance', 'members', 'sources', 'noticeboard', 'notifications', 'danger']);
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
    const src = source('components/PatchProfileGlimpses.svelte');
    expect(src).toMatch(/isUnclaimed[\s\S]*?\/patches\/\{slug\}\/events/);
  });
});

describe('Patch Settings drills down on a narrow screen', () => {
  const page = source('pages/PatchSettings.svelte');

  it('opts into the drill-down with its bare path as the list', () => {
    expect(page).toContain('<SettingsShell title="Patch Settings" {sections} {indexHref}>');
    expect(page).toContain('indexHref = $derived(sourcesOnly ? null : `/patches/${slug}/settings`)');
  });

  it('leaves the bare path alone on a narrow screen and redirects it on a wide one', () => {
    expect(page).toContain('if (indexHref && isNarrow()) return;');
  });
});

describe('the workspace tabs are a bottom bar on a narrow screen', () => {
  const shell = source('components/PatchShell.svelte');

  it('never has more than five tabs to fit, for any viewer', () => {
    const most = workspaceTabs({ isAdmin: true, membershipRole: 'admin' });
    expect(ids(most)).toEqual(['governance', 'members', 'events', 'noticeboard', 'settings']);
  });

  it('pins the tabs to the foot at the quilt rail\'s breakpoint and height', () => {
    expect(shell).toMatch(/@media \(max-width: 768px\) \{[\s\S]*\.workspace-tabs \{\s*position: fixed;[\s\S]*height: var\(--pw-nav-h\);/);
  });

  it('draws no bar for a single tab, and clears the bar only when there is one', () => {
    expect(shell).toContain('class:single={tabs.length < 2}');
    expect(shell).toContain('class:over-tab-bar={tabs.length >= 2}');
  });

  it('lifts the sticky vote bar above whichever tab bar is at the foot', () => {
    expect(source('components/StickyVoteBar.svelte')).toContain('bottom: var(--pw-nav-h);');
  });
});

describe('the way up from a workspace page with no section list on screen', () => {
  it('goes to the section list a detail page belongs to', async () => {
    const { workspaceUpLink } = await import('../lib/patchWorkspace.js');
    expect(workspaceUpLink('governanceProposal', 'p')).toEqual({ href: '/patches/p/governance/proposals', label: 'Proposals' });
    expect(workspaceUpLink('governanceProposalNew', 'p').href).toBe('/patches/p/governance/proposals');
    expect(workspaceUpLink('governanceDocDetail', 'p')).toEqual({ href: '/patches/p/governance/docs', label: 'Documents' });
    expect(workspaceUpLink('governanceRulesPropose', 'p').href).toBe('/patches/p/governance/docs');
    expect(workspaceUpLink('patchNotice', 'p')).toEqual({ href: '/patches/p/noticeboard', label: 'Noticeboard' });
  });

  it('is nothing on a section page, whose own list is the way around', async () => {
    const { workspaceUpLink } = await import('../lib/patchWorkspace.js');
    for (const r of ['governanceHub', 'governanceProposals', 'governanceDocs', 'governanceRecord', 'patchMembers', 'patchEvents', 'patchNoticeboard', 'patchSettingsInfo']) {
      expect(workspaceUpLink(r, 'p')).toBeNull();
    }
  });

  it("opens the phone's patch bar, preferring a shell's own link, then a linked breadcrumb", () => {
    const shell = source('components/PatchShell.svelte');
    expect(shell).toContain("setContext('workspaceUp'");
    expect(shell.indexOf('if (registeredUp) return registeredUp;')).toBeLessThan(shell.indexOf('breadcrumbExtra.filter((seg) => seg.href)'));
    expect(shell).toMatch(/<PatchBarMenu[\s\S]*\{upLink\}/);
    expect(source('components/PatchBarMenu.svelte')).toContain("let back = $derived(upLink || { href: '/', label: getInstanceName() });");
    expect(source('App.svelte')).toContain('up={workspaceUpLink(routeName, routeParams.slug)}');
  });

  it('Patch Settings hands its back link to that row instead of drawing it in the page', () => {
    const settings = source('components/SettingsShell.svelte');
    expect(settings).toContain("getContext('workspaceUp')");
    expect(settings).toContain('{#if indexHref && !workspaceUp}');
  });
});

describe('on a phone the patch name is the menu', () => {
  const shell = source('components/PatchShell.svelte');
  const bar = source('components/PatchBarMenu.svelte');
  const rel = source('components/PatchRelationship.svelte');

  it('puts the patch bar in the global bar and drops the crumb and cluster row below 768px', () => {
    expect(shell).toContain('<GlobalBar patchContext>');
    expect(shell).toMatch(/\.crumb-group,\s*\.workspace-cluster \{\s*display: none;/);
    expect(source('components/GlobalBar.svelte')).toMatch(/\.top-bar\.patch-context \.new-menu-container \{\s*display: none;/);
  });

  it("keeps a visitor's next rung in the sheet, not in the bar or a strip", () => {
    expect(bar.slice(0, bar.indexOf('{#if sheetOpen}'))).not.toContain('<PatchRelationship');
    expect(shell).not.toContain('visitor-rung');
    expect(rel).not.toContain("mode === 'primary'");
  });

  it('puts standing, the public profile, posting, subscribing and reporting in the sheet', () => {
    const sheet = bar.slice(bar.indexOf('<div class="patch-sheet"'));
    expect(sheet).toMatch(/<PatchRelationship[\s\S]*size="sm"/);
    expect(sheet).toContain('View the public profile');
    expect(sheet).toContain("{postingRight === 'direct' ? 'New event' : 'Suggest an event'}");
    expect(sheet).toContain('Subscribe');
    expect(sheet).toContain('Report');
  });
});
