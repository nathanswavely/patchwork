/**
 * What changed in a patch's governance rules, named once.
 *
 * Two surfaces describe the same change: the diff a reviewer reads, and the
 * one-line summary that is submitted as `change_summary` and becomes the
 * commit message in the charter's history. That line is the entry the
 * governance Record keeps forever, so the two must not be able to disagree.
 *
 * They did. The diff walked every key either rules file carried; the
 * summary inspected a hand-written list of five. Thirteen fields could
 * change without the summary noticing, so a patch that moved both of its
 * decision venues off Patchwork — about as consequential as a patch's rules
 * get (docs/adr/052, docs/adr/053) — filed that act in its own minute book
 * as "Update governance rules". Changing three settings at once produced a
 * summary naming one of them, which is worse: it reads as the whole.
 *
 * So both are derived from here. `changedRuleFields` is the one answer to
 * what moved, `ruleLabel` and `formatRuleValue` the one set of words for it,
 * and `summarizeRuleChanges` is built from those rather than beside them. A
 * field added to the rules file is picked up by all of it without being
 * named anywhere, and a label edited here moves the diff and the record
 * together.
 *
 * This is a third register, deliberately not `governanceRules.js`. That
 * module states what a patch's rules *are*, in the words a page uses to
 * answer a member's question. These are the words a *change* is named in,
 * where the old value and the new one stand next to each other.
 */

export const RULE_LABELS = {
  decision_method: 'Decision Method',
  quorum_percent: 'Quorum',
  default_vote_duration_hours: 'Default Voting Period',
  amendment_threshold: 'Amendment Threshold',
  amendment_auto_apply: 'Auto-Apply Amendments',
  succession_policy: 'Succession Policy',
  min_voting_tenure_days: 'Min Voting Tenure',
  subject_recusal: 'Recuse a proposal’s subject',
  leadership_venue: 'Where admins are chosen',
  proposal_venue: 'Where proposals are decided',
  leadership_model: 'Leadership',
  membership_policy: 'Membership Policy',
  follower_permissions: 'Follower Permissions',
};

const VALUE_LABELS = {
  admin: 'Admin decides',
  majority: 'Majority vote',
  supermajority: 'Supermajority (2/3)',
  consensus: 'Full consensus',
  patchwork: 'In Patchwork',
  elsewhere: 'Somewhere else, recorded here',
  longest_tenure: 'Longest-tenured member',
  instance_admin: 'Instance admin intervenes',
  freeze: 'Patch freezes',
  open: 'Open',
  approval_required: 'Approval required',
  invite_only: 'Invite only',
};

const DURATION_LABELS = {
  24: '24 hours',
  48: '48 hours',
  72: '3 days',
  168: '1 week',
  336: '2 weeks',
};

/** The words for one rule, or its own name humanized where it has none. */
export function ruleLabel(key) {
  return RULE_LABELS[key] || key.replace(/_/g, ' ').replace(/^./, (c) => c.toUpperCase());
}

/** The words for one rule's value. */
export function formatRuleValue(key, val) {
  if (val === undefined || val === null) return '—';
  if (key === 'quorum_percent') return `${val}%`;
  if (key === 'default_vote_duration_hours') return DURATION_LABELS[val] || `${val}h`;
  if (key === 'amendment_auto_apply') return val ? 'Yes' : 'No';
  if (key === 'min_voting_tenure_days') {
    if (val === 0) return 'Immediate';
    return `${val} days`;
  }
  if (key === 'follower_permissions' && typeof val === 'object') {
    const perms = [];
    if (val.events !== false) perms.push('Events');
    if (val.proposals !== false) perms.push('Proposals');
    if (val.charters === true) perms.push('Charters');
    if (val.members !== false) perms.push('Members');
    return perms.length ? perms.join(', ') : 'None';
  }
  if (typeof val === 'boolean') return val ? 'Yes' : 'No';
  return VALUE_LABELS[val] || String(val);
}

export function deepEqual(a, b) {
  if (a === b) return true;
  if (typeof a !== typeof b) return false;
  if (typeof a === 'object' && a !== null && b !== null) {
    const keysA = Object.keys(a);
    const keysB = Object.keys(b);
    if (keysA.length !== keysB.length) return false;
    return keysA.every((k) => deepEqual(a[k], b[k]));
  }
  return false;
}

/**
 * Every key either side carries, not just the ones named above. A rules
 * file holds fields this editor does not edit, and a reader that only knows
 * its own labels would let one of them change without saying so — which is
 * the worse half of a governance record: silence that reads as "nothing
 * else moved".
 */
export function ruleKeys(currentRules, proposedRules) {
  return [...new Set([
    ...Object.keys(RULE_LABELS),
    ...Object.keys(currentRules || {}),
    ...Object.keys(proposedRules || {}),
  ])];
}

/** The rules that moved, in the order the diff lists them. */
export function changedRuleFields(currentRules, proposedRules) {
  if (!currentRules || !proposedRules) return [];
  return ruleKeys(currentRules, proposedRules)
    .filter((key) => !deepEqual(currentRules[key], proposedRules[key]));
}

// How many rules a one-line summary names before it stops naming and
// counts. Five is where a proposal title stops being readable in a list;
// the leading count and the "and N more" together still account for every
// rule that moved, so nothing is described as the whole that is not.
const NAMED_LIMIT = 5;

/**
 * One line describing the whole change, for the Summary of changes field
 * and so for the governance Record.
 *
 * One rule gets its old and new value, which is the whole fact. More than
 * one leads with the count, because a title that opens by naming a setting
 * reads as though that setting were all that moved. Where there are too
 * many to list, the count still covers the ones it does not name.
 */
export function summarizeRuleChanges(currentRules, proposedRules) {
  const changed = changedRuleFields(currentRules, proposedRules);
  if (changed.length === 0) return 'Update governance rules';
  if (changed.length === 1) {
    const key = changed[0];
    const from = formatRuleValue(key, currentRules[key]);
    const to = formatRuleValue(key, proposedRules[key]);
    return `${ruleLabel(key)}: ${from} → ${to}`;
  }
  const named = changed.slice(0, NAMED_LIMIT).map(ruleLabel);
  const rest = changed.length - named.length;
  const list = rest > 0 ? `${named.join(', ')}, and ${rest} more` : named.join(', ');
  return `${changed.length} governance rules changed: ${list}`;
}
