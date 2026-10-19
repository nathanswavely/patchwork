// Whether a patch about to be created already exists under the same name.
//
// A founder created her press twice, four minutes apart, and nothing said so:
// the second one quietly took a suffixed address nobody chose
// (`loose-threads-press-c439`), started its own election, and kept ringing her
// bell after she archived it (F-033). Names are not unique here and must not
// be. docs/adr/034 accepts that a restore can leave two patches with one
// name, and two bands can share one. So this warns and never refuses.
//
// It answers from the listing the quilt already serves (GET /api/v1/nodes),
// which shows public active and unclaimed patches to anybody, plus the
// caller's own under scope=my, which is the only way a private patch of
// theirs can be found. Nothing here can reveal a private patch to someone
// outside it, because nothing here can see one.

/** Names compared the way a person reads them: case, padding and runs of spaces ignored. */
export function normalizePatchName(s) {
  return String(s ?? '').trim().replace(/\s+/g, ' ').toLocaleLowerCase();
}

/** Worth asking about: anything shorter is a name still being typed. */
export function worthChecking(name) {
  return normalizePatchName(name).length >= 3;
}

/**
 * The first patch whose name matches `name`, preferring one the caller is
 * already in: it is the likeliest one they meant, and the only way a private
 * patch of theirs is found at all. `mine` covers following as well as
 * belonging (that is what scope=my lists), so it chooses the link and never
 * the wording. Returns `{ slug, name, status, mine }` or null.
 */
export function findSameNamed(name, publicItems = [], myItems = []) {
  const want = normalizePatchName(name);
  if (!want) return null;
  const match = (n) => normalizePatchName(n?.name) === want;
  const mine = (myItems || []).find(match);
  if (mine) return { slug: mine.slug, name: mine.name, status: mine.status, mine: true };
  const other = (publicItems || []).find(match);
  if (other) return { slug: other.slug, name: other.name, status: other.status, mine: false };
  return null;
}
