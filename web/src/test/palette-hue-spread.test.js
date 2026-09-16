import { describe, it, expect } from 'vitest';
import { PALETTES, PALETTE_KEYS, paletteForPatch } from '../lib/quiltTheme.js';
import { WALL } from '../lib/fabricWall.js';
import { hueDegrees } from '../lib/mutedColors.js';

// docs/adr/112. A patch that has chosen nothing is hash-assigned a palette,
// and for a long time it was hash-assigned one of eight lifted from punk
// record sleeves. Six of those eight primaries sit inside a 55 degree arc
// through red. Nobody saw it, because a tile draws three fabrics and the
// variety a reader sees comes from the secondaries and grounds.
//
// Measured on the live instance before this changed: 54 of 58 patches were
// hash-assigned, and they resolved to *nine* distinct identity colors, 67%
// of them red. Identity color is what a card, a map marker and a profile
// banner wear, so the quilt was saying much less about who a patch was than
// it looked like it was saying.
//
// These tests pin the spread. The failure they exist to catch is somebody
// adding four lovely new reds.

const BIN = 30;
const BINS = 360 / BIN;

function binOf(hex) {
  const h = hueDegrees(hex);
  return h === null ? 'grey' : Math.floor(h / BIN) * BIN;
}

describe('the hash-assignable palettes cover the hue wheel', () => {
  it('populates all but one 30 degree band', () => {
    const filled = new Set(PALETTE_KEYS.map((k) => binOf(PALETTES[k].primary)));
    const empty = [...Array(BINS)].map((_, i) => i * BIN).filter((b) => !filled.has(b));
    // 270-300 is empty because the *fabric wall* has nothing there — the
    // nearest are ink-blue at 268 and violet at 301. Closing it means adding
    // a swatch to the wall, not inventing a color here. If this list ever
    // grows past that one band, the set has drifted back toward a cluster.
    expect(empty).toEqual([270]);
  });

  it('lets no single band hold more than a quarter of them', () => {
    const counts = new Map();
    for (const k of PALETTE_KEYS) {
      const b = binOf(PALETTES[k].primary);
      counts.set(b, (counts.get(b) || 0) + 1);
    }
    const worst = Math.max(...counts.values());
    expect(worst / PALETTE_KEYS.length).toBeLessThan(0.25);
  });

  it('keeps the red arc under half the set', () => {
    // The arc that was 6 of 8.
    const red = PALETTE_KEYS.filter((k) => {
      const h = hueDegrees(PALETTES[k].primary);
      return h !== null && (h >= 320 || h <= 40);
    });
    expect(red.length / PALETTE_KEYS.length).toBeLessThan(0.5);
  });
});

describe('hash assignment spreads a real population of patches', () => {
  // The palettes covering the wheel is necessary but not sufficient — the
  // hash has to reach them. This walks a synthetic population the way the
  // quilt does, through the real entry point.
  const ids = [...Array(400)].map((_, i) => `0192f${i.toString(16).padStart(8, '0')}`);

  it('gives 400 unchosen patches more than twelve identity colors', () => {
    const colors = new Set(ids.map((id) => paletteForPatch(id, null).primary));
    expect(colors.size).toBeGreaterThan(12);
  });

  it('puts no more than a quarter of them in any one band', () => {
    const counts = new Map();
    for (const id of ids) {
      const b = binOf(paletteForPatch(id, null).primary);
      counts.set(b, (counts.get(b) || 0) + 1);
    }
    // The same bound the set-level test uses, deliberately. A uniform hash
    // over the set lands in each band at roughly the set's own share, so a
    // tighter bound here would not be testing the hash — it would be a
    // second, stricter cap on the palette list, hidden in the wrong test,
    // failing whenever the two disagreed.
    expect(Math.max(...counts.values()) / ids.length).toBeLessThan(0.25);
  });
});

describe('the wall cuts are cut from the wall', () => {
  const onWall = new Set(WALL.map((sw) => sw.hex.toLowerCase()));

  it('draws every fabric of every palette added since the albums', () => {
    // The album palettes predate the wall (docs/adr/029) and are grandfathered;
    // everything since is picked off it, so a cut can never introduce a color
    // an admin could not also choose by hand.
    const albums = new Set([
      'adolescents', 'pinkRazors', 'greatestSongs', 'allroysRevenge',
      'anthem', 'allTheShoes', 'bottlesToTheGround', 'liberalAnimation',
    ]);
    for (const key of PALETTE_KEYS) {
      if (albums.has(key)) continue;
      const p = PALETTES[key];
      for (const hex of [p.primary, p.secondary, p.bg]) {
        expect(onWall.has(hex.toLowerCase()), `${key}: ${hex} is not on the wall`).toBe(true);
      }
    }
  });

  it('grounds some cuts dark and some pale', () => {
    // A set that grounded every cut in Stage Black would spread the hues and
    // flatten the quilt's value range instead — the same mistake, moved.
    const grounds = PALETTE_KEYS.map((k) => PALETTES[k].bg.toLowerCase());
    const dark = grounds.filter((g) => ['#0a0a0a', '#2c2d29', '#2d2619', '#261922'].includes(g));
    expect(dark.length).toBeGreaterThan(4);
    expect(dark.length).toBeLessThan(grounds.length - 4);
  });
});

describe('a pinned palette keeps resolving', () => {
  it('still answers to every album key a patch may have stored', () => {
    // node.appearance.palette holds these strings on real rows. Renaming or
    // dropping one silently drops that patch back to hash assignment.
    for (const key of [
      'adolescents', 'pinkRazors', 'greatestSongs', 'allroysRevenge',
      'anthem', 'allTheShoes', 'bottlesToTheGround', 'liberalAnimation',
    ]) {
      expect(PALETTES[key], `${key} went missing`).toBeTruthy();
      expect(paletteForPatch('any-id', { palette: key }).paletteKey).toBe(key);
    }
  });

  it('hands a hash-assigned patch a key its editor can pre-select', () => {
    // PatchForm's setup mode seeds a claimant from the listing's current
    // effective appearance so they see what is already there rather than a
    // reroll, and it speaks only in palette keys. A null here would hand
    // them the reroll that comment exists to prevent.
    const pal = paletteForPatch('0192f00000000001', null);
    expect(pal.paletteKey).toBeTruthy();
    expect(PALETTES[pal.paletteKey]).toBeTruthy();
  });
});
