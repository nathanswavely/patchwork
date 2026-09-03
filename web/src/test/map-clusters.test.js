import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { clusterNodes, patchActivity } from '../lib/mapClusters.js';

// A fake Leaflet map: clustering only ever asks where a point lands in
// pixels, which is the whole reason the grouping is testable without a
// browser. Degrees in, pixels out, 1:1 — so a "44px radius" is 44 units here.
const fakeMap = {
  latLngToLayerPoint: ([lat, lng]) => ({
    x: lng,
    y: lat,
    distanceTo(other) {
      return Math.hypot(this.x - other.x, this.y - other.y);
    },
  }),
};

const patch = (name, lat, lng, counts = {}) => ({
  id: name,
  name,
  latitude: lat,
  longitude: lng,
  member_count: counts.members || 0,
  event_count: counts.events || 0,
  follower_count: counts.followers || 0,
});

describe('map clustering (docs/adr/078)', () => {
  it('groups markers closer than the radius and leaves the rest alone', () => {
    // Two within 10px of each other, one 500px away.
    const groups = clusterNodes(fakeMap, [
      patch('a', 0, 0),
      patch('b', 0, 10),
      patch('far', 0, 500),
    ], 44);

    expect(groups).toHaveLength(2);
    const sizes = groups.map((g) => g.members.length).sort();
    expect(sizes).toEqual([1, 2]);
  });

  it('anchors a cluster on its heaviest member, not on input order', () => {
    // Alphabetical order puts the small patch first — which is exactly the
    // order the API returns, and the reason the seeding is explicit. The
    // marker a reader can click should be the one the quilt would have drawn
    // largest.
    const groups = clusterNodes(fakeMap, [
      patch('aaa-small', 0, 0, { members: 1 }),
      patch('zzz-big', 0, 12, { members: 40 }),
    ], 44);

    expect(groups).toHaveLength(1);
    expect(groups[0].lead.name).toBe('zzz-big');
  });

  it('sits a cluster at its members mean and a lone marker on its own point', () => {
    const groups = clusterNodes(fakeMap, [
      patch('a', 10, 0),
      patch('b', 20, 0),
      patch('lone', 0, 900),
    ], 44);

    const cluster = groups.find((g) => g.members.length > 1);
    const lone = groups.find((g) => g.members.length === 1);
    expect(cluster.latlng).toEqual([15, 0]);
    expect(lone.latlng).toEqual([0, 900]);
  });

  it('never drops a patch: every one lands in exactly one group', () => {
    const nodes = Array.from({ length: 30 }, (_, i) => patch(`p${i}`, 0, i * 5));
    const groups = clusterNodes(fakeMap, nodes, 44);
    const seen = groups.flatMap((g) => g.members.map((m) => m.id));
    expect(seen).toHaveLength(30);
    expect(new Set(seen).size).toBe(30);
  });

  it('ignores patches with no map location', () => {
    const groups = clusterNodes(fakeMap, [
      patch('placed', 0, 0),
      { id: 'unplaced', name: 'unplaced', latitude: null, longitude: null },
    ], 44);
    expect(groups).toHaveLength(1);
    expect(groups[0].lead.id).toBe('placed');
  });

  it('counts followers at a discount, like the quilt sizes tiles', () => {
    // Mirrors quiltLayout's patchActivity — the map reads the quilt's own
    // measure of which patches matter rather than inventing a second one.
    expect(patchActivity({ member_count: 2, event_count: 3, follower_count: 9 }))
      .toBe(2 + 3 + 3);
  });
});

// The viewport is the person's after the first fit (docs/adr/078), which
// leaves one case that cannot explain itself: everything matching is
// elsewhere and the map has gone blank.
describe('the out-of-view affordance', () => {
  const src = readFileSync(
    resolve(__dirname, '../components/MapView.svelte'),
    'utf8',
  );

  it('announces only when nothing at all is in view', () => {
    // Not "some are off screen" — at a street zoom that is nearly always
    // true and the notice would be permanent furniture.
    expect(src).toMatch(/offscreen\s*=\s*ids\.length === 0 \? placed : 0/);
  });

  it('recounts on pan without rebuilding markers', () => {
    // Panning cannot change the grouping, so it must not pay for one.
    expect(src).toMatch(/instance\.on\('moveend', reportInView\)/);
    // updateMarkers is the expensive path; moveend must not reach it.
    expect(src).not.toMatch(/on\('moveend'[^)]*updateMarkers/);
  });

  it('shares one visibility pass with the in-view lens', () => {
    // Both ask the same question — which markers are on screen — and this
    // branch and the in-view lens (docs/adr/074) each arrived with their own
    // answer to it. Two passes with two ideas of where the edge is would
    // disagree the first time one of them changed: the list would narrow to
    // patches the map claims are elsewhere. One pass, two consumers.
    expect(src.match(/latLngToContainerPoint/g) || []).toHaveLength(1);
    expect(src).not.toMatch(/countOffscreen/);
  });

  it('restores the same framing the map opened with', () => {
    // One function behind both, so the view a person is given and the view
    // they can ask back cannot drift apart.
    expect(src).toMatch(/function showAll\(\)/);
    expect(src).toMatch(/hasFit = true;[\s\S]{0,400}?showAll\(\)/);
  });

  it('stands down when the pane is already saying it', () => {
    // The in-view lens landed on main with its own empty state — "No patches
    // in view — 33 elsewhere on the quilt", with its own way back. Two
    // notices and two buttons for one condition is worse than either alone,
    // so the map speaks only when the pane does not.
    const home = readFileSync(
      resolve(__dirname, '../pages/SocialHome.svelte'),
      'utf8',
    );
    expect(src).toMatch(/announceOffscreen = true/);
    expect(src).toMatch(/offscreen > 0 && announceOffscreen/);
    expect(home).toMatch(/announceOffscreen=\{!inViewActive\}/);
  });
});
