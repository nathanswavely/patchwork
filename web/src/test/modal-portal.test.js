import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const read = (p) => readFileSync(resolve(__dirname, '..', p), 'utf8');

// A modal's scrim has to cover the whole page, and it renders where its
// component was mounted. Mounted inside the docked profile — which sits in
// the cards pane, a positioned box at z-index 10 — the scrim was pinned to
// that layer, and the global bar, the rail and the filter chips all drew
// on top of it while the dialog sat in the middle. A portal to <body> fixed
// that; a native modal <dialog> fixes it by construction, since
// showModal() puts it in the top layer, above every stacking context.
describe('the modal scrim', () => {
  it('is a native modal dialog, in the top layer', () => {
    const modal = read('components/Modal.svelte');
    expect(modal).toMatch(/<dialog class="lt-dialog modal"[^>]*use:ltDialog=\{\{ open, onClose \}\}/);
    expect(read('lib/lt.js')).toMatch(/export function ltDialog\(el, opts\)/);
    expect(read('vendor/liquid-textile/behaviors.js')).toMatch(/el\.showModal\(\)/);
  });

  it('needs no portal', () => {
    expect(read('components/Modal.svelte')).not.toContain('use:portal');
  });
});
