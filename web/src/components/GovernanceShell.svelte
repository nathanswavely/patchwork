<script>
  import { getContext } from 'svelte';
  import SettingsShell from './SettingsShell.svelte';

  let { activeSection = 'overview', children } = $props();

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);

  // Whoever is not in the room reads the patch's answer on its deliberation
  // (docs/adr/2026-09-18-the-default-should-match-the-assumption.md). A
  // follower is an outsider here, exactly as they are for the member list:
  // following needs nobody's approval, so counting one as an insider would
  // let any signed-in stranger admit themselves by clicking Follow.
  let recordInsider = $derived(
    patch.value.isAdmin ||
    patch.value.membershipRole === 'member' ||
    patch.value.membershipRole === 'admin'
  );
  let recordWithheld = $derived(
    !recordInsider && patch.value.node?.public_governance_record === 'nobody'
  );

  // Canonical paths (ADR 003) — the current URL is always canonical, so
  // active-state matching depends on these (legacy hrefs never match).
  //
  // Proposals and Record are dropped rather than left to render empty. A tab
  // is navigation, and a door onto a room the server has emptied is a false
  // door (docs/adr/042) — the inverse of F-052, where `charters: false` hid a
  // room that had public documents in it. Documents stays: charters carry
  // their own per-document visibility (docs/adr/036) and this setting does
  // not reach them.
  let sections = $derived([
    { label: 'Overview', href: `/patches/${slug}/governance` },
    { label: 'Documents', href: `/patches/${slug}/governance/docs` },
    ...(recordWithheld ? [] : [
      { label: 'Proposals', href: `/patches/${slug}/governance/proposals` },
      { label: 'Record', href: `/patches/${slug}/governance/record` },
    ]),
  ]);
</script>

<SettingsShell title="Governance" {sections}>
  {#snippet children()}
    {@render children()}
  {/snippet}
</SettingsShell>
