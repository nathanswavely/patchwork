/**
 * The Label states whether the quilt federates (docs/adr/061), and the
 * door says what leaving costs (docs/adr/060).
 *
 * Source text again — these confirm the copy exists in both branches and
 * cannot confirm it is legible. Both states were read in a browser.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('the Label states whether the quilt federates (docs/adr/061)', () => {
  const src = source('pages/Label.svelte');

  it('states it in both directions, not only the flattering one', () => {
    expect(src).toMatch(/Federating\. Public patches here can be followed/);
    expect(src).toMatch(/Not federating\. Patches here can't be followed/);
  });

  it('states multi-quilt in both directions too, on its own line', () => {
    // docs/adr/061 decision 5: federation is whether the quilt can be
    // followed, multi-quilt whether it can be read. Independent facts.
    expect(src).toMatch(/Readable by other quilts\./);
    expect(src).toMatch(/Not readable by other quilts\./);
    expect(src).toMatch(/\{#if label\.multi_quilt\}/);
  });

  it('never derives one capability from the other', () => {
    // A single {#if} covering both lines would make a followable quilt
    // claim to be readable, which is a lie the config can't produce.
    const federationIf = (src.match(/\{#if label\.federation\}/g) || []).length;
    const multiIf = (src.match(/\{#if label\.multi_quilt\}/g) || []).length;
    expect(federationIf).toBeGreaterThanOrEqual(1);
    expect(multiIf).toBe(1);
  });

  it('puts it with the materials, beside the running version', () => {
    // "What this runs on" already ends with a derived, unstored fact.
    const runsOn = src.slice(src.indexOf('Running Patchwork'), src.indexOf('Running Patchwork') + 1200);
    expect(runsOn).toMatch(/label\.federation/);
    expect(runsOn).toMatch(/label\.multi_quilt/);
  });

  it('prices the exit where the exit is described', () => {
    // docs/adr/060: the community travels and the audience does not.
    expect(src).toMatch(/What travels is the community/);
    expect(src).toMatch(/starts over with the\s+followers it had on other sites/);
  });

  it('only prices the exit when there is an audience to lose', () => {
    const door = src.slice(src.indexOf('class="the-door"'));
    expect(door).toMatch(/\{#if label\.federation\}/);
  });
});

describe('the export panel names what stays behind', () => {
  it('lists followers alongside the keys', () => {
    // Naming the keys and not the audience is the more misleading half.
    expect(source('pages/AdminQuiltSettings.svelte')).toMatch(
      /as do followers from other sites/
    );
  });
});
