/**
 * The two texts a governance diff compares, rebuilt from what
 * `GET /api/v1/governance/{id}/diff` actually sends.
 *
 * The server answers with `hunks` — a complete word-level diff, every run of
 * text tagged `equal`, `insert` or `delete` (internal/governance/diff.go) —
 * plus a `unified` patch. It has never sent `old_content`/`new_content`,
 * which is what the history page read, so it always fell back to the raw
 * patch text. The hunks are the whole of both versions: the old text is
 * every run that was not inserted, the new text every run that was not
 * deleted.
 *
 * Returns null when the payload carries no hunks, so a caller can fall back
 * to the unified text rather than render two empty documents as "no
 * changes".
 */
export function textsFromDiff(diff) {
  const hunks = diff?.hunks;
  if (!Array.isArray(hunks)) return null;
  let oldText = '';
  let newText = '';
  for (const h of hunks) {
    const text = h?.text ?? '';
    if (h?.type !== 'insert') oldText += text;
    if (h?.type !== 'delete') newText += text;
  }
  return { oldText, newText };
}
