/**
 * Vocabulary helper for progressive disclosure.
 * Maps backend terms to textile terms with plain-language subtitles.
 */

const VOCAB = {
  node_container: {
    textile: 'Patch',
    plain: 'Group',
    desc: 'A community, collective, or group',
  },
  thread: {
    textile: 'Thread',
    plain: 'Connection',
    desc: 'An inferred link from shared members between patches',
  },
  event: {
    textile: 'Event',
    plain: 'Event',
    desc: 'A scheduled happening',
  },
  proposal: {
    textile: 'Proposal',
    plain: 'Proposal',
    desc: 'A proposal for the community to vote on',
  },
  instance: {
    textile: 'Patchwork',
    plain: 'Instance',
    desc: 'A community platform instance',
  },
  governance: {
    textile: 'Charter',
    plain: 'Charter',
    desc: 'The community rules and guidelines',
  },
  fork: {
    textile: 'Fork',
    plain: 'Fork',
    desc: 'A community that branched off from another',
  },
};

const FAMILIAR_KEY = 'vocabulary_familiar';
const VISIT_KEY = 'vocabulary_visits';
const FAMILIAR_THRESHOLD = 5;

/**
 * Get the textile term for a backend concept.
 */
export function getLabel(key) {
  return VOCAB[key]?.textile || key;
}

/**
 * Get the plain-language subtitle for a backend concept.
 */
export function getSubtitle(key) {
  return VOCAB[key]?.plain || '';
}

/**
 * Check whether the user is familiar with the textile vocabulary.
 */
export function isFamiliar() {
  try {
    return localStorage.getItem(FAMILIAR_KEY) === 'true';
  } catch {
    return false;
  }
}

/**
 * Track visits and mark the user as familiar after enough visits.
 */
export function markFamiliar() {
  try {
    if (isFamiliar()) return;
    const visits = parseInt(localStorage.getItem(VISIT_KEY) || '0', 10) + 1;
    localStorage.setItem(VISIT_KEY, String(visits));
    if (visits >= FAMILIAR_THRESHOLD) {
      localStorage.setItem(FAMILIAR_KEY, 'true');
    }
  } catch {
    // localStorage unavailable
  }
}
