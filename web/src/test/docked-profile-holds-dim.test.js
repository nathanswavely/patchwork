import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// docs/adr/112 decision 6c. A profile docked beside the quilt (docs/adr/094)
// keeps its patch lit for as long as it is open. That differs from the hover
// dim in three ways, and each one is a way it could go wrong:
//
//   it persists, so it can go stale as the view moves;
//   it is a base rather than an override, so hovering elsewhere must give the
//     light back rather than clear it;
//   it is deliberate, so it should not wait out a dwell meant for pans.

const canvas = readFileSync(
  resolve(__dirname, '../components/QuiltCanvas.svelte'),
  'utf8',
);
const home = readFileSync(resolve(__dirname, '../pages/SocialHome.svelte'), 'utf8');

describe('a docked profile holds the light', () => {
  it('takes the docked patch as a prop', () => {
    expect(canvas).toMatch(/dockedPatchId = null,/);
    expect(home).toMatch(/dockedPatchId=\{dockedSeed\?\.id \|\| null\}/);
  });

  it('keeps it as a base focus, separate from the hovered one', () => {
    expect(canvas).toMatch(/let baseFocusId = null;/);
    expect(canvas).toMatch(/let hoverFocusId = null;/);
    // The transient one wins while it lasts.
    expect(canvas).toMatch(/const target = hoverFocusId \|\| baseFocusId;/);
  });

  it('engages without the dwell, because opening a profile is deliberate', () => {
    const fn = canvas.match(/function setBaseFocus\(patchId\) \{[\s\S]*?\n  \}/);
    expect(fn, 'setBaseFocus not found').toBeTruthy();
    expect(fn[0]).toMatch(/applyFocus\(\{ immediate: true \}\)/);
  });
});

describe('hovering elsewhere borrows the light and gives it back', () => {
  // A docked profile has not stopped being the patch the reader opened just
  // because the pointer wandered. Clearing on release would leave the quilt
  // undimmed with a profile still open beside it.
  it('returns to the docked patch instead of clearing', () => {
    const fn = canvas.match(/function releaseDim\(\) \{[\s\S]*?\n  \}/)[0];
    expect(fn).toMatch(/hoverFocusId = null;/);
    expect(fn).toMatch(/if \(baseFocusId\) \{\s*\n\s*applyFocus\(\{ immediate: true \}\);/);
  });

  it('still clears on release when nothing is docked', () => {
    const fn = canvas.match(/function releaseDim\(\) \{[\s\S]*?\n  \}/)[0];
    expect(fn).toMatch(/setTimeout\(clearDim, DIM_RELEASE_MS\)/);
  });
});

describe('a sustained focus is re-asked as the view moves', () => {
  // Pan the docked tile off screen and a dim that never re-checks sits there
  // with nothing lit. A hover cannot go stale this way, because moving the
  // pointer is what ends it.
  it('re-applies when the in-view set changes', () => {
    expect(canvas).toMatch(/if \(baseFocusId\) applyFocus\(\{ immediate: true \}\);/);
  });

  it('does not hide that behind the in-view callback', () => {
    // A consumer that docks a profile without wanting ADR 074's lens would
    // otherwise hold a stale dim forever.
    const effect = canvas.match(/let lastInView = '';[\s\S]*?\n  \}\);/)[0];
    expect(effect).toMatch(/if \(onInViewChange\) onInViewChange\(ids\);/);
    expect(effect).not.toMatch(/if \(!onInViewChange\) return;/);
  });
});
