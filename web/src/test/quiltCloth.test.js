import { describe, it, expect } from 'vitest';
import { createLattice, warpPolygon, CLOTH, fabricGrain, weaveFor, WEAVES } from '../lib/quiltCloth.js';
import { blockPieces, BLOCKS, getBlockIndex } from '../lib/quiltBlocks.js';

const PALETTE = { primary: '#DA0956', secondary: '#1493CC', bg: '#F5CEC2' };

/** Pull every coordinate pair out of a path's `d`. */
function coords(d) {
  return (d.match(/-?\d+\.?\d*/g) || []).map(Number);
}

describe('shared lattice', () => {
  it('gives both neighbours of a seam the identical curve', () => {
    const L = createLattice(60);
    // The seam between a tile at (0,0) and its right neighbour at (1,0) is
    // the vertical segment on column line 1. Both must trace it identically,
    // which is what makes the quilt watertight.
    const e = L.edgeV(1, 0);
    const left = L.tilePath(0, 0, 1);
    const right = L.tilePath(1, 0, 1);
    const ctrl = `${Math.round(e.ctrl[0] * 100) / 100} ${Math.round(e.ctrl[1] * 100) / 100}`;
    expect(left).toContain(ctrl);
    expect(right).toContain(ctrl);
  });

  it('places a shared corner at one point for all four tiles touching it', () => {
    const L = createLattice(60);
    const p = L.point(1, 1);
    for (const [col, row] of [[0, 0], [1, 0], [0, 1], [1, 1]]) {
      const d = L.tilePath(col, row, 1);
      const xs = coords(d);
      const hit = xs.some((v, i) => i % 2 === 0
        && Math.abs(v - p[0]) < 0.02 && Math.abs(xs[i + 1] - p[1]) < 0.02);
      expect(hit, `tile ${col},${row} should touch the shared corner`).toBe(true);
    }
  });

  it('is deterministic across lattices with the same parameters', () => {
    expect(createLattice(60).tilePath(2, 3, 2)).toBe(createLattice(60).tilePath(2, 3, 2));
  });

  it('scales with the base unit', () => {
    const a = createLattice(30).point(4, 4);
    const b = createLattice(60).point(4, 4);
    expect(b[0]).toBeCloseTo(a[0] * 2, 6);
    expect(b[1]).toBeCloseTo(a[1] * 2, 6);
  });

  it('draws each interior seam once and marks the quilt edge', () => {
    // Two tiles side by side: 7 unique segments, 6 of them on the edge.
    const L = createLattice(60);
    const { chunks, outer, segments } = L.seamGeometry([
      { col: 0, row: 0, size: 1 },
      { col: 1, row: 0, size: 1 },
    ]);
    expect(segments).toBe(7);
    const interior = [...chunks.values()].join('').match(/M/g).length;
    const edge = [...outer.values()].join('').match(/M/g).length;
    expect(interior).toBe(7);
    expect(edge).toBe(6);
  });

  it('bends the lattice when the cloth sags, and not when it does not', () => {
    const flat = createLattice(60, { sag: 0 });
    const sagged = createLattice(60, { sag: 0.2 });
    expect(sagged.point(3, 3)).not.toEqual(flat.point(3, 3));
    expect(createLattice(60, { sag: 0 }).point(3, 3)).toEqual(flat.point(3, 3));
  });
});

describe('block warp', () => {
  it('lands the block’s outer edge exactly on the tile’s seam', () => {
    const L = createLattice(60);
    const warp = L.makeWarp(0, 0, 1);
    // The unit square's corners must map to the lattice points the tile
    // outline is built from — that is what "nothing clipped off" means.
    for (const [u, v, c, r] of [[0, 0, 0, 0], [1, 0, 1, 0], [1, 1, 1, 1], [0, 1, 0, 1]]) {
      const got = warp(u, v);
      const want = L.point(c, r);
      expect(got[0]).toBeCloseTo(want[0], 6);
      expect(got[1]).toBeCloseTo(want[1], 6);
    }
  });

  it('keeps the boundary exact however hard the centre is pulled straight', () => {
    const L = createLattice(60);
    const loose = L.makeWarp(0, 0, 1, 0);
    const tight = createLattice(60, { pull: 1 }).makeWarp(0, 0, 1, 0);
    // Boundary agrees...
    expect(tight(0.5, 0)[0]).toBeCloseTo(loose(0.5, 0)[0], 6);
    expect(tight(0.5, 0)[1]).toBeCloseTo(loose(0.5, 0)[1], 6);
    // ...while the middle does not.
    expect(tight(0.5, 0.5)[0]).not.toBeCloseTo(loose(0.5, 0.5)[0], 3);
  });

  it('walks a straight block edge as several points, not two', () => {
    const L = createLattice(60);
    const warp = L.makeWarp(0, 0, 2);
    const square = [[0, 0], [1, 0], [1, 1], [0, 1]];
    // Steps per edge are len * tileSize * quality, so a 2x2 tile at quality
    // 1 already splits each side in two.
    const coarse = coords(warpPolygon(square, warp, 2, 1)).length / 2;
    const fine = coords(warpPolygon(square, warp, 2, 8)).length / 2;
    expect(coarse).toBe(8);
    expect(fine).toBeGreaterThan(coarse * 4);
  });

  it('rotates in block space, so a rotated tile still fills its seam', () => {
    const L = createLattice(60);
    const warp = L.makeWarp(0, 0, 1, 90);
    // Every corner of the unit square still lands on a lattice corner,
    // whichever one — a rotated block must not hang off its own tile.
    const corners = [L.point(0, 0), L.point(1, 0), L.point(1, 1), L.point(0, 1)];
    for (const [u, v] of [[0, 0], [1, 0], [1, 1], [0, 1]]) {
      const got = warp(u, v);
      const near = corners.some((c) => Math.abs(c[0] - got[0]) < 1e-6 && Math.abs(c[1] - got[1]) < 1e-6);
      expect(near).toBe(true);
    }
  });
});

describe('block geometry capture', () => {
  it('captures a piece for every shape each curated block draws', () => {
    for (let i = 0; i < BLOCKS.length; i++) {
      const pieces = blockPieces('patch', PALETTE, { block: BLOCKS[i].key });
      expect(pieces.length, `${BLOCKS[i].key} should emit pieces`).toBeGreaterThan(0);
      for (const p of pieces) {
        expect(p.pts.length).toBeGreaterThanOrEqual(3);
        expect(p.fill).toMatch(/^#/);
        for (const [x, y] of p.pts) {
          expect(Number.isFinite(x) && Number.isFinite(y)).toBe(true);
          expect(x).toBeGreaterThanOrEqual(-0.001);
          expect(x).toBeLessThanOrEqual(1.001);
          expect(y).toBeGreaterThanOrEqual(-0.001);
          expect(y).toBeLessThanOrEqual(1.001);
        }
      }
    }
  });

  it('captures a drafted block’s faces', () => {
    const draft = { grid: 2, seams: [[0, 0, 8, 8]], colors: { '0,0': [0, 1] } };
    const pieces = blockPieces('patch', { ...PALETTE, slots: ['#111111', '#222222'] }, { block: draft });
    expect(pieces.length).toBeGreaterThan(0);
  });

  it('honours a pinned block over the id hash', () => {
    const a = blockPieces('patch-a', PALETTE, { block: 'pinwheel' });
    const b = blockPieces('patch-b', PALETTE, { block: 'pinwheel' });
    expect(a).toEqual(b);
    expect(getBlockIndex('patch-a', { block: 'pinwheel' }))
      .toBe(getBlockIndex('patch-b', { block: 'pinwheel' }));
  });
});

describe('cloth constants', () => {
  it('states the tuned values the bench settled on', () => {
    // These are a design decision, not an implementation detail: a silent
    // edit here changes how every quilt looks. Pinning them means a change
    // has to be deliberate.
    expect(CLOTH.amp).toBe(0.02);
    expect(CLOTH.bow).toBe(0.008);
    expect(CLOTH.pull).toBe(0.65);
    expect(CLOTH.seamW).toBe(0.6);
    expect(CLOTH.chunk).toBe(3);
  });

  it('reads weave over a dark fabric more strongly than a pale one', () => {
    expect(fabricGrain('#0a0a0a')).toBeGreaterThan(fabricGrain('#F5CEC2'));
  });

  it('gives one fabric one weave, every time', () => {
    expect(weaveFor('#DA0956')).toBe(weaveFor('#DA0956'));
    expect(weaveFor('#DA0956')).toBeLessThan(WEAVES);
  });
});
