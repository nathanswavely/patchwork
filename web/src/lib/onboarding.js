/**
 * First-run onboarding dismissal.
 *
 * The app redirects zero-membership users to /welcome (App.svelte). Skipping
 * onboarding must genuinely exit it — on an empty instance there is nothing
 * to follow, so without a persisted dismissal the redirect loops forever.
 *
 * Stored in localStorage keyed by user id: per-browser, and scoped so a
 * different account on a shared machine still gets its own first run.
 */
const PREFIX = 'patchwork_onboarding_dismissed:';

export function isOnboardingDismissed(userId) {
  if (!userId) return false;
  try {
    return localStorage.getItem(PREFIX + userId) === '1';
  } catch {
    return false;
  }
}

export function dismissOnboarding(userId) {
  if (!userId) return;
  try {
    localStorage.setItem(PREFIX + userId, '1');
  } catch {
    // Storage unavailable (private mode) — the redirect will fire again next
    // load, but skip still works within this session via in-page navigation.
  }
}

/**
 * Per-patch onboarding dismissals and progress flags (docs/adr/040).
 *
 * Unlike the first-run flag above, these are scoped to both the signed-in
 * user AND a specific patch — a person's relationship to one patch says
 * nothing about another. All keyed on the patch's node id (stable across a
 * slug change), never the slug itself.
 */
const UNLOCK_PREFIX = 'patchwork_unlock_dismissed:';
const CHECKLIST_DISMISSED_PREFIX = 'patchwork_setup_checklist_dismissed:';
const CHECKLIST_SHARED_PREFIX = 'patchwork_setup_checklist_shared:';
// Renamed from …_governance_visited, and the key changes with it on
// purpose: the old flag was written by opening the governance hub, so every
// admin who ever looked is carrying a tick they never earned. A new key
// retires those rather than grandfathering a false one.
const CHECKLIST_GOVERNANCE_PREFIX = 'patchwork_setup_checklist_governance_decided:';

function patchScopedKey(prefix, userId, patchId) {
  return `${prefix}${userId}:${patchId}`;
}

function readFlag(prefix, userId, patchId) {
  if (!userId || !patchId) return false;
  try {
    return localStorage.getItem(patchScopedKey(prefix, userId, patchId)) === '1';
  } catch {
    return false;
  }
}

function writeFlag(prefix, userId, patchId) {
  if (!userId || !patchId) return;
  try {
    localStorage.setItem(patchScopedKey(prefix, userId, patchId), '1');
  } catch {
    // Storage unavailable (private mode) — the panel reappears next load.
  }
}

/** Unlock panel (CONTEXT.md "Unlock panel"): dismissed once per patch. */
export function isUnlockPanelDismissed(userId, patchId) {
  return readFlag(UNLOCK_PREFIX, userId, patchId);
}
export function dismissUnlockPanel(userId, patchId) {
  writeFlag(UNLOCK_PREFIX, userId, patchId);
}

/** Setup checklist (CONTEXT.md "Setup checklist"): dismissed once per admin per patch. */
export function isSetupChecklistDismissed(userId, patchId) {
  return readFlag(CHECKLIST_DISMISSED_PREFIX, userId, patchId);
}
export function dismissSetupChecklist(userId, patchId) {
  writeFlag(CHECKLIST_DISMISSED_PREFIX, userId, patchId);
}

/**
 * "Share your patch" checklist item: no stored state to derive from, so
 * clicking the copy-link button is itself the completion signal.
 */
export function isPatchLinkShared(userId, patchId) {
  return readFlag(CHECKLIST_SHARED_PREFIX, userId, patchId);
}
export function markPatchLinkShared(userId, patchId) {
  writeFlag(CHECKLIST_SHARED_PREFIX, userId, patchId);
}

/**
 * "Decide how you govern" checklist item: primarily derived from real
 * governance state (a published charter beyond the lining), but a patch
 * that never writes a charter would nag forever without this fallback, so
 * a governance act performed on the hub counts as decided too.
 *
 * Written by an act, never by a visit. Opening the hub used to set it, on
 * the reasoning that an admin who got there had considered the question;
 * what a founder saw instead was the checklist ticking "Decide how you
 * govern" off the moment the page rendered, having decided nothing. The
 * callers are the hub's own decisions — naming or clearing a successor,
 * adding or removing a seat, setting a seat's term, proposing a rules
 * change — each marking only after the write succeeds. The item is
 * `skippable`, which is the out for a patch that governs nothing; a tick
 * is for a patch that did something.
 */
export function isGovernanceDecided(userId, patchId) {
  return readFlag(CHECKLIST_GOVERNANCE_PREFIX, userId, patchId);
}
export function markGovernanceDecided(userId, patchId) {
  writeFlag(CHECKLIST_GOVERNANCE_PREFIX, userId, patchId);
}
