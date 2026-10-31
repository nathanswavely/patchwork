/**
 * A patch can say it is a nonprofit (#448): its admins at creation and in
 * Settings -> Info, and whoever suggests a listing, with the reviewer able to
 * correct it. It is a field and not a tag, so nothing here goes near the
 * TagPicker.
 *
 * There is no Svelte render library in this project, so component wiring is
 * asserted against source text.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('PatchSettingsInfo — the nonprofit switch', () => {
  const src = source('pages/PatchSettingsInfo.svelte');

  it('reads the node and saves nonprofit alone', () => {
    expect(src).toMatch(/checked=\{node\?\.nonprofit === true\}/);
    expect(src).toMatch(/saveField\('nonprofit', value\)/);
  });
});

describe('PatchForm — nonprofit at creation and setup', () => {
  const src = source('pages/PatchForm.svelte');

  it('seeds from the listing in setup, so a claimant sees what the suggester said', () => {
    expect(src).toMatch(/let nonprofit = \$state\(initial\?\.nonprofit === true\)/);
  });

  it('sends it on create and on the setup PATCH', () => {
    expect(src).toMatch(/nonprofit: nonprofit \|\| undefined,/);
    expect(src).toMatch(/suggest_tags: suggestTags\.length > 0 \? suggestTags : undefined,\s+nonprofit,\s+ai_use: /);
  });
});

describe('SubmitPatch — a suggester can say it', () => {
  const src = source('pages/SubmitPatch.svelte');

  it('is a checkbox bound to nonprofit and sent with the suggestion', () => {
    expect(src).toMatch(/type="checkbox" bind:checked=\{nonprofit\}/);
    expect(src).toMatch(/nonprofit: nonprofit \|\| undefined,/);
  });
});

describe('AdminSubmissions — the reviewer sees and may correct it', () => {
  const src = source('pages/AdminSubmissions.svelte');

  it('starts from the suggester\'s answer and sends the reviewer\'s on approval', () => {
    expect(src).toMatch(/nonprofits\[sub\.id\] = sub\.nonprofit === true/);
    expect(src).toMatch(/body\.nonprofit = nonprofitInputs\[id\] === true/);
  });
});

describe('PatchProfileHead — the badge', () => {
  const src = source('components/PatchProfileHead.svelte');

  it('wears a Nonprofit pill that says who declared it', () => {
    expect(src).toMatch(/\{#if node\.nonprofit\}/);
    expect(src).toMatch(/class="lt-pill"/);
    expect(src).toMatch(/>Nonprofit<\/span>/);
  });
});
