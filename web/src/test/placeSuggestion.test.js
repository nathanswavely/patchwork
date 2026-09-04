/**
 * A gazetteer suggests; a person places (docs/adr/082).
 *
 * The behaviour these guard is a *refusal*: the suggestion must not become a
 * saved coordinate on its own. Nothing in this project renders Svelte in a
 * test, so the wiring assertions read source text — the browser check that a
 * confirmed placement posts coordinates and an unconfirmed one posts none is
 * the other half, and neither replaces the other.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { suggestPlace, worthLookingUp } from '../lib/placeSuggestion.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

beforeEach(() => {
  global.fetch = vi.fn();
});
afterEach(() => {
  vi.restoreAllMocks();
});

function respond(body) {
  global.fetch.mockResolvedValue({
    ok: true,
    status: 200,
    headers: { get: () => 'application/json' },
    json: async () => body,
  });
}

describe('suggestPlace', () => {
  it('returns the place when the gazetteer found one', async () => {
    respond({
      found: true,
      available: true,
      label: 'The Selvage, North Prince Street, Lancaster',
      place: { name: 'The Selvage', latitude: 40.0392, longitude: -76.305 },
    });
    const got = await suggestPlace('The Selvage, Lancaster');
    expect(got).toEqual({
      latitude: 40.0392,
      longitude: -76.305,
      label: 'The Selvage, North Prince Street, Lancaster',
    });
  });

  it('returns null on a miss, which is an ordinary answer', async () => {
    respond({ found: false, available: true });
    expect(await suggestPlace('above the record shop on Prince St')).toBeNull();
  });

  it('returns null where no gazetteer is installed', async () => {
    respond({ found: false, available: false });
    expect(await suggestPlace('433 Ice Avenue, Lancaster')).toBeNull();
  });

  // An outage must look exactly like a polite miss. The person is placing a
  // marker by hand either way, and a red toast over an optional convenience
  // would be the feature making itself somebody's problem.
  it('swallows a failing request rather than surfacing an error', async () => {
    global.fetch.mockRejectedValue(new Error('network down'));
    await expect(suggestPlace('433 Ice Avenue')).resolves.toBeNull();
  });

  it('refuses a place the index answered without usable coordinates', async () => {
    respond({ found: true, available: true, label: 'Nowhere', place: { latitude: null, longitude: null } });
    expect(await suggestPlace('somewhere')).toBeNull();
  });

  it('does not ask about a fragment', async () => {
    expect(worthLookingUp('La')).toBe(false);
    expect(worthLookingUp('')).toBe(false);
    expect(worthLookingUp(null)).toBe(false);
    expect(worthLookingUp('433 Ice Avenue')).toBe(true);
    expect(await suggestPlace('La')).toBeNull();
    expect(global.fetch).not.toHaveBeenCalled();
  });
});

describe('the create form asks, and waits to be told', () => {
  const src = source('pages/PatchForm.svelte');

  // The whole point of ADR 082. An auto-filled marker plus a submit button is
  // acceptance by silence: nobody has to look at the coordinate for it to be
  // saved, so a wrong guess ships because the form was submitted.
  it('sends coordinates only once something is placed', () => {
    expect(src).toMatch(/latitude: placed \? latitude : undefined/);
    expect(src).toMatch(/longitude: placed \? longitude : undefined/);
  });

  it('sends them on the claim-setup path too, not just creation', () => {
    const sent = src.match(/latitude: placed \? latitude : undefined/g) || [];
    expect(sent.length).toBe(2);
  });

  it('looks the address up when the field is left, not on every keystroke', () => {
    expect(src).toMatch(/onblur=\{addressSettled\}/);
    expect(src).not.toMatch(/oninput=\{addressSettled\}/);
  });

  // A marker the person placed is theirs. Proposing over it is the inversion
  // the confirm step exists to prevent.
  it('never proposes over a placement already made', () => {
    expect(src).toMatch(/if \(placed\) return;/);
  });

  it('does not ask the same address twice', () => {
    expect(src).toMatch(/lastLookedUp/);
  });
});

describe('patch settings suggests only to an unplaced patch', () => {
  const src = source('pages/PatchSettingsInfo.svelte');

  it('bails out of the lookup when the patch is already on the map', () => {
    expect(src).toMatch(/async function openPicker/);
    expect(src).toMatch(/if \(onMap\) return;/);
  });

  it('looks up when the picker opens rather than on every settings visit', () => {
    expect(src).toMatch(/onclick=\{openPicker\}/);
  });

  it('drops the suggestion when the picker closes', () => {
    expect(src).toMatch(/function closePicker[\s\S]{0,120}suggestion = null/);
  });
});

describe('the picker draws a proposal differently from a placement', () => {
  const src = source('components/MapLocationPicker.svelte');

  // If a guess looks identical to a placement, the confirm step is a formality
  // and the model's claim that a coordinate was placed stops being true.
  it('marks a suggested marker as provisional', () => {
    expect(src).toMatch(/let provisional = \$state\(false\)/);
    expect(src).toMatch(/markerIcon\(proposed/);
    expect(src).toMatch(/stroke-dasharray/);
  });

  it('stops calling it a guess once the person moves it', () => {
    expect(src).toMatch(/if \(provisional\) \{[\s\S]{0,120}provisional = false/);
  });

  it('never proposes over a saved location', () => {
    expect(src).toMatch(/if \(hasMapLocation\(lat, lng\)\) return;/);
  });

  it('names what it thinks it found, so there is something to judge', () => {
    expect(src).toMatch(/Suggested from the address/);
  });
});
