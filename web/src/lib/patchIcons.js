/**
 * Motif registry — the curated set of marks a patch can carry beside its
 * name (quilt name badges, patch cards).
 *
 * A patch's motif resolves in order (see docs/adr/004 and docs/adr/021):
 *   1. appearance.icon, when it names a known motif (chosen by admins)
 *   2. the patch's first motif-bearing tag, in the patch admin's stored
 *      order — the mapping lives in the instance-curated vocabulary
 *      (tags.motif), loaded via setTagMotifs, never hardcoded here
 *   3. the quilt mark (SquaresFour) — same shape language as the brand
 * Unknown appearance.icon slugs (e.g. a foreign instance's custom motif in
 * a merged multi-quilt view) fall through to 2 and 3, never error.
 *
 * Icons come from the same Phosphor set as the rest of the UI. Quilt labels
 * are built imperatively (plain DOM, rebuilt on every zoom tick), so instead
 * of mounting a Svelte component per label we mount each icon component once,
 * extract its SVG markup, and cache it — after that, badges are cheap
 * innerHTML stamps.
 */
import { mount, unmount, flushSync } from 'svelte';
import {
  Palette,
  Images,
  MusicNotes,
  Buildings,
  MaskHappy,
  FilmSlate,
  BookOpen,
  ForkKnife,
  Scissors,
  UsersThree,
  GraduationCap,
  SoccerBall,
  Code,
  Heartbeat,
  PaintBrush,
  Stamp,
  Radio,
  SneakerMove,
  SquaresFour,
  Star,
  Heart,
  Guitar,
  MicrophoneStage,
  Camera,
  Coffee,
  Flower,
  Storefront,
  Megaphone,
  Bicycle,
  Skull,
  Lightning,
  VinylRecord,
  Butterfly,
  Hammer,
  Globe,
} from 'phosphor-svelte';

// The curated motif set, keyed by slug (the value stored in
// appearance.icon). Order is the picker's display order.
export const MOTIFS = {
  quilt: { key: 'quilt', name: 'Quilt mark', component: SquaresFour },
  palette: { key: 'palette', name: 'Palette', component: Palette },
  paintBrush: { key: 'paintBrush', name: 'Paint brush', component: PaintBrush },
  images: { key: 'images', name: 'Gallery', component: Images },
  camera: { key: 'camera', name: 'Camera', component: Camera },
  filmSlate: { key: 'filmSlate', name: 'Film slate', component: FilmSlate },
  musicNotes: { key: 'musicNotes', name: 'Music notes', component: MusicNotes },
  guitar: { key: 'guitar', name: 'Guitar', component: Guitar },
  micStage: { key: 'micStage', name: 'Microphone', component: MicrophoneStage },
  vinyl: { key: 'vinyl', name: 'Vinyl', component: VinylRecord },
  radio: { key: 'radio', name: 'Radio', component: Radio },
  maskHappy: { key: 'maskHappy', name: 'Theater mask', component: MaskHappy },
  sneaker: { key: 'sneaker', name: 'Dance', component: SneakerMove },
  bookOpen: { key: 'bookOpen', name: 'Open book', component: BookOpen },
  stamp: { key: 'stamp', name: 'Stamp', component: Stamp },
  scissors: { key: 'scissors', name: 'Scissors', component: Scissors },
  hammer: { key: 'hammer', name: 'Hammer', component: Hammer },
  buildings: { key: 'buildings', name: 'Venue', component: Buildings },
  storefront: { key: 'storefront', name: 'Storefront', component: Storefront },
  forkKnife: { key: 'forkKnife', name: 'Fork & knife', component: ForkKnife },
  coffee: { key: 'coffee', name: 'Coffee', component: Coffee },
  usersThree: { key: 'usersThree', name: 'People', component: UsersThree },
  megaphone: { key: 'megaphone', name: 'Megaphone', component: Megaphone },
  gradCap: { key: 'gradCap', name: 'Grad cap', component: GraduationCap },
  code: { key: 'code', name: 'Code', component: Code },
  soccerBall: { key: 'soccerBall', name: 'Ball', component: SoccerBall },
  bicycle: { key: 'bicycle', name: 'Bicycle', component: Bicycle },
  heartbeat: { key: 'heartbeat', name: 'Heartbeat', component: Heartbeat },
  flower: { key: 'flower', name: 'Flower', component: Flower },
  butterfly: { key: 'butterfly', name: 'Butterfly', component: Butterfly },
  globe: { key: 'globe', name: 'Globe', component: Globe },
  star: { key: 'star', name: 'Star', component: Star },
  skull: { key: 'skull', name: 'Skull', component: Skull },
  lightning: { key: 'lightning', name: 'Lightning', component: Lightning },
};

export const MOTIF_KEYS = Object.keys(MOTIFS);

// The pre-feature default: the quilt-grid mark.
const DEFAULT_MOTIF = 'quilt';

// Tag name → motif slug, fed from the instance's vocabulary (tags.motif)
// by loadTags(). Empty until the vocabulary loads; patches then render the
// quilt mark, which is the correct degraded state, not an error.
let tagMotifs = {};

/** Install the vocabulary's tag → motif mapping (called by loadTags). */
export function setTagMotifs(map) {
  tagMotifs = map || {};
}

/**
 * Resolve a patch's effective motif slug:
 * chosen (appearance.icon, if known) → first motif-bearing tag → quilt mark.
 * Tags arrive from the API in the patch admin's stored (priority) order.
 * @param {{tags?: string[], appearance?: {icon?: string}|null}} patch
 */
export function motifKeyForPatch(patch) {
  const chosen = patch?.appearance?.icon;
  if (chosen && MOTIFS[chosen]) return chosen;
  for (const tag of patch?.tags || []) {
    const slug = tagMotifs[tag];
    if (slug && MOTIFS[slug]) return slug;
  }
  return DEFAULT_MOTIF;
}

/** The Svelte component for a patch's effective motif. */
export function motifComponentForPatch(patch) {
  return MOTIFS[motifKeyForPatch(patch)].component;
}

const markupCache = new Map();

/** Render a Phosphor component once and cache its inner SVG markup. */
function iconMarkup(Component, weight) {
  const key = Component;
  if (!markupCache.has(key)) {
    const host = document.createElement('div');
    const instance = mount(Component, { target: host, props: { size: 16, weight } });
    flushSync();
    const svg = host.querySelector('svg');
    markupCache.set(key, svg ? svg.innerHTML : '');
    unmount(instance);
  }
  return markupCache.get(key);
}

/** Build a plain SVG element for a Phosphor icon (no live component). */
function buildIconSvg(Component, size, color, weight) {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('width', size);
  svg.setAttribute('height', size);
  svg.setAttribute('viewBox', '0 0 256 256');
  svg.setAttribute('fill', color);
  svg.style.flexShrink = '0';
  svg.innerHTML = iconMarkup(Component, weight);
  return svg;
}

/**
 * Create an inline SVG element for a patch's motif.
 * Returns an SVG DOM element ready to append.
 */
export function createMotifElement(patch, size = 12, color = '#fff') {
  return buildIconSvg(motifComponentForPatch(patch), size, color, 'fill');
}

/**
 * Role mark (CONTEXT.md): the quilt name badge's belonging indicator.
 * Gold star = admin or member (belonging). Never shown for a follow —
 * see createFollowedHeart below for that case.
 */
export function createMyPatchStar(size = 12) {
  return buildIconSvg(Star, size, '#D4A843', 'fill');
}

/**
 * Role mark (CONTEXT.md): small filled heart marking a patch the user
 * follows (but doesn't belong to) on quilt name badges. Distinct from the star —
 * a follow is never rendered as belonging.
 */
export function createFollowedHeart(size = 12) {
  return buildIconSvg(Heart, size, '#C02624', 'fill');
}
