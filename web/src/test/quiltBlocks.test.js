import { describe, it, expect } from 'vitest';
import * as d3 from 'd3';
import { renderBlock, renderDraftBlock } from '../lib/quiltBlocks.js';
import { pointInPolygon } from '../lib/draftGeometry.js';

const BUNDLE = ['#F2E205', '#F20574', '#0D0D0D'];
const PALETTE = { primary: BUNDLE[0], secondary: BUNDLE[1], bg: BUNDLE[2], slots: BUNDLE };

function draw(fn) {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  fn(d3.select(svg).append('g'));
  return svg;
}

// A 2x2 draft, no seams: the four cells are whole pieces, three of them
// cut from slot 0 and one from slot 1.
const DRAFT = { grid: 2, seams: [], colors: { '0,0': [0], '0,1': [0], '1,0': [0], '1,1': [1] } };

describe('drafted block rendering', () => {
  it('gathers the pieces of one fabric into a single path', () => {
    // Drawn one element per piece, abutting pieces each cover half of the
    // pixel their shared edge lands in and the ground shows through as a
    // hairline — the grid that appeared over blocks at most tile sizes.
    const svg = draw((g) => renderDraftBlock(g, 100, DRAFT, PALETTE));
    const paths = [...svg.querySelectorAll('path')];
    expect(paths).toHaveLength(2); // one per fabric in use, not one per piece
    expect(svg.querySelectorAll('polygon')).toHaveLength(0);

    const slot0 = paths.find((p) => p.getAttribute('fill') === BUNDLE[0]);
    expect(slot0.getAttribute('d').match(/Z/g)).toHaveLength(3); // still 3 pieces
  });

  it('outlines every piece in its own fill so no ground shows at a seam', () => {
    const svg = draw((g) => renderDraftBlock(g, 100, DRAFT, PALETTE));
    for (const el of svg.querySelectorAll('rect, polygon, path')) {
      expect(el.getAttribute('stroke')).toBe(el.getAttribute('fill'));
      expect(el.getAttribute('vector-effect')).toBe('non-scaling-stroke');
    }
  });

  it('seals curated blocks too', () => {
    const svg = draw((g) =>
      renderBlock(g, 100, 'patch-id', PALETTE, { block: 'pinwheel', rotation: 0, bundle: BUNDLE }));
    const pieces = [...svg.querySelectorAll('rect, polygon, path')];
    expect(pieces.length).toBeGreaterThan(0);
    for (const el of pieces) expect(el.getAttribute('stroke')).toBe(el.getAttribute('fill'));
  });
});

describe('curated blocks', () => {
  it('rail fence leaves no bare spot', () => {
    // Cut from two different line families, its stripes left a wedge of the
    // block uncovered — the canvas showed through the bottom-right corner.
    // Areas alone don't catch it: that draft also overlapped itself by the
    // size of the wedge, so the five areas summed to the whole square.
    const s = 120;
    const svg = draw((g) =>
      renderBlock(g, s, 'patch-id', PALETTE, { block: 'railFence', rotation: 0, bundle: BUNDLE }));
    const stripes = [...svg.querySelectorAll('polygon')]
      .map((el) => el.getAttribute('points').trim().split(/\s+/).map((pt) => pt.split(',').map(Number)));

    const bare = [];
    for (let i = 1; i < 40; i++) {
      for (let j = 1; j < 40; j++) {
        const x = (i * s) / 40, y = (j * s) / 40;
        if (!stripes.some((poly) => pointInPolygon(poly, x, y))) bare.push([x, y]);
      }
    }
    expect(bare).toEqual([]);
  });
});
