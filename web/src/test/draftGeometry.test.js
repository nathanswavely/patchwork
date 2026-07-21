import { describe, it, expect } from 'vitest';
import {
  addSeam,
  anchorsFor,
  isLegalAnchor,
  isValidDraft,
  clipSeamToCell,
  facesForCell,
  facesForDraft,
  pieceAt,
  polygonArea,
  polygonCentroid,
  SEAM_BUDGET,
} from '../lib/draftGeometry.js';

const totalArea = (faces) => faces.reduce((s, f) => s + polygonArea(f), 0);

describe('anchors', () => {
  it('fine grids get corners + quarter subdivisions on walls', () => {
    // 1x1: perimeter of a 4-unit square at every integer → 16 points.
    expect(anchorsFor(1)).toHaveLength(16);
    expect(isLegalAnchor(1, 1, 0)).toBe(true); // 25% on top wall
    expect(isLegalAnchor(1, 0, 3)).toBe(true); // 75% on left wall
    expect(isLegalAnchor(1, 1, 1)).toBe(false); // interior, not on a wall
    expect(isLegalAnchor(1, 5, 0)).toBe(false); // out of range
  });

  it('coarse grids drop to midpoints only', () => {
    // grid 6: odd coordinates are illegal even on walls.
    expect(isLegalAnchor(6, 1, 0)).toBe(false);
    expect(isLegalAnchor(6, 2, 0)).toBe(true); // midpoint of first cell wall
    expect(isLegalAnchor(6, 0, 24)).toBe(true); // far corner
    // grid 5 keeps quarters.
    expect(isLegalAnchor(5, 1, 0)).toBe(true);
  });

  it('interior wall anchors exist on shared walls', () => {
    // (4, 2) sits on the wall between cells (0,0) and (0,1) of a 2x2, at 50%.
    expect(isLegalAnchor(2, 4, 2)).toBe(true);
  });
});

describe('draft validation', () => {
  const good = { grid: 2, seams: [[0, 0, 8, 8]], colors: { '0,0': [0, 1] } };

  it('accepts a well-formed draft', () => {
    expect(isValidDraft(good)).toBe(true);
    expect(isValidDraft({ grid: 1 })).toBe(true); // seams/colors optional
  });

  it('rejects structural violations', () => {
    expect(isValidDraft({ ...good, grid: 11 })).toBe(false);
    expect(isValidDraft({ ...good, grid: 0 })).toBe(false);
    expect(isValidDraft({ ...good, seams: Array(SEAM_BUDGET + 1).fill([0, 0, 8, 8]) })).toBe(false);
    expect(isValidDraft({ ...good, seams: [[0, 0, 0, 0]] })).toBe(false); // degenerate
    expect(isValidDraft({ ...good, seams: [[1, 1, 8, 8]] })).toBe(false); // interior start
    expect(isValidDraft({ ...good, colors: { '2,0': [0] } })).toBe(false); // cell out of range
    expect(isValidDraft({ ...good, colors: { '0,0': [6] } })).toBe(false); // slot out of range
    expect(isValidDraft({ ...good, colors: { '0,0': [-1] } })).toBe(false);
    expect(isValidDraft({ grid: 6, seams: [[1, 0, 24, 24]] })).toBe(false); // quarter anchor on coarse grid
  });
});

describe('seam clipping', () => {
  it('clips a block-spanning diagonal to the cell chord', () => {
    // Diagonal of a 2x2 block through cell (1,1).
    expect(clipSeamToCell([0, 0, 8, 8], 4, 4, 8, 8)).toEqual([4, 4, 8, 8]);
  });

  it('returns null for seams that miss, graze, or run along a wall', () => {
    expect(clipSeamToCell([0, 0, 8, 8], 0, 4, 4, 8)).toBeNull(); // corner graze
    expect(clipSeamToCell([0, 0, 4, 0], 0, 0, 4, 4)).toBeNull(); // along top wall
    expect(clipSeamToCell([0, 0, 0, 4], 0, 0, 4, 4)).toBeNull(); // along left wall
    expect(clipSeamToCell([0, 12, 8, 12], 0, 0, 4, 4)).toBeNull(); // far away
  });
});

describe('faces', () => {
  it('an uncut cell is one piece with full area', () => {
    const faces = facesForCell([], 0, 0);
    expect(faces).toHaveLength(1);
    expect(polygonArea(faces[0])).toBe(16);
  });

  it('one diagonal makes two pieces; both diagonals make four (QST)', () => {
    expect(facesForCell([[0, 0, 4, 4]], 0, 0)).toHaveLength(2);
    const qst = facesForCell([[0, 0, 4, 4], [4, 0, 0, 4]], 0, 0);
    expect(qst).toHaveLength(4);
    expect(totalArea(qst)).toBeCloseTo(16, 9);
    // Every quarter triangle has equal area.
    for (const f of qst) expect(polygonArea(f)).toBeCloseTo(4, 9);
  });

  it('a seam crossing many cells splits each locally', () => {
    // Full diagonal of a 3x3: hits cells (0,0),(1,1),(2,2); others untouched.
    const cells = facesForDraft({ grid: 3, seams: [[0, 0, 12, 12]] });
    const counts = new Map(cells.map(({ r, c, faces }) => [`${r},${c}`, faces.length]));
    expect(counts.get('0,0')).toBe(2);
    expect(counts.get('1,1')).toBe(2);
    expect(counts.get('2,2')).toBe(2);
    expect(counts.get('0,1')).toBe(1);
    expect(counts.get('2,0')).toBe(1);
  });

  it('face identity is stable under seam reordering', () => {
    const a = facesForCell([[0, 0, 4, 4], [4, 0, 0, 4], [2, 0, 2, 4]], 0, 0);
    const b = facesForCell([[2, 0, 2, 4], [4, 0, 0, 4], [0, 0, 4, 4]], 0, 0);
    expect(a.length).toBe(b.length);
    for (let i = 0; i < a.length; i++) {
      const [ax, ay] = polygonCentroid(a[i]);
      const [bx, by] = polygonCentroid(b[i]);
      expect(ax).toBeCloseTo(bx, 6);
      expect(ay).toBeCloseTo(by, 6);
    }
  });

  it('area is conserved no matter how a cell is cut', () => {
    const seams = [
      [0, 0, 4, 4],
      [4, 0, 0, 4],
      [2, 0, 2, 4],
      [0, 2, 4, 2],
      [1, 0, 4, 3],
    ];
    expect(totalArea(facesForCell(seams, 0, 0))).toBeCloseTo(16, 9);
  });
});

describe('recreating curated blocks', () => {
  it('Hourglass: both diagonals of a 1x1 grid', () => {
    const cells = facesForDraft({ grid: 1, seams: [[0, 0, 4, 4], [4, 0, 0, 4]] });
    expect(cells).toHaveLength(1);
    const faces = cells[0].faces;
    expect(faces).toHaveLength(4);
    // Centroid sort: top, left, right, bottom.
    const centroids = faces.map(polygonCentroid);
    expect(centroids[0][1]).toBeLessThan(centroids[1][1]); // top first
    expect(centroids[1][0]).toBeLessThan(centroids[2][0]); // left before right
  });

  it('Pinwheel: one diagonal per cell of a 2x2, alternating', () => {
    const draft = {
      grid: 2,
      seams: [
        [0, 0, 4, 4], // cell (0,0): TL→BR
        [8, 0, 4, 4], // cell (0,1): TR→BL
        [4, 4, 0, 8], // cell (1,0): TR→BL
        [4, 4, 8, 8], // cell (1,1): TL→BR
      ],
    };
    const cells = facesForDraft(draft);
    expect(cells).toHaveLength(4);
    for (const { faces } of cells) expect(faces).toHaveLength(2); // each cell = 1 HST
  });

  it('Flying Geese: a goose in one cell via two corner-to-midpoint seams', () => {
    const faces = facesForCell([[0, 4, 2, 0], [2, 0, 4, 4]], 0, 0);
    expect(faces).toHaveLength(3); // goose triangle + two sky corners
    expect(totalArea(faces)).toBeCloseTo(16, 9);
  });
});

describe('seam merging', () => {
  it('a seam continuing an existing one merges into their union', () => {
    // (0,0)-(4,4) then (4,4)-(8,8): shares an endpoint, same line.
    const seams = addSeam([[0, 0, 4, 4]], [4, 4, 8, 8]);
    expect(seams).toEqual([[0, 0, 8, 8]]);
  });

  it('merges regardless of endpoint order (shared endpoint, reversed direction)', () => {
    const seams = addSeam([[4, 4, 0, 0]], [8, 8, 4, 4]);
    expect(seams).toEqual([[0, 0, 8, 8]]);
  });

  it('overlapping collinear seams merge into their union span', () => {
    const seams = addSeam([[0, 0, 4, 4]], [2, 2, 8, 8]);
    expect(seams).toEqual([[0, 0, 8, 8]]);
  });

  it('a seam fully contained in an existing collinear seam is a no-op', () => {
    const seams = addSeam([[0, 0, 8, 8]], [2, 2, 4, 4]);
    expect(seams).toEqual([[0, 0, 8, 8]]);
  });

  it('an existing seam fully contained in the new one is absorbed', () => {
    const seams = addSeam([[2, 2, 4, 4]], [0, 0, 8, 8]);
    expect(seams).toEqual([[0, 0, 8, 8]]);
  });

  it('chains: a seam bridging two others merges all three into one', () => {
    const existing = [[0, 0, 2, 2], [6, 6, 8, 8]];
    const seams = addSeam(existing, [2, 2, 6, 6]);
    expect(seams).toEqual([[0, 0, 8, 8]]);
  });

  it('chains regardless of the order seams were drawn in', () => {
    const existing = [[6, 6, 8, 8], [0, 0, 2, 2]];
    const seams = addSeam(existing, [2, 2, 6, 6]);
    expect(seams).toEqual([[0, 0, 8, 8]]);
  });

  it('touching but non-collinear seams do not merge', () => {
    // Both start at (0,0) but go different directions.
    const seams = addSeam([[0, 0, 4, 4]], [0, 0, 4, 0]);
    expect(seams).toHaveLength(2);
    expect(seams).toEqual(expect.arrayContaining([[0, 0, 4, 4], [0, 0, 4, 0]]));
  });

  it('parallel collinear seams that do not touch stay separate', () => {
    const seams = addSeam([[0, 0, 2, 2]], [4, 4, 8, 8]);
    expect(seams).toHaveLength(2);
    expect(seams).toEqual(expect.arrayContaining([[0, 0, 2, 2], [4, 4, 8, 8]]));
  });

  it('leaves unrelated seams untouched', () => {
    const seams = addSeam([[0, 4, 4, 0]], [4, 4, 8, 8]);
    expect(seams).toEqual(expect.arrayContaining([[0, 4, 4, 0], [4, 4, 8, 8]]));
    expect(seams).toHaveLength(2);
  });

  it('a merge keeps the seam budget from being charged twice', () => {
    // 24 independent parallel seams (a full budget), then a seam that
    // continues one of them: the union replaces it, net count unchanged.
    const full = Array.from({ length: SEAM_BUDGET }, (_, i) => [0, i, 1, i]);
    const seams = addSeam(full, [1, 0, 2, 0]); // continues [0,0,1,0]
    expect(seams).toHaveLength(SEAM_BUDGET);
    expect(seams).toEqual(expect.arrayContaining([[0, 0, 2, 0]]));
  });

  it('merging can reduce the seam count below budget even when starting at budget', () => {
    // 22 unrelated row seams, plus two collinear-but-gapped seams on the
    // same line (y=0, x:0-2 and x:6-8) — 24 seams total, at budget.
    const rows = Array.from({ length: 22 }, (_, i) => [0, i + 1, 1, i + 1]);
    const full = [...rows, [0, 0, 2, 0], [6, 0, 8, 0]];
    expect(full).toHaveLength(SEAM_BUDGET);
    // Bridging the gap merges the two collinear seams into one: net -1.
    const seams = addSeam(full, [2, 0, 6, 0]);
    expect(seams.length).toBe(SEAM_BUDGET - 1);
    expect(seams).toEqual(expect.arrayContaining([[0, 0, 8, 0]]));
  });
});

describe('pieceAt', () => {
  it('locates the piece under a point', () => {
    const draft = { grid: 1, seams: [[0, 0, 4, 4]] };
    // Above the TL→BR diagonal (top-right half).
    expect(pieceAt(draft, 3, 1)).toEqual({ r: 0, c: 0, index: expect.any(Number) });
    const above = pieceAt(draft, 3, 1);
    const below = pieceAt(draft, 1, 3);
    expect(above.index).not.toBe(below.index);
  });

  it('returns null outside the block', () => {
    expect(pieceAt({ grid: 1, seams: [] }, 5, 5)).toBeNull();
    expect(pieceAt({ grid: 1, seams: [] }, -1, 0)).toBeNull();
  });
});
