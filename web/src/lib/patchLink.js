/**
 * Recognising a pasted patch link (docs/adr/024).
 *
 * Browsing another quilt happens on their soil, so every path that starts
 * from a copied address — Connected Quilts, the scoped finder, an event's
 * co-host picker, and now a moved-to pointer (docs/adr/090) — has to decide
 * whether `https://host/patches/slug` names a patch it can open in-app.
 *
 * The shape was written out four times before this file existed. One copy is
 * a rule; four copies is a rule that will disagree with itself the first time
 * somebody widens it.
 */

const PATCH_LINK = /^https?:\/\/([^/]+)\/patches\/([a-z0-9-]+)\/?$/;

/**
 * Parse a pasted address into { host, slug }, or null when it is not a patch
 * link. A link to anything else on a quilt is not one: the remote patch card
 * can only render a patch.
 */
export function parsePatchLink(url) {
  const m = String(url || '').trim().match(PATCH_LINK);
  if (!m) return null;
  return { host: m[1], slug: m[2] };
}

/**
 * The in-app path for a pasted patch link, or '' when there is none.
 *
 * A link home resolves to the patch itself; anything else resolves to the
 * read-only remote patch card. `currentHost` is a parameter rather than a
 * read of `window` so this can be reasoned about (and tested) off a page.
 */
export function patchLinkPath(url, currentHost) {
  const parsed = parsePatchLink(url);
  if (!parsed) return '';
  if (parsed.host === currentHost) return `/patches/${parsed.slug}`;
  return `/quilts/${parsed.host}/patches/${parsed.slug}`;
}

/** The host of any address, for showing where a link goes. '' if unparseable. */
export function linkHost(url) {
  try {
    return new URL(String(url)).host.replace(/^www\./, '');
  } catch {
    return '';
  }
}
