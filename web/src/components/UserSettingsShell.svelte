<script>
  import SettingsShell from './SettingsShell.svelte';

  let { children } = $props();

  const sections = [
    { label: 'Profile', href: '/settings' },
    { label: 'Notifications', href: '/settings/notifications' },
    { label: 'Security', href: '/settings/security' },
    { label: 'My Patches', href: '/settings/patches' },
    { label: 'My Quilts', href: '/settings/quilts' },
  ];
</script>

<!-- No padding here — this shell only ever renders inside SocialShell's
     .social-main, which owns the content padding (issue #17). The global
     .work-content class is intentionally not used: it's shared with
     AdminShell/PatchShell, which live outside .social-main and still need
     their own padding. -->
<div>
  <SettingsShell title="Settings" {sections}>
    {#snippet children()}
      {@render children()}
    {/snippet}
  </SettingsShell>
</div>
