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

  // The wording of every string below belongs to whoever holds the copy
  // ledger (tools/copy-ledger), and it has already been rewritten once.
  // So these assert the *decision* — that both directions are stated, and
  // that neither capability is derived from the other — rather than the
  // sentences, which are free to change without breaking the suite.
  function branchesOf(flag) {
    const at = src.indexOf(`{#if label.${flag}}`);
    if (at === -1) return null;
    const end = src.indexOf('{/if}', at);
    const body = src.slice(at, end);
    const split = body.indexOf('{:else}');
    if (split === -1) return null;
    const strip = (t) => t.replace(/\{[^}]*\}/g, '').replace(/<[^>]*>/g, '').trim();
    return { affirmative: strip(body.slice(0, split)), negative: strip(body.slice(split)) };
  }

  it('states each capability in both directions, not only the flattering one', () => {
    // docs/adr/061 decision 3: a line that renders only when the answer is
    // yes is marketing, not a label. Both branches must carry real text.
    for (const flag of ['federation', 'multi_quilt']) {
      const b = branchesOf(flag);
      expect(b, `${flag} has no {#if}/{:else} pair`).not.toBeNull();
      expect(b.affirmative.length).toBeGreaterThan(20);
      expect(b.negative.length).toBeGreaterThan(20);
      expect(b.negative).not.toBe(b.affirmative);
    }
  });

  it('never derives one capability from the other', () => {
    // docs/adr/061 decision 5: federation is whether the quilt can be
    // followed, multi-quilt whether it can be read. One {#if} covering both
    // would make a followable quilt claim to be readable — a state the
    // config cannot actually produce.
    expect((src.match(/\{#if label\.multi_quilt\}/g) || []).length).toBe(1);
    expect((src.match(/\{#if label\.federation\}/g) || []).length).toBeGreaterThanOrEqual(1);
  });

  it('puts them with the materials, beside the running version', () => {
    // "What this runs on" already ends with a derived, unstored fact.
    const runsOn = src.slice(src.indexOf('Running Patchwork'), src.indexOf('Running Patchwork') + 1200);
    expect(runsOn).toMatch(/label\.federation/);
    expect(runsOn).toMatch(/label\.multi_quilt/);
  });

  it('prices the exit where the exit is described, only when there is an audience to lose', () => {
    // docs/adr/060: the community travels and the audience does not. The
    // sentence is the stewards' to write; what must not quietly vanish is
    // the paragraph, its gate, and the fact it names.
    const door = src.slice(src.indexOf('class="the-door"'));
    expect(door).toMatch(/class="door-cost\b/);
    expect(door).toMatch(/\{#if label\.federation\}/);
    expect(door).toMatch(/followers/i);
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
