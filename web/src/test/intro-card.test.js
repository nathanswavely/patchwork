import { describe, it, expect, beforeEach } from 'vitest';
import { isIntroDismissed, dismissIntro } from '../lib/introCard.js';

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
