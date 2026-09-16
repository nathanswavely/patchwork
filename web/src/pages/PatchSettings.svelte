<script>
  import { getContext } from 'svelte';
  import { getPath, navigate, replaceRoute } from '../stores/router.svelte.js';
  import { patchSettingsSections } from '../lib/patchWorkspace.js';
  import SettingsShell from '../components/SettingsShell.svelte';
  import PatchSettingsInfo from './PatchSettingsInfo.svelte';
  import PatchSettingsAppearance from './PatchSettingsAppearance.svelte';
  import PatchSettingsMembers from './PatchSettingsMembers.svelte';
  import PatchSettingsDanger from './PatchSettingsDanger.svelte';
  import PatchSettingsNotifications from './PatchSettingsNotifications.svelte';
  import PatchSettingsNoticeboard from './PatchSettingsNoticeboard.svelte';
  import PatchSettingsSources from './PatchSettingsSources.svelte';
  import PatchSettingsVerification from './PatchSettingsVerification.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let isAdmin = $derived(patch.value.isAdmin);
  let isUnclaimed = $derived(patch.value.isUnclaimed);

  // `isAdmin` answers two different questions with one word: the node
  // payload sets it for this patch's own admins and, separately, for any
  // instance admin (nodes.go, matching the write-side authz — an instance
  // admin really can edit any patch). So Settings opens either way, and
  // opened silently: nothing on the page said which of the two you were,
  // and someone who merely follows a patch can find themselves reading its
  // settings with no sign that it is the instance role doing it. Say so.
  // It changes nothing about what is permitted — it names whose authority
  // is being used, which is the part the page was leaving out.
  let asInstanceAdmin = $derived(isAdmin && patch.value.membershipRole !== 'admin');

  let currentPath = $derived(getPath());

  // Section subset depends on claim state: unclaimed patches drop Members and
  // Notifications and gain Verification (docs/adr/030). patchSettingsSections
  // owns that decision.
  let sectionDefs = $derived(patchSettingsSections({ isUnclaimed }));
  let sections = $derived(
    sectionDefs.map((s) => ({ label: s.label, href: `/patches/${slug}/settings/${s.id}` }))
  );

  let activePage = $derived.by(() => {
    if (currentPath.endsWith('/appearance')) return 'appearance';
    if (currentPath.endsWith('/members')) return 'members';
    if (currentPath.endsWith('/sources')) return 'sources';
    if (currentPath.endsWith('/notifications')) return 'notifications';
    if (currentPath.endsWith('/noticeboard')) return 'noticeboard';
    if (currentPath.endsWith('/verification')) return 'verification';
    if (currentPath.endsWith('/danger')) return 'danger';
    return 'info';
  });

  // Default to /info at the bare /settings path, and steer away from any
  // section that doesn't apply to this patch's claim state — a stale link or
  // a status change shouldn't strand someone on a section that isn't in the
  // sidebar (e.g. /members on an unclaimed patch, /verification on a claimed
  // one). Wait for the patch to load so the redirect reads the real state.
  $effect(() => {
    if (patch.value.loading) return;
    if (currentPath === `/patches/${slug}/settings` || currentPath === `/patches/${slug}/settings/`) {
      replaceRoute(`/patches/${slug}/settings/info`);
      return;
    }
    const valid = new Set(sectionDefs.map((s) => s.id));
    if (!valid.has(activePage)) {
      replaceRoute(`/patches/${slug}/settings/info`);
    }
  });
</script>

{#if !isAdmin}
  <p class="muted">You do not have permission to view settings.</p>
{:else}
  {#if asInstanceAdmin}
    <p class="instance-admin-note">
      You are here as an admin of this quilt, not as an admin of this patch.
      Changes you save are this patch's.
    </p>
  {/if}
  <SettingsShell title="Patch Settings" {sections}>
    {#if activePage === 'info'}
      <PatchSettingsInfo />
    {:else if activePage === 'appearance'}
      <PatchSettingsAppearance />
    {:else if activePage === 'members'}
      <PatchSettingsMembers />
    {:else if activePage === 'sources'}
      <PatchSettingsSources />
    {:else if activePage === 'notifications'}
      <PatchSettingsNotifications />
    {:else if activePage === 'noticeboard'}
      <PatchSettingsNoticeboard />
    {:else if activePage === 'verification'}
      <PatchSettingsVerification />
    {:else if activePage === 'danger'}
      <PatchSettingsDanger />
    {/if}
  </SettingsShell>
{/if}

<style>
  /* Sits above the settings shell rather than inside a section, because it
     is about the whole page: every section below is being edited under this
     authority, not just the first one. */
  .instance-admin-note {
    margin: 0 0 1rem;
    padding: 0.6rem 0.8rem;
    border: 1px solid var(--color-border);
    border-left: 3px solid var(--color-accent);
    border-radius: 4px;
    background: var(--color-surface);
    color: var(--color-text-muted);
    font-size: 0.85rem;
  }
</style>
