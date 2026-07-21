/**
 * Quilt block pattern renderer.
 * Curated blocks: traditional patterns drawn by named renderers; each
 * patch gets one from its ID hash unless its appearance pins one.
 * Drafted blocks (docs/adr/029): appearance.block is an embedded draft
 * object — grid + seams + piece colors — rendered via draftGeometry.
 * Blocks are rendered as SVG elements inside a square tile.
 */

import { facesForDraft, isValidDraft } from './draftGeometry.js';

// --- BLOCK RENDERERS ---
// Each takes (g, s, p) where g=SVG group, s=square size, p={primary, secondary, bg}

function pinwheel(g, s, p) {
  const h = s / 2;
  // 4 triangles rotating around center.
  g.append('polygon').attr('points', `0,0 ${h},0 ${h},${h}`).attr('fill', p.primary);
  g.append('polygon').attr('points', `${h},0 ${s},0 ${h},${h}`).attr('fill', p.bg);
  g.append('polygon').attr('points', `${s},0 ${s},${h} ${h},${h}`).attr('fill', p.secondary);
  g.append('polygon').attr('points', `${s},${h} ${s},${s} ${h},${h}`).attr('fill', p.bg);
  g.append('polygon').attr('points', `${s},${s} ${h},${s} ${h},${h}`).attr('fill', p.primary);
  g.append('polygon').attr('points', `${h},${s} 0,${s} ${h},${h}`).attr('fill', p.bg);
  g.append('polygon').attr('points', `0,${s} 0,${h} ${h},${h}`).attr('fill', p.secondary);
  g.append('polygon').attr('points', `0,${h} 0,0 ${h},${h}`).attr('fill', p.bg);
}

function ohioStar(g, s, p) {
  const t = s / 3;
  // Background.
  g.append('rect').attr('width', s).attr('height', s).attr('fill', p.bg);
  // Corner squares.
  g.append('rect').attr('width', t).attr('height', t).attr('fill', p.secondary);
  g.append('rect').attr('x', 2*t).attr('width', t).attr('height', t).attr('fill', p.secondary);
  g.append('rect').attr('y', 2*t).attr('width', t).attr('height', t).attr('fill', p.secondary);
  g.append('rect').attr('x', 2*t).attr('y', 2*t).attr('width', t).attr('height', t).attr('fill', p.secondary);
  // Center square.
  g.append('rect').attr('x', t).attr('y', t).attr('width', t).attr('height', t).attr('fill', p.primary);
  // Star points (triangles in the edge squares).
  const m = t / 2;
  // Top.
  g.append('polygon').attr('points', `${t},0 ${t+m},${t} ${2*t},0`).attr('fill', p.primary);
  // Bottom.
  g.append('polygon').attr('points', `${t},${s} ${t+m},${2*t} ${2*t},${s}`).attr('fill', p.primary);
  // Left.
  g.append('polygon').attr('points', `0,${t} ${t},${t+m} 0,${2*t}`).attr('fill', p.primary);
  // Right.
  g.append('polygon').attr('points', `${s},${t} ${2*t},${t+m} ${s},${2*t}`).attr('fill', p.primary);
}

function brokenDishes(g, s, p) {
  const h = s / 2;
  // 4 squares, each split diagonally.
  // Top-left.
  g.append('polygon').attr('points', `0,0 ${h},0 0,${h}`).attr('fill', p.primary);
  g.append('polygon').attr('points', `${h},0 ${h},${h} 0,${h}`).attr('fill', p.bg);
  // Top-right.
  g.append('polygon').attr('points', `${h},0 ${s},0 ${s},${h}`).attr('fill', p.bg);
  g.append('polygon').attr('points', `${h},0 ${s},${h} ${h},${h}`).attr('fill', p.secondary);
  // Bottom-left.
  g.append('polygon').attr('points', `0,${h} ${h},${h} ${h},${s}`).attr('fill', p.secondary);
  g.append('polygon').attr('points', `0,${h} ${h},${s} 0,${s}`).attr('fill', p.bg);
  // Bottom-right.
  g.append('polygon').attr('points', `${h},${h} ${s},${h} ${h},${s}`).attr('fill', p.bg);
  g.append('polygon').attr('points', `${s},${h} ${s},${s} ${h},${s}`).attr('fill', p.primary);
}

function flyingGeese(g, s, p) {
  const rowH = s / 4;
  g.append('rect').attr('width', s).attr('height', s).attr('fill', p.bg);
  for (let i = 0; i < 4; i++) {
    const y = i * rowH;
    const color = i % 2 === 0 ? p.primary : p.secondary;
    g.append('polygon').attr('points', `0,${y + rowH} ${s/2},${y} ${s},${y + rowH}`).attr('fill', color);
  }
}

function fourPatch(g, s, p) {
  const h = s / 2;
  g.append('rect').attr('width', h).attr('height', h).attr('fill', p.primary);
  g.append('rect').attr('x', h).attr('width', h).attr('height', h).attr('fill', p.secondary);
  g.append('rect').attr('y', h).attr('width', h).attr('height', h).attr('fill', p.secondary);
  g.append('rect').attr('x', h).attr('y', h).attr('width', h).attr('height', h).attr('fill', p.primary);
}

function ninePatch(g, s, p) {
  const t = s / 3;
  for (let r = 0; r < 3; r++) {
    for (let c = 0; c < 3; c++) {
      const color = (r + c) % 2 === 0 ? p.primary : ((r + c) % 3 === 0 ? p.secondary : p.bg);
      g.append('rect').attr('x', c * t).attr('y', r * t).attr('width', t).attr('height', t).attr('fill', color);
    }
  }
}

function hourglass(g, s, p) {
  const h = s / 2;
  g.append('rect').attr('width', s).attr('height', s).attr('fill', p.bg);
  // Top triangle.
  g.append('polygon').attr('points', `0,0 ${s},0 ${h},${h}`).attr('fill', p.primary);
  // Bottom triangle.
  g.append('polygon').attr('points', `0,${s} ${s},${s} ${h},${h}`).attr('fill', p.secondary);
}

function sawtoothStar(g, s, p) {
  const t = s / 4;
  g.append('rect').attr('width', s).attr('height', s).attr('fill', p.bg);
  // Center.
  g.append('rect').attr('x', t).attr('y', t).attr('width', 2*t).attr('height', 2*t).attr('fill', p.primary);
  // 4 sawtooth points.
  g.append('polygon').attr('points', `${t},0 ${2*t},${t} ${3*t},0`).attr('fill', p.secondary); // top
  g.append('polygon').attr('points', `${t},${s} ${2*t},${3*t} ${3*t},${s}`).attr('fill', p.secondary); // bottom
  g.append('polygon').attr('points', `0,${t} ${t},${2*t} 0,${3*t}`).attr('fill', p.secondary); // left
  g.append('polygon').attr('points', `${s},${t} ${3*t},${2*t} ${s},${3*t}`).attr('fill', p.secondary); // right
  // Corner squares.
  g.append('rect').attr('width', t).attr('height', t).attr('fill', p.secondary);
  g.append('rect').attr('x', 3*t).attr('width', t).attr('height', t).attr('fill', p.secondary);
  g.append('rect').attr('y', 3*t).attr('width', t).attr('height', t).attr('fill', p.secondary);
  g.append('rect').attr('x', 3*t).attr('y', 3*t).attr('width', t).attr('height', t).attr('fill', p.secondary);
}

function railFence(g, s, p) {
  const w = s / 3;
  // 3 diagonal stripes.
  g.append('polygon').attr('points', `0,0 ${w},0 0,${s}`).attr('fill', p.primary);
  g.append('polygon').attr('points', `${w},0 ${2*w},0 0,${s} 0,${s * 2/3}`).attr('fill', p.bg);
  g.append('polygon').attr('points', `${2*w},0 ${s},0 ${s},${s/3} 0,${s}`).attr('fill', p.secondary);
  g.append('polygon').attr('points', `${s},${s/3} ${s},${2*s/3} ${w},${s} 0,${s}`).attr('fill', p.bg);
  g.append('polygon').attr('points', `${s},${2*s/3} ${s},${s} ${2*w},${s}`).attr('fill', p.primary);
}

function logCabin(g, s, p) {
  // Concentric rectangles spiraling from center.
  const layers = 4;
  const step = s / (layers * 2);
  for (let i = 0; i < layers; i++) {
    const inset = i * step;
    const sz = s - inset * 2;
    const color = i === 0 ? p.primary : (i % 2 === 0 ? p.secondary : p.bg);
    g.append('rect').attr('x', inset).attr('y', inset).attr('width', sz).attr('height', sz).attr('fill', color);
  }
  // Center.
  const cInset = layers * step;
  const cSz = s - cInset * 2;
  g.append('rect').attr('x', cInset).attr('y', cInset).attr('width', cSz).attr('height', cSz).attr('fill', p.primary);
}

function bearsPaw(g, s, p) {
  const t = s / 4;
  g.append('rect').attr('width', s).attr('height', s).attr('fill', p.bg);
  // Center cross.
  g.append('rect').attr('x', t).attr('y', t).attr('width', 2*t).attr('height', 2*t).attr('fill', p.primary);
  // Corner "paws" — each is a small square + triangle.
  const pawCorners = [[0,0], [3*t,0], [0,3*t], [3*t,3*t]];
  pawCorners.forEach(([px, py]) => {
    g.append('rect').attr('x', px).attr('y', py).attr('width', t).attr('height', t).attr('fill', p.secondary);
  });
  // Claw triangles adjacent to paws.
  g.append('polygon').attr('points', `${t},0 ${t},${t} ${2*t},0`).attr('fill', p.secondary);
  g.append('polygon').attr('points', `0,${t} ${t},${t} 0,${2*t}`).attr('fill', p.secondary);
  g.append('polygon').attr('points', `${3*t},0 ${3*t},${t} ${2*t},0`).attr('fill', p.secondary);
  g.append('polygon').attr('points', `${s},${t} ${3*t},${t} ${s},${2*t}`).attr('fill', p.secondary);
  g.append('polygon').attr('points', `${t},${s} ${t},${3*t} ${2*t},${s}`).attr('fill', p.secondary);
  g.append('polygon').attr('points', `0,${3*t} ${t},${3*t} 0,${2*t}`).attr('fill', p.secondary);
  g.append('polygon').attr('points', `${3*t},${s} ${3*t},${3*t} ${2*t},${s}`).attr('fill', p.secondary);
  g.append('polygon').attr('points', `${s},${3*t} ${3*t},${3*t} ${s},${2*t}`).attr('fill', p.secondary);
}

function windmill(g, s, p) {
  const h = s / 2;
  // 4 quadrants, each with a triangle creating a spinning effect.
  g.append('rect').attr('width', s).attr('height', s).attr('fill', p.bg);
  g.append('polygon').attr('points', `0,0 ${h},0 ${h},${h}`).attr('fill', p.primary);
  g.append('polygon').attr('points', `${s},0 ${s},${h} ${h},${h}`).attr('fill', p.secondary);
  g.append('polygon').attr('points', `${s},${s} ${h},${s} ${h},${h}`).attr('fill', p.primary);
  g.append('polygon').attr('points', `0,${s} 0,${h} ${h},${h}`).attr('fill', p.secondary);
}

// --- BLOCK REGISTRY ---
// Keys are the opaque slugs stored in node.appearance.block.

const BLOCKS = [
  { key: 'pinwheel', name: 'Pinwheel', render: pinwheel },
  { key: 'ohioStar', name: 'Ohio Star', render: ohioStar },
  { key: 'brokenDishes', name: 'Broken Dishes', render: brokenDishes },
  { key: 'flyingGeese', name: 'Flying Geese', render: flyingGeese },
  { key: 'fourPatch', name: 'Four Patch', render: fourPatch },
  { key: 'ninePatch', name: 'Nine Patch', render: ninePatch },
  { key: 'hourglass', name: 'Hourglass', render: hourglass },
  { key: 'sawtoothStar', name: 'Sawtooth Star', render: sawtoothStar },
  { key: 'railFence', name: 'Rail Fence', render: railFence },
  { key: 'logCabin', name: 'Log Cabin', render: logCabin },
  { key: 'bearsPaw', name: 'Bear\'s Paw', render: bearsPaw },
  { key: 'windmill', name: 'Windmill', render: windmill },
];

const BLOCK_INDEX_BY_KEY = new Map(BLOCKS.map((b, i) => [b.key, i]));

export { BLOCKS };

function hashStr(s) {
  let h = 0;
  for (let i = 0; i < s.length; i++) {
    h = ((h << 5) - h + s.charCodeAt(i)) | 0;
  }
  return h;
}

/** Get block index (0-11) for a patch ID, honoring a pinned appearance.block. */
export function getBlockIndex(patchId, appearance) {
  const pinned = BLOCK_INDEX_BY_KEY.get(appearance?.block);
  if (pinned !== undefined) return pinned;
  return Math.abs(hashStr(patchId)) % BLOCKS.length;
}

/** Get rotation (0, 90, 180, 270), honoring a pinned appearance.rotation. */
export function getRotation(patchId, appearance) {
  const r = appearance?.rotation;
  if (r === 0 || r === 90 || r === 180 || r === 270) return r;
  return [0, 90, 180, 270][Math.abs(hashStr(patchId + '_rot')) % 4];
}

/**
 * Render a drafted block (docs/adr/029) into an SVG group.
 * Pieces are colored by bundle slot via draft.colors["r,c"][faceIndex];
 * missing entries fall back to slot 0. Slot indices past the bundle wrap
 * so a shrunk bundle still renders every piece.
 * @param {d3.Selection} group — the <g> element to draw into
 * @param {number} size — square size in pixels
 * @param {object} draft — {grid, seams, colors} in quarter-cell units
 * @param {object} palette — paletteForPatch result ({primary, secondary, bg, slots})
 */
export function renderDraftBlock(group, size, draft, palette) {
  const slots = palette.slots?.length
    ? palette.slots
    : [palette.primary, palette.secondary, palette.bg];
  const scale = size / (4 * draft.grid);
  const colorFor = (slot) => slots[slot] ?? slots[slot % slots.length];
  group.append('rect').attr('width', size).attr('height', size).attr('fill', palette.bg);
  for (const { r, c, faces } of facesForDraft(draft)) {
    const cellSlots = draft.colors?.[`${r},${c}`] || [];
    faces.forEach((poly, i) => {
      group
        .append('polygon')
        .attr('points', poly.map(([x, y]) => `${x * scale},${y * scale}`).join(' '))
        .attr('fill', colorFor(cellSlots[i] ?? 0));
    });
  }
}

/**
 * Render a quilt block pattern into an SVG group.
 * @param {d3.Selection} group — the <g> element to draw into
 * @param {number} size — square size in pixels
 * @param {string} patchId — unique patch ID for deterministic assignment
 * @param {object} palette — { primary, secondary, bg, slots? }
 * @param {object|null} appearance — node.appearance; a pinned curated
 *   block/rotation wins over the hash, a valid draft object renders via
 *   renderDraftBlock, unknown keys fall back to the hash
 */
export function renderBlock(group, size, patchId, palette, appearance = null) {
  const rotation = getRotation(patchId, appearance);

  // Create a sub-group for rotation.
  const blockG = group.append('g');

  if (rotation !== 0) {
    const cx = size / 2;
    const cy = size / 2;
    blockG.attr('transform', `rotate(${rotation}, ${cx}, ${cy})`);
  }

  const block = appearance?.block;
  if (block && typeof block === 'object' && isValidDraft(block)) {
    renderDraftBlock(blockG, size, block, palette);
    return;
  }

  BLOCKS[getBlockIndex(patchId, appearance)].render(blockG, size, palette);
}

/**
 * Render a ghost (decorative) block at low opacity.
 */
export function renderGhostBlock(group, size, index, palette) {
  const blockIdx = index % BLOCKS.length;
  const blockG = group.append('g').attr('opacity', 0.15);
  BLOCKS[blockIdx].render(blockG, size, palette);
}
