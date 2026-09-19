/**
 * Tenure is capped at the patch's age, and the patch may state its age
 * (docs/adr/098).
 *
 * Settings -> Info carries a "Founded" date. Blank means the group started
 * with its patch; a date means it predates it, which is how an organisation
 * moving rules it already lives by keeps its full voting-tenure bar. The
 * field saves alone, as a single-key PATCH, so an untouched value is never
 * sent back and never reset.
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

describe('PatchSettingsInfo — the Founded date', () => {
  const src = source('pages/PatchSettingsInfo.svelte');

  it('is a date input bounded at today, seeded from the node', () => {
    expect(src).toMatch(/class="founded-input"\s+type="date"\s+max=\{today\}\s+bind:value=\{foundedAt\}/);
    expect(src).toMatch(/foundedAt = node\.founded_at \|\| ''/);
    expect(src).toMatch(/const today = new Date\(\)\.toISOString\(\)\.slice\(0, 10\)/);
  });

  it('saves founded_at alone, so nothing else on the form is sent back', () => {
    expect(src).toMatch(/method: 'PATCH', body: \{ founded_at: value \}/);
  });

  it('says why the date matters, in one plain sentence', () => {
    expect(src).toMatch(/When this group started, if it predates its patch\. Voting tenure is never\s+required to be longer than the group has existed\./);
  });

  it('offers Save and Cancel only once the date has changed', () => {
    expect(src).toMatch(/let foundedDirty = \$derived\(foundedAt !== \(node\?\.founded_at \|\| ''\)\)/);
    expect(src).toMatch(/\{#if foundedDirty\}/);
    expect(src).toMatch(/onclick=\{\(\) => saveFoundedAt\(foundedAt\)\}/);
  });
});
