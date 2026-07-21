<script>
  import { getContext } from 'svelte';
  import { getPath, navigate, replaceRoute } from '../stores/router.svelte.js';
  import SettingsShell from '../components/SettingsShell.svelte';
  import PatchSettingsInfo from './PatchSettingsInfo.svelte';
  import PatchSettingsAppearance from './PatchSettingsAppearance.svelte';
  import PatchSettingsMembers from './PatchSettingsMembers.svelte';
  import PatchSettingsDanger from './PatchSettingsDanger.svelte';
  import PatchSettingsNotifications from './PatchSettingsNotifications.svelte';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let isAdmin = $derived(patch.value.isAdmin);

  let currentPath = $derived(getPath());

  let sections = $derived([
    { label: 'Patch Info', href: `/patches/${slug}/settings/info` },
    { label: 'Appearance', href: `/patches/${slug}/settings/appearance` },
    { label: 'Members', href: `/patches/${slug}/settings/members` },
    { label: 'Notifications', href: `/patches/${slug}/settings/notifications` },
    { label: 'Danger Zone', href: `/patches/${slug}/settings/danger` },
  ]);

  // Default to /info if at bare /settings path
  $effect(() => {
    if (currentPath === `/patches/${slug}/settings` || currentPath === `/patches/${slug}/settings/`) {
      replaceRoute(`/patches/${slug}/settings/info`);
    }
  });

  let activePage = $derived.by(() => {
    if (currentPath.endsWith('/appearance')) return 'appearance';
    if (currentPath.endsWith('/members')) return 'members';
    if (currentPath.endsWith('/notifications')) return 'notifications';
    if (currentPath.endsWith('/danger')) return 'danger';
    return 'info';
  });
</script>

{#if !isAdmin}
  <p class="muted">You do not have permission to view settings.</p>
{:else}
  <SettingsShell title="Patch Settings" {sections}>
    {#if activePage === 'info'}
      <PatchSettingsInfo />
    {:else if activePage === 'appearance'}
      <PatchSettingsAppearance />
    {:else if activePage === 'members'}
      <PatchSettingsMembers />
    {:else if activePage === 'notifications'}
      <PatchSettingsNotifications />
    {:else if activePage === 'danger'}
      <PatchSettingsDanger />
    {/if}
  </SettingsShell>
{/if}
