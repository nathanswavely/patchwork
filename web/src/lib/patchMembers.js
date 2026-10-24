import { api } from './api.js';

/**
 * Every row of a patch's member listing, for a picker that has to offer all
 * of them (a nomination, a candidacy).
 *
 * The listing is paged like every other (docs/adr/095), so one request is
 * the first twenty people and a picker built on it silently hides the
 * twenty-first from nomination. The cursor goes back as `after=`, which is
 * what `parsePaginationParams` reads: the election picker sent `cursor=`,
 * the server ignored it and answered page one again, and member 101 onward
 * never appeared (#367).
 *
 * `role` asks the server for one exact role rather than filtering a mixed
 * page here. Capped, and stops on a cursor that does not move, so a server
 * that ignores the cursor costs a few requests rather than a loop.
 */
export async function allMembers(slug, { role = '', maxPages = 20 } = {}) {
  const found = [];
  let after = '';
  for (let page = 0; page < maxPages; page++) {
    const params = new URLSearchParams({ limit: '100' });
    if (role) params.set('role', role);
    if (after) params.set('after', after);
    const data = await api(`nodes/${slug}/members?${params}`);
    found.push(...(data?.items || []));
    const next = data?.next_cursor || '';
    if (!next || next === after) break;
    after = next;
  }
  return found;
}
