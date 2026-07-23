import { describe, it, expect, beforeEach } from 'vitest';
import {
  setSearchQuery,
  setSelectedTags,
  getSearchQuery,
  getSelectedTags,
  getActiveFilterCount,
  resetFilters,
  getChipsCollapsed,
  setChipsCollapsed,
} from '../stores/quilt.svelte.js';

// The filter doctrine (docs/adr/033): tags + search chip are standing state,
// set only explicitly, announced by on-surface chips, cleared in one step.
describe('filter state', () => {
  beforeEach(() => {
    resetFilters();
  });

  it('clears both the tag selection and the search chip in one step', () => {
    setSearchQuery('gallery');
    setSelectedTags(['music', 'venue']);
    expect(getSearchQuery()).toBe('gallery');
    expect(getSelectedTags()).toEqual(['music', 'venue']);

    resetFilters();

    expect(getSearchQuery()).toBe('');
    expect(getSelectedTags()).toEqual([]);
  });

  it('counts the search chip as one active chip', () => {
    expect(getActiveFilterCount()).toBe(0);
    setSelectedTags(['music', 'venue']);
    expect(getActiveFilterCount()).toBe(2);
    setSearchQuery('zine');
    expect(getActiveFilterCount()).toBe(3);
    setSearchQuery('   ');
    expect(getActiveFilterCount()).toBe(2);
  });

  it('persists the chip collapse preference', () => {
    setChipsCollapsed(true);
    expect(getChipsCollapsed()).toBe(true);
    expect(localStorage.getItem('patchwork-filter-chips-collapsed')).toBe('1');
    setChipsCollapsed(false);
    expect(getChipsCollapsed()).toBe(false);
    expect(localStorage.getItem('patchwork-filter-chips-collapsed')).toBe('0');
  });
});
