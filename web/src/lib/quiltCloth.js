/**
 * The quilt's cloth: seams, warp, folds and weave.
 *
 * The rule the whole file turns on is that **a seam belongs to the boundary
 * between two tiles, not to either tile**. Before this, every tile clipped
 * itself to one of five wobbly outlines chosen from its own id, so the gap
 * between two neighbours was the sum of two independent insets — anywhere
 * from 0.4% to 2.4% of a tile, pinching and flaring along a single seam, and
 * scaling 20x across the zoom range because it lived in world geometry.
 *
 * Here the wobble lives on a lattice both neighbours read. Every grid
 * intersection gets a hashed drift and every one-unit seam segment a hashed
 * bow; a tile traces those shared points and curves, walking them backwards
 * on the far side. A reversed quadratic is the identical curve, so the join
 * is exact: the quilt is watertight, and the seam becomes ink drawn on top
 * rather than a hole left underneath.
 *
 * Everything here is pure geometry and deterministic. Nothing touches the
 * DOM except the two texture bakes, which draw into a detached canvas.
 *
 * docs/adr/066 records the decision and what was measured. CONTEXT.md holds
 * the vocabulary: the tile-to-tile boundary is a *tile seam* (plain "seam"
 * is the block drafter's, sewn within one block; sashing is the framing
 * between remote-quilt regions — neither is this).
 */

// ---------------------------------------------------------------------------
// CLOTH — the tuned constants.
//
// Every one of these was set by eye in the seam bench and then measured. Units
// are stated because they are easy to confuse: a value in *tiles* is a
// fraction of one grid unit and survives any base unit, while a value in
// *screen px* is deliberately independent of zoom.
// ---------------------------------------------------------------------------

export const CLOTH = {
  // --- Shared lattice ---
  /** Drift of each grid intersection, in tiles. Both neighbours read it. */
  amp: 0.02,
  /** Bow of each one-unit seam segment, in tiles. */
  bow: 0.008,

  // --- Block warp ---
  /**
   * How far the warp eases back toward the rigid square as you move inward.
   * The tile's own boundary is unaffected at any value, so this only decides
   * how much a block's middle is allowed to wander.
   */
  pull: 0.65,
  /**
   * Steps per tile of piece edge when walking a straight edge through the
   * warp. Below about 4 the long edges visibly cut the corner off the curve.
   * Costs build time and no measurable frame time.
   */
  quality: 12,

  // --- Folds ---
  /** Cloth drawn toward each fold, in tiles. Moves the lattice itself. */
  sag: 0.06,
  /** Strength of the baked fold shading. */
  wrinkleAmt: 0.2,
  /** Fold wavelength, in tiles. */
  wrinkleScale: 3.25,

  // --- Seam ink (screen px — deliberately does not scale with zoom) ---
  /** The stitch itself. */
  seamW: 0.6,
  /** Offset of the pressed-allowance highlight, toward the lamp. */
  ridge: 0.3,

  // --- Fabric depth (tiles) ---
  /** Inner lip hugging each seam. Constant per seam, not per tile size. */
  bevel: 0.02,
  /** Batting puff — widens and deepens the soft falloff either side. */
  puff: 0.33,
  /** Raking light, as asymmetric shading across every seam. */
  lightAmt: 0.18,

  // --- Weave ---
  /** Swatch repeat, in tiles. */
  weaveScale: 0.2,
  /** Weave opacity before the per-fabric lightness correction. */
  weaveDepth: 0.15,

  // --- Structure ---
  /**
   * Tiles per batched path. A path is only skipped when its bounding box
   * misses the viewport, so one path spanning the whole quilt is a path that
   * always rasterises. Three measured best: 22 -> 79 fps at 269 patches.
   */
  chunk: 3,

  // --- Detail tiers (zoom multiples) ---
  /** Below this the weave is not resolvable and is not drawn. */
  weaveFrom: 1.2,
  /** Below this the topstitch is not resolvable and is not drawn. */
  stitchFrom: 2,
  /**
   * A pattern fill is re-tiled whenever the scale changes — free to pan
   * across, the dominant cost of every zoom. Nobody can read a thread count
   * mid-gesture, so the weave stands down and returns this long after the
   * view settles (ms).
   */
  weaveSettleMs: 140,
};

// ---------------------------------------------------------------------------
// Deterministic noise
// ---------------------------------------------------------------------------

/** Integer hash -> [0,1). Same inputs, same cloth, every load. */
export function hash3(x, y, s) {
  let h = (x | 0) * 374761393 + (y | 0) * 668265263 + (s | 0) * 1442695041;
  h = Math.imul(h ^ (h >>> 13), 1274126177);
  h = h ^ (h >>> 16);
  return ((h >>> 0) % 1000000) / 1000000;
}

function vnoise(x, y, s) {
  const xi = Math.floor(x), yi = Math.floor(y);
  const xf = x - xi, yf = y - yi;
  const u = xf * xf * (3 - 2 * xf), v = yf * yf * (3 - 2 * yf);
  const a = hash3(xi, yi, s), b = hash3(xi + 1, yi, s);
  const c = hash3(xi, yi + 1, s), d = hash3(xi + 1, yi + 1, s);
  return (a * (1 - u) + b * u) * (1 - v) + (c * (1 - u) + d * u) * v;
}

function fbm(x, y, s, oct) {
  let sum = 0, amp = 0.5, fr = 1;
  for (let i = 0; i < oct; i++) {
    sum += amp * (vnoise(x * fr, y * fr, s + i * 37) * 2 - 1);
    amp *= 0.5; fr *= 2;
  }
  return sum;
}

/**
 * The cloth's height field, in grid units.
 *
 * Used for two different things — it bends the lattice, and it is baked into
 * the shading layer. They agree because they are the same field, which is the
 * whole reason a fold reads as a fold rather than as distortion with a smudge
 * over it. The domain warp is what turns blobs into creases.
 */
export function clothHeight(gx, gy, scale) {
  const x = gx / scale, y = gy / scale;
  const wx = fbm(x * 0.7, y * 0.7, 5, 2);
  const wy = fbm(x * 0.7 + 5.2, y * 0.7 + 1.3, 91, 2);
  return fbm(x + wx * 0.9, y + wy * 0.9, 173, 3);
}

// ---------------------------------------------------------------------------
// The lattice
// ---------------------------------------------------------------------------

function fmt(v) { return Math.round(v * 100) / 100; }

/**
 * Build a lattice for one layout.
 *
 * Returned as a closure rather than module state because the base unit and
 * every amplitude are fixed for the life of a layout and change together on
 * rebuild — a cache keyed on anything less would have to be invalidated by
 * hand, and the failure mode is a quilt half-drawn with stale geometry.
 *
 * @param {number} bu — world px per grid unit
 * @param {object} [opts] — overrides for any CLOTH value
 */
export function createLattice(bu, opts = {}) {
  const o = { ...CLOTH, ...opts };
  const amp = o.amp, bow = o.bow, sag = o.sag, wScale = o.wrinkleScale;
  const chunkSize = o.chunk;
  const points = new Map();

  /** A grid intersection, drifted and then pulled toward any nearby fold. */
  function point(c, r) {
    const key = c + ',' + r;
    const hit = points.get(key);
    if (hit) return hit;
    let x = c + (hash3(c, r, 101) - 0.5) * 2 * amp;
    let y = r + (hash3(c, r, 103) - 0.5) * 2 * amp;
    if (sag > 0) {
      // A fold pulls cloth toward its ridge: move along the negative
      // gradient of the height field.
      const e = 0.25;
      const gx = (clothHeight(c + e, r, wScale) - clothHeight(c - e, r, wScale)) / (2 * e);
      const gy = (clothHeight(c, r + e, wScale) - clothHeight(c, r - e, wScale)) / (2 * e);
      x -= gx * sag;
      y -= gy * sag;
    }
    const p = [x * bu, y * bu];
    points.set(key, p);
    return p;
  }

  /** Horizontal segment along row line r, spanning col c..c+1. */
  function edgeH(c, r) {
    const a = point(c, r), b = point(c + 1, r);
    const d = (hash3(c, r, 107) - 0.5) * 4 * bow * bu;
    return { a, b, ctrl: [(a[0] + b[0]) / 2, (a[1] + b[1]) / 2 + d] };
  }

  /** Vertical segment along col line c, spanning row r..r+1. */
  function edgeV(c, r) {
    const a = point(c, r), b = point(c, r + 1);
    const d = (hash3(c, r, 109) - 0.5) * 4 * bow * bu;
    return { a, b, ctrl: [(a[0] + b[0]) / 2 + d, (a[1] + b[1]) / 2] };
  }

  /** One segment as a path command. Reversing keeps the identical curve. */
  function seg(e, reverse) {
    const t = reverse ? e.a : e.b;
    return `Q${fmt(e.ctrl[0])} ${fmt(e.ctrl[1])} ${fmt(t[0])} ${fmt(t[1])}`;
  }

  /**
   * A tile's outline, walked clockwise out of shared segments. Because the
   * far side of every segment is the identical curve reversed, two
   * neighbours meet exactly and no background shows between them.
   */
  function tilePath(col, row, size) {
    const start = point(col, row);
    let d = `M${fmt(start[0])} ${fmt(start[1])}`;
    for (let i = 0; i < size; i++) d += seg(edgeH(col + i, row), false);
    for (let i = 0; i < size; i++) d += seg(edgeV(col + size, row + i), false);
    for (let i = size - 1; i >= 0; i--) d += seg(edgeH(col + i, row + size), true);
    for (let i = size - 1; i >= 0; i--) d += seg(edgeV(col, row + i), true);
    return d + 'Z';
  }

  /** Which spatial bucket a grid coordinate falls in. */
  function chunkKey(c, r) {
    if (!chunkSize) return 'all';
    return Math.floor(c / chunkSize) + '_' + Math.floor(r / chunkSize);
  }

  /**
   * Every unique boundary segment in the quilt, drawn once and grouped into
   * spatial chunks.
   *
   * An interior seam is claimed by two tiles and would otherwise be stroked
   * twice, at slightly different positions and so at doubled darkness. A
   * segment claimed by only one tile is the quilt's own edge, which is what
   * the binding is drawn along.
   */
  function seamGeometry(tiles) {
    const seen = new Map();
    for (const t of tiles) {
      for (let i = 0; i < t.size; i++) {
        bump(`H${t.col + i},${t.row}`);
        bump(`H${t.col + i},${t.row + t.size}`);
        bump(`V${t.col},${t.row + i}`);
        bump(`V${t.col + t.size},${t.row + i}`);
      }
    }
    function bump(k) { seen.set(k, (seen.get(k) || 0) + 1); }

    const chunks = new Map();
    const outer = new Map();
    let segments = 0;
    for (const [k, count] of seen) {
      const [c, r] = k.slice(1).split(',').map(Number);
      const e = k[0] === 'H' ? edgeH(c, r) : edgeV(c, r);
      const sub = `M${fmt(e.a[0])} ${fmt(e.a[1])}${seg(e, false)}`;
      const ck = chunkKey(c, r);
      chunks.set(ck, (chunks.get(ck) || '') + sub);
      if (count === 1) outer.set(ck, (outer.get(ck) || '') + sub);
      segments++;
    }
    return { chunks, outer, segments };
  }

  /**
   * A map from block coordinates (the unit square) into world space, for one
   * tile.
   *
   * The tile is a size x size field of lattice cells, each bounded by the
   * same four shared curves the seams are drawn from. A point lands in one
   * cell and that cell's Coons patch carries it across — a surface that
   * interpolates all four of its boundary curves exactly. Cells share their
   * curves, so the map is continuous over the whole tile; the tile's outer
   * curves *are* its seams, so the block meets the seam with nothing clipped
   * off and nothing left short.
   *
   * Without this the block is drawn in a rigid square and clipped, and
   * wherever a seam bows outward the fabric has already run out: measured at
   * 23% of the boundary missed at the default wobble, 0% warped.
   */
  function makeWarp(col, row, size, rotation = 0) {
    const cells = new Map();
    const pull = o.pull;
    const rad = (rotation * Math.PI) / 180;
    const cosR = Math.cos(rad), sinR = Math.sin(rad);

    function cellOf(i, j) {
      const key = i + ',' + j;
      let cell = cells.get(key);
      if (!cell) {
        cell = {
          top: edgeH(col + i, row + j),
          bottom: edgeH(col + i, row + j + 1),
          left: edgeV(col + i, row + j),
          right: edgeV(col + i + 1, row + j),
        };
        cells.set(key, cell);
      }
      return cell;
    }

    return function warp(u0, v0) {
      // Rotation happens in block space, before the warp, so a rotated block
      // stretches the same way an unrotated one does.
      let u = u0, v = v0;
      if (rotation) {
        const dx = u0 - 0.5, dy = v0 - 0.5;
        u = dx * cosR - dy * sinR + 0.5;
        v = dx * sinR + dy * cosR + 0.5;
      }
      const gu = u * size, gv = v * size;
      const i = Math.min(size - 1, Math.max(0, Math.floor(gu)));
      const j = Math.min(size - 1, Math.max(0, Math.floor(gv)));
      const lu = gu - i, lv = gv - j;
      const cl = cellOf(i, j);
      const T = qbez(cl.top, lu), B = qbez(cl.bottom, lu);
      const L = qbez(cl.left, lv), R = qbez(cl.right, lv);
      const P00 = cl.top.a, P10 = cl.top.b, P01 = cl.bottom.a, P11 = cl.bottom.b;
      let x = (1 - lv) * T[0] + lv * B[0] + (1 - lu) * L[0] + lu * R[0]
        - ((1 - lu) * (1 - lv) * P00[0] + lu * (1 - lv) * P10[0]
          + (1 - lu) * lv * P01[0] + lu * lv * P11[0]);
      let y = (1 - lv) * T[1] + lv * B[1] + (1 - lu) * L[1] + lu * R[1]
        - ((1 - lu) * (1 - lv) * P00[1] + lu * (1 - lv) * P10[1]
          + (1 - lu) * lv * P01[1] + lu * lv * P11[1]);
      if (pull > 0) {
        // Weight is 1 along the tile's own boundary however hard the middle
        // is pulled straight, so the seam stays exact at any setting.
        const dEdge = Math.min(u, 1 - u, v, 1 - v) * 2;
        const w = 1 - pull * dEdge;
        const sx = (col + u * size) * bu, sy = (row + v * size) * bu;
        x = sx + (x - sx) * w;
        y = sy + (y - sy) * w;
      }
      return [x, y];
    };
  }

  return { point, edgeH, edgeV, tilePath, chunkKey, seamGeometry, makeWarp, bu };
}

function qbez(e, t) {
  const m = 1 - t, w0 = m * m, w1 = 2 * m * t, w2 = t * t;
  return [
    e.a[0] * w0 + e.ctrl[0] * w1 + e.b[0] * w2,
    e.a[1] * w0 + e.ctrl[1] * w1 + e.b[1] * w2,
  ];
}

/**
 * One polygon of block coordinates, walked through a warp.
 *
 * A straight edge in block space is a curve in world space, so each is walked
 * in steps; long edges get more steps than short ones.
 */
export function warpPolygon(pts, warp, size, quality = CLOTH.quality) {
  let d = '';
  const n = pts.length;
  for (let i = 0; i < n; i++) {
    const a = pts[i], b = pts[(i + 1) % n];
    const len = Math.hypot(b[0] - a[0], b[1] - a[1]);
    const steps = Math.max(1, Math.round(len * size * quality));
    for (let s = 0; s < steps; s++) {
      const t = s / steps;
      const [x, y] = warp(a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t);
      d += (d ? 'L' : 'M') + fmt(x) + ' ' + fmt(y);
    }
  }
  return d + 'Z';
}

/**
 * Weave read over a fabric is a proportion of its own lightness, not a fixed
 * amount of white — otherwise a near-black piece looks dusty and a pale one
 * looks bare.
 */
export function fabricGrain(hex) {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex || '');
  if (!m) return 1;
  const n = parseInt(m[1], 16);
  const lum = (0.2126 * ((n >> 16) & 255) + 0.7152 * ((n >> 8) & 255) + 0.0722 * (n & 255)) / 255;
  return 0.55 + 0.75 * (1 - lum);
}

// ---------------------------------------------------------------------------
// Baked textures
//
// Both are drawn once into a detached canvas and handed over as an image, so
// nothing here is evaluated per frame. A pan stays one transform on one group
// however detailed the cloth is.
// ---------------------------------------------------------------------------

const bakeCache = new Map();

/**
 * The fold shading: the height field's slope against a fixed lamp, written
 * out as premultiplied light and shadow.
 *
 * Drawn with ordinary alpha rather than a blend mode, so it never forces the
 * quilt into an isolated compositing buffer. Its resolution is a fixed pixel
 * budget rather than the quilt's size — folds are low frequency and the image
 * stretches, and sized to the quilt a 779-patch board spent 341ms here.
 */
export function bakeWrinkles(cols, rows, opts = {}) {
  const o = { ...CLOTH, ...opts };
  const key = `w${o.wrinkleScale}_${o.wrinkleAmt}_${cols}x${rows}`;
  const hit = bakeCache.get(key);
  if (hit) return hit;

  const TARGET_PX = 60000;
  const ppu = Math.min(24, Math.sqrt(TARGET_PX / Math.max(1, cols * rows)));
  const w = Math.max(32, Math.round(cols * ppu));
  const h = Math.max(32, Math.round(rows * ppu));
  const cv = document.createElement('canvas');
  cv.width = w; cv.height = h;
  const ctx = cv.getContext('2d');
  const img = ctx.createImageData(w, h);
  const px = img.data;
  const lx = -0.7071, ly = -0.7071;   // the lamp the raking light also uses
  const e = 0.5 / ppu;
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const gx = x / ppu, gy = y / ppu;
      const dx = (clothHeight(gx + e, gy, o.wrinkleScale) - clothHeight(gx - e, gy, o.wrinkleScale)) / (2 * e);
      const dy = (clothHeight(gx, gy + e, o.wrinkleScale) - clothHeight(gx, gy - e, o.wrinkleScale)) / (2 * e);
      const s = (dx * lx + dy * ly) * o.wrinkleAmt;
      const i = (y * w + x) * 4;
      const lit = s > 0;
      px[i] = lit ? 255 : 0;
      px[i + 1] = lit ? 250 : 0;
      px[i + 2] = lit ? 238 : 6;
      px[i + 3] = Math.round(Math.min(1, Math.abs(s)) * 255);
    }
  }
  ctx.putImageData(img, 0, 0);
  const url = cv.toDataURL('image/png');
  bakeCache.set(key, url);
  return url;
}

/**
 * One tileable weave swatch. Every fabric reuses it — they differ by the
 * transform on the pattern, not by another image, so the GPU uploads one
 * texture however many fabrics a quilt has.
 */
export function bakeWeave(kind) {
  const key = `v${kind}`;
  const hit = bakeCache.get(key);
  if (hit) return hit;

  // The swatch is drawn in world units, so it magnifies with the quilt. Baked
  // at the thread count alone it upscales into a screen door by 4x zoom, so
  // it is baked at 3x the threads it shows and downsampled the rest of the
  // time — which costs one 48px image, once.
  const THREADS = 8;
  const N = THREADS * 6;
  const t = N / THREADS;               // px per thread
  const cv = document.createElement('canvas');
  cv.width = N; cv.height = N;
  const ctx = cv.getContext('2d');
  const img = ctx.createImageData(N, N);
  const px = img.data;
  for (let y = 0; y < N; y++) {
    for (let x = 0; x < N; x++) {
      const tx = Math.floor(x / t), ty = Math.floor(y / t);
      let v;
      if (kind === 0) v = (tx + ty) % 2 ? 0.55 : -0.55;                   // plain
      else if (kind === 1) v = (tx + ty) % 4 < 2 ? 0.5 : -0.5;            // twill
      else v = (vnoise(x * 0.3, y * 0.12, 61) - 0.5) * 2;                 // slub
      // Break the grid so the repeat does not read as a screen door, and
      // soften each thread's edge so it survives being magnified.
      v += (hash3(tx, ty, 401) - 0.5) * 0.5;
      v *= 0.55 + 0.45 * Math.sin((Math.PI * ((x % t) + 0.5)) / t)
                       * Math.sin((Math.PI * ((y % t) + 0.5)) / t);
      const i = (y * N + x) * 4;
      const lit = v > 0;
      px[i] = px[i + 1] = px[i + 2] = lit ? 255 : 0;
      px[i + 3] = Math.round(Math.min(1, Math.abs(v)) * 0.5 * 255);
    }
  }
  ctx.putImageData(img, 0, 0);
  const url = cv.toDataURL('image/png');
  bakeCache.set(key, url);
  return url;
}

/** Number of distinct weave swatches. */
export const WEAVES = 3;

/** Pick a fabric's weave from its colour, so one fabric always wears one. */
export function weaveFor(fill) {
  let h = 0;
  for (let i = 0; i < fill.length; i++) h = Math.imul(h ^ fill.charCodeAt(i), 16777619);
  return Math.abs(h) % WEAVES;
}
