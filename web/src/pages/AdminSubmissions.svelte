<script>
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { navigate } from '../stores/router.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import TagPicker from '../components/TagPicker.svelte';

  let submissions = $state([]);
  let loading = $state(true);

  $effect(() => { loadSubmissions(); });

  // Trust anchor for self-service claims (docs/adr/030): suggested from the
  // submitter's website, but only applied because the reviewing admin says so.
  let domainInputs = $state({});
  // Tags start as what the submitter picked; the reviewer can correct them
  // before approving, and whatever is here is what the patch gets.
  let tagInputs = $state({});

  async function loadSubmissions() {
    loading = true;
    try {
      const data = await api('admin/submissions');
      submissions = data.items || [];
      const inputs = {};
      const tags = {};
      for (const sub of submissions) {
        inputs[sub.id] = sub.suggested_verification_domain || '';
        tags[sub.id] = sub.tags || [];
      }
      domainInputs = inputs;
      tagInputs = tags;
    } catch {
      submissions = [];
    } finally {
      loading = false;
    }
  }

  async function handleAction(id, action) {
    try {
      const body = { action };
      if (action === 'approve') {
        body.verification_domain = (domainInputs[id] || '').trim();
        body.tags = tagInputs[id] || [];
      }
      await api(`admin/submissions/${id}`, { method: 'PATCH', body });
      showToast(action === 'approve' ? 'Submission approved' : 'Submission rejected', 'success');
      await loadSubmissions();
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
  <h1>Patch Submissions</h1>
  <p class="muted" style="margin-bottom: 1.5rem;">Patches people have suggested.</p>

  {#if loading}
    <Skeleton lines={4} height="1rem" />
  {:else if submissions.length === 0}
    <p class="muted">No pending submissions.</p>
  {:else}
    <div class="submission-list">
      {#each submissions as sub (sub.id)}
        <div class="submission-card card">
          <div class="sub-header">
            <h3>{sub.name}</h3>
            <span class="muted">{formatDate(sub.created_at)}</span>
          </div>
          {#if sub.description}
            <p class="sub-desc">{sub.description}</p>
          {/if}
          {#if sub.website}
            <p class="sub-website"><a href={sub.website} target="_blank" rel="noopener">{sub.website}</a></p>
          {/if}
          <div class="sub-tags">
            <span class="field-label">Tags</span>
            <TagPicker bind:selected={tagInputs[sub.id]} />
            <span class="muted">The submitter's picks.</span>
          </div>
          <div class="sub-meta muted">
            Submitted by {sub.submitter_display_name || sub.submitter_username || 'unknown'}
          </div>
          <div class="sub-domain">
            <label for="vd-{sub.id}">Verified domain for claims</label>
            <input
              id="vd-{sub.id}"
              type="text"
              placeholder="none (admin review only)"
              bind:value={domainInputs[sub.id]}
            />
            <span class="muted">Owners prove control of this domain to claim the patch. Leave empty if you can't vouch for it.</span>
          </div>
          <div class="sub-actions">
            <button class="btn btn-primary btn-sm" onclick={() => handleAction(sub.id, 'approve')}>Approve</button>
            <button class="btn btn-danger btn-sm" onclick={() => handleAction(sub.id, 'reject')}>Reject</button>
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

  .submission-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .sub-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.35rem;
  }

  .sub-header h3 {
    font-size: 1rem;
  }

  .sub-desc {
    font-size: 0.88rem;
    color: var(--color-text-muted);
    margin-bottom: 0.5rem;
  }

  .sub-website {
    font-size: 0.82rem;
    margin-bottom: 0.5rem;
  }

  .sub-tags {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    margin-bottom: 0.5rem;
  }

  .sub-tags .field-label {
    font-size: 0.78rem;
    font-weight: 500;
  }

  .sub-tags .muted {
    font-size: 0.72rem;
  }

  .sub-meta {
    font-size: 0.78rem;
    margin-bottom: 0.5rem;
  }

  .sub-domain {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    margin-bottom: 0.65rem;
  }

  .sub-domain label {
    font-size: 0.78rem;
    font-weight: 500;
  }

  .sub-domain input {
    max-width: 320px;
  }

  .sub-domain .muted {
    font-size: 0.72rem;
  }

  .sub-actions {
    display: flex;
    gap: 0.5rem;
  }

</style>
