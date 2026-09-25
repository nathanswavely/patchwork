/**
 * Whether this page is being read as a visitor sees it (F-126).
 *
 * The server decides this from the same `as=visitor` parameter, and when
 * it is set it answers every patch-scoped read as though nobody were
 * signed in. The client has to agree, or the page contradicts its own
 * banner: the first version showed a visitor's rooms under an admin's
 * Settings door, and then — once that was fixed — offered "Suggest an
 * event" and "Report" to a reader the server was treating as the public,
 * neither of which a signed-out visitor is shown.
 *
 * Read off the URL rather than held in a variable somebody sets, for the
 * reason written in lib/api.js: a flag assigned from an effect is assigned
 * after the components under it have already asked their questions.
 *
 * This is presentation only. Nothing here decides what anybody may do —
 * every such decision is the server's, and the server makes it from the
 * same parameter.
 */
export function viewingAsVisitor() {
  try {
    return new URLSearchParams(window.location.search).get('as') === 'visitor';
  } catch {
    return false;
  }
}

/**
 * isLoggedIn, as this patch page should answer it.
 *
 * A preview of what the public sees is a preview of a page with nobody
 * signed in, so the affordances that exist only for a signed-in reader
 * are the ones a preview must not show.
 */
export function signedInForPatchView(isLoggedInNow) {
  return isLoggedInNow && !viewingAsVisitor();
}
