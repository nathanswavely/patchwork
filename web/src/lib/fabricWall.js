/**
 * The fabric wall: the one curated set of swatches every bundle draws
 * from (docs/adr/029, CONTEXT.md "Fabric wall"). Users pick fabrics off
 * the wall — there is no free color picker. The wall is seeded from the
 * classic album-art palettes (quiltTheme.js) and filled out to cover the
 * hue wheel in the same punk-poster register, so any combination
 * coexists on the quilt.
 *
 * Bundles store hex values, not wall keys, so foreign quilts with a
 * different wall still render (the backend never validates membership —
 * docs/adr/004). Wall keys exist for the picker UI; nearestSwatch maps a
 * stored hex back to a wall entry for display.
 */

export const WALL = [
  // Neutrals
  { key: 'raw-cotton', name: 'Raw Cotton', hex: '#F2EEE4' },
  { key: 'parchment', name: 'Parchment', hex: '#EFF1CE' },
  { key: 'oatmeal', name: 'Oatmeal', hex: '#E4E5C4' },
  { key: 'flax', name: 'Flax', hex: '#D9D6AF' },
  { key: 'muslin-pink', name: 'Muslin Pink', hex: '#F5CEC2' },
  { key: 'dove', name: 'Dove', hex: '#C9C2B2' },
  { key: 'stone', name: 'Stone', hex: '#8A8273' },
  { key: 'taupe', name: 'Taupe', hex: '#625749' },
  { key: 'charcoal', name: 'Charcoal', hex: '#2C2D29' },
  { key: 'stage-black', name: 'Stage Black', hex: '#0A0A0A' },
  // Browns
  { key: 'espresso', name: 'Espresso', hex: '#2D2619' },
  { key: 'chestnut', name: 'Chestnut', hex: '#5A2517' },
  { key: 'saddle', name: 'Saddle', hex: '#753C1E' },
  { key: 'rust', name: 'Rust', hex: '#A0430A' },
  { key: 'caramel', name: 'Caramel', hex: '#B1752C' },
  { key: 'camel', name: 'Camel', hex: '#D9A066' },
  // Reds
  { key: 'oxblood', name: 'Oxblood', hex: '#952117' },
  { key: 'brick', name: 'Brick', hex: '#C02624' },
  { key: 'scarlet', name: 'Scarlet', hex: '#EC341C' },
  { key: 'tomato', name: 'Tomato', hex: '#E85D4A' },
  { key: 'coral', name: 'Coral', hex: '#EF7674' },
  { key: 'merlot', name: 'Merlot', hex: '#7A1F2B' },
  // Pinks
  { key: 'punch', name: 'Punch', hex: '#DA0956' },
  { key: 'bubblegum', name: 'Bubblegum', hex: '#E7658E' },
  { key: 'rose', name: 'Rose', hex: '#E1A4BC' },
  { key: 'magenta', name: 'Magenta', hex: '#B23282' },
  { key: 'mulberry', name: 'Mulberry', hex: '#8E2F5C' },
  { key: 'peach', name: 'Peach', hex: '#F7B79B' },
  // Purples
  { key: 'violet', name: 'Violet', hex: '#6A3FA0' },
  { key: 'plum', name: 'Plum', hex: '#4A2C6F' },
  { key: 'aubergine', name: 'Aubergine', hex: '#261922' },
  { key: 'lilac', name: 'Lilac', hex: '#9B7EBD' },
  // Blues
  { key: 'navy', name: 'Navy', hex: '#16324F' },
  { key: 'ink-blue', name: 'Ink Blue', hex: '#3A4E8A' },
  { key: 'workwear', name: 'Workwear', hex: '#1493CC' },
  { key: 'sky', name: 'Sky', hex: '#039BE6' },
  { key: 'periwinkle', name: 'Periwinkle', hex: '#7690C1' },
  { key: 'faded-denim', name: 'Faded Denim', hex: '#88ABD1' },
  { key: 'chambray', name: 'Chambray', hex: '#9FC3DA' },
  { key: 'petrol', name: 'Petrol', hex: '#0F4C5C' },
  // Teals & greens
  { key: 'spruce', name: 'Spruce', hex: '#204B4B' },
  { key: 'bottle-green', name: 'Bottle Green', hex: '#2E7D5B' },
  { key: 'seafoam', name: 'Seafoam', hex: '#94CDBE' },
  { key: 'sage', name: 'Sage', hex: '#81AE7F' },
  { key: 'fern', name: 'Fern', hex: '#5E8258' },
  { key: 'olive', name: 'Olive', hex: '#3E5622' },
  { key: 'moss', name: 'Moss', hex: '#94AC0E' },
  { key: 'slime', name: 'Slime', hex: '#88DE16' },
  { key: 'pistachio', name: 'Pistachio', hex: '#C5D86D' },
  // Yellows & oranges
  { key: 'lemon', name: 'Lemon', hex: '#F5EB06' },
  { key: 'hi-vis', name: 'Hi-Vis', hex: '#FCFD1B' },
  { key: 'goldenrod', name: 'Goldenrod', hex: '#F4CD2E' },
  { key: 'mustard', name: 'Mustard', hex: '#D89E13' },
  { key: 'butterscotch', name: 'Butterscotch', hex: '#F6C87F' },
  { key: 'amber', name: 'Amber', hex: '#E88D14' },
  { key: 'safety-orange', name: 'Safety Orange', hex: '#E3480B' },
];

function rgb(hex) {
  return [
    parseInt(hex.slice(1, 3), 16),
    parseInt(hex.slice(3, 5), 16),
    parseInt(hex.slice(5, 7), 16),
  ];
}

/**
 * The wall entry closest to a hex color (RGB distance). Used to display
 * which swatch a stored bundle color corresponds to; never used to
 * rewrite the stored value.
 */
export function nearestSwatch(hex) {
  if (typeof hex !== 'string' || hex.length < 7 || hex[0] !== '#') return null;
  const [r, g, b] = rgb(hex);
  let best = null;
  let bestD = Infinity;
  for (const sw of WALL) {
    const [wr, wg, wb] = rgb(sw.hex);
    const d = (r - wr) ** 2 + (g - wg) ** 2 + (b - wb) ** 2;
    if (d < bestD) {
      bestD = d;
      best = sw;
    }
  }
  return best;
}
