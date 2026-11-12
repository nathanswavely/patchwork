/**
 * The membership, dashboard and quilt-surface findings decided 2026-10-20:
 * F-026, F-028, F-030, F-044, F-048, F-054, F-055, F-056.
 *
 * Source text only, like every frontend test here; each change was also
 * read in a browser against a copy of the simulation instance.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { patchSettingsSections } from '../lib/patchWorkspace.js';
import { invitationNote } from '../lib/invitations.js';

// Comments stripped, so a sentence quoted in one cannot pass for markup.
function shipped(rel) {
  return readFileSync(resolve(__dirname, '..', rel), 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^[ \t]*\/\/.*$/gm, '');
}

describe('F-026: the role chip holds what the role changes, the exit last', () => {
  const src = shipped('components/PatchRelationship.svelte');
  const menu = src.slice(src.indexOf('standing-menu" bind:this={menuEl}>'));
  const firstMenu = menu.slice(0, menu.indexOf('</div>\n      </div>'));

  it('lists Settings and Notifications, then Leave', () => {
    expect(firstMenu).toContain('{#each standingLinks as link (link.href)}');
    expect(firstMenu.indexOf('standingLinks')).toBeLessThan(firstMenu.indexOf('{standing.exit}'));
    expect(firstMenu).toContain('lt-menu-divider');
  });

  it("points an admin at the patch's settings and anyone else at their own", () => {
    expect(src).toContain("{ label: 'Settings', href: `/patches/${slug}/settings` }");
    expect(src).toContain("{ label: 'Notifications', href: `/patches/${slug}/settings/notifications` }");
    expect(src).toContain("{ label: 'Settings', href: '/settings/patches' }");
    expect(src).toContain("{ label: 'Notifications', href: '/settings/notifications' }");
  });
});

describe('F-028: My Patches says what is shared, not who can reach whom', () => {
  const src = shipped('pages/UserSettingsPatches.svelte');
  it('reads "Contact: not shared" and "Contact: N shared"', () => {
    expect(src).toContain('Contact: {m.contact_items_shared} shared');
    expect(src).toContain('Contact: not shared');
    expect(src).not.toContain('Not reachable');
    expect(src).not.toContain('Reachable by');
  });
});

describe('F-030: the role mark on a tile carries its name', () => {
  const src = shipped('components/QuiltCanvas.svelte');
  it('names admin, member and follow apart', () => {
    expect(src).toContain(`if (role === 'admin') return "You're an admin here";`);
    expect(src).toContain(`if (role === 'member') return "You're a member here";`);
    expect(src).toContain("if (role === 'follower') return 'You follow this patch';");
  });
  it('gives the mark an accessible name and the tile tip the same words', () => {
    expect(src).toContain(".attr('role', 'img').attr('aria-label', name)");
    expect(src).toContain("drawn.g.append('title').text(name)");
    expect(src).toMatch(/tip-role[\s\S]{0,300}roleMarkName\(role\)/);
  });
});

describe('F-056: one thing called Members, and the controls are called what they are', () => {
  it('names the settings section Roles and requests, keeping its path', () => {
    const members = patchSettingsSections().find((s) => s.id === 'members');
    expect(members.label).toBe('Roles and requests');
    expect(patchSettingsSections().some((s) => s.label === 'Members')).toBe(false);
  });

  it("links the Members tab to it, for the patch's admins", () => {
    const src = shipped('pages/PatchMembers.svelte');
    expect(src).toMatch(/\{#if isAdmin\}\s*<a class="manage-link" href="\/patches\/\{slug\}\/settings\/members"[\s\S]{0,200}\}\}>Roles and requests<\/a>/);
  });
});

describe('F-048 and F-044: an invitation shows where the people are, and says who asked', () => {
  const src = shipped('pages/PatchMembers.svelte');

  it('reads the invited list the server sends only to admins', () => {
    expect(src).toContain('if (!after) invited = data.invited || [];');
    expect(src).toContain('{#if invited.length > 0}');
    expect(src).toContain('{invitationNote(person)}');
  });

  it('never counts an invited person as a member', () => {
    expect(src).not.toMatch(/memberCount\s*=.*invited/);
  });

  it('shows the same line on Roles and requests', () => {
    expect(shipped('pages/PatchSettingsMembers.svelte')).toContain('{invitationNote(person)}');
  });

  it('names the inviter and the day', () => {
    expect(invitationNote({ invited_at: '2026-11-03', invited_by: { display_name: 'June Okafor', username: 'june' } }))
      .toBe('Invited by June Okafor, Nov 3, 2026');
    expect(invitationNote({ invited_by: { display_name: '', username: 'june' } })).toBe('Invited by june');
    expect(invitationNote({ invited_at: '2026-11-03', invited_by: { id: '', username: '', display_name: '' } }))
      .toBe('Invited Nov 3, 2026');
  });
});

describe('F-045: answering a request reconciles the bell at once', () => {
  const src = shipped('pages/PatchSettingsMembers.svelte');
  it('refreshes the unread count after approve and after reject', () => {
    expect(src).toContain("import { refreshUnread } from '../stores/notifications.svelte.js';");
    expect(src.match(/loadPendingMembers\(\), refreshUnread\(\)/g)).toHaveLength(2);
  });
});

describe('F-054 and F-055: the Dashboard splits what you owe from what is happening', () => {
  const src = shipped('pages/Dashboard.svelte');

  it('has Needs you, Waiting on an answer and Coming up, and no Attention Needed', () => {
    expect(src).toContain('>Needs you</h2>');
    expect(src).toContain('>Waiting on an answer</h2>');
    expect(src).toContain('>Coming up</h2>');
    expect(src).not.toContain('Attention Needed');
  });

  it('keeps events out of Needs you', () => {
    const needs = src.slice(src.indexOf('>Needs you</h2>'), src.indexOf('>Waiting on an answer</h2>'));
    expect(needs).not.toContain('upcomingEvents');
    expect(src).toContain('let needsYou = $derived(invitations.length > 0 || pendingLines.length > 0 || proposalLines.length > 0);');
  });

  it('names each patch by its name, with the label itself as the link', () => {
    expect(src).not.toMatch(/\{slug\} \(/);
    expect(src).toContain('join {line.count === 1 ? \'request\' : \'requests\'} to answer in <strong>{line.name}</strong>');
    expect(src).toContain('your vote in <strong>{line.name}</strong>');
    expect(src).toContain('name: nameBySlug.get(slug) || slug');
  });

  it("tells a requester their request is waiting, and since when", () => {
    expect(src).toContain("let ownRequests = $derived(memberships.filter((m) => m.status === 'pending'));");
    expect(src).toContain('Your request to join <strong>{m.node_name}</strong> is waiting on an admin.');
    expect(src).toContain('Sent {formatDay(m.joined_at)}');
  });

  it('lists the invitations waiting on this person among what they owe', () => {
    expect(src).toContain('{#each invitations as inv (inv.id)}');
    expect(src).toContain('invited you to join <strong>{inv.node_name}</strong>');
  });
});
