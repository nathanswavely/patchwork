/**
 * Which markers on the map get to wear their name (docs/adr/078).
 *
 * A marker is a point: it has no area, so the quilt's size gate
 * (`LABEL_MIN_PX`, and the roomy ramp that softens it) has no analogue here
 * and is deliberately not copied. What does port is the other half of the
 * quilt's badge engine — a gap a newcomer must clear, and a smaller one an
 * incumbent is forgiven, so a name already on screen doesn't blink off and
 * back on while somebody pinches.
 *
 * Zoom never appears below. Zoom is only what moves markers apart; the rule
 * is separation, which also settles the case zoom cannot — two patches at
 * one address never separate however far in you go.
 */

// Clear space a newcomer owes every name already placed.
export const LABEL_GAP = 14;
// What an incumbent owes instead. Smaller, so the name you are reading
// survives its neighbour drifting a few pixels closer.
export const LABEL_KEEP_GAP = 8;

// Where a name may sit relative to its marker's anchor (the teardrop's
// tip), in the order it is tried. Right of the pin reads best — it is where
// the eye goes and it keeps the name clear of the pin's own shadow — but a
// name that cannot go right is worth having on the left, above, or below.
// One name in a crowd is worth more than a tidy rule.
const LABEL_HEIGHT = 15;
// `dir` and `offset` are how Leaflet is told to draw it; dx/dy/align are how
// the box is measured here. They describe the same placement.
const LABEL_POSITIONS = [
  { dx: 18, dy: -22, align: 'left', dir: 'right', offset: [3, -22] },
  { dx: -18, dy: -22, align: 'right', dir: 'left', offset: [-3, -22] },
  { dx: 0, dy: -40, align: 'center', dir: 'top', offset: [0, -34] },
  { dx: 0, dy: 12, align: 'center', dir: 'bottom', offset: [0, 2] },
];

// The teardrop, as an obstacle: 26 x 34 standing on its anchor point.
const MARKER_W = 26;
const MARKER_H = 34;

// Text measurement is the expensive part and names don't change, so each is
// measured once per font and remembered.
let measureCtx = null;
const widths = new Map();

function textWidth(name, font) {
  const key = `${font} ${name}`;
  const cached = widths.get(key);
  if (cached !== undefined) return cached;
  measureCtx ||= document.createElement('canvas').getContext('2d');
  measureCtx.font = font;
  const w = measureCtx.measureText(name).width;
  widths.set(key, w);
  return w;
}

function overlaps(a, b, pad) {
  return !(a.right + pad < b.left
    || b.right + pad < a.left
    || a.bottom + pad < b.top
    || b.bottom + pad < a.top);
}

/**
 * Decide the labelled set.
 *
 * `groups` must arrive in priority order — heaviest patch first, which is
 * what clusterNodes already returns. Clusters are never candidates: a
 * cluster has as many names as it has members, so it carries none.
 *
 * Every marker and cluster is an obstacle before any name is placed. A name
 * that lands on a teardrop or a count disc is worse than a name withheld:
 * it hides the very thing it was describing, and the disc it covers belongs
 * to a patch that never agreed to give up its pin.
 *
 * `visible` bounds the area a name may occupy — the map minus whatever the
 * cards pane covers, since a name under the pane is a name nobody reads.
 *
 * `incumbents` is the previous pass's result; membership buys the smaller
 * gap, nothing else. Returns a Map of id → the position its name took, which
 * the caller renders from and hands back next time.
 */
export function placeLabels(groups, project, font, incumbents = new Set(), visible = null) {
  const obstacles = [];
  const candidates = [];

  for (const group of groups) {
    const pt = project(group.latlng);
    if (group.members.length > 1) {
      // A count disc, centred on its point.
      const r = (group.members.length > 20 ? 44 : group.members.length > 9 ? 38 : 32) / 2;
      obstacles.push({ left: pt.x - r, right: pt.x + r, top: pt.y - r, bottom: pt.y + r });
      continue;
    }
    obstacles.push({
      left: pt.x - MARKER_W / 2,
      right: pt.x + MARKER_W / 2,
      top: pt.y - MARKER_H,
      bottom: pt.y,
    });
    candidates.push({ node: group.lead, pt });
  }

  const placed = [];
  const chosen = new Map();

  for (const { node, pt } of candidates) {
    const w = textWidth(node.name, font);
    const gap = incumbents.has(node.id) ? LABEL_KEEP_GAP : LABEL_GAP;

    for (const pos of LABEL_POSITIONS) {
      const left = pos.align === 'left' ? pt.x + pos.dx
        : pos.align === 'right' ? pt.x + pos.dx - w
        : pt.x - w / 2;
      const box = {
        left,
        right: left + w,
        top: pt.y + pos.dy - LABEL_HEIGHT / 2,
        bottom: pt.y + pos.dy + LABEL_HEIGHT / 2,
      };

      if (visible && (box.right > visible.right || box.left < visible.left
        || box.top < visible.top || box.bottom > visible.bottom)) continue;

      // Features are absolute: a name never sits on a marker or a disc.
      if (obstacles.some((o) => overlaps(box, o, 2))) continue;
      if (placed.some((other) => overlaps(box, other, gap))) continue;

      placed.push(box);
      chosen.set(node.id, pos);
      break;
    }
  }

  return chosen;
}
