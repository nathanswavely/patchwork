import { describe, it, expect, beforeEach } from 'vitest';
import { isIntroDismissed, dismissIntro } from '../lib/introCard.js';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

// The intro card (CONTEXT.md "Intro card"): dismissed once, gone forever.
// Unlike onboarding.js's per-account dismissal, there's no user id for an
// anonymous visitor to key on, so this is one unscoped browser-level flag.
describe('intro card dismissal', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('is not dismissed by default', () => {
    expect(isIntroDismissed()).toBe(false);
  });

  it('persists dismissal permanently', () => {
    dismissIntro();
    expect(isIntroDismissed()).toBe(true);
  });
});

// Placement (CONTEXT.md: "Overlaid on a corner of the surface, never
// covering it"). The compact strip used to centre itself under the global
// bar — empty space over a canvas, and exactly where a page surface puts its
// heading, which it covered 22px of on every one of them.
describe('intro card placement', () => {
  const card = () => source('components/IntroCard.svelte');
  const shell = () => source('components/SocialShell.svelte');

  it('gives both variants one corner instead of one each', () => {
    const src = card();
    expect(src).not.toContain('top: calc(56px + 12px)');
    expect(src).not.toContain('.intro-card:not(.compact) {');
  });

  it('shares the shell breakpoint for the bottom tab bar', () => {
    // The card's own 640px against the shell's 768px left a band where the
    // rail was already a bar and the card still sat on 35px of it.
    expect(card()).toContain('@media (max-width: 768px)');
    expect(card()).not.toContain('@media (max-width: 640px)');
  });

  it('clears the mobile tab bar and the canvas pill, which are controls', () => {
    const src = card();
    expect(src).toContain('var(--pw-canvas-chrome-bottom');
    expect(src).toContain('.intro-card.canvas');
  });

  it('has the shell reserve its foot, not each page surface', () => {
    // One rule where the gutter is already owned, rather than a margin every
    // future page has to remember to leave.
    expect(shell()).toContain('var(--intro-card-h');
    expect(card()).toContain("setProperty('--intro-card-h'");
  });
});

// Which form a landing gets (docs/adr/040, docs/adr/075).
describe('intro card variant', () => {
  it('gives every discovery surface the full card', () => {
    // The full card is the only form carrying "no ads, no personalized
    // algorithm", Join, and the worded decline. Discovery mode and the
    // events list are cold landings in their own right; the scope variants
    // of the canvases are the same surfaces under a different lens.
    const src = source('components/IntroCard.svelte');
    const set = src.slice(src.indexOf('FULL_ROUTES'), src.indexOf('CANVAS_ROUTES'));
    for (const r of ['home', 'homeMy', 'map', 'mapMy', 'discover', 'eventList', 'eventListMy']) {
      expect(set).toContain(`'${r}'`);
    }
  });

  it('keeps the compact strip for deep links', () => {
    // An event someone was sent, a patch profile — the card must not compete
    // with the content they came for.
    const src = source('components/IntroCard.svelte');
    const set = src.slice(src.indexOf('FULL_ROUTES'), src.indexOf('CANVAS_ROUTES'));
    expect(set).not.toContain("'eventDetail'");
    expect(set).not.toContain("'patchProfile'");
  });
});
