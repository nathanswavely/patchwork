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
 * What actually happens when this viewer posts an event to this patch.
 *
 * Mirrors the server's rule in events.go (docs/adr/026) rather than the
 * patch's state, because the label has to name the outcome: "New event"
 * when it publishes, "Suggest an event" when it enters review (docs/adr/042).
 * The old profile-side check short-circuited on `isUnclaimed` before it
 * considered direct-post rights, so instance admins and trusted
 * contributors were offered a review queue they bypass.
 *
 * `isMemberOrAdmin` is the role test, not "has a membership row": following
 * is frictionless and grants no write rights, so a follower suggests like
 * anyone else. Note the node payload's `is_member` is true for followers
 * too — pass the role, not that flag.
 *
 * @returns {'direct'|'suggest'|'none'}
 */
export function eventPostingRight({
  signedIn = false,
  isInstanceAdmin = false,
  trustedContributor = false,
  isUnclaimed = false,
  isMemberOrAdmin = false,
  isBanned = false,
  submissionsEnabled = true,
  acceptSuggestions = false,
} = {}) {
  if (!signedIn || isBanned) return 'none';
  if (isInstanceAdmin) return 'direct';

  if (isUnclaimed) {
    if (trustedContributor) return 'direct';
    return submissionsEnabled ? 'suggest' : 'none';
  }

  if (isMemberOrAdmin) return 'direct';
  if (!submissionsEnabled) return 'none';
  return acceptSuggestions ? 'suggest' : 'none';
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
