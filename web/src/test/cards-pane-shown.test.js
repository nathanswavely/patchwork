import { describe, it, expect, beforeEach } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  isPaneHidden,
  setPaneHidden,
  togglePaneHidden,
  paneWidthCSS,
  paneFraction,
} from '../stores/quilt.svelte.js';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

// Whether the cards pane is shown (docs/adr/111): one bit the reader sets.
// These assert the arithmetic and the source text; the behaviour itself —
// the travelling control, the dock override, the canvas re-centre — is only
// checkable in a browser, since this project has no Svelte render library.
describe('showing and hiding the cards pane', () => {
  beforeEach(() => {
    localStorage.clear();
    setPaneHidden(false);
  });

  it('starts shown — nothing changes for a reader who never presses it', () => {
    localStorage.clear();
    expect(paneWidthCSS(false)).toBe('45%');
    expect(paneFraction(false)).toBe(0.45);
  });

  it('toggles', () => {
    expect(isPaneHidden()).toBe(false);
    togglePaneHidden();
    expect(isPaneHidden()).toBe(true);
    togglePaneHidden();
    expect(isPaneHidden()).toBe(false);
  });

  it('persists — it is an arrangement, not a lens', () => {
    setPaneHidden(true);
    expect(localStorage.getItem('patchwork-cards-pane-hidden')).toBe('1');
    setPaneHidden(false);
    expect(localStorage.getItem('patchwork-cards-pane-hidden')).toBe('0');
  });

  it('gives the whole window to the canvas when hidden', () => {
    expect(paneWidthCSS(true)).toBe('0px');
    expect(paneFraction(true)).toBe(0);
  });

  // The CSS width and the fraction are two expressions of one bit, so they
  // must agree — a canvas inset that disagreed with the pane's width is the
  // failure the published fact exists to prevent.
  it('states the same width in CSS as in the fraction', () => {
    expect(parseFloat(paneWidthCSS(false)) / 100).toBe(paneFraction(false));
    expect(parseFloat(paneWidthCSS(true))).toBe(0);
    expect(paneFraction(true)).toBe(0);
  });

  // Intermediate widths were built and thrown out on use (docs/adr/111):
  // there is no middle between "I am reading the list" and "get out of the
  // way", so there is no second stop to store.
  it('is one bit, with no stops between', () => {
    const store = source('stores/quilt.svelte.js');
    expect(store).not.toContain('PANE_STOPS');
    expect(store).not.toContain('paneColumns');
  });
});

describe('the width is one published fact', () => {
  // 45% used to be a literal in three places across two files, agreeing by
  // luck. The shell's chips have no other reason to know the pane exists.
  it('leaves no hardcoded 45% behind in either component', () => {
    expect(source('pages/SocialHome.svelte')).not.toMatch(/\b0\.45\b/);
    expect(source('components/SocialShell.svelte')).not.toMatch(/calc\(45% \+ 16px\)/);
  });

  it('sizes the pane, clears the chips and parks the control from one property', () => {
    const home = source('pages/SocialHome.svelte');
    expect(home).toContain('width: var(--pw-cards-pane-w');
    expect(home).toContain('right: calc(var(--pw-cards-pane-w, 45%) + 12px)');
    expect(source('components/SocialShell.svelte')).toContain('var(--pw-cards-pane-w');
  });

  it('feeds the canvases an inset derived from the same bit', () => {
    expect(source('pages/SocialHome.svelte')).toContain('paneFraction(paneHidden)');
  });

  // The cards grid is exactly what it was before this control existed.
  it('leaves the list itself alone', () => {
    expect(source('pages/SocialHome.svelte')).toMatch(/\.cards-grid \{\s*display: grid;\s*grid-template-columns: repeat\(2, 1fr\);/);
  });
});

describe('the control has one home', () => {
  const home = source('pages/SocialHome.svelte');

  // It parks against the pane's edge and travels with it, so it is in the
  // same place relative to the thing it moves whether that thing is there or
  // not. A control that can delete its own container cannot live inside it.
  it('lives on the canvas, never in the list header', () => {
    const header = home.slice(home.indexOf('<div class="cards-header">'), home.indexOf('<div class="cards-scroll">'));
    expect(header).not.toContain('pane-toggle');
    expect(home).toContain('class="pane-toggle"');
  });

  it('is the rail toggle mirrored — same icon, same weights', () => {
    expect(home).toContain('SidebarSimple');
    expect(home).toContain("weight={paneHidden ? 'duotone' : 'fill'}");
    expect(home).toMatch(/\.pane-toggle :global\(svg\) \{\s*transform: scaleX\(-1\);/);
    // The pair has to stay a pair: the rail's own toggle uses the same two.
    expect(source('components/SocialShell.svelte'))
      .toContain("weight={sidebarCollapsed ? 'duotone' : 'fill'}");
  });

  it('is absent below the breakpoint, where the pill already answers this', () => {
    expect(home).toContain('{#if winW > 768}');
    expect(home).toMatch(/\.pane-toggle \{\s*display: none;/);
  });
});

describe('what a hidden pane does to what lives in it', () => {
  const home = source('pages/SocialHome.svelte');

  // docs/adr/111, amending docs/adr/074: the lens needs two panes on screen
  // and hidden leaves one. Suspended, never cleared — the store keeps the
  // setting, exactly as the mobile gate does.
  it('suspends the in-view lens rather than clearing it', () => {
    expect(home).toContain('winW > 768 && !paneHidden');
    expect(home).not.toMatch(/paneHidden[^\n]*setInViewOnly\(false\)/);
  });

  // docs/adr/111, amending docs/adr/094: a zero-width slot is no slot, so
  // hidden yields while a profile is docked — and the stored bit is left
  // alone, so dismissing returns the pane to hidden.
  it('opens the pane for a docked profile without overwriting the setting', () => {
    expect(home).toContain('isPaneHidden() && !dockNeedsPane');
  });

  // With a profile docked over a pane the reader had hidden, the stored bit
  // says hidden while the pane is visibly open. A plain toggle there would
  // flip the bit to *shown*, destroy the setting, and change nothing on
  // screen — a press that does the opposite of its label, invisibly. The
  // button means one thing in every state: the canvas alone.
  it('clears the dock too rather than toggling a bit nobody can see', () => {
    expect(home).toMatch(
      /function paneButtonPress\(\) \{\s*if \(dockNeedsPane\) \{\s*onDockClose\(\);\s*setPaneHidden\(true\);/
    );
    expect(home).toContain('onclick={paneButtonPress}');
    expect(home).not.toContain('onclick={togglePaneHidden}');
  });

  // Clipped rather than unmounted, so the width transition has something to
  // carry out — which leaves forty controls in the tab order unless inerted.
  it('takes the clipped list out of the tab order', () => {
    expect(home).toContain('inert={paneHidden || null}');
    expect(home).toMatch(/\.cards-pane\.pane-hidden \{[^}]*overflow: hidden;/);
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
    expect(source('components/MapView.svelte')).not.toContain("transition('paneWidth')");
  });
});
