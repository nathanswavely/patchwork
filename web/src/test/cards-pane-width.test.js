import { describe, it, expect, beforeEach } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  PANE_STOPS,
  getPaneStop,
  setPaneStop,
  cyclePaneStop,
  getLastOpenPaneStop,
  paneWidthCSS,
  paneColumns,
  paneFraction,
} from '../stores/quilt.svelte.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

// The cards pane's width (docs/adr/111): one fact the reader sets, at three
// stops named in cards. These assert the arithmetic and the source text; the
// behaviour itself — the migrating control, the dock override, the canvas
// re-centre — is only checkable in a browser, since this project has no
// Svelte render library.
describe('cards pane width', () => {
  beforeEach(() => {
    localStorage.clear();
    setPaneStop('two');
  });

  it('cycles two columns -> one column -> hidden -> two columns', () => {
    expect(getPaneStop()).toBe('two');
    cyclePaneStop();
    expect(getPaneStop()).toBe('one');
    cyclePaneStop();
    expect(getPaneStop()).toBe('hidden');
    cyclePaneStop();
    expect(getPaneStop()).toBe('two');
  });

  it('remembers the last open stop so hidden has somewhere to return to', () => {
    setPaneStop('one');
    setPaneStop('hidden');
    // Hidden is where you come back *from*: it never becomes the stop a
    // docked profile borrows, or the cycle would have no way out.
    expect(getLastOpenPaneStop()).toBe('one');
  });

  it('persists — it is an arrangement, not a lens', () => {
    setPaneStop('hidden');
    expect(localStorage.getItem('patchwork-cards-pane-stop')).toBe('hidden');
  });

  it('ignores a stop it does not recognise rather than going blank', () => {
    setPaneStop('one');
    setPaneStop('enormous');
    expect(getPaneStop()).toBe('one');
  });

  // One column is the width at which a single card is exactly as wide as
  // each of the two were: pane/2 + 10px, given 16px side padding and a 12px
  // grid gap. What the reader gains is quilt, not a bigger card.
  it('halves the pane for one column, leaving the card its size', () => {
    for (const winW of [1280, 1440, 1920, 2560]) {
      const twoPane = paneFraction('two', winW) * winW;
      const onePane = paneFraction('one', winW) * winW;
      const twoCard = (twoPane - 32 - 12) / 2;
      const oneCard = onePane - 32;
      expect(Math.abs(oneCard - twoCard)).toBeLessThan(1);
    }
  });

  it('gives the whole window to the canvas when hidden', () => {
    expect(paneFraction('hidden', 1440)).toBe(0);
  });

  it('never lets the pane take more than half the window', () => {
    for (const winW of [769, 800, 900, 1024]) {
      expect(paneFraction('one', winW)).toBeLessThanOrEqual(0.5);
      expect(paneFraction('two', winW)).toBeLessThanOrEqual(0.5);
    }
  });

  // The CSS width and the fraction are two expressions of one number, so
  // they must agree — the 10px in one is the 10px in the other.
  it('states the same width in CSS as in the fraction', () => {
    expect(paneWidthCSS('two')).toBe('45%');
    expect(paneWidthCSS('one')).toBe('calc(22.5% + 10px)');
    expect(paneWidthCSS('hidden')).toBe('0px');
    const winW = 1440;
    expect(paneFraction('one', winW) * winW).toBeCloseTo(0.225 * winW + 10, 5);
  });

  it('names the column count from the stop, not from the width', () => {
    expect(paneColumns('two')).toBe(2);
    expect(paneColumns('one')).toBe(1);
    expect(PANE_STOPS).toEqual(['two', 'one', 'hidden']);
  });
});

describe('the width is one published fact', () => {
  // 45% used to be a literal in three places across two files, agreeing by
  // luck. The shell's chips have no other reason to know the pane exists.
  it('leaves no hardcoded 45% behind in either file', () => {
    expect(source('pages/SocialHome.svelte')).not.toMatch(/\b0\.45\b/);
    expect(source('components/SocialShell.svelte')).not.toMatch(/calc\(45% \+ 16px\)/);
  });

  it('sizes the pane and clears the chips from the same custom property', () => {
    expect(source('pages/SocialHome.svelte')).toContain('width: var(--pw-cards-pane-w');
    expect(source('components/SocialShell.svelte')).toContain('var(--pw-cards-pane-w');
  });

  it('feeds the canvases an inset derived from the stop', () => {
    expect(source('pages/SocialHome.svelte')).toContain('paneFraction(effectivePaneStop, winW)');
  });
});

describe('what a hidden pane does to what lives in it', () => {
  const home = source('pages/SocialHome.svelte');

  // docs/adr/111 decision 5, amending docs/adr/074: the lens needs two panes
  // on screen and hidden leaves one. Suspended, never cleared — the store
  // keeps the setting, exactly as the mobile gate does.
  it('suspends the in-view lens rather than clearing it', () => {
    expect(home).toContain('winW > 768 && !paneHidden');
    expect(home).not.toMatch(/paneHidden[^\n]*setInViewOnly\(false\)/);
  });

  // docs/adr/111 decision 6, amending docs/adr/094: a zero-width slot is no
  // slot, so hidden yields to the last open stop while a profile is docked —
  // and the stored stop is left alone, so dismissing returns it to hidden.
  it('opens the pane for a docked profile without overwriting the stop', () => {
    expect(home).toContain("paneStop === 'hidden' && dockNeedsPane ? getLastOpenPaneStop() : paneStop");
    expect(home).not.toMatch(/dockedSlug[^\n]*setPaneStop\(/);
  });

  // The control has two homes and only ever one on screen: a control that
  // can delete its own container has to survive deleting it.
  it('keeps a way back in once the header has gone with the pane', () => {
    expect(home).toContain('{#if paneHidden}');
    expect(home).toContain('class="pane-reopen"');
    expect(home).toContain('setPaneStop(getLastOpenPaneStop())');
  });
});

describe('the canvas follows the pane', () => {
  const canvas = source('components/QuiltCanvas.svelte');

  // Nothing caught an inset change before: the ResizeObserver watches
  // .quilt-pane, which is inset:0 and never changes size when the pane does.
  it('re-centres on an inset change', () => {
    expect(canvas).toContain("svgSelection.transition('paneWidth')");
    expect(canvas).toContain('let lastInsetRight = null');
  });

  it('re-centres without re-zooming — the reader keeps their zoom and pan', () => {
    const block = canvas.slice(canvas.indexOf('let lastInsetRight'));
    const call = block.slice(0, block.indexOf('onMount'));
    expect(call).toContain('.scale(t.k)');
    expect(call).not.toContain('clampFitScale');
  });

  // MapView keeps its own stated rule that narrowing never moves the
  // viewport by itself: map space is real geography, quilt space is not.
  it('leaves the map where the reader left it', () => {
    const map = source('components/MapView.svelte');
    expect(map).not.toContain("transition('paneWidth')");
  });
});
