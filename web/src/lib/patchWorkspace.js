/**
 * Which workspace surfaces a patch exposes, given the viewer's relationship
 * and the patch's claim state. Kept as pure functions so the tab/section
 * subset is unit-testable on its own — the Svelte shells only map the ids
 * returned here onto icons, hrefs, and page components.
 */

/**
 * Workspace tabs for the patch shell.
 *
 * Unclaimed patches (#6, docs/adr/026) get a purpose-built subset: nobody
 * runs them yet, so there is no governance and no membership to manage. The
 * only live surfaces are the community-recorded events calendar (with the
 * instance admin's submission-review queue) and, for that admin, a pared-down
 * Settings. Claimed patches keep the full role-gated tab row.
 *
 * @param {object} opts
 * @param {boolean} opts.isUnclaimed
 * @param {boolean} opts.isAdmin
 * @param {string}  opts.membershipRole
 * @param {object|null} opts.followerPermissions
 * @returns {Array<{id: string, label: string}>}
 */
export function workspaceTabs({
  isUnclaimed = false,
  isAdmin = false,
  membershipRole = '',
  followerPermissions = null,
} = {}) {
  if (isUnclaimed) {
    const t = [{ id: 'events', label: 'Events' }];
    if (isAdmin) t.push({ id: 'settings', label: 'Settings' });
    return t;
  }

  const t = [{ id: 'governance', label: 'Governance' }];
  const fp = followerPermissions;
  const isFollower = membershipRole === 'follower';

  if (!isFollower || fp?.members !== false)
    t.push({ id: 'members', label: 'Members' });
  if (!isFollower || fp?.events !== false)
    t.push({ id: 'events', label: 'Events' });
  if (isAdmin)
    t.push({ id: 'settings', label: 'Settings' });

  return t;
}

/**
 * Patch Settings sections.
 *
 * Unclaimed patches drop Members and Notifications — both meaningless for a
 * patch with no membership — and gain Verification, the pre-claim concerns
 * (the trust anchor and claim state, docs/adr/030). Info, Appearance, and the
 * Danger Zone (archiving still applies) are shared with claimed patches.
 *
 * @param {object} opts
 * @param {boolean} opts.isUnclaimed
 * @returns {Array<{id: string, label: string}>}
 */
export function patchSettingsSections({ isUnclaimed = false } = {}) {
  if (isUnclaimed) {
    return [
      { id: 'info', label: 'Patch Info' },
      { id: 'appearance', label: 'Appearance' },
      { id: 'sources', label: 'Event Sources' },
      { id: 'verification', label: 'Verification' },
      { id: 'danger', label: 'Danger Zone' },
    ];
  }

  return [
    { id: 'info', label: 'Patch Info' },
    { id: 'appearance', label: 'Appearance' },
    { id: 'members', label: 'Members' },
    { id: 'sources', label: 'Event Sources' },
    { id: 'notifications', label: 'Notifications' },
    { id: 'danger', label: 'Danger Zone' },
  ];
}
