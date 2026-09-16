/**
 * Muted colors (docs/adr/112).
 *
 * The second way a patch's colors are drawn. One rule, stated once:
 *
 *   Muted never adds a color a patch didn't choose. It takes away what it
 *   has to and keeps the rest.
 *
 * Hue is carried through untouched, lightness is *set* to a step on a ramp
 * every tile in the quilt shares, and chroma is only ever **capped** — never
 * raised. Capping rather than setting is what makes the sentence above true,
 * and it dissolves the achromatic case instead of answering it: Stage Black
 * has no chroma to cap, so it stays a grey ramp, because that patch genuinely
 * chose to be achromatic.
 *
 * Normalizing lightness is the part that does the work. The quilt shouts in
 * two channels — chroma, and value contrast — and value contrast is the one
 * that carries across a room. Capping chroma alone quiets each tile and
 * leaves the quilt just as loud from the distance the complaint was made at.
 *
 * Everything here is pure. No DOM, no state, no store: the mode lives in
 * stores/colors.svelte.js and quiltTheme.js decides when to call this.
 */

// ---------------------------------------------------------------------------
// THE RAMP — the tuned constants.
//
// Provisional. docs/adr/112 leaves the exact values to the bench and the real
// quilt, not to taste in the abstract; what is *not* provisional is that the
// ramp is shared by every tile and indexed by slot, because that is what makes
// the transform injective on slots.
// ---------------------------------------------------------------------------

/**
 * OKLCH chroma ceiling. Colors above it come down; colors below it stay put.
 * A ceiling, never a target — see the file header.
 */
export const CHROMA_CAP = 0.09;

/**
 * Lightness per bundle slot, in OKLCH L.
 *
 * Indexed by slot rather than sorted, and the order is deliberate:
 *
 *   slot 0 — the identity color, and the one a card, marker and banner wear.
 *            Mid-light, so it is the salient one and `textOnColor` lands on
 *            ink for it predictably.
 *   slot 1 — the secondary. Light.
 *   slot 2 — the ground a curated block fills behind itself (`p.bg`). Dark,
 *            so a muted quilt reads as figures on a dark cloth rather than
 *            as dark marks on a pale one.
 *   slots 3-5 — drafted blocks only (docs/adr/029).
 *
 * No two adjacent slots are closer than 0.22 in L. That gap is the guarantee
 * that replaces the one the old bundle gave by accident: a drafted block's
 * pieces are separable *by construction* here, where building a ramp around
 * the chosen color could not promise it for a color near either end of the
 * range.
 */
export const SLOT_LIGHTNESS = [0.64, 0.86, 0.28, 0.50, 0.75, 0.38];

// ---------------------------------------------------------------------------
// OKLCH — sRGB conversion (Björn Ottosson's Oklab).
// ---------------------------------------------------------------------------

function srgbToLinear(c) {
  return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
}

function linearToSrgb(c) {
  return c <= 0.0031308 ? 12.92 * c : 1.055 * Math.pow(c, 1 / 2.4) - 0.055;
}

const hexRe = /^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/;

/** Expand #abc and drop an alpha pair, so bundle hexes all parse the same. */
function normalizeHex(hex) {
  if (typeof hex !== 'string' || !hexRe.test(hex)) return null;
  let h = hex.slice(1);
  if (h.length === 3) h = h[0] + h[0] + h[1] + h[1] + h[2] + h[2];
  return '#' + h.slice(0, 6).toLowerCase();
}

/** @returns {{L: number, C: number, h: number}|null} h in radians. */
export function hexToOklch(hex) {
  const n = normalizeHex(hex);
  if (!n) return null;
  const r = srgbToLinear(parseInt(n.slice(1, 3), 16) / 255);
  const g = srgbToLinear(parseInt(n.slice(3, 5), 16) / 255);
  const b = srgbToLinear(parseInt(n.slice(5, 7), 16) / 255);

  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);

  const L = 0.2104542553 * l + 0.7936177850 * m - 0.0040720468 * s;
  const A = 1.9779984951 * l - 2.4285922050 * m + 0.4505937099 * s;
  const B = 0.0259040371 * l + 0.7827717662 * m - 0.8086757660 * s;

  return { L, C: Math.hypot(A, B), h: Math.atan2(B, A) };
}

/** Linear-light sRGB triple for an OKLCH color, un-clamped. */
function oklchToLinearRgb(L, C, h) {
  const A = C * Math.cos(h);
  const B = C * Math.sin(h);
  const l_ = L + 0.3963377774 * A + 0.2158037573 * B;
  const m_ = L - 0.1055613458 * A - 0.0638541728 * B;
  const s_ = L - 0.0894841775 * A - 1.2914855480 * B;
  const l = l_ * l_ * l_;
  const m = m_ * m_ * m_;
  const s = s_ * s_ * s_;
  return [
     4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.7076147010 * s,
  ];
}

function inGamut([r, g, b]) {
  const e = 1e-4;
  return r >= -e && r <= 1 + e && g >= -e && g <= 1 + e && b >= -e && b <= 1 + e;
}

/**
 * OKLCH to a hex string, reducing chroma until the color fits in sRGB.
 *
 * Lightness and hue are held and chroma gives way, which is the right
 * sacrifice here for the same reason the cap exists: this file may take color
 * away and may not invent it, and a hue shift to stay in gamut would be
 * inventing one. Binary search rather than a clamp on the channels — clamping
 * RGB shifts hue silently, which is the failure this avoids.
 */
export function oklchToHex(L, C, h) {
  let rgb = oklchToLinearRgb(L, C, h);
  if (!inGamut(rgb)) {
    let lo = 0;
    let hi = C;
    for (let i = 0; i < 24; i++) {
      const mid = (lo + hi) / 2;
      if (inGamut(oklchToLinearRgb(L, mid, h))) lo = mid;
      else hi = mid;
    }
    rgb = oklchToLinearRgb(L, lo, h);
  }
  return (
    '#' +
    rgb
      .map((c) => {
        const v = Math.round(Math.min(1, Math.max(0, linearToSrgb(c))) * 255);
        return v.toString(16).padStart(2, '0');
      })
      .join('')
  );
}

// ---------------------------------------------------------------------------
// THE TRANSFORM
// ---------------------------------------------------------------------------

/**
 * One fabric of a muted tile: the patch's identity color, carried to the
 * lightness this slot draws at, with its chroma capped.
 *
 * @param {string} identityHex — the patch's identity color (bundle slot 0)
 * @param {number} slotIndex — which slot of the block this fabric fills
 * @returns {string} hex, or the input unchanged if it could not be parsed
 */
export function mutedSlot(identityHex, slotIndex = 0) {
  const c = hexToOklch(identityHex);
  if (!c) return identityHex;
  const L = SLOT_LIGHTNESS[slotIndex % SLOT_LIGHTNESS.length];
  return oklchToHex(L, Math.min(c.C, CHROMA_CAP), c.h);
}

/**
 * A whole muted bundle: `count` fabrics, all one hue, each at its own step.
 *
 * Injective on slots by construction — no two slots share a lightness, so a
 * drafted block can never flatten (docs/adr/112 decision 2). The bundle the
 * patch actually stored is *ignored* rather than transformed: past slot 0 it
 * says nothing about how muted should look, and reading it back would only
 * reintroduce the hue collisions this exists to remove.
 */
export function mutedSlots(identityHex, count) {
  const n = Math.max(1, Math.min(count | 0 || 1, SLOT_LIGHTNESS.length));
  const out = [];
  for (let i = 0; i < n; i++) out.push(mutedSlot(identityHex, i));
  return out;
}

/**
 * A single standalone color — a tag chip, a neighbour quilt's sashing, a
 * marker — muted as slot 0, so it agrees with the tile the same patch draws.
 */
export function mutedColor(hex) {
  return mutedSlot(hex, 0);
}

/**
 * The hue a color carries, in degrees, or null for one with no meaningful
 * hue. Not used by the transform — this is for the bench, which counts how
 * many distinguishable hues a real quilt actually has once muted
 * (docs/adr/112's measurement gate).
 */
export function hueDegrees(hex, achromaticBelow = 0.02) {
  const c = hexToOklch(hex);
  if (!c || c.C < achromaticBelow) return null;
  return ((c.h * 180) / Math.PI + 360) % 360;
}
