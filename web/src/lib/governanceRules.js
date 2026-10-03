/**
 * The words this product uses for a patch's governance rules.
 *
 * Extracted so the page that *reads* the rules and the form that *changes*
 * them cannot come to call the same setting two different things. While
 * there was only one caller it did not matter; the moment there are two,
 * one definition is the difference between a member checking a rule and a
 * member checking two rules.
 *
 * Only the option wording lives here, not the form's field labels. A form
 * label ("Decision Method", beside a select) and a line on a page stating
 * what is true today are different registers, and forcing one string to be
 * both makes both worse.
 */

export const DECISION_OPTIONS = [
  { value: 'admin', label: 'Admin decides' },
  { value: 'majority', label: 'Majority vote' },
  { value: 'supermajority', label: 'Supermajority (2/3)' },
  { value: 'consensus', label: 'Full consensus' },
];

// Amendment thresholds are always votes. "Admin decides" is a decision
// method, not a threshold: under it, amendments never reach a vote.
export const THRESHOLD_OPTIONS = [
  { value: 'majority', label: 'Majority vote' },
  { value: 'supermajority', label: 'Supermajority (2/3)' },
  { value: 'consensus', label: 'Full consensus' },
];

export const VOTING_PERIOD_OPTIONS = [
  { value: 24, label: '24 hours' },
  { value: 48, label: '48 hours' },
  { value: 72, label: '72 hours (3 days)' },
  { value: 168, label: '1 week' },
  { value: 336, label: '2 weeks' },
];

// What happens when inactivity empties the last admin seat
// (internal/notifications/inactivity.go). The Collaborative and Formal
// templates store 'nomination' and 'election' here; the select used to know
// neither, so it rendered with nothing chosen and a save would have quietly
// rewritten the patch's policy. The sweep runs the longest-tenure rule for
// both today, and each hint says so rather than promising a mechanic the
// patch does not run.
//
// "The three longest-standing members step in" was wrong twice over after
// docs/adr/102. Interim promotion needs presence as well as tenure, and
// where nobody clears that bar it promotes nobody. Two people read the old
// sentence in the present tense against patches where nobody had stepped
// in: "Six elections have come and gone and the press has had no admins
// throughout. Either 'step in' means something other than what it says, or
// it did not happen." It did not happen, and the sentence should have
// allowed for that.
export const SUCCESSION_OPTIONS = [
  { value: 'longest_tenure', label: 'Longest-tenured members step in',
    hint: 'The longest-standing members who have taken part recently become interim admins. If nobody has, nobody steps in.' },
  { value: 'nomination', label: 'Nomination',
    hint: 'Admins name their successors. If none are left to, the longest-standing members who have taken part recently step in.' },
  { value: 'election', label: 'Election',
    hint: 'Members elect the next admins. Until then the longest-standing members who have taken part recently step in, and if nobody has the seats stay empty until the election.' },
  { value: 'instance_admin', label: 'Instance admin intervenes',
    hint: 'An instance admin is notified and decides who runs the patch.' },
  { value: 'freeze', label: 'Patch freezes',
    hint: 'Nobody is promoted; the patch keeps running with no admin.' },
];

export const TENURE_OPTIONS = [
  { value: 0, label: 'Immediate' },
  { value: 7, label: '7 days' },
  { value: 30, label: '30 days' },
  { value: 90, label: '90 days' },
];

// What each rule does, in one sentence. The method is the one knob whose
// consequence a founder cannot work out from its label: "Full consensus"
// reads as agreement and means any single member can defeat anything.
export const DECISION_HINTS = {
  admin: 'An admin decides every proposal. An admin can still put one to an advisory vote first, and the result comes back to them.',
  majority: 'More approvals than rejections carries a proposal.',
  supermajority: 'Two thirds of the ballots cast must approve.',
  consensus: 'One reject defeats a proposal, however many approve it.',
};

export const MEMBERSHIP_OPTIONS = [
  { value: 'open', label: 'Open' },
  { value: 'approval_required', label: 'Approval required' },
  { value: 'invite_only', label: 'Invite only' },
];

export const LEADERSHIP_LABELS = {
  maintainer: 'One maintainer',
  meritocratic: 'Admins nominate, members ratify',
  elected: 'Elected council',
};

export const VENUE_LABELS = {
  patchwork: 'Here, in Patchwork',
  elsewhere: 'Somewhere else, and recorded here',
};

/** The option's own words, or the stored value where it has none. */
export function wordFor(options, value) {
  const hit = options.find((o) => String(o.value) === String(value));
  if (hit) return hit.label;
  if (value === '' || value === undefined || value === null) return 'Not set';
  return String(value);
}

function days(n) {
  if (!n) return '';
  return n === 1 ? '1 day' : n + ' days';
}

/**
 * Every governance rule a patch is running, as label and value pairs, in
 * the order somebody asks about them.
 *
 * A rule belonging to a mechanic the patch does not run is left out rather
 * than shown empty: a maintainer patch has no term length, and printing
 * "Term length: not set" invites somebody to go and set one. docs/adr/049
 * is about exactly this kind of false narration.
 */
export function readableRules(rules, followerPermissions = null) {
  if (!rules) return [];
  const out = [];
  const elected = rules.leadership_model === 'elected';
  const decidesElsewhere = rules.proposal_venue === 'elsewhere';
  const leadsElsewhere = rules.leadership_venue === 'elsewhere';

  out.push({
    label: 'Where proposals are decided',
    value: VENUE_LABELS[rules.proposal_venue || 'patchwork'],
  });
  if (!decidesElsewhere) {
    out.push({
      label: 'How a proposal carries',
      value: wordFor(DECISION_OPTIONS, rules.decision_method),
      hint: DECISION_HINTS[rules.decision_method] || '',
    });
    out.push({
      label: 'Quorum',
      value: rules.quorum_percent > 0
        ? rules.quorum_percent + '% of those who may vote'
        : 'None. Any number of votes counts.',
    });
    // Only where the patch sets one. An admin-decided patch stores 0 here
    // and holds a vote only when its admin asks for advice, and `wordFor`
    // cannot read that 0 as absence the way it could for tenure, where 0
    // is a real answer meaning "immediately". A patch with no period read
    // "How long a vote stays open: 0".
    if (rules.default_vote_duration_hours > 0) {
      out.push({
        label: 'How long a vote stays open',
        value: wordFor(VOTING_PERIOD_OPTIONS, rules.default_vote_duration_hours),
      });
    }
    out.push({
      label: 'Before a new member may vote',
      value: rules.min_voting_tenure_days > 0 ? days(rules.min_voting_tenure_days) : 'Straight away',
    });
    out.push({
      label: 'Somebody a proposal is about',
      value: rules.subject_recusal ? 'Does not vote on it' : 'Votes like anyone else',
    });
    out.push({
      label: 'To amend a charter',
      value: wordFor(THRESHOLD_OPTIONS, rules.amendment_threshold),
    });
    out.push({
      label: 'An amendment that carries',
      value: rules.amendment_auto_apply ? 'Takes effect at once' : 'Waits for an admin to apply it',
    });
  }

  out.push({
    label: 'Where admins are chosen',
    value: VENUE_LABELS[rules.leadership_venue || 'patchwork'],
  });
  if (!leadsElsewhere) {
    out.push({
      label: 'How admins are made',
      value: LEADERSHIP_LABELS[rules.leadership_model] || 'Not set',
    });
    if (elected && rules.admin_term_months > 0) {
      out.push({ label: 'How long a term runs', value: rules.admin_term_months + ' months' });
    }
    if (elected && rules.nomination_days > 0) {
      out.push({ label: 'How long an election takes nominations', value: days(rules.nomination_days) });
    }
    if (rules.max_admins > 0) {
      out.push({ label: 'How many admins at once', value: String(rules.max_admins) });
    }
    if (rules.inactivity_days > 0) {
      // One literal, not three joined. Concatenating put the fragment
      // "Warned at" into the copy ledger as if it were a reviewable line;
      // the ledger drops this whole string instead, the way it already
      // drops every other value assembled around a number here. A
      // fragment nobody can judge is worse than an absence, and the
      // sentence this mirrors — on the council block, from F-115 — is in
      // the ledger and reviewed.
      out.push({
        label: 'An admin who stops taking part',
        value: `Warned at ${days(rules.inactivity_days)}, seat declared vacant at ${days(rules.inactivity_days * 2)}`,
      });
    }
    out.push({
      label: 'If the last admin seat empties',
      value: wordFor(SUCCESSION_OPTIONS, rules.succession_policy),
      hint: (SUCCESSION_OPTIONS.find((o) => o.value === rules.succession_policy) || {}).hint || '',
    });
  }

  // What a follower may reach. This is the half somebody came looking for
  // and could not find: the notice saying "you can grant it again in
  // Governance" is about these four (F-117).
  if (followerPermissions) {
    const granted = [
      followerPermissions.events && 'events',
      followerPermissions.proposals && 'proposals',
      followerPermissions.charters && 'charters',
      followerPermissions.members && 'the member list',
    ].filter(Boolean);
    out.push({
      label: 'What a follower can see',
      value: granted.length ? granted.join(', ') : 'Nothing beyond the patch page',
    });
  }
  return out;
}
