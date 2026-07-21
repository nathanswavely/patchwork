/**
 * Unit tests for the map-location pure helpers (issue #4).
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  hasMapLocation,
  isValidCoord,
  formatCoord,
  roundCoord,
} from '../lib/mapLocation.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('hasMapLocation', () => {
  it('is true only when both coordinates are present and finite', () => {
    expect(hasMapLocation(40.03, -76.3)).toBe(true);
    expect(hasMapLocation(0, 0)).toBe(true); // null island is a real point
  });

  it('is false when either coordinate is missing', () => {
    expect(hasMapLocation(null, -76.3)).toBe(false);
    expect(hasMapLocation(40.03, null)).toBe(false);
    expect(hasMapLocation(null, null)).toBe(false);
    expect(hasMapLocation(undefined, undefined)).toBe(false);
  });
});

describe('isValidCoord', () => {
  it('accepts in-range pairs including the exact bounds', () => {
    expect(isValidCoord(40.0379, -76.3055)).toBe(true);
    expect(isValidCoord(90, 180)).toBe(true);
    expect(isValidCoord(-90, -180)).toBe(true);
  });

  it('rejects out-of-range coordinates', () => {
    expect(isValidCoord(90.1, 0)).toBe(false);
    expect(isValidCoord(-90.1, 0)).toBe(false);
    expect(isValidCoord(0, 180.1)).toBe(false);
    expect(isValidCoord(0, -180.1)).toBe(false);
  });

  it('rejects non-finite values', () => {
    expect(isValidCoord(NaN, 0)).toBe(false);
    expect(isValidCoord(0, Infinity)).toBe(false);
  });
});

describe('formatCoord', () => {
  it('formats to five decimals', () => {
    expect(formatCoord(40.037912, -76.305512)).toBe('40.03791, -76.30551');
  });

  it('returns empty string when unset', () => {
    expect(formatCoord(null, null)).toBe('');
    expect(formatCoord(40.03, null)).toBe('');
  });
});

describe('roundCoord', () => {
  it('rounds to five decimals', () => {
    expect(roundCoord(40.0379123)).toBe(40.03791);
    expect(roundCoord(-76.3055987)).toBe(-76.3056);
  });
});

// Wiring guard: the settings info page must send latitude/longitude on the
// map-location save, and an explicit null pair on remove.
describe('PatchSettingsInfo map-location wiring', () => {
  const src = source('pages/PatchSettingsInfo.svelte');

  it('saves latitude/longitude with an explicit save step', () => {
    expect(src).toContain('MapLocationPicker');
    expect(src).toMatch(/body:\s*\{\s*latitude:\s*lat,\s*longitude:\s*lng\s*\}/);
  });

  it('clears the position with null coordinates on remove', () => {
    expect(src).toMatch(/latitude:\s*null,\s*longitude:\s*null/);
  });
});
