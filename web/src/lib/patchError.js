/**
 * What a person is told when a patch will not load.
 *
 * The two components that render a patch printed the API's own words under
 * their heading, so an unreachable patch read "Patch not found / node not
 * found": two registers stacked, one of them a term CONTEXT.md and CLAUDE.md
 * both say never reaches the UI. A printmaker, on hitting it: "Twice, in two
 * registers, one of them a word I have never seen anywhere else on this site.
 * node. I am a printmaker. I do not know what a node is."
 *
 * Mapping the backend's generic terms to the product's is the frontend's job,
 * which is the whole reason the backend is allowed generic terms at all. A
 * 404's heading already says everything there is to say, so it gets no
 * second line; anything else is a failure rather than a vocabulary, and the
 * server's text is the most useful thing there is to print about it.
 */
export function patchLoadError(e) {
  // An archived patch, told only to somebody who administers it — the
  // server sends this field to nobody else (docs/adr/034 keeps the refusal
  // absolute, and this changes what its own admin is told, not who is
  // refused).
  if (e?.data?.archived) {
    return 'This patch is archived, so nobody can open it. An instance admin can restore it.';
  }
  if (e?.status === 404) return '';
  return e?.message || 'Could not load this patch. Try again in a moment.';
}

/** The archived patch's name, for the heading. Empty for every other case. */
export function archivedPatchName(e) {
  return e?.data?.archived ? e.data.name || '' : '';
}
