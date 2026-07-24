<script>
  /**
   * Report affordance — lets any signed-in person flag a patch, event, or
   * person to the instance admins. Posts to POST /api/v1/reports, which
   * feeds the admin review queue (/admin/reports). Renders nothing for
   * signed-out visitors or when there's no target id yet.
   *
   * The reason vocabulary mirrors the "Unacceptable behavior" section of
   * the default lining (internal/governance/defaults.go), so what people
   * report against matches the standards they agreed to.
   */
  import { Flag } from 'phosphor-svelte';
  import Modal from './Modal.svelte';
  import { api } from '../lib/api.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';

  let {
    entityType,       // 'node' | 'event' | 'user'
    entityId = '',
    entityName = '',
  } = $props();

  const REASONS = [
    'Harassment or intimidation',
    'Hate speech or discrimination',
    'Threats or incitement',
    'Sharing private information',
    'Spam or scam',
    'Impersonation',
    'Something else',
  ];

  const NOUN = { node: 'patch', event: 'event', user: 'person' };

  let open = $state(false);
  let reason = $state(REASONS[0]);
  let details = $state('');
  let submitting = $state(false);

  function reset() {
    reason = REASONS[0];
    details = '';
  }

  async function submit() {
    if (submitting) return;
    submitting = true;
    try {
      await api('reports', {
        method: 'POST',
        body: { entity_type: entityType, entity_id: entityId, reason, details },
      });
      showToast('Report submitted. An admin will review it.', 'success');
      open = false;
      reset();
    } catch (e) {
      showToast(e.message || 'Could not submit report', 'error');
    } finally {
      submitting = false;
    }
  }
</script>

{#if isLoggedIn() && entityId}
  <button class="report-trigger" onclick={() => (open = true)} title="Report this {NOUN[entityType]}">
    <Flag size={13} weight="duotone" />
    <span>Report</span>
  </button>

  <Modal {open} label="Report this {NOUN[entityType]}" onClose={() => { open = false; }}>
    <h2 class="report-title">Report this {NOUN[entityType]}</h2>
    <p class="report-sub">
      Reports go to the instance admins, who review them against the
      community lining.{entityName ? ` You're reporting ${entityName}.` : ''}
    </p>

    <label class="form-label">
      Reason
      <select bind:value={reason}>
        {#each REASONS as r}<option value={r}>{r}</option>{/each}
      </select>
    </label>

    <label class="form-label">
      Details <span class="optional">(optional)</span>
      <textarea
        bind:value={details}
        rows="4"
        placeholder="Anything that helps an admin understand what happened."
      ></textarea>
    </label>

    <div class="report-actions">
      <button class="btn btn-secondary" onclick={() => { open = false; }} disabled={submitting}>
        Cancel
      </button>
      <button class="btn btn-primary" onclick={submit} disabled={submitting}>
        {submitting ? 'Submitting…' : 'Submit report'}
      </button>
    </div>
  </Modal>
{/if}

<style>
  .report-trigger {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.25rem 0.5rem;
    border: none;
    background: none;
    color: var(--color-text-muted);
    font-size: 0.8rem;
    font-weight: 500;
    cursor: pointer;
    border-radius: var(--radius);
    transition: color 100ms ease, background 100ms ease;
  }

  .report-trigger:hover {
    color: var(--color-text);
    background: var(--color-overlay);
  }

  .report-title {
    font-size: 1.15rem;
    font-weight: 700;
    margin-bottom: 0.4rem;
    padding-right: 1.5rem;
  }

  .report-sub {
    font-size: 0.85rem;
    color: var(--color-text-muted);
    line-height: 1.5;
    margin-bottom: 1.25rem;
  }

  .form-label {
    display: block;
    font-size: 0.82rem;
    font-weight: 600;
    margin-bottom: 1rem;
  }

  .optional {
    font-weight: 400;
    color: var(--color-text-muted);
  }

  .form-label select,
  .form-label textarea {
    display: block;
    width: 100%;
    margin-top: 0.4rem;
    padding: 0.5rem 0.6rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-bg);
    color: var(--color-text);
    font-size: 0.88rem;
    font-family: inherit;
  }

  .form-label textarea {
    resize: vertical;
  }

  .report-actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    margin-top: 0.5rem;
  }
</style>
