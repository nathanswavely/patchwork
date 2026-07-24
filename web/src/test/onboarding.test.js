import { describe, it, expect, beforeEach } from 'vitest';
import {
  isOnboardingDismissed,
  dismissOnboarding,
  isUnlockPanelDismissed,
  dismissUnlockPanel,
  isSetupChecklistDismissed,
  dismissSetupChecklist,
  isPatchLinkShared,
  markPatchLinkShared,
  isGovernanceHubVisited,
  markGovernanceHubVisited,
} from '../lib/onboarding.js';

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

// Per-patch onboarding flags (docs/adr/040): unlike the first-run flag
// above, each is scoped to BOTH a user id and a patch id — belonging to
// one patch says nothing about another.
describe('per-patch onboarding flags', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  const pairs = [
    ['unlock panel', isUnlockPanelDismissed, dismissUnlockPanel],
    ['setup checklist', isSetupChecklistDismissed, dismissSetupChecklist],
    ['patch link shared', isPatchLinkShared, markPatchLinkShared],
    ['governance hub visited', isGovernanceHubVisited, markGovernanceHubVisited],
  ];

  for (const [label, isSet, setFlag] of pairs) {
    describe(label, () => {
      it('is unset by default', () => {
        expect(isSet('user-1', 'patch-1')).toBe(false);
      });

      it('persists once set', () => {
        setFlag('user-1', 'patch-1');
        expect(isSet('user-1', 'patch-1')).toBe(true);
      });

      it('scopes per user', () => {
        setFlag('user-1', 'patch-1');
        expect(isSet('user-2', 'patch-1')).toBe(false);
      });

      it('scopes per patch', () => {
        setFlag('user-1', 'patch-1');
        expect(isSet('user-1', 'patch-2')).toBe(false);
      });

      it('is a no-op without both a user id and a patch id', () => {
        expect(isSet(undefined, 'patch-1')).toBe(false);
        expect(isSet('user-1', undefined)).toBe(false);
        setFlag(undefined, 'patch-1');
        setFlag('user-1', undefined);
        expect(isSet(undefined, 'patch-1')).toBe(false);
        expect(isSet('user-1', undefined)).toBe(false);
      });
    });
  }
});
