<script>
  /**
   * Verification settings for an unclaimed patch (docs/adr/030). Two pre-claim
   * concerns the instance admin owns: the verification domain — the one vetted
   * trust anchor every self-service claim proves control of — and the current
   * claim state. Both are instance-admin surfaces; the section only appears for
   * unclaimed patches (patchSettingsSections).
   */
  import { getContext } from 'svelte';
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';

  const patch = getContext('patch');
  let slug = $derived(patch.value.slug);
  let node = $derived(patch.value.node);

  // Editable copy of the domain, seeded from the loaded value and re-seeded
  // whenever the patch reloads (after a save).
  let domain = $state('');
  let seeded = false;
  $effect(() => {
    if (!seeded && patch.value.verificationDomain !== undefined) {
      domain = patch.value.verificationDomain || '';
      seeded = true;
    }
  });

  let dirty = $derived(domain.trim() !== (patch.value.verificationDomain || ''));
  let saving = $state(false);

  async function saveDomain() {
    saving = true;
    try {
      await api(`admin/nodes/${slug}/verification-domain`, {
        method: 'PATCH',
        body: { domain: domain.trim() },
      });
      showToast(domain.trim() ? 'Verification domain saved' : 'Verification domain cleared', 'success');
      seeded = false;
      patch.value.reload();
    } catch (e) {
      showToast(e.message || 'Failed to save verification domain', 'error');
    } finally {
      saving = false;
    }
  }

  // Claim state: how many claims are open on this patch right now. Read from
  // the instance-wide pending queue and narrowed to this node — review itself
  // lives in the admin panel, linked below.
  let openClaims = $state(0);
  let claimsLoaded = $state(false);
  $effect(() => {
    if (node?.id) loadClaims(node.id);
  });

  async function loadClaims(nodeID) {
    try {
      const data = await api('admin/claims?status=pending');
      const items = data.items || [];
      openClaims = items.filter((c) => c.node_id === nodeID).length;
    } catch {
      openClaims = 0;
    } finally {
      claimsLoaded = true;
    }
  }
</script>

<div class="verification">
  <section class="v-section">
    <h3 class="v-heading">Verification domain</h3>
    <p class="muted v-hint">
      The one domain the platform has vetted as belonging to this organization.
      Self-service claims prove control of it (DNS record or a meta tag). Leave
      it empty to require admin review for every claim. This is separate from
      the website field, which carries no trust.
    </p>
    <div class="v-field">
      <input
        type="text"
        class="v-input"
        placeholder="example.org"
        bind:value={domain}
        disabled={saving}
        autocomplete="off"
        spellcheck="false"
      />
      {#if dirty}
        <div class="v-actions">
          <button class="btn btn-primary btn-sm" onclick={saveDomain} disabled={saving}>
            {saving ? 'Saving…' : 'Save'}
          </button>
          <button
            class="btn btn-secondary btn-sm"
            onclick={() => { domain = patch.value.verificationDomain || ''; }}
            disabled={saving}
          >Cancel</button>
        </div>
      {/if}
    </div>
    {#if !domain.trim()}
      <p class="muted v-note">No verification domain set — claims can only be approved by admin review.</p>
    {/if}
  </section>

  <section class="v-section">
    <h3 class="v-heading">Claim state</h3>
    <p class="muted v-hint">
      This patch is unclaimed. When someone proves they run it, claiming turns
      it into their patch.
    </p>
    {#if claimsLoaded}
      {#if openClaims > 0}
        <p class="v-claims">
          {openClaims} open {openClaims === 1 ? 'claim' : 'claims'} awaiting review.
        </p>
      {:else}
        <p class="muted v-note">No open claims.</p>
      {/if}
    {/if}
    <a
      href="/admin/claims"
      class="btn btn-secondary btn-sm"
      onclick={(e) => { e.preventDefault(); navigate('/admin/claims'); }}
    >Review claims</a>
  </section>
</div>

<style>
  .verification {
    max-width: 520px;
  }

  .v-section {
    padding: 0.75rem 0;
    border-top: 1px solid var(--color-border);
  }

  .v-section:first-child {
    border-top: none;
  }

  .v-heading {
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
    margin-bottom: 0.4rem;
  }

  .v-hint {
    font-size: 0.8rem;
    line-height: 1.5;
    margin-bottom: 0.75rem;
  }

  .v-field {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .v-input {
    flex: 1;
    min-width: 200px;
    padding: 0.45rem 0.6rem;
    font-size: 0.88rem;
    border: 1px solid var(--color-border);
    border-radius: 4px;
    background: var(--color-surface);
    color: var(--color-text);
    font-family: inherit;
  }

  .v-input:focus {
    outline: none;
    border-color: var(--color-primary);
  }

  .v-actions {
    display: flex;
    gap: 0.4rem;
  }

  .v-note {
    font-size: 0.8rem;
    margin-top: 0.5rem;
  }

  .v-claims {
    font-size: 0.88rem;
    font-weight: 500;
    margin-bottom: 0.75rem;
  }

  .btn-sm {
    padding: 0.25rem 0.6rem;
    font-size: 0.78rem;
  }
</style>
