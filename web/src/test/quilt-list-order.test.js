import { describe, it, expect, beforeEach } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { quiltOrder } from '../lib/quiltLayout.js';
import {
  getListOrder,
  setListOrder,
  getInViewOnly,
  setInViewOnly,
  toggleInViewOnly,
  setSelectedTags,
  setSearchQuery,
  resetFilters,
} from '../stores/quilt.svelte.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

function makePatches(n, activityFor = () => 0) {
  return Array.from({ length: n }, (_, i) => ({
    id: `patch-${String(i).padStart(3, '0')}`,
    name: `Patch ${i}`,
    member_count: activityFor(i),
    event_count: 0,
  }));
}

// Quilt order (docs/adr/074): the list reads the quilt, so the order is the
// layout engine's own placement pass — not a ranking computed alongside it.
describe('quilt order', () => {
  it('returns every patch exactly once and no filler squares', () => {
    // Filler squares are placed into the same array patches are; they are not
    // patches and must never reach the list.
    const patches = makePatches(30, i => (i === 0 ? 24 : 0));
    const order = quiltOrder(patches, []);
    expect(order).toHaveLength(patches.length);
    expect(new Set(order).size).toBe(patches.length);
    expect(order.some(id => id.startsWith('filler'))).toBe(false);
  });

  it('starts at the centre — the largest tile is placed first', () => {
    const order = quiltOrder(makePatches(30, i => (i === 0 ? 24 : 0)), []);
    expect(order[0]).toBe('patch-000');
  });

  it('walks outward by affinity, not by name', () => {
    // patch-029 sorts last alphabetically and shares members with the centre;
    // it must still come second, because that is where its tile is sewn.
    const patches = makePatches(30, i => (i === 0 ? 24 : 0));
    const links = [{ source: 'patch-000', target: 'patch-029', strength: 9 }];
    const order = quiltOrder(patches, links);
    expect(order[1]).toBe('patch-029');
  });

  it('is empty for an empty quilt rather than throwing', () => {
    expect(quiltOrder([], [])).toEqual([]);
    expect(quiltOrder(null, null)).toEqual([]);
  });
});

// The list's own state (docs/adr/074). Both are session-ephemeral and neither
// belongs to the filter.
describe('list order and the in-view lens', () => {
  beforeEach(() => {
    resetFilters();
    setListOrder('quilt');
    setInViewOnly(false);
  });

  it('defaults to quilt order, not alphabetical', () => {
    expect(getListOrder()).toBe('quilt');
  });

  it('leaves the lens off until someone turns it on', () => {
    expect(getInViewOnly()).toBe(false);
    toggleInViewOnly();
    expect(getInViewOnly()).toBe(true);
  });

  it('keeps the lenses independent — clearing the filter touches neither', () => {
    // docs/adr/022: touching one lens never changes another.
    setListOrder('alpha');
    setInViewOnly(true);
    setSelectedTags(['music']);
    setSearchQuery('zine');

    resetFilters();

    expect(getListOrder()).toBe('alpha');
    expect(getInViewOnly()).toBe(true);
  });
});

// A control belongs to the surface it changes (docs/adr/074).
describe('discovery control placement', () => {
  it('keeps the Quilt/Map switch out of the list header', () => {
    const src = source('pages/SocialHome.svelte');
    expect(src).not.toContain('view-toggle');
    expect(src).not.toContain('view-option');
  });

  it('puts the Quilt/Map switch on the canvas instead', () => {
    const src = source('components/SocialShell.svelte');
    expect(src).toContain('canvas-view');
    expect(src).toContain("scopedPath('map', quiltScope)");
  });

  it('offers Recently added, reading when a community arrived', () => {
    // docs/adr/074's third order, unblocked by nodes.activated_at. It must
    // read the arrival date, never created_at — on a directory-seeded quilt
    // those differ by the whole backlog (docs/adr/076).
    const src = source('pages/SocialHome.svelte');
    expect(src).toContain('Recently added');
    expect(src).toContain('activated_at');
    expect(src).not.toContain('b.created_at');
  });

  it('falls back to when a listing appeared, where there is no arrival', () => {
    // Ordering by arrival alone put 47 of the reference instance's 52
    // patches in an undated tail: an unclaimed listing has no arrival by
    // definition. The order asks the looser question — how new is this to
    // the quilt — which every patch can answer.
    const src = source('pages/SocialHome.svelte');
    const block = src.slice(src.indexOf("getListOrder() === 'recent'"));
    expect(block.slice(0, 400)).toContain('p.activated_at || p.created_at');
  });

  it('keeps the arrival strict where it is announced, not merely ordered', () => {
    // The bulletin must never announce a directory import as arrivals
    // (docs/adr/076), so the fallback lives in the sort and nowhere else.
    const bulletin = readFileSync(
      resolve(process.cwd(), '..', 'internal/notifications/bulletin.go'), 'utf8');
    expect(bulletin).toContain('activated_at IS NOT NULL');
    expect(bulletin).not.toContain('created_at');
  });

  it('gives the list its own order and lens controls', () => {
    const src = source('pages/SocialHome.svelte');
    expect(src).toContain('list-controls');
    expect(src).toContain('toggleInViewOnly');
    expect(src).toContain('Quilt order');
  });

  it('states the lens in the count rather than narrowing silently', () => {
    const src = source('pages/SocialHome.svelte');
    expect(src).toContain('in view');
  });

  it('names the lens in the empty state and offers one step back', () => {
    // docs/adr/022: composed narrowing must explain itself where it produces
    // nothing.
    const src = source('pages/SocialHome.svelte');
    expect(src).toContain('No patches in view');
    expect(src).toContain('elsewhere on the quilt');
    // The surface switch must invalidate the previous canvas's report.
    expect(src).toContain('inViewIds = null');
  });

  it('withholds the lens where only one pane is on screen', () => {
    // Below the breakpoint the panes toggle and the cards header is hidden,
    // so a lens set there would narrow a list from a viewport nobody can see.
    const src = source('pages/SocialHome.svelte');
    expect(src).toContain('lensAvailable');
    expect(src).toContain('winW > 768');
  });
});
