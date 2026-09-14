/**
 * docs/adr/095 — a public patch is not a public roster.
 *
 * The server decides which rows leave; these assert the three things the
 * client is on the hook for. A tab that names what is behind it, a page that
 * can tell "no members yet" from "the list is not public", and a profile
 * glimpse that does neither of the two wrong things: announcing an empty
 * patch to a follower, or heading a list of admins with a count of members.
 *
 * Source-text assertions, per CLAUDE.md: there is no Svelte render library
 * here, so the pure helper is exercised directly and the components are read.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { workspaceTabs } from '../lib/patchWorkspace.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

const ids = (tabs) => tabs.map((t) => t.id);
const labelOf = (tabs, id) => tabs.find((t) => t.id === id)?.label;

describe('the Members tab follows what the patch publishes', () => {
  it('everyone: a signed-out visitor gets the tab, headed Members', () => {
    const tabs = workspaceTabs({ publicMemberList: 'everyone' });
    expect(ids(tabs)).toContain('members');
    expect(labelOf(tabs, 'members')).toBe('Members');
  });

  it('admins: the tab says Admins, because that is what is behind it', () => {
    const tabs = workspaceTabs({ publicMemberList: 'admins' });
    expect(labelOf(tabs, 'members')).toBe('Admins');
  });

  it('nobody: no tab at all for an outsider', () => {
    expect(ids(workspaceTabs({ publicMemberList: 'nobody' }))).not.toContain('members');
  });

  // A follower is an outsider for this setting (docs/adr/095 consequences).
  it('nobody: a follower is an outsider and loses the tab too', () => {
    const tabs = workspaceTabs({ membershipRole: 'follower', publicMemberList: 'nobody' });
    expect(ids(tabs)).not.toContain('members');
  });

  it('the room keeps the full tab at every setting', () => {
    for (const setting of ['everyone', 'admins', 'nobody']) {
      for (const role of ['member', 'admin']) {
        const tabs = workspaceTabs({ membershipRole: role, publicMemberList: setting });
        expect(ids(tabs), `${role} at ${setting}`).toContain('members');
        expect(labelOf(tabs, 'members'), `${role} at ${setting}`).toBe('Members');
      }
    }
  });

  it('an instance admin holding no role here still gets it', () => {
    const tabs = workspaceTabs({ isAdmin: true, publicMemberList: 'nobody' });
    expect(ids(tabs)).toContain('members');
  });

  // Two different questions that both touch this tab: the follower key hides
  // a tab over a read that stays public, this one is the read. Neither may
  // swallow the other.
  it('the follower permission and the roster setting compose', () => {
    const tabs = workspaceTabs({
      membershipRole: 'follower',
      followerPermissions: { members: false },
      publicMemberList: 'everyone',
    });
    expect(ids(tabs)).not.toContain('members');
  });

  it('an unchanged default keeps every existing patch on Members', () => {
    expect(labelOf(workspaceTabs({}), 'members')).toBe('Members');
  });
});

describe('the members page distinguishes empty from withheld', () => {
  const src = source('pages/PatchMembers.svelte');

  it('reads the setting off the listing that applied it', () => {
    expect(src).toContain("publicList = data.public_member_list || 'everyone'");
  });

  it('an instance admin and the room count as insiders', () => {
    expect(src).toMatch(/insider = \$derived\(isAdmin \|\| membershipRole === 'member' \|\| membershipRole === 'admin'\)/);
  });

  it('says the list is not published rather than that the patch is empty', () => {
    expect(src).toContain("This patch doesn't publish its member list.");
    // The withheld branch comes before the empty one, or a withheld list
    // renders as "No members yet" — the false sentence this exists to avoid.
    expect(src.indexOf('rosterWithheld')).toBeLessThan(src.indexOf('No members yet'));
  });

  it('still states the size when the list is withheld (decision 3)', () => {
    const withheld = src.slice(src.indexOf('{:else if rosterWithheld}'), src.indexOf('{:else if members.length === 0}'));
    expect(withheld).toContain('memberCount');
  });

  it('explains a short list rather than leaving it to look like a bug', () => {
    expect(src).toContain("Only this patch's admins are listed publicly.");
  });
});

describe('the profile glimpse', () => {
  const src = source('components/PatchProfileGlimpses.svelte');

  it('collapses the section when the list is withheld', () => {
    expect(src).toMatch(/showMembers = \$derived\(!isUnclaimed && !rosterWithheld/);
  });

  it('heads the section Admins when that is all it holds', () => {
    expect(src).toContain("{rosterAdminsOnly ? 'Admins' : 'Members'}");
  });

  it('drops the member total beside an Admins heading', () => {
    expect(src).toContain('{#if !rosterAdminsOnly && members.length > 0 && memberTotal > members.length}');
  });
});

describe('the setting is edited where the roster is managed', () => {
  const src = source('pages/PatchSettingsMembers.svelte');

  it('offers the three states', () => {
    for (const v of ['everyone', 'admins', 'nobody']) {
      expect(src).toContain(`value: '${v}'`);
    }
  });

  it('writes it through the patch endpoint', () => {
    expect(src).toContain("body: { public_member_list: v }");
  });

  // The three things the control cannot show on its face, each of which
  // changes whether an admin's choice does what they think it does. Matched
  // against collapsed whitespace, since the copy wraps in the markup.
  it('says who always sees the room, that the count stays public, and that this hides the list rather than the people', () => {
    const copy = src.replace(/\s+/g, ' ');
    expect(copy).toContain('Admins and members always see everyone');
    expect(copy).toContain('The member count stays public either way');
    expect(copy).toContain('this hides the list, not the people');
  });

  it('puts the setting back if the write fails, rather than lying about it', () => {
    expect(src).toContain('publicList = prev;');
  });
});
