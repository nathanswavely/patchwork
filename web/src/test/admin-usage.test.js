/**
 * The admin Usage tab
 * (docs/adr/2026-09-18-counting-visitors-without-watching-anyone.md).
 *
 * The page's job is two things at once: show the daily totals, and say on
 * its face what the counter refuses to keep. Most of what is pinned here is
 * the second, plus that the switch and the numbers share one page and one
 * setting the privacy policy also reads.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('Admin usage page', () => {
  const src = source('pages/AdminUsage.svelte');

  it('reads the window it was asked for from the admin usage endpoint', () => {
    expect(src).toContain("api(`admin/usage?days=${n}`)");
  });

  it('turns counting on and off through the same setting the privacy policy reads', () => {
    expect(src).toContain("api('admin/settings', { method: 'PATCH', body: { usage_stats: value } })");
    expect(src).toContain('The privacy policy now says so.');
  });

  it('says what is never kept', () => {
    expect(src).toContain('no cookie, and nothing kept that names a person, an address, or a moment');
    expect(src).toContain('Which patch or person a load was for is never kept.');
  });

  it('does not add distinct daily visitors across days', () => {
    expect(src).toContain('Visitors are distinct within one day only, so they are not added across days.');
    expect(src).toContain('peak_visitors');
  });

  it('clears counts only after a confirmation, and says it is audited', () => {
    expect(src).toContain("api('admin/usage', { method: 'DELETE' })");
    expect(src).toContain("confirm('Delete every daily count this quilt has kept? This cannot be undone.')");
    expect(src).toContain('The deletion is recorded in the audit log.');
  });

  it('draws the chart without a charting library', () => {
    expect(src).not.toContain('d3');
    expect(src).toContain('class="bar"');
  });
});

describe('Admin usage wiring', () => {
  it('is routed, gated, and in the admin tab row', () => {
    const app = source('App.svelte');
    expect(app).toContain("addRoute('/admin/usage', 'adminUsage')");
    expect(app).toContain("import AdminUsage from './pages/AdminUsage.svelte'");
    expect(app).toContain("{:else if routeName === 'adminUsage'}");

    const shell = source('components/AdminShell.svelte');
    expect(shell).toContain("{ label: 'Usage', href: '/admin/usage', icon: ChartBar }");
  });

  it('is findable from the admin finder', () => {
    const finder = source('lib/finderProviders.js');
    expect(finder).toContain("href: '/admin/usage'");
  });
});
