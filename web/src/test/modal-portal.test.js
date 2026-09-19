import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const read = (p) => readFileSync(resolve(__dirname, '..', p), 'utf8');

// A modal's scrim has to cover the whole page, and it renders where its
// component was mounted. Mounted inside the docked profile — which sits in
// the cards pane, a positioned box at z-index 10 — the scrim was pinned to
// that layer, and the global bar, the rail and the filter chips all drew
// on top of it while the dialog sat in the middle.
describe('the modal scrim', () => {
  it('is moved to <body>, out of every stacking context it was mounted in', () => {
    const modal = read('components/Modal.svelte');
    expect(modal).toMatch(/import \{ portal \} from '\.\.\/lib\/portal\.js'/);
    expect(modal).toMatch(/<div class="modal-backdrop" use:portal/);
    const portalSrc = read('lib/portal.js');
    expect(portalSrc).toMatch(/document\.body\.appendChild\(node\)/);
    expect(portalSrc).toMatch(/destroy\(\) \{\s*node\.remove\(\);/);
  });

  it('keeps the backdrop as the only node of its block', () => {
    // Svelte tears a block down by walking siblings from its first node to
    // its last; a moved node's siblings are body's.
    const modal = read('components/Modal.svelte');
    const block = modal.match(/\{#if open\}([\s\S]*?)\{\/if\}/)[1];
    const roots = block.trim().match(/^<div class="modal-backdrop"[\s\S]*<\/div>$/);
    expect(roots).not.toBeNull();
  });
});
