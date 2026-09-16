/**
 * Quilt palettes.
 *
 * Each patch's tile is drawn with a palette: chosen by its admins via the
 * Patch Appearance settings (stored in node.appearance.palette), or
 * hash-assigned from the patch ID when unset. Unknown palette keys fall
 * back to hash assignment — see docs/adr/004.
 *
 * Two kinds, one list. The **album palettes** came first, lifted from punk
 * record sleeves:
 * https://joetamponi.com/blog/8-awesome-inspirational-color-palettes-from-punk-rock-records
 *
 * The **wall cuts** below them exist because those eight are clustered in
 * hue — six of the eight primaries sit inside a 55 degree arc through red.
 * A quilt drawn from three fabrics per tile hides that, because the variety
 * a reader sees comes from the secondaries and grounds. Measured on the
 * live instance: 54 of 58 patches were hash-assigned, and they resolved to
 * nine distinct identity colors, 67% of them red. See docs/adr/112, which
 * found it, and could not ship the thing it was written for until this was
 * fixed.
 *
 * So the hash draws from a set that covers the wheel. Every wall cut is
 * three swatches off the fabric wall (docs/adr/029) and is named for the
 * fabric it leads with, the way a quilter names a cut — no invented lore,
 * and no pretending it came off a record sleeve.
 */

import { WALL } from './fabricWall.js';
import { getColorMode } from '../stores/colors.svelte.js';
import { mutedColor, mutedSlots } from './mutedColors.js';

// --- PALETTES ---
// Each has 3 colors for quilt blocks: primary, secondary, bg.
// Plus a `colors` array of all source colors for the appearance picker UI.

const ALBUM_PALETTES = {
  adolescents: {
    key: 'adolescents',
    name: 'Adolescents',
    subtitle: 'Adolescents, 1981',
    primary: '#039BE6',
    secondary: '#EC341C',
    bg: '#0a0a0a',
    colors: ['#039BE6', '#000000', '#EC341C'],
  },
  pinkRazors: {
    key: 'pinkRazors',
    name: 'Pink Razors',
    subtitle: 'Chixdiggit!, 2005',
    primary: '#DA0956',
    secondary: '#1493CC',
    bg: '#F5CEC2',
    colors: ['#E7658E', '#F5CEC2', '#F6C87F', '#1493CC', '#DA0956', '#9FC3DA'],
  },
  greatestSongs: {
    key: 'greatestSongs',
    name: 'Greatest Songs',
    subtitle: 'NOFX, 2004',
    primary: '#88DE16',
    secondary: '#88ABD1',
    bg: '#E4E5C4',
    colors: ['#88DE16', '#E4E5C4', '#625749', '#2D2619', '#88ABD1'],
  },
  allroysRevenge: {
    key: 'allroysRevenge',
    name: "Allroy's Revenge",
    subtitle: 'All, 1989',
    primary: '#B23282',
    secondary: '#FCFD1B',
    bg: '#0a0a0a',
    colors: ['#000000', '#B23282', '#FCFD1B'],
  },
  anthem: {
    key: 'anthem',
    name: 'Anthem',
    subtitle: 'Less Than Jake, 2003',
    primary: '#C02624',
    secondary: '#D89E13',
    bg: '#261922',
    colors: ['#D89E13', '#C02624', '#B1752C', '#753C1E', '#261922', '#F5EB06'],
  },
  allTheShoes: {
    key: 'allTheShoes',
    name: 'All the Shoes',
    subtitle: 'NOFX, 1997',
    primary: '#5A2517',
    secondary: '#E1A4BC',
    bg: '#EFF1CE',
    colors: ['#5A2517', '#EFF1CE', '#E1A4BC', '#81AE7F', '#0F090E'],
  },
  bottlesToTheGround: {
    key: 'bottlesToTheGround',
    name: 'Bottles to the Ground',
    subtitle: 'NOFX, 2000',
    primary: '#E3480B',
    secondary: '#94AC0E',
    bg: '#D9D6AF',
    colors: ['#94AC0E', '#E3480B', '#D9D6AF', '#94CDBE', '#2C2D29'],
  },
  liberalAnimation: {
    key: 'liberalAnimation',
    name: 'Liberal Animation',
    subtitle: 'NOFX, 1988',
    primary: '#952117',
    secondary: '#F4CD2E',
    bg: '#5E8258',
    colors: ['#952117', '#F4CD2E', '#5E8258', '#3A4E8A', '#7690C1', '#204B4B'],
  },
};

/**
 * Wall cuts: [primary, secondary, ground], by fabric-wall key.
 *
 * Ordered by the primary's hue so the coverage is readable as a list, and
 * chosen to put at least one primary in every 30 degree band the wall can
 * reach. The wall has nothing between 270 and 300 degrees, so neither does
 * this — that is a gap in the wall, and closing it means adding a swatch
 * there rather than inventing one here.
 *
 * Grounds alternate dark and pale on purpose. A set that grounded every cut
 * in Stage Black would spread the hues and flatten the quilt's value range
 * instead, which is the same mistake in the other channel.
 */
const WALL_CUTS = [
  ['punch', 'butterscotch', 'stage-black'],
  ['brick', 'chambray', 'parchment'],
  ['rust', 'seafoam', 'raw-cotton'],
  ['amber', 'ink-blue', 'charcoal'],
  ['mustard', 'petrol', 'flax'],
  ['goldenrod', 'merlot', 'charcoal'],
  ['moss', 'muslin-pink', 'espresso'],
  ['fern', 'peach', 'oatmeal'],
  ['bottle-green', 'lemon', 'aubergine'],
  ['seafoam', 'mulberry', 'stage-black'],
  ['spruce', 'coral', 'raw-cotton'],
  ['petrol', 'butterscotch', 'parchment'],
  ['workwear', 'safety-orange', 'oatmeal'],
  ['sky', 'brick', 'stage-black'],
  ['ink-blue', 'camel', 'dove'],
  ['violet', 'hi-vis', 'stage-black'],
  ['lilac', 'spruce', 'muslin-pink'],
  ['mulberry', 'pistachio', 'raw-cotton'],
];

const wallIndex = new Map(WALL.map((sw) => [sw.key, sw]));

/**
 * Loud on a bad key rather than quiet. A typo here would otherwise paint
 * `undefined` onto a tile, and the fabric wall's keys are the one thing
 * these cuts depend on that lives in another file.
 */
function swatch(key) {
  const sw = wallIndex.get(key);
  if (!sw) throw new Error(`quiltTheme: no fabric on the wall called "${key}"`);
  return sw;
}

/** camelCase key from the two fabrics that name the cut. Stored in
 *  node.appearance.palette once a patch pins one, so it never changes. */
function cutKey(primaryKey, secondaryKey) {
  const camel = (k) =>
    k.split('-').map((p, i) => (i ? p[0].toUpperCase() + p.slice(1) : p)).join('');
  return camel(primaryKey) + camel(secondaryKey)[0].toUpperCase() + camel(secondaryKey).slice(1);
}

const WALL_PALETTES = Object.fromEntries(
  WALL_CUTS.map(([p, s, b]) => {
    const primary = swatch(p);
    const secondary = swatch(s);
    const bg = swatch(b);
    const key = cutKey(p, s);
    return [
      key,
      {
        key,
        name: primary.name,
        subtitle: `${secondary.name} · ${bg.name}`,
        primary: primary.hex,
        secondary: secondary.hex,
        bg: bg.hex,
        colors: [primary.hex, secondary.hex, bg.hex],
      },
    ];
  }),
);

export const PALETTES = { ...ALBUM_PALETTES, ...WALL_PALETTES };

export const PALETTE_KEYS = Object.keys(PALETTES);

// --- HASH ---

function hashStr(s) {
  let h = 0;
  for (let i = 0; i < s.length; i++) {
    h = ((h << 5) - h + s.charCodeAt(i)) | 0;
  }
  return h;
}

// --- COLOR UTILITIES ---

function parseHex(hex) {
  return [
    parseInt(hex.slice(1, 3), 16),
    parseInt(hex.slice(3, 5), 16),
    parseInt(hex.slice(5, 7), 16),
  ];
}

function toHex(r, g, b) {
  return '#' + [r, g, b].map(v => Math.round(v).toString(16).padStart(2, '0')).join('');
}

/**
 * Blend colorB toward colorA by the given amount (0 = colorB unchanged, 1 = fully colorA).
 */
function blendToward(hexA, hexB, amount) {
  const [ar, ag, ab] = parseHex(hexA);
  const [br, bg, bb] = parseHex(hexB);
  return toHex(
    br + (ar - br) * amount,
    bg + (ag - bg) * amount,
    bb + (ab - bb) * amount,
  );
}

export function darken(hex, amount = 0.2) {
  if (!hex || hex.charAt(0) !== '#') return hex;
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  return `rgb(${Math.round(r * (1 - amount))},${Math.round(g * (1 - amount))},${Math.round(b * (1 - amount))})`;
}

/**
 * Pick readable text (ink or paper) for an arbitrary fill color.
 * WCAG relative luminance; threshold favors ink on mid-tones since
 * dark-on-color reads better than white-on-color at equal ratios.
 */
export function textOnColor(hex) {
  if (!hex || hex.charAt(0) !== '#' || hex.length < 7) return '#ffffff';
  const chan = (s) => {
    const c = parseInt(s, 16) / 255;
    return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
  };
  const lum =
    0.2126 * chan(hex.slice(1, 3)) +
    0.7152 * chan(hex.slice(3, 5)) +
    0.0722 * chan(hex.slice(5, 7));
  return lum > 0.18 ? '#151820' : '#ffffff';
}

// --- PALETTE FUNCTIONS ---

const bundleHexRe = /^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/;

/**
 * Get the colors a patch's tile draws with.
 * A bundle (appearance.bundle, 1-6 fabrics off the wall — docs/adr/029)
 * wins; else a pinned known palette (a pre-cut bundle); else deterministic
 * hash assignment. Slot 0 = primary = the identity color; slots feed the
 * drafted-block renderer.
 * @param {string} patchId
 * @param {object|null} appearance — node.appearance ({palette, block, rotation, bundle})
 */
export function paletteForPatch(patchId, appearance, { raw = false } = {}) {
  const base = chosenPaletteForPatch(patchId, appearance);
  if (raw || getColorMode() !== 'muted') return base;

  // One hue, the shared ramp, chroma capped (docs/adr/112). The *count* of
  // slots is carried over rather than filled out to six: muted may take
  // color away and may not add structure, so a patch whose bundle is one
  // fabric keeps drawing one flat fabric here too.
  const slots = mutedSlots(base.primary, base.slots.length);
  return {
    primary: slots[0],
    secondary: slots[1] || slots[0],
    bg: slots[2] || darken(slots[0], 0.55),
    slots,
    paletteKey: base.paletteKey,
  };
}

/** What the patch actually chose, before any viewer-side register. */
function chosenPaletteForPatch(patchId, appearance) {
  const bundle = Array.isArray(appearance?.bundle)
    ? appearance.bundle.filter((c) => typeof c === 'string' && bundleHexRe.test(c)).slice(0, 6)
    : [];
  if (bundle.length) {
    return {
      primary: bundle[0],
      secondary: bundle[1] || bundle[0],
      bg: bundle[2] || darken(bundle[0], 0.55),
      slots: bundle,
      paletteKey: null,
    };
  }
  const palette = PALETTES[appearance?.palette] || PALETTES[PALETTE_KEYS[Math.abs(hashStr(patchId)) % PALETTE_KEYS.length]];

  return {
    primary: palette.primary,
    secondary: palette.secondary,
    bg: palette.bg,
    slots: [palette.primary, palette.secondary, palette.bg],
    paletteKey: palette.key,
  };
}

/**
 * The single color that represents a patch anywhere it isn't drawn as a
 * full tile (card banners, quilt name badges): its palette primary.
 * @param {{id: string, appearance?: object|null}} patch
 */
export function identityColorForPatch(patch) {
  return paletteForPatch(patch.id, patch.appearance).primary;
}

/**
 * Get a palette for decorative/ghost tiles.
 */
export function ghostPalette(index) {
  const palette = PALETTES[PALETTE_KEYS[Math.abs(index * 7 + 3) % PALETTE_KEYS.length]];
  if (getColorMode() === 'muted') {
    const [primary, secondary, bg] = mutedSlots(palette.primary, 3);
    return { primary, secondary, bg };
  }
  return { primary: palette.primary, secondary: palette.secondary, bg: palette.bg };
}

/**
 * Get the primary color for a tag (used in tag chips, legend, etc.)
 * Colors tags, not patches — patches use identityColorForPatch.
 */
export function colorForTag(tag) {
  if (!tag) return '#7a7870';
  // Hash the tag to pick a palette, use its primary.
  const palette = PALETTES[PALETTE_KEYS[Math.abs(hashStr(tag)) % PALETTE_KEYS.length]];
  // Muted reaches every color that stands for something, and a forty-chip
  // tag cloud under a muted quilt is most of what a reader was looking at
  // (docs/adr/112). Tags are not patches, but they are not decoration either.
  return getColorMode() === 'muted' ? mutedColor(palette.primary) : palette.primary;
}

