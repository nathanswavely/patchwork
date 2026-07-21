<script>
  import GovernanceShell from '../components/GovernanceShell.svelte';
  import GovernanceOverview from '../components/GovernanceOverview.svelte';
</script>

<GovernanceShell activeSection="overview">
  {#snippet children()}
    <GovernanceOverview />
  {/snippet}
</GovernanceShell>
