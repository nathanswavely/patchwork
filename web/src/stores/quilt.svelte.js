/**
 * Shared quilt state — used by QuiltCanvas, sidebar panels, and overlays.
 * Extracted from Home.svelte so it's accessible across the app.
 */
import { api } from '../lib/api.js';
import { setTagMotifs } from '../lib/patchIcons.js';

// --- Instance info ---
let instanceName = $state('Patchwork');
let instanceDescription = $state('');
let instanceModules = $state({ map: true, governance: true, ledger: false });
let instanceSubmissionsEnabled = $state(true);
let instanceIconUrl = $state('');
let instanceStats = $state({ node_count: 0, event_count: 0, member_count: 0 });
let instanceLoaded = $state(false);
// Neighbor quilts (docs/adr/024): the instance's public adjacency list,
// shown to every visitor in the quilt switcher.
let neighborQuilts = $state([]);
let instanceTimezone = $state('');
let instanceDomain = $state('');
// Whether this quilt federates (docs/adr/059). Gates the patch handle in
// Subscribe: without it the actor endpoints are not mounted, so the
// address would resolve to nothing.
let instanceFederation = $state(false);
// Whether this quilt can send mail (docs/adr/071). Signup reads it to decide
// which floor a new account gets — an emailable address, or recovery codes —
// and the sign-in page to decide whether offering to email a link is honest.
// Defaults true so a failed /instance fetch still shows the email option
// rather than hiding the only door someone may have.
let instanceEmailEnabled = $state(true);

// --- Filter state (docs/adr/033) ---
// The filter — tag selection plus the search chip — is standing state that
// narrows every discovery surface. It is set only by explicit acts (chip
// toggles, the search dropdown's "Show matches on the quilt" row), announced
// by the on-surface filter chips wherever it bites, and survives navigation.
// Reload is the only reset: module state, deliberately not persisted.

let allTags = $state([]);
// Full vocabulary rows [{id, name, motif, node_count}] — the tag pickers
// and the admin vocabulary page need more than names.
let tagVocabulary = $state([]);
let tagCounts = $state({}); // tag name -> public patch count, from GET /api/v1/tags
// Whether the vocabulary round-trip has resolved. An empty `allTags` means
// two different things before and after it does — "this quilt curates no
// tags" and "we haven't asked yet" — and a surface that branches on the
// vocabulary needs to tell them apart. Discovery mode shipped straight to
// its answer for exactly this reason: it read the pre-load emptiness as a
// quilt with no question to ask.
let tagsLoaded = $state(false);
let selectedTags = $state([]);
let searchQuery = $state('');

// --- Getters ---
export function getInstanceName() { return instanceName; }
export function getInstanceDescription() { return instanceDescription; }
export function getNeighborQuilts() { return neighborQuilts; }
export function getInstanceDomain() { return instanceDomain; }
export function getInstanceFederation() { return instanceFederation; }
export function getEmailEnabled() { return instanceEmailEnabled; }
export function getInstanceIconUrl() { return instanceIconUrl; }
export function getInstanceModules() { return instanceModules; }
export function getInstanceStats() { return instanceStats; }
export function isInstanceLoaded() { return instanceLoaded; }
export function getSubmissionsEnabled() { return instanceSubmissionsEnabled; }
// Where this quilt keeps time (docs/adr/045): the rung an event's zone
// falls through to when neither it nor its patch names one. Read by the
// event form so a new event defaults to the quilt's clock rather than
// the organizer's laptop.
export function getInstanceTimezone() { return instanceTimezone; }
export function getAllTags() { return allTags; }
export function getTagVocabulary() { return tagVocabulary; }
export function getTagCounts() { return tagCounts; }
export function areTagsLoaded() { return tagsLoaded; }
export function getSelectedTags() { return selectedTags; }

// Tags in usage order (docs/adr/075): the most-worn on this quilt first,
// unworn ones alphabetical after. This is the whole suggestion engine —
// what this quilt wears, identical for every viewer including anonymous
// ones, and verifiable in one click ("nine patches wear this"). Never what
// other people follow, which at community scale is computed from a handful
// of humans and discloses the memberships docs/adr/006 protects.
//
// Raw vocabulary order was never a decision; both discovery mode and the
// filter chips read this instead.
export function getRankedTags() {
  return [...allTags].sort(
    (a, b) => (tagCounts[b] || 0) - (tagCounts[a] || 0) || a.localeCompare(b)
  );
}
export function getSearchQuery() { return searchQuery; }

// --- Setters ---
export function setSelectedTags(tags) { selectedTags = tags; }
export function setSearchQuery(q) { searchQuery = q; }

export function toggleTag(tag) {
  if (selectedTags.includes(tag)) {
    selectedTags = selectedTags.filter(t => t !== tag);
  } else {
    selectedTags = [...selectedTags, tag];
  }
}

export function clearTags() {
  selectedTags = [];
}

// Drops the whole filter at once — the chips' Clear button.
export function resetFilters() {
  selectedTags = [];
  searchQuery = '';
}

// How many chips are active — the collapsed button's badge count. The
// search chip counts as one: it narrows like any tag.
export function getActiveFilterCount() {
  return selectedTags.length + (searchQuery.trim() ? 1 : 0);
}

// --- Order and the in-view lens (docs/adr/074) ---
// Two things the cards list owns. They sit beside the filter because they are
// the same kind of state — standing, session-ephemeral, never in the URL —
// but neither is part of the filter, and clearing the filter leaves both
// alone: touching one lens never changes another (docs/adr/022).

// Order is not narrowing; it decides what comes first, not what is shown.
// Quilt order — the layout engine's own placement order, centre-out — is the
// default. A→Z is for looking a name up rather than finding something, a job
// the search dropdown already does better. Recently added answers "what is
// new here" from `activated_at`, which is when a community arrived rather
// than when a row was written (docs/adr/076).
let listOrder = $state('quilt'); // 'quilt' | 'alpha' | 'recent'

// The in-view lens narrows the list to the patches inside the canvas's
// viewport. The first surface-local lens: it bites on the quilt and the map
// and has no meaning on the events page, so unlike the filter it does not
// travel. Off until someone turns it on — the canvas zoom-fits at rest, so an
// always-on binding would look like it was doing nothing until it suddenly
// wasn't. Deliberately not addressable: quilt space is re-sewn as membership
// changes, so a saved viewport would come to mean different patches.
let inViewOnly = $state(false);

export function getListOrder() { return listOrder; }
export function setListOrder(order) { listOrder = order; }
export function getInViewOnly() { return inViewOnly; }
export function setInViewOnly(on) { inViewOnly = !!on; }
export function toggleInViewOnly() { inViewOnly = !inViewOnly; }

// --- Chip collapse preference (docs/adr/033) ---
// One shared preference across every chips home (canvas overlay, top of the
// events page): a person is a chips-open or chips-collapsed person, and the
// interface never disagrees with itself about which. Defaults open on
// desktop, closed on mobile. The mobile canvas sheet is exempt — a sheet is
// open-while-using, never a preference.
const CHIPS_KEY = 'patchwork-filter-chips-collapsed';
// null = no stored preference; the default falls back to the viewport width.
let chipsCollapsed = $state(
  localStorage.getItem(CHIPS_KEY) != null
    ? localStorage.getItem(CHIPS_KEY) === '1'
    : null
);

// Viewport width, tracked reactively so the collapse default recomputes on
// resize. Reading window.innerWidth directly inside getChipsCollapsed()
// wouldn't work: it's not a reactive dependency, so a $derived over it would
// freeze at whatever width the component first rendered at (docs/adr/033).
let viewportWidth = $state(typeof window !== 'undefined' ? window.innerWidth : 1024);
if (typeof window !== 'undefined') {
  window.addEventListener('resize', () => { viewportWidth = window.innerWidth; });
}

export function getChipsCollapsed() {
  return chipsCollapsed ?? viewportWidth < 768;
}
export function setChipsCollapsed(collapsed) {
  chipsCollapsed = collapsed;
  localStorage.setItem(CHIPS_KEY, collapsed ? '1' : '0');
}

// --- The cards pane's width (docs/adr/111) ---
// How wide the list beside a discovery surface is, at three stops the reader
// sets: two columns, one column, hidden. The stop names how many cards sit
// side by side rather than a percentage, because the pane exists to hold
// cards and that is the only unit a reader can see. Hidden is the width's
// zero, not a second concept — one fact, one control, one stored value.
//
// It sits here beside the chips rather than in the surface because the
// *shell* needs it too: the filter chips clear the pane's right edge, and
// they live in SocialShell, which has no other reason to know the pane
// exists. Until this, 45% was a literal in three places across two files.
//
// An arrangement, not a lens: it persists, the way the collapsed rail and
// the collapsed chips do and the way the filter, the order and the in-view
// lens deliberately do not (docs/adr/022, docs/adr/074).
const PANE_KEY = 'patchwork-cards-pane-stop';
export const PANE_STOPS = ['two', 'one', 'hidden'];

// One column is `22.5% + 10px` — the width at which a single card is as wide
// as each of the two were, given the pane's 16px side padding and the grid's
// 12px gap. So changing stops hands back half the pane and leaves the card's
// size alone: what the reader gains is quilt, not a bigger card.
//
// Half a scrollbar out in practice (295px vs 287px on Windows at 1440),
// because the scrollbar sits inside the pane and is subtracted once either
// way. Deliberately not corrected: the exact term is `+ scrollbar/2`, and
// the scrollbar is 15–17px on Windows, zero under macOS overlay scrollbars,
// and only there while the list overflows — measuring it would make the
// pane's width depend on how many patches this quilt happens to have.
const PANE_CSS = { two: '45%', one: 'calc(22.5% + 10px)', hidden: '0px' };
const PANE_COLUMNS = { two: 2, one: 1, hidden: 1 };

const storedStop = PANE_STOPS.includes(localStorage.getItem(PANE_KEY))
  ? localStorage.getItem(PANE_KEY)
  : 'two';

let paneStop = $state(storedStop);
// The stop a docked profile borrows, and the one the cycle returns to from
// hidden. Never 'hidden' — hidden is where you come back *from*, so a reader
// who left the pane hidden last session returns to two columns.
let lastOpenStop = $state(storedStop === 'hidden' ? 'two' : storedStop);

export function getPaneStop() { return paneStop; }
export function getLastOpenPaneStop() { return lastOpenStop; }
export function paneWidthCSS(stop) { return PANE_CSS[stop] ?? PANE_CSS.two; }
export function paneColumns(stop) { return PANE_COLUMNS[stop] ?? 2; }

// The fraction of the window the pane covers — what the canvases take as
// `insetRight`. Derived rather than stored so it can never drift from the
// CSS width above; the 10px is the same 10px.
export function paneFraction(stop, winW) {
  if (stop === 'hidden' || !winW) return 0;
  if (stop === 'one') return Math.min(0.5, 0.225 + 10 / winW);
  return 0.45;
}

export function setPaneStop(stop) {
  if (!PANE_STOPS.includes(stop)) return;
  paneStop = stop;
  if (stop !== 'hidden') lastOpenStop = stop;
  localStorage.setItem(PANE_KEY, stop);
}

// Two columns -> one column -> hidden -> two columns. The chevron points the
// way the pane's edge is about to travel, so the control never has to be
// read to be understood.
export function cyclePaneStop() {
  setPaneStop(PANE_STOPS[(PANE_STOPS.indexOf(paneStop) + 1) % PANE_STOPS.length]);
}

// --- Loaders ---
export async function loadInstance() {
  if (instanceLoaded) return;
  try {
    const data = await api('instance');
    if (data?.name) {
      instanceName = data.name;
      document.title = data.name;
    }
    if (data?.description !== undefined) instanceDescription = data.description;
    if (data?.modules) instanceModules = data.modules;
    if (data?.submissions_enabled !== undefined) instanceSubmissionsEnabled = data.submissions_enabled;
    if (data?.icon_url) instanceIconUrl = data.icon_url;
    if (data?.stats) instanceStats = data.stats;
    if (data?.branding?.color) {
      document.documentElement.style.setProperty('--color-primary', data.branding.color);
    }
    if (data?.neighbor_quilts) neighborQuilts = data.neighbor_quilts;
    if (data?.domain) instanceDomain = data.domain;
    if (data?.federation !== undefined) instanceFederation = data.federation;
    if (data?.email_enabled !== undefined) instanceEmailEnabled = data.email_enabled;
    if (data?.geography?.timezone) instanceTimezone = data.geography.timezone;
    instanceLoaded = true;
  } catch { /* keep defaults */ }
}

// Called after the admin changes name or icon so open surfaces (scope
// switcher, document title) refresh without a reload. The timestamp param
// busts the icon's short-lived HTTP cache.
export function applyIdentityChange({ name } = {}) {
  if (name) {
    instanceName = name;
    document.title = name;
  }
  instanceIconUrl = `/api/v1/instance/icon?t=${Date.now()}`;
}

export async function loadTags() {
  try {
    const data = await api('tags');
    const rows = (data.items || data || []).map(t =>
      typeof t === 'string' ? { name: t } : t
    );
    tagVocabulary = rows;
    allTags = rows.map(t => t.name);
    // Feed the vocabulary's tag → motif mapping to the motif resolver
    // (docs/adr/021: the mapping is data, not frontend code), and keep
    // per-tag public patch counts for usage-ranked surfaces.
    const motifs = {};
    const counts = {};
    for (const t of rows) {
      if (t.motif) motifs[t.name] = t.motif;
      counts[t.name] = t.node_count || 0;
    }
    setTagMotifs(motifs);
    tagCounts = counts;
  } catch { allTags = []; tagVocabulary = []; tagCounts = {}; }
  finally { tagsLoaded = true; }
}
