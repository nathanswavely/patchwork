/**
 * The noticeboard (docs/adr/081): members-only, replies per notice, a
 * closed moderation kit, quiet by default. These pin the three surfaces to
 * the decisions — the parts a render test would catch are checked in a
 * browser, since no render library exists here.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { workspaceTabs } from '../lib/patchWorkspace.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('The tab is the room\'s', () => {
  it('a follower never gets the noticeboard, whatever the follower permissions say', () => {
    const tabs = workspaceTabs({
      membershipRole: 'follower',
      followerPermissions: { events: true, members: true, proposals: true, charters: true, noticeboard: true },
    });
    expect(tabs.map((t) => t.id)).not.toContain('noticeboard');
  });

  it('a member and an admin both do', () => {
    for (const role of ['member', 'admin']) {
      expect(workspaceTabs({ membershipRole: role, isAdmin: role === 'admin' }).map((t) => t.id)).toContain('noticeboard');
    }
  });
});

describe('The board: born quiet, composer says so', () => {
  const src = source('pages/PatchNoticeboard.svelte');

  it('Tell members is a checkbox that starts off', () => {
    expect(src).toContain('let tellMembers = $state(false);');
    expect(src).toContain('<strong>Tell members</strong>');
    expect(src).toContain('tell_members: tellMembers');
  });

  it('the replies switch starts from the patch default', () => {
    expect(src).toContain('if (composing) repliesOpen = repliesDefault;');
    expect(src).toContain('replies_open: repliesOpen');
  });

  it('says who reads it, in one sentence', () => {
    expect(src).toContain("Read by this patch's admins and members, and nobody else.");
  });

  it('wears the members-told mark, and no unread count anywhere', () => {
    expect(src).toContain('members told');
    expect(src).not.toMatch(/unread/i);
  });
});

describe('The notice: flat replies, the kit and nothing more', () => {
  const src = source('pages/NoticeDetail.svelte');

  it('has no reply-to-a-reply and no reactions', () => {
    expect(src).not.toMatch(/parent_id/);
    expect(src).not.toMatch(/\/reactions|reaction-bar|allowedEmoji/);
    expect(src).not.toMatch(/@mention|@patch/);
  });

  it('shows the reply box only while replies are open, and keeps the replies when they close', () => {
    expect(src).toContain('{#if notice.replies_open}');
    expect(src).toContain('Replies are off on this notice.');
  });

  it('offers the switch and take-down to the author or an admin, editing to the author alone', () => {
    expect(src).toContain('{#if mayEdit}');
    expect(src).toContain('{#if mayManage}');
    expect(src).toMatch(/Switch replies off/);
  });

  it('reports a notice or a reply, never your own', () => {
    expect(src).toContain('<ReportButton entityType="notice"');
    expect(src).toContain('<ReportButton entityType="reply"');
    expect(src).toContain('notice.author_id !== me.id');
    expect(src).toContain('reply.author_id !== me.id');
  });
});

describe('The settings: two switches and the queue, three actions', () => {
  const src = source('pages/PatchSettingsNoticeboard.svelte');

  it('saves the two patch settings on the node', () => {
    expect(src).toContain('save({ notice_posting: v }');
    expect(src).toContain('save({ notice_replies_default: v }');
  });

  it('offers exactly dismiss, remove, close_replies', () => {
    const actions = [...src.matchAll(/<option value="([a-z_]+)"/g)].map((m) => m[1]);
    expect(actions.sort()).toEqual(['close_replies', 'dismiss', 'remove']);
    expect(src).not.toMatch(/suspend_user|remove_content|reset_appearance|"warn"/);
  });

  it('reads the patch queue, not the instance panel', () => {
    expect(src).toMatch(/api\(`nodes\/\$\{slug\}\/reports\?status=/);
    expect(src).not.toContain('admin/reports');
  });
});

describe('ReportButton knows the two new nouns', () => {
  it('names a notice and a reply', () => {
    const src = source('components/ReportButton.svelte');
    expect(src).toContain("notice: 'notice'");
    expect(src).toContain("reply: 'reply'");
  });
});
