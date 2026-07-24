import { describe, it, expect } from 'vitest';
import { quiltLayout } from '../lib/quiltLayout.js';

/** Build n patches; activityFor(i) sets member_count for patch i. */
function makePatches(n, activityFor = () => 0) {
  return Array.from({ length: n }, (_, i) => ({
    id: `patch-${String(i).padStart(3, '0')}`,
    name: `Patch ${i}`,
    member_count: activityFor(i),
    event_count: 0,
  }));
}

/** Ideal sizes keyed by patch id (idealSize, not currentSize — placement flexes ±1). */
function idealSizes(patches) {
  const { tiles } = quiltLayout(patches, []);
  return new Map(tiles.filter(t => !t.isFiller).map(t => [t.id, t.idealSize]));
}

describe('quilt tile sizing', () => {
  it('keeps every tile the same size when no patch has members or events', () => {
    // The case that motivated this: a fresh quilt of unclaimed patches used to
    // hand out a 4x4 and several 3x3s purely by sort order. What matters is
    // that no patch is singled out — not which tier a flat quilt settles on
    // (the flat-quilt floor below picks that).
    const sizes = idealSizes(makePatches(20));
    expect(new Set(sizes.values()).size).toBe(1);
  });

  it('gives tied patches the same size', () => {
    const sizes = idealSizes(makePatches(20, () => 5));
    expect(new Set(sizes.values()).size).toBe(1);
  });

  it('does not award a large tile for trivial activity', () => {
    // One patch leads the quilt but with only 2 members — below the 2x2 floor.
    // It must not out-size its neighbours, whatever tier the quilt settles on.
    const sizes = idealSizes(makePatches(20, i => (i === 0 ? 2 : 0)));
    expect(sizes.get('patch-000')).toBe(sizes.get('patch-001'));
  });

  it('awards each size once its activity floor is cleared', () => {
    const patches = makePatches(30, i => {
      if (i === 0) return 24; // 4x4 floor
      if (i === 1) return 10; // 3x3 floor
      if (i === 2) return 3; // 2x2 floor
      return 0;
    });
    const sizes = idealSizes(patches);
    expect(sizes.get('patch-000')).toBe(4);
    expect(sizes.get('patch-001')).toBe(3);
    expect(sizes.get('patch-002')).toBe(2);
    expect(sizes.get('patch-003')).toBe(1);
  });

  it('allows more than one 4x4 in a large quilt', () => {
    // 100 patches, 4 genuinely huge: the 4% cap leaves room for all of them.
    const sizes = idealSizes(makePatches(100, i => (i < 4 ? 40 : 0)));
    const maxTiles = [...sizes.values()].filter(s => s === 4);
    expect(maxTiles).toHaveLength(4);
  });

  it('caps large tiles in a small quilt when patches are ranked apart', () => {
    // 10 patches, all clearing the 4x4 floor but at distinct levels. The cap
    // preserves hierarchy: only the top slice gets max tiles, the rest step
    // down rather than the whole quilt going 4x4.
    const sizes = idealSizes(makePatches(10, i => 40 - i));
    expect([...sizes.values()].filter(s => s === 4)).toHaveLength(1);
    expect(sizes.get('patch-009')).toBeLessThan(4);
  });

  it('keeps tied patches equal even when the tie clears a large-tile floor', () => {
    // Ties must not be split by the rank cap — with everyone equally active
    // there is no honest basis for picking which patch renders bigger.
    const sizes = idealSizes(makePatches(10, () => 40));
    expect(new Set(sizes.values()).size).toBe(1);
  });

  it('counts a follower for less than a member', () => {
    // Same headcount, but B's are followers — it must not tie A.
    const patches = [
      { id: 'a', name: 'A', member_count: 9, event_count: 0, follower_count: 0 },
      { id: 'b', name: 'B', member_count: 0, event_count: 0, follower_count: 9 },
    ];
    const sizes = idealSizes(patches);
    expect(sizes.get('a')).toBeGreaterThan(sizes.get('b'));
  });

  it('lets an unclaimed patch grow on followers alone', () => {
    // Unclaimed patches can't have members or events, so followers are the
    // only signal they can accrue — ignoring them would pin them at 1x1.
    const patches = makePatches(20).map((p, i) =>
      i === 0 ? { ...p, follower_count: 30 } : p
    );
    const sizes = idealSizes(patches);
    expect(sizes.get('patch-000')).toBeGreaterThan(1);
  });

  it('does not let a handful of followers inflate a tile', () => {
    const patches = makePatches(20).map((p, i) =>
      i === 0 ? { ...p, follower_count: 2 } : p
    );
    // Two followers is below the 1-member-equivalent discount, so this patch
    // stays level with the rest of the quilt rather than growing.
    const sizes = idealSizes(patches);
    expect(sizes.get('patch-000')).toBe(sizes.get('patch-001'));
  });

  it('lifts a wholly flat quilt off the minimum tier', () => {
    // At 1x1 the tile chrome stops being proportional: the `?` badge clamps to
    // its 14px floor and the outline stroke is a flat constant, so a minimum
    // tile wears an oversized badge and a heavy border. Promoting the whole
    // quilt together fixes the proportions without inventing a hierarchy.
    const sizes = idealSizes(makePatches(20));
    expect([...sizes.values()]).toEqual(Array(20).fill(2));
  });

  it('leaves the quilt alone once any patch has earned a larger tile', () => {
    // The promotion is only for wholly flat quilts — a single patch clearing
    // the 2x2 floor means the sizing already has something real to say, and
    // the quiet patches must stay small so that contrast survives.
    const sizes = idealSizes(makePatches(20, i => (i === 0 ? 3 : 0)));
    expect(sizes.get('patch-000')).toBe(2);
    expect(sizes.get('patch-001')).toBe(1);
  });

  it('is stable across reordered input when patches are tied', () => {
    const patches = makePatches(20, i => (i < 5 ? 30 : 0));
    const first = idealSizes(patches);
    const second = idealSizes([...patches].reverse());
    for (const [id, size] of first) {
      expect(second.get(id)).toBe(size);
    }
  });
});
