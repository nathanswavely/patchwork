<script>
  let { currentRules = {}, proposedRules = {} } = $props();

  const LABELS = {
    decision_method: 'Decision Method',
    quorum_percent: 'Quorum',
    voting_period_hours: 'Default Voting Period',
    amendment_threshold: 'Amendment Threshold',
    auto_apply: 'Auto-Apply Amendments',
    succession_policy: 'Succession Policy',
    min_voting_tenure_days: 'Min Voting Tenure',
    membership_policy: 'Membership Policy',
    follower_permissions: 'Follower Permissions',
  };

  const VALUE_LABELS = {
    admin_decides: 'Admin decides',
    majority: 'Majority vote',
    supermajority: 'Supermajority (2/3)',
    consensus: 'Full consensus',
    longest_tenure: 'Longest-tenured member',
    instance_admin: 'Instance admin intervenes',
    freeze: 'Patch freezes',
    open: 'Open',
    approval_required: 'Approval required',
    invite_only: 'Invite only',
  };

  const DURATION_LABELS = {
    24: '24 hours',
    48: '48 hours',
    72: '3 days',
    168: '1 week',
    336: '2 weeks',
  };

  function formatValue(key, val) {
    if (val === undefined || val === null) return '\u2014';
    if (key === 'quorum_percent') return `${val}%`;
    if (key === 'voting_period_hours') return DURATION_LABELS[val] || `${val}h`;
    if (key === 'auto_apply') return val ? 'Yes' : 'No';
    if (key === 'min_voting_tenure_days') {
      if (val === 0) return 'Immediate';
      return `${val} days`;
    }
    if (key === 'follower_permissions' && typeof val === 'object') {
      const perms = [];
      if (val.events !== false) perms.push('Events');
      if (val.proposals !== false) perms.push('Proposals');
      if (val.charters !== false) perms.push('Charters');
      if (val.members !== false) perms.push('Members');
      return perms.length ? perms.join(', ') : 'None';
    }
    return VALUE_LABELS[val] || String(val);
  }

  function deepEqual(a, b) {
    if (a === b) return true;
    if (typeof a !== typeof b) return false;
    if (typeof a === 'object' && a !== null && b !== null) {
      const keysA = Object.keys(a);
      const keysB = Object.keys(b);
      if (keysA.length !== keysB.length) return false;
      return keysA.every(k => deepEqual(a[k], b[k]));
    }
    return false;
  }

  const ALL_KEYS = Object.keys(LABELS);

  let changedFields = $derived(
    ALL_KEYS.filter(key => !deepEqual(currentRules[key], proposedRules[key]))
  );

  let unchangedCount = $derived(ALL_KEYS.length - changedFields.length);
</script>

{#if changedFields.length > 0}
  <div class="rules-diff">
    <h3>Proposed Rule Changes</h3>
    <div class="diff-list">
      {#each changedFields as key}
        <div class="diff-row">
          <span class="diff-label">{LABELS[key]}</span>
          <span class="diff-values">
            <span class="diff-old">{formatValue(key, currentRules[key])}</span>
            <span class="diff-arrow">&rarr;</span>
            <span class="diff-new">{formatValue(key, proposedRules[key])}</span>
          </span>
        </div>
      {/each}
    </div>
    {#if unchangedCount > 0}
      <p class="unchanged-note">({unchangedCount} field{unchangedCount > 1 ? 's' : ''} unchanged)</p>
    {/if}
  </div>
{:else}
  <div class="rules-diff">
    <p class="muted">No changes to governance rules.</p>
  </div>
{/if}

<style>
  .rules-diff {
    padding: 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    margin-bottom: 1rem;
  }

  .rules-diff h3 {
    font-size: 0.9rem;
    margin-bottom: 0.75rem;
  }

  .diff-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .diff-row {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 1rem;
    font-size: 0.85rem;
    padding: 0.35rem 0;
    border-bottom: 1px solid var(--color-border);
  }

  .diff-row:last-child {
    border-bottom: none;
  }

  .diff-label {
    font-weight: 500;
    color: var(--color-text);
    white-space: nowrap;
  }

  .diff-values {
    display: flex;
    align-items: baseline;
    gap: 0.4rem;
    text-align: right;
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .diff-old {
    color: var(--color-error);
    text-decoration: line-through;
    font-size: 0.82rem;
  }

  .diff-arrow {
    color: var(--color-text-muted);
    font-size: 0.8rem;
  }

  .diff-new {
    color: var(--color-success);
    font-weight: 600;
    font-size: 0.85rem;
  }

  .unchanged-note {
    font-size: 0.8rem;
    color: var(--color-text-muted);
    margin-top: 0.75rem;
    text-align: right;
  }
</style>
