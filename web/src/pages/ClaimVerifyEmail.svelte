<script>
  // Landing page for the email-claim verification link (docs/adr/030).
  // Requires an explicit confirm click: mail scanners prefetch links, so the
  // GET is read-only and only the button's POST completes the claim. No
  // login required — possessing the link is the proof, and ownership goes
  // to the claimant's account regardless of who clicks.
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';

  const token = new URLSearchParams(window.location.search).get('token') || '';

  let loading = $state(true);
  let info = $state(null);
  let error = $state('');
  let confirming = $state(false);
  let done = $state(false);

  $effect(() => { loadInfo(); });

  async function loadInfo() {
    loading = true;
    error = '';
    try {
      info = await api(`claims/verify-email?token=${encodeURIComponent(token)}`);
    } catch (e) {
      error = e.message || 'This verification link is not valid.';
    } finally {
      loading = false;
    }
  }

  async function handleConfirm() {
    confirming = true;
    error = '';
    try {
      const result = await api('claims/verify-email', { method: 'POST', body: { token } });
      done = true;
      showToast('Patch claimed! Ownership transferred.', 'success');
      navigate(`/patches/${result.slug}`);
    } catch (e) {
      error = e.message || 'Verification failed';
    } finally {
      confirming = false;
    }
  }
</script>

<div class="verify-email-page page-fade">
  <h1>Confirm your claim</h1>

  {#if loading}
    <Skeleton lines={3} height="1rem" />
  {:else if error && !info}
    <div class="card">
      <p class="error-text">{error}</p>
      <p class="muted">The link may have been used already, or the claim was withdrawn or resolved.</p>
    </div>
  {:else if info?.expired}
    <div class="card">
      <p>This verification link has expired.</p>
      <p class="muted">Request a new email from the claim page for <strong>{info.node_name}</strong>.</p>
      <a
        href="/patches/{info.slug}/claim"
        class="btn btn-secondary"
        onclick={(e) => { e.preventDefault(); navigate(`/patches/${info.slug}/claim`); }}
      >Go to claim page</a>
    </div>
  {:else if info}
    <div class="card">
      <p>You're confirming the claim of <strong>{info.node_name}</strong>.</p>
      <p class="muted">This transfers ownership of the listing to the account that opened the claim.</p>
      <button class="btn btn-primary" onclick={handleConfirm} disabled={confirming || done}>
        {confirming ? 'Confirming...' : 'Confirm claim'}
      </button>
    </div>
    {#if error}
      <p class="error-text" style="margin-top: 0.75rem;">{error}</p>
    {/if}
  {/if}
</div>

<style>
  .verify-email-page {
    max-width: 520px;
    margin: 0 auto;
  }

  h1 {
    font-size: 1.4rem;
    margin-bottom: 1rem;
  }

  .card p {
    margin-bottom: 0.75rem;
  }

  .card .btn {
    margin-top: 0.25rem;
  }
</style>
