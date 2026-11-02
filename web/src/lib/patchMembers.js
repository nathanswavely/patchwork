import { allPages } from './pages.js';

/**
 * Every row of a patch's member listing, for a picker that has to offer all
 * of them (a nomination, a candidacy).
 *
 * The listing is paged like every other (docs/adr/095), so one request is
 * the first twenty people and a picker built on it silently hides the
 * twenty-first from nomination. The election picker once sent its page
 * token as `cursor=`, the server ignored it and answered page one again,
 * and member 101 onward never appeared (#367); `allPages` owns the token
 * now.
 *
 * `role` asks the server for one exact role rather than filtering a mixed
 * page here.
 */
export function allMembers(slug, { role = '', maxPages = 20 } = {}) {
  return allPages(`nodes/${slug}/members`, role ? { role } : {}, { maxPages });
}
