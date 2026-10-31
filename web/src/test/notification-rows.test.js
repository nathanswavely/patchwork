/**
 * How a notification row names its sender, says when it arrived, and where
 * the bell opens it. The sender line replaced a per-type icon: the title
 * already says the type, and not every notification comes from a patch, so
 * the four senders (a patch, another quilt, an admin queue, this quilt about
 * you) each read differently.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describeNotification, notificationTime } from '../lib/notificationRow.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('the sender line', () => {
  it('names the patch a notification is about', () => {
    const n = { type: 'proposal.new', title: 'New proposal: Dues', body: 'Raise them', node_name: 'Brayer Press' };
    expect(describeNotification(n)).toEqual({ context: 'Brayer Press', title: 'New proposal: Dues', body: 'Raise them' });
  });

  it('shows no line for a patch notification whose patch is unknown', () => {
    expect(describeNotification({ type: 'event.suggested', title: 't' }).context).toBe('');
  });

  it('names a followed patch on another quilt and its host, and does not repeat the name as the body', () => {
    const n = {
      type: 'remote.event',
      title: 'New event: Open studio night',
      body: 'Gallery Row',
      link: '/quilts/arts.lancaster.example/patches/gallery-row?event=abc',
    };
    expect(describeNotification(n)).toEqual({
      context: 'Gallery Row on arts.lancaster.example',
      title: 'New event: Open studio night',
      body: '',
    });
  });

  it('labels an instance queue Admin, even when the row carries a patch', () => {
    for (const type of ['admin.submission', 'admin.claim_request', 'governance.succession_needed']) {
      expect(describeNotification({ type, title: 't', node_name: 'Brayer Press' }).context).toBe('Admin');
    }
  });

  it('gives what this quilt says about you no sender line', () => {
    for (const type of ['account.warned', 'account.email_changed', 'report.resolved', 'trust_request.approved', 'submission.declined', 'quilt.bulletin']) {
      expect(describeNotification({ type, title: 't', body: 'b', node_name: 'X' })).toEqual({ context: '', title: 't', body: 'b' });
    }
  });
});

describe('the time line', () => {
  const now = new Date('2026-10-29T15:00:00Z').getTime();
  const ago = (ms) => new Date(now - ms).toISOString();
  const MIN = 60000;
  const HOUR = 60 * MIN;
  const DAY = 24 * HOUR;

  it('says how long ago in words, not abbreviations', () => {
    expect(notificationTime(ago(10 * 1000), now)).toBe('Just now');
    expect(notificationTime(ago(MIN), now)).toBe('1 minute ago');
    expect(notificationTime(ago(20 * MIN), now)).toBe('20 minutes ago');
    expect(notificationTime(ago(HOUR), now)).toBe('1 hour ago');
    expect(notificationTime(ago(5 * HOUR), now)).toBe('5 hours ago');
    expect(notificationTime(ago(DAY + HOUR), now)).toBe('Yesterday');
    expect(notificationTime(ago(4 * DAY), now)).toBe('4 days ago');
  });

  it('gives a date after a week, with the year only when it is not this one', () => {
    expect(notificationTime(ago(17 * DAY), now)).toBe('Oct 12');
    expect(notificationTime('2025-12-01T12:00:00Z', now)).toBe('Dec 1, 2025');
  });

  it('says nothing for a missing or unreadable time', () => {
    expect(notificationTime('', now)).toBe('');
    expect(notificationTime('not a date', now)).toBe('');
  });
});

describe('the bell opens a dropdown on a desktop and the page on a phone', () => {
  const bar = source('components/GlobalBar.svelte');
  const dropdown = source('components/NotificationDropdown.svelte');
  const shell = source('components/SocialShell.svelte');

  it('the bar no longer opens a side panel', () => {
    expect(bar).not.toContain('SidePanel');
    expect(bar).toContain('<NotificationDropdown');
  });

  it('a handheld bell navigates rather than opening anything', () => {
    expect(bar).toMatch(/if \(isHandheld\(\)\) \{\s*navigate\('\/notifications'\);\s*return;/);
  });

  it('the bottom shelf still links straight to the page', () => {
    expect(shell).toMatch(/href="\/notifications"\s*class="rail-item rail-notif"/);
  });

  it('closes on an outside click, on Escape, and on navigation', () => {
    expect(bar).toMatch(/notifMenuOpen && !e\.target\.closest\('\.notif-menu-container'\)/);
    expect(bar).toMatch(/e\.key === 'Escape' && notifMenuOpen/);
    expect(bar).toMatch(/void path;[\s\S]{0,120}notifMenuOpen = false/);
  });

  it('leads to the full page and to notification settings', () => {
    expect(dropdown).toContain('href="/notifications"');
    expect(dropdown).toContain('href="/settings/notifications"');
  });

  it('shows the latest few, not a page of them', () => {
    expect(dropdown).toContain("api('notifications?limit=10')");
  });
});

describe('the notifications page', () => {
  const page = source('pages/Notifications.svelte');

  it('filters by all or unread, and has no type chips', () => {
    expect(page).not.toContain('categoryFilter');
    expect(page).not.toContain("label: 'Proposals'");
    expect(page).toMatch(/>All<\/button>/);
    expect(page).toMatch(/>Unread<\/button>/);
  });

  it('links to notification settings', () => {
    expect(page).toContain('href="/settings/notifications"');
  });

  it('pages with the cursor', () => {
    expect(page).toContain('&after=${nextCursor}');
  });
});
