<script>
  // Every word and every comparison in here comes from the shared module,
  // so this diff and the one-line summary the governance Record keeps
  // cannot come to describe the same change differently (web/src/lib/rulesDiff.js).
  import {
    ruleLabel,
    formatRuleValue,
    changedRuleFields,
    ruleKeys,
  } from '../lib/rulesDiff.js';

  // `heading` names what the list is: an applied change's page says what
  // changed, the editor and a pending proposal what would.
  let { currentRules = {}, proposedRules = {}, heading = 'Proposed Rule Changes' } = $props();

  let changedFields = $derived(changedRuleFields(currentRules, proposedRules));

  let unchangedCount = $derived(ruleKeys(currentRules, proposedRules).length - changedFields.length);
</script>

{#if changedFields.length > 0}
  <div class="rules-diff">
    <h3>{heading}</h3>
    <div class="diff-list">
      {#each changedFields as key}
        <div class="diff-row">
          <span class="diff-label">{ruleLabel(key)}</span>
          <span class="diff-values">
            <span class="diff-old">{formatRuleValue(key, currentRules[key])}</span>
            <span class="diff-arrow">&rarr;</span>
            <span class="diff-new">{formatRuleValue(key, proposedRules[key])}</span>
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
    <p class="lt-muted">No changes to governance rules.</p>
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
