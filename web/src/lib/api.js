/**
 * API client wrapper for Patchwork backend.
 * Prepends /api/v1/, sets headers, parses JSON, throws on errors.
 */
import { viewingAsVisitor } from './preview.js';

/**
 * Seeing a patch the way a stranger sees it, without signing out (F-126).
 *
 * While the page's URL carries `as=visitor`, every GET carries it too and
 * the server answers as though nobody were signed in — through the same
 * gates the public goes through, so the preview cannot drift from the
 * thing it previews. Writes never carry it: the server ignores it on
 * anything but a GET, and sending it anyway would only invite the
 * question.
 *
 * The parameter is read off the URL on every call rather than held in a
 * variable somebody sets. A flag assigned from an effect is assigned
 * *after* the components mounted under it have already fetched, which is
 * exactly what went wrong first time: the profile head asked before the
 * flag existed and came back holding admin standing, so the page drew a
 * Settings door over a visitor's view of the rooms below it.
 */

export async function api(path, options = {}) {
  const method = (options.method || 'GET').toUpperCase();
  let cleaned = path.replace(/^\//, '');
  if (method === 'GET' && viewingAsVisitor()) {
    cleaned += (cleaned.includes('?') ? '&' : '?') + 'as=visitor';
  }
  const url = `/api/v1/${cleaned}`;

  const headers = {
    'Content-Type': 'application/json',
    ...(options.headers || {}),
  };

  // Add mutation header for non-GET requests
  if (method !== 'GET') {
    headers['X-Patchwork-Request'] = 'true';
  }

  const fetchOptions = {
    method,
    headers,
    credentials: 'same-origin',
  };

  if (options.body !== undefined && method !== 'GET') {
    fetchOptions.body = typeof options.body === 'string'
      ? options.body
      : JSON.stringify(options.body);
  }

  const res = await fetch(url, fetchOptions);

  // Handle 204 No Content
  if (res.status === 204) {
    return null;
  }

  let data;
  try {
    data = await res.json();
  } catch {
    if (!res.ok) {
      throw new Error(`Request failed: ${res.status} ${res.statusText}`);
    }
    return null;
  }

  if (!res.ok) {
    const message = data?.error || data?.message || `Request failed: ${res.status}`;
    const err = new Error(message);
    err.status = res.status;
    err.data = data;
    throw err;
  }

  return data;
}
