/**
 * The admin panel's Overview (CONTEXT.md): what waits on the instance admin
 * and what is unattended, and nothing else.
 *
 * The rule these tests hold is the one the page drifted from before: no
 * size, growth or activity figure anywhere on it. The rest is that every
 * line leads somewhere the act can be done, and that a quilt with nothing
 * waiting says so rather than rendering nothing.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('Admin Overview', () => {
  const src = source('pages/AdminDashboard.svelte');

  it('reads the overview endpoint, not the retired stats one', () => {
    expect(src).toContain("api('admin/overview')");
    expect(src).not.toContain("api('admin/stats')");
  });

  it('is headed Overview, as the tab and the glossary name it', () => {
    expect(src).toContain('<h1>Overview</h1>');
    expect(src).not.toContain('Admin Dashboard');
  });

  it('carries no size, growth or activity figures', () => {
    for (const stat of ['total_users', 'active_users_30d', 'total_nodes', 'total_events',
      'recent_signups_7d', 'open_proposals', 'passed_proposals', 'rejected_proposals']) {
      expect(src, stat).not.toContain(stat);
    }
    expect(src).not.toMatch(/Total Users|Signups|Quick Links/);
  });

  it('says so when nothing is waiting, instead of rendering nothing', () => {
    expect(src).toContain('Nothing is waiting on you.');
  });

  it('lists a queue only when something is in it, with the age of the oldest', () => {
    expect(src).toContain('.filter((q) => q.count > 0');
    expect(src).toContain('oldest {formatRelative(q.oldest_at)}');
  });

  it('sends each inbox line to the Review section where the decision is made (docs/adr/118)', () => {
    expect(src).toContain("reports: { one: 'report', many: 'reports', href: '/admin/review/reports' }");
    expect(src).toContain("href: '/admin/review/submissions'");
    expect(src).toContain("href: '/admin/review/event-submissions'");
    expect(src).toContain("href: '/admin/review/claims'");
    expect(src).toContain("href: '/admin/review/tags'");
  });

  it('keeps unrouted names below the inbox as work nobody is waiting on', () => {
    expect(src).toContain('Nobody is waiting on these.');
    expect(src).toContain("handleNav(e, '/admin/settings/aggregators')");
  });

  it('leads an adminless patch to its members settings, where promotion is permitted under custody', () => {
    expect(src).toContain('{p.name} has no admin.');
    expect(src).toContain('`/patches/${p.slug}/settings/members`');
    expect(src).toContain('No members to hand it to.');
  });

  it('surfaces a failing feed only on unclaimed patches, and sends it to that patch’s sources', () => {
    expect(src).toContain('care.failing_unclaimed_sources');
    expect(src).toContain('`/patches/${s.node_slug}/settings/sources`');
  });

  it('states mail off and a missing passkey plainly, with nowhere to dismiss them', () => {
    expect(src).toContain('Mail is off.');
    expect(src).toContain('You have no passkey.');
    expect(src).toContain("handleNav(e, '/settings/security')");
    // No control on the page at all: nothing here is dismissed or acted on
    // in place, every act happens on the tab a line leads to.
    expect(src).not.toContain('<button');
  });
});
