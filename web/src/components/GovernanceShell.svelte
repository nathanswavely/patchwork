<script>
  import { getContext } from 'svelte';
  import SettingsShell from './SettingsShell.svelte';

  let { activeSection = 'overview', children } = $props();

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);

  // Canonical paths (ADR 003) — the current URL is always canonical, so
  // active-state matching depends on these (legacy hrefs never match).
  let sections = $derived([
    { label: 'Overview', href: `/patches/${slug}/governance` },
    { label: 'Documents', href: `/patches/${slug}/governance/docs` },
    { label: 'Proposals', href: `/patches/${slug}/governance/proposals` },
  ]);
</script>

<SettingsShell title="Governance" {sections}>
  {#snippet children()}
    {@render children()}
  {/snippet}
</SettingsShell>
