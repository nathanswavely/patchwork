/**
 * What each governance template answers to "who can join".
 *
 * Every template ships a governance-rules.json carrying a membership_policy
 * (internal/governance/defaults.go), and the setup form seeds its own "Who
 * can join" control from it — a claimant who picks Minimal is asking for an
 * invite-only patch, and the form should already say so before they read the
 * question. The values are mirrored here rather than fetched because the seed
 * has to be there the moment the template radio moves, and a round trip per
 * toggle would let the person submit ahead of the answer.
 *
 * Mirrored, so it can drift: `governance-template-policy.test.js` reads the
 * Go constants and fails when these disagree.
 */
export const TEMPLATE_MEMBERSHIP_POLICY = {
  minimal: 'invite_only',
  casual: 'open',
  collaborative: 'approval_required',
  formal: 'approval_required',
};

/**
 * The membership policy a template suggests, or '' for a template this map
 * does not know — a caller seeding a control must leave it alone rather than
 * seed it with a guess.
 *
 * @param {string} template
 * @returns {string}
 */
export function templateMembershipPolicy(template) {
  return TEMPLATE_MEMBERSHIP_POLICY[template] || '';
}
