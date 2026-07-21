import { describe, it, expect, beforeEach } from 'vitest';
import { isOnboardingDismissed, dismissOnboarding } from '../lib/onboarding.js';

// The dismissal flag is what lets "Skip" genuinely exit first-run onboarding.
// Without it, App.svelte's zero-membership redirect loops a user on an empty
// instance (nothing to follow) straight back to /welcome forever.
describe('onboarding dismissal', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('is not dismissed by default', () => {
    expect(isOnboardingDismissed('user-1')).toBe(false);
  });

  it('persists dismissal for a user', () => {
    dismissOnboarding('user-1');
    expect(isOnboardingDismissed('user-1')).toBe(true);
  });

  it('scopes dismissal per user (shared browser)', () => {
    dismissOnboarding('user-1');
    expect(isOnboardingDismissed('user-2')).toBe(false);
  });

  it('treats a missing user id as not dismissed and never throws', () => {
    expect(isOnboardingDismissed(undefined)).toBe(false);
    dismissOnboarding(undefined); // no-op, must not write a bogus key
    expect(localStorage.getItem('patchwork_onboarding_dismissed:undefined')).toBeNull();
  });
});
