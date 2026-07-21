<script>
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';

  let claims = $state([]);
  let loading = $state(true);

  $effect(() => { loadClaims(); });

  async function loadClaims() {
    loading = true;
    try {
      const data = await api('admin/claims');
      claims = data.items || [];
    } catch {
      claims = [];
    } finally {
      loading = false;
    }
  }

  async function handleAction(id, action) {
    try {
      await api(`admin/claims/${id}`, { method: 'PATCH', body: { action } });
      showToast(action === 'approve' ? 'Claim approved. Ownership transferred.' : 'Claim rejected', 'success');
      await loadClaims();
    } catch (e) {
      showToast(e.message || 'Failed', 'error');
    }
  }

  function formatDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  }
</script>

<div class="page-fade">
  <h1>Patch Claims</h1>
  <p class="muted" style="margin-bottom: 1.5rem;">Users requesting ownership of unclaimed patches.</p>

  {#if loading}
    <Skeleton lines={4} height="1rem" />
  {:else if claims.length === 0}
    <p class="muted">No pending claims.</p>
  {:else}
    <div class="claim-list">
      {#each claims as claim (claim.id)}
        <div class="claim-card card">
          <div class="claim-header">
            <h3>{claim.node_name}</h3>
            <span class="badge">{claim.method}</span>
          </div>
          <div class="claim-meta muted">
            Claimed by {claim.claimant_display_name || claim.claimant_username} &middot; {formatDate(claim.created_at)}
            {#if claim.verification_domain}
              &middot; verified domain: {claim.verification_domain}
            {/if}
            {#if claim.email}
              &middot; email: {claim.email}
            {/if}
          </div>
          {#if claim.evidence}
            <p class="claim-evidence">{claim.evidence}</p>
          {/if}
          <div class="claim-actions">
            <button class="btn btn-primary btn-sm" onclick={() => handleAction(claim.id, 'approve')}>Approve &amp; Transfer</button>
            <button class="btn btn-danger btn-sm" onclick={() => handleAction(claim.id, 'reject')}>Reject</button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  h1 {
    font-size: 1.2rem;
    margin-bottom: 0.25rem;
  }

  .claim-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .claim-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.25rem;
  }

  .claim-header h3 {
    font-size: 1rem;
  }

  .claim-meta {
    font-size: 0.78rem;
    margin-bottom: 0.5rem;
  }

  .claim-evidence {
    font-size: 0.85rem;
    padding: 0.5rem 0.75rem;
    background: var(--color-bg);
    border-radius: var(--radius);
    margin-bottom: 0.5rem;
  }

  .claim-actions {
    display: flex;
    gap: 0.5rem;
  }

  .btn-sm {
    padding: 0.25rem 0.6rem;
    font-size: 0.75rem;
  }
</style>
