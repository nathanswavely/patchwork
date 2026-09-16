import { describe, it, expect } from 'vitest';
import { WALL } from '../lib/fabricWall.js';
import { PALETTES } from '../lib/quiltTheme.js';
import {
  CHROMA_CAP,
  SLOT_LIGHTNESS,
  hexToOklch,
  mutedColor,
  mutedSlot,
  mutedSlots,
} from '../lib/mutedColors.js';

// docs/adr/112. These pin the sentence the whole design turns on —
//
//   "Muted never adds a color a patch didn't choose. It takes away what it
//    has to and keeps the rest."
//
// — plus the two structural promises that sentence doesn't state: that the
// transform stays injective on slots, and that lightness is normalized
// across every tile rather than built around the color a patch picked.
//
// Every one of these is a rule somebody could break while making the quilt
// look nicer, with nothing else in the suite noticing.

const EVERY_CHOSEN_COLOR = [
  ...WALL.map((sw) => sw.hex),
  ...Object.values(PALETTES).flatMap((p) => p.colors),
];

describe('muted never adds a color a patch did not choose', () => {
  it('never raises chroma above what was chosen', () => {
    for (const hex of EVERY_CHOSEN_COLOR) {
      const before = hexToOklch(hex).C;
      for (let slot = 0; slot < SLOT_LIGHTNESS.length; slot++) {
        const after = hexToOklch(mutedSlot(hex, slot)).C;
        // Rounding to 8-bit channels moves chroma a hair either way; what
        // must not happen is a *rise* toward the cap from below it.
        expect(after).toBeLessThanOrEqual(Math.min(before, CHROMA_CAP) + 0.005);
      }
    }
  });

  it('leaves a chosen hue where it was', () => {
    for (const hex of EVERY_CHOSEN_COLOR) {
      const before = hexToOklch(hex);
      if (before.C < 0.03) continue; // no meaningful hue to preserve
      const after = hexToOklch(mutedColor(hex));
      const delta = Math.abs(
        ((((after.h - before.h) * 180) / Math.PI + 540) % 360) - 180,
      );
      expect(delta).toBeLessThan(2.5);
    }
  });

  it('leaves an achromatic patch achromatic', () => {
    // Stage Black chose to have no color. Muted has none to give it back,
    // and must not invent one out of rounding noise — this is the case the
    // cap dissolves instead of answering with a fallback chain.
    for (const key of ['stage-black', 'raw-cotton', 'charcoal', 'dove']) {
      const hex = WALL.find((sw) => sw.key === key).hex;
      for (let slot = 0; slot < SLOT_LIGHTNESS.length; slot++) {
        expect(hexToOklch(mutedSlot(hex, slot)).C).toBeLessThan(0.025);
      }
    }
  });

  it('caps the loudest fabrics on the wall', () => {
    // The ones the complaint was actually about.
    for (const key of ['hi-vis', 'lemon', 'slime', 'punch', 'safety-orange']) {
      const hex = WALL.find((sw) => sw.key === key).hex;
      expect(hexToOklch(hex).C).toBeGreaterThan(CHROMA_CAP);
      expect(hexToOklch(mutedColor(hex)).C).toBeLessThanOrEqual(
        CHROMA_CAP + 0.005,
      );
    }
  });
});

describe('the transform stays injective on slots', () => {
  // renderDraftBlock colors pieces by `slots[slot] ?? slots[slot % len]`, so
  // any transform that lands two slots on one color merges adjacent pieces
  // and the seam between them stops existing — a drafted Ohio Star flattens
  // into a rectangle. The block is the half of a patch's identity that isn't
  // color; the mode that exists to preserve identity may not delete it.
  it('gives six distinct fabrics for every color on the wall', () => {
    for (const sw of WALL) {
      const slots = mutedSlots(sw.hex, 6);
      expect(new Set(slots).size, `${sw.key} collapsed: ${slots}`).toBe(6);
    }
  });

  it('keeps every adjacent pair separable in value', () => {
    for (const sw of WALL) {
      const ls = mutedSlots(sw.hex, 6).map((h) => hexToOklch(h).L);
      for (let i = 0; i < ls.length - 1; i++) {
        expect(Math.abs(ls[i] - ls[i + 1])).toBeGreaterThan(0.15);
      }
    }
  });
});

describe('lightness is normalized across the quilt, not per patch', () => {
  // This is the half that quiets the quilt from across a room. Capping
  // chroma alone calms each tile and leaves the field just as loud, which is
  // the distance the complaint was made at.
  it('draws every patch at the same lightness for a given slot', () => {
    for (let slot = 0; slot < SLOT_LIGHTNESS.length; slot++) {
      const ls = WALL.map((sw) => hexToOklch(mutedSlot(sw.hex, slot)).L);
      expect(Math.max(...ls) - Math.min(...ls)).toBeLessThan(0.02);
    }
  });

  it('puts the ground darker than the identity color it sits behind', () => {
    // Slot 2 is `p.bg`, the ground a curated block fills behind itself. A
    // muted quilt should read as figures on a dark cloth.
    const identity = hexToOklch(mutedSlot('#DA0956', 0)).L;
    const ground = hexToOklch(mutedSlot('#DA0956', 2)).L;
    expect(ground).toBeLessThan(identity);
  });
});

describe('the transform is total', () => {
  it('returns a usable hex for every color on the wall', () => {
    for (const sw of WALL) {
      for (let slot = 0; slot < SLOT_LIGHTNESS.length; slot++) {
        expect(mutedSlot(sw.hex, slot)).toMatch(/^#[0-9a-f]{6}$/);
      }
    }
  });

  it('hands back anything it cannot parse, rather than throwing', () => {
    for (const bad of [null, undefined, '', 'red', '#12', 'rgb(1,2,3)']) {
      expect(() => mutedColor(bad)).not.toThrow();
      expect(mutedColor(bad)).toBe(bad);
    }
  });

  it('accepts the shorthand and alpha hexes a stored bundle may carry', () => {
    // quiltTheme's bundleHexRe admits #abc and #rrggbbaa.
    expect(mutedColor('#DA0956')).toBe(mutedColor('#da0956ff'));
    expect(mutedColor('#f00')).toBe(mutedColor('#ff0000'));
  });
});
