/**
 * Narrow a list of tag names to the ones containing what was typed, keeping
 * the list's own order. One rule for the desktop filter card and the phone
 * filter sheet, so the same letters find the same tags on both.
 */
export function findTags(tags, text) {
  const needle = (text || '').trim().toLowerCase();
  return needle ? tags.filter((t) => t.toLowerCase().includes(needle)) : tags;
}
