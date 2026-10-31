/**
 * Whether AI made what is posted (docs/adr/2026-10-20-a-post-says-whether-ai-made-it.md).
 *
 * An answer is 'none', 'assisted' or 'generated', or null for not asked or
 * not stated. Only 'generated' is ever shown to readers. The quilt's
 * `ai_disclosure` mode decides whether forms ask: 'off' never asks and sends
 * nothing, 'optional' asks and blocks nothing, 'required' asks and refuses
 * to submit until every question on the form is answered.
 */

export const AI_USE_OPTIONS = [
  { value: 'none', label: 'No AI' },
  { value: 'assisted', label: 'AI helped (editing, cleanup, translation)' },
  { value: 'generated', label: 'AI made this' },
];

// The value a form sends for one question. Off sends nothing, so a PATCH
// leaves the stored answer alone; otherwise the form sends what it shows,
// including null for a question left unanswered.
export function aiField(mode, value) {
  if (mode === 'off') return undefined;
  return value || null;
}

// The message to show when the quilt requires an answer and one of the
// questions the form showed is still unanswered, or '' when nothing is
// missing. Pass only the values of questions that are on screen: the image
// question is not asked without an image.
export function aiAnswerMissing(mode, ...values) {
  if (mode !== 'required') return '';
  return values.some((v) => !v) ? 'Say whether AI was used before posting.' : '';
}

// A row in a list shows one tag for the whole item: whether its words or its
// image were made with AI. The item's own page says which.
export function listAiValue(item) {
  return item?.ai_use === 'generated' || item?.image_ai === 'generated' ? 'generated' : null;
}

// The report reason a false or missing answer is reported under (decision
// 7). It travels as this code rather than as its words, because the queues
// act on it: the first moderation action is to apply the label.
export const UNLABELLED_AI = 'unlabelled_ai';

// A report's reason in words. Every other reason is already its own words.
export function reportReasonLabel(reason) {
  return reason === UNLABELLED_AI ? 'AI-made content without a label' : reason;
}
