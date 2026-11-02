import { api } from './api.js';

/**
 * Every row of a cursor-paged list endpoint, for a caller that has to hold
 * all of them rather than show the first page.
 *
 * Every list endpoint is paged (`parsePaginationParams`: twenty rows unless
 * asked, a hundred at most), so one request is the first page and a caller
 * that stops there silently loses the rest. The cursor goes back as
 * `after=`, the only name the server reads: a client that spells it
 * `cursor=` gets page one again forever (#367).
 *
 * Capped, and stops on a cursor that does not move, so a server that
 * ignores the cursor costs a few requests rather than a loop.
 *
 * `key` names the array for the one listing that does not call it `items`
 * (admin/nodes says `nodes`). `onPage` sees each whole payload, for a caller
 * that also wants what rides beside the rows: the members listing's
 * `invited` and counts, which no array of rows can carry.
 */
export async function allPages(path, params = {}, { maxPages = 20, key = 'items', onPage = null } = {}) {
  const found = [];
  let after = '';
  for (let page = 0; page < maxPages; page++) {
    const query = new URLSearchParams({ limit: '100', ...params });
    if (after) query.set('after', after);
    const data = await api(`${path}?${query}`);
    onPage?.(data);
    found.push(...(data?.[key] || []));
    const next = data?.next_cursor || '';
    if (!next || next === after) break;
    after = next;
  }
  return found;
}

/**
 * The caller's own relationships, `GET /api/v1/me/nodes`, all of them.
 *
 * Read by the membership store, the dashboard, Settings → Patches and the
 * event form, and each used to read one page: a person in more than twenty patches lost
 * every row past the twentieth, so a patch they followed said "Follow" on
 * its My Quilt card. `status: 'archived'` asks for the archived patches
 * they administer instead; `role: 'admin,member'` asks for active standing
 * in those roles only.
 */
export function allMyMemberships({ status = '', role = '' } = {}) {
  const params = {};
  if (status) params.status = status;
  if (role) params.role = role;
  return allPages('me/nodes', params);
}
