/**
 * Draft geometry: the pieced-block engine behind the block drafter
 * (docs/adr/029, CONTEXT.md "Tile appearance").
 *
 * A drafted block is a square grid (1x1..10x10) plus seams — straight
 * lines between wall anchors. All coordinates are integers in
 * quarter-cell units: a grid-n block spans 0..4n on both axes, so cell
 * (r, c) is the square [4c, 4c+4] x [4r, 4r+4] and the 25/50/75% anchor
 * subdivisions land on integers.
 *
 * A seam may span many cells but acts locally: clipped to a cell it is a
 * full chord (anchors sit on cell walls), so every piece is a convex
 * region of exactly one cell, and splitting a convex polygon by a chord
 * yields convex polygons — the whole engine is incremental chord
 * splitting, no global arrangement needed.
 *
 * Piece identity: faces of a cell are sorted by centroid (y, then x),
 * making the index into appearance.block.colors["r,c"] deterministic
 * regardless of seam order.
 */

export const MAX_GRID = 10;
export const SEAM_BUDGET = 24;
export const BUNDLE_SLOTS = 6;

/** Grids above this size offer midpoint anchors only. */
export const FINE_ANCHOR_MAX_GRID = 5;

const EPS = 1e-9;

// --- ANCHORS ---

/**
 * Is (x, y) a legal anchor on a grid-n block?
 * Legal anchors sit on a cell wall (x or y on a whole-cell line) at a
 * quarter subdivision; above 5x5 only halves survive.
 */
export function isLegalAnchor(grid, x, y) {
  if (!Number.isInteger(x) || !Number.isInteger(y)) return false;
  const max = 4 * grid;
  if (x < 0 || y < 0 || x > max || y > max) return false;
  if (x % 4 !== 0 && y % 4 !== 0) return false; // must lie on a cell wall
  if (grid > FINE_ANCHOR_MAX_GRID && (x % 2 !== 0 || y % 2 !== 0)) return false;
  return true;
}

/** All legal anchors for a grid, as [x, y] pairs (row-major). */
export function anchorsFor(grid) {
  const anchors = [];
  const max = 4 * grid;
  const step = grid > FINE_ANCHOR_MAX_GRID ? 2 : 1;
  for (let y = 0; y <= max; y += step) {
    for (let x = 0; x <= max; x += step) {
      if (isLegalAnchor(grid, x, y)) anchors.push([x, y]);
    }
  }
  return anchors;
}

// --- DRAFT VALIDATION (client-side mirror of the backend rules) ---

/** Structural check of a draft block object. Returns true/false. */
export function isValidDraft(draft) {
  if (!draft || typeof draft !== 'object' || Array.isArray(draft)) return false;
  const { grid, seams = [], colors = {} } = draft;
  if (!Number.isInteger(grid) || grid < 1 || grid > MAX_GRID) return false;
  if (!Array.isArray(seams) || seams.length > SEAM_BUDGET) return false;
  for (const s of seams) {
    if (!Array.isArray(s) || s.length !== 4) return false;
    const [x1, y1, x2, y2] = s;
    if (!isLegalAnchor(grid, x1, y1) || !isLegalAnchor(grid, x2, y2)) return false;
    if (x1 === x2 && y1 === y2) return false;
  }
  if (typeof colors !== 'object' || colors === null || Array.isArray(colors)) return false;
  for (const [key, slots] of Object.entries(colors)) {
    const m = /^(\d+),(\d+)$/.exec(key);
    if (!m) return false;
    if (+m[1] >= grid || +m[2] >= grid) return false;
    if (!Array.isArray(slots)) return false;
    for (const v of slots) {
      if (!Number.isInteger(v) || v < 0 || v >= BUNDLE_SLOTS) return false;
    }
  }
  return true;
}

// --- POLYGON HELPERS ---

export function polygonArea(poly) {
  let a = 0;
  for (let i = 0; i < poly.length; i++) {
    const [x1, y1] = poly[i];
    const [x2, y2] = poly[(i + 1) % poly.length];
    a += x1 * y2 - x2 * y1;
  }
  return Math.abs(a) / 2;
}

export function polygonCentroid(poly) {
  // Area-weighted centroid (signed shoelace); falls back to vertex mean
  // for degenerate slivers.
  let a = 0, cx = 0, cy = 0;
  for (let i = 0; i < poly.length; i++) {
    const [x1, y1] = poly[i];
    const [x2, y2] = poly[(i + 1) % poly.length];
    const cross = x1 * y2 - x2 * y1;
    a += cross;
    cx += (x1 + x2) * cross;
    cy += (y1 + y2) * cross;
  }
  if (Math.abs(a) < EPS) {
    const n = poly.length;
    return [
      poly.reduce((s, p) => s + p[0], 0) / n,
      poly.reduce((s, p) => s + p[1], 0) / n,
    ];
  }
  return [cx / (3 * a), cy / (3 * a)];
}

/** Point-in-convex-polygon (inclusive of boundary). */
export function pointInPolygon(poly, x, y) {
  let sign = 0;
  for (let i = 0; i < poly.length; i++) {
    const [x1, y1] = poly[i];
    const [x2, y2] = poly[(i + 1) % poly.length];
    const cross = (x2 - x1) * (y - y1) - (y2 - y1) * (x - x1);
    if (Math.abs(cross) < EPS) continue;
    const s = Math.sign(cross);
    if (sign === 0) sign = s;
    else if (s !== sign) return false;
  }
  return true;
}

// --- SEAM CLIPPING ---

/**
 * Clip segment (x1,y1)-(x2,y2) to the axis-aligned square
 * [xmin,xmax]x[ymin,ymax] (Liang-Barsky). Returns [ax, ay, bx, by] or
 * null when the seam misses the cell, only grazes it, or lies along one
 * of its walls (a wall-collinear seam splits nothing).
 */
export function clipSeamToCell(seam, xmin, ymin, xmax, ymax) {
  const [x1, y1, x2, y2] = seam;
  const dx = x2 - x1, dy = y2 - y1;
  let t0 = 0, t1 = 1;
  const edges = [
    [-dx, x1 - xmin],
    [dx, xmax - x1],
    [-dy, y1 - ymin],
    [dy, ymax - y1],
  ];
  for (const [p, q] of edges) {
    if (Math.abs(p) < EPS) {
      if (q < -EPS) return null; // parallel and outside
    } else {
      const t = q / p;
      if (p < 0) {
        if (t > t1) return null;
        if (t > t0) t0 = t;
      } else {
        if (t < t0) return null;
        if (t < t1) t1 = t;
      }
    }
  }
  if (t1 - t0 < EPS) return null; // grazes a corner or misses
  const ax = x1 + t0 * dx, ay = y1 + t0 * dy;
  const bx = x1 + t1 * dx, by = y1 + t1 * dy;
  // Wall-collinear: both endpoints on the same wall line → no split.
  if (Math.abs(ax - bx) < EPS && (Math.abs(ax - xmin) < EPS || Math.abs(ax - xmax) < EPS)) return null;
  if (Math.abs(ay - by) < EPS && (Math.abs(ay - ymin) < EPS || Math.abs(ay - ymax) < EPS)) return null;
  return [ax, ay, bx, by];
}

// --- CHORD SPLITTING ---

/**
 * Split a convex polygon by the line through (ax,ay)-(bx,by).
 * Returns [poly] when the line misses the interior, else [left, right].
 */
function splitConvexByLine(poly, ax, ay, bx, by) {
  const dx = bx - ax, dy = by - ay;
  const side = poly.map(([x, y]) => {
    const s = dx * (y - ay) - dy * (x - ax);
    return Math.abs(s) < EPS ? 0 : Math.sign(s);
  });
  if (!side.includes(1) || !side.includes(-1)) return [poly];

  const left = [], right = [];
  for (let i = 0; i < poly.length; i++) {
    const j = (i + 1) % poly.length;
    const [x1, y1] = poly[i];
    const [x2, y2] = poly[j];
    if (side[i] >= 0) left.push(poly[i]);
    if (side[i] <= 0) right.push(poly[i]);
    if (side[i] * side[j] < 0) {
      // Edge crosses the line: solve for the intersection point.
      const denom = dx * (y2 - y1) - dy * (x2 - x1);
      const t = (dy * (x1 - ax) - dx * (y1 - ay)) / denom;
      const px = x1 + t * (x2 - x1);
      const py = y1 + t * (y2 - y1);
      left.push([px, py]);
      right.push([px, py]);
    }
  }
  const out = [];
  if (left.length >= 3 && polygonArea(left) > EPS) out.push(left);
  if (right.length >= 3 && polygonArea(right) > EPS) out.push(right);
  return out.length ? out : [poly];
}

// --- FACES ---

/**
 * The pieces of cell (r, c): the cell square split by every seam that
 * crosses it, sorted by centroid (y, then x) for stable identity.
 * Returns an array of polygons in quarter-cell units.
 */
export function facesForCell(seams, r, c) {
  const xmin = 4 * c, ymin = 4 * r, xmax = xmin + 4, ymax = ymin + 4;
  let faces = [[[xmin, ymin], [xmax, ymin], [xmax, ymax], [xmin, ymax]]];
  for (const seam of seams) {
    const clipped = clipSeamToCell(seam, xmin, ymin, xmax, ymax);
    if (!clipped) continue;
    const [ax, ay, bx, by] = clipped;
    // The clipped seam is a full chord of the cell, hence of every face
    // it passes through — splitting by its line is exact, never over-cuts
    // (see file comment).
    faces = faces.flatMap((f) => splitConvexByLine(f, ax, ay, bx, by));
  }
  faces.sort((f1, f2) => {
    const [cx1, cy1] = polygonCentroid(f1);
    const [cx2, cy2] = polygonCentroid(f2);
    if (Math.abs(cy1 - cy2) > 1e-6) return cy1 - cy2;
    return cx1 - cx2;
  });
  return faces;
}

/**
 * All pieces of a draft: [{r, c, faces}] for every cell, row-major.
 * Face index within a cell is the piece's identity in
 * draft.colors["r,c"].
 */
export function facesForDraft(draft) {
  const out = [];
  const seams = draft.seams || [];
  for (let r = 0; r < draft.grid; r++) {
    for (let c = 0; c < draft.grid; c++) {
      out.push({ r, c, faces: facesForCell(seams, r, c) });
    }
  }
  return out;
}

/**
 * Locate the piece containing point (x, y) in quarter-cell units.
 * Returns {r, c, index} or null. Used by the drafter's click-to-color.
 */
export function pieceAt(draft, x, y) {
  const max = 4 * draft.grid;
  if (x < 0 || y < 0 || x > max || y > max) return null;
  const c = Math.min(draft.grid - 1, Math.floor(x / 4));
  const r = Math.min(draft.grid - 1, Math.floor(y / 4));
  const faces = facesForCell(draft.seams || [], r, c);
  for (let i = 0; i < faces.length; i++) {
    if (pointInPolygon(faces[i], x, y)) return { r, c, index: i };
  }
  return null;
}
