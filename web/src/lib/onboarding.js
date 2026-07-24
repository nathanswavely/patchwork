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
const CHECKLIST_GOVERNANCE_PREFIX = 'patchwork_setup_checklist_governance_visited:';

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
 * that never amends anything would nag forever without this fallback — an
 * admin who has actually visited the governance hub counts as decided.
 */
export function isGovernanceHubVisited(userId, patchId) {
  return readFlag(CHECKLIST_GOVERNANCE_PREFIX, userId, patchId);
}
export function markGovernanceHubVisited(userId, patchId) {
  writeFlag(CHECKLIST_GOVERNANCE_PREFIX, userId, patchId);
}
