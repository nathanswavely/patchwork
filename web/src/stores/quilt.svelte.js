/**
 * Shared quilt state — used by QuiltCanvas, sidebar panels, and overlays.
 * Extracted from Home.svelte so it's accessible across the app.
 */
import { api } from '../lib/api.js';
import { setTagMotifs } from '../lib/patchIcons.js';

// --- Instance info ---
let instanceName = $state('Patchwork');
let instanceModules = $state({ map: true, governance: true, ledger: false });
let instanceSubmissionsEnabled = $state(true);
let instanceIconUrl = $state('');
let instanceLoaded = $state(false);
// Neighbor quilts (docs/adr/024): the instance's public adjacency list,
// shown to every visitor in the quilt switcher.
let neighborQuilts = $state([]);
let instanceDomain = $state('');

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
let selectedTags = $state([]);
let searchQuery = $state('');

// --- Getters ---
export function getInstanceName() { return instanceName; }
export function getNeighborQuilts() { return neighborQuilts; }
export function getInstanceDomain() { return instanceDomain; }
export function getInstanceIconUrl() { return instanceIconUrl; }
export function getInstanceModules() { return instanceModules; }
export function getSubmissionsEnabled() { return instanceSubmissionsEnabled; }
export function getAllTags() { return allTags; }
export function getTagVocabulary() { return tagVocabulary; }
export function getTagCounts() { return tagCounts; }
export function getSelectedTags() { return selectedTags; }
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

// --- Chip collapse preference (docs/adr/033) ---
// One shared preference across every chips home (canvas overlay, top of the
// events page): a person is a chips-open or chips-collapsed person, and the
// interface never disagrees with itself about which. Defaults open on
// desktop, closed on mobile. The mobile canvas sheet is exempt — a sheet is
// open-while-using, never a preference.
const CHIPS_KEY = 'patchwork-filter-chips-collapsed';
// null = no stored preference; the default is computed at read time, not
// module-load time — the viewport may not have real dimensions yet when
// this module first evaluates.
let chipsCollapsed = $state(
  localStorage.getItem(CHIPS_KEY) != null
    ? localStorage.getItem(CHIPS_KEY) === '1'
    : null
);

export function getChipsCollapsed() {
  return chipsCollapsed ?? window.innerWidth < 768;
}
export function setChipsCollapsed(collapsed) {
  chipsCollapsed = collapsed;
  localStorage.setItem(CHIPS_KEY, collapsed ? '1' : '0');
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
    if (data?.modules) instanceModules = data.modules;
    if (data?.submissions_enabled !== undefined) instanceSubmissionsEnabled = data.submissions_enabled;
    if (data?.icon_url) instanceIconUrl = data.icon_url;
    if (data?.branding?.color) {
      document.documentElement.style.setProperty('--color-primary', data.branding.color);
    }
    if (data?.neighbor_quilts) neighborQuilts = data.neighbor_quilts;
    if (data?.domain) instanceDomain = data.domain;
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
}
