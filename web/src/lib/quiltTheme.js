/**
 * Quilt palettes (inspired by punk rock album art).
 *
 * Each patch's tile is drawn with a palette: chosen by its admins via the
 * Patch Appearance settings (stored in node.appearance.palette), or
 * hash-assigned from the patch ID when unset. Unknown palette keys fall
 * back to hash assignment — see docs/adr/004.
 *
 * Palettes from: https://joetamponi.com/blog/8-awesome-inspirational-color-palettes-from-punk-rock-records
 */

// --- PALETTES ---
// Each has 3 colors for quilt blocks: primary, secondary, bg.
// Plus a `colors` array of all source colors for the appearance picker UI.

export const PALETTES = {
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
export function paletteForPatch(patchId, appearance) {
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
  return palette.primary;
}

