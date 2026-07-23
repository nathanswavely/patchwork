/**
 * Diacritic-insensitive text matching, shared by every surface that matches
 * a typed query against names — the search dropdown and the search-chip
 * narrowing (quilt, cards, map, events). "tornado" finds Tornādo Tornädo:
 * queries are typed on plain keyboards; names are spelled how their owners
 * spell them. Neither side should have to give that up to meet the other.
 */

// NFD splits letters from their combining marks; stripping the marks folds
// ā/ä/á → a. Lowercasing happens here too so callers fold exactly once.
export function fold(s) {
  return (s || '').normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();
}

// Case- and diacritic-insensitive substring test.
export function textMatches(haystack, needle) {
  if (!needle) return true;
  return fold(haystack).includes(fold(needle));
}
