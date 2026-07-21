<script>
  import { getLabel, getSubtitle, isFamiliar } from '../lib/vocab.js';

  let { term, tag = 'span' } = $props();

  const label = $derived(getLabel(term));
  const subtitle = $derived(getSubtitle(term));
  const familiar = $derived(isFamiliar());
</script>

{#if tag === 'h1'}
  <h1 class="vocab-label">
    {label}
    {#if !familiar && subtitle}
      <small class="vocab-subtitle">({subtitle})</small>
    {/if}
  </h1>
{:else if tag === 'h2'}
  <h2 class="vocab-label">
    {label}
    {#if !familiar && subtitle}
      <small class="vocab-subtitle">({subtitle})</small>
    {/if}
  </h2>
{:else if tag === 'h3'}
  <h3 class="vocab-label">
    {label}
    {#if !familiar && subtitle}
      <small class="vocab-subtitle">({subtitle})</small>
    {/if}
  </h3>
{:else}
  <span class="vocab-label">
    {label}
    {#if !familiar && subtitle}
      <small class="vocab-subtitle">({subtitle})</small>
    {/if}
  </span>
{/if}

<style>
  .vocab-label {
    display: inline;
  }
  .vocab-subtitle {
    font-weight: normal;
    color: var(--color-text-muted);
    font-size: 0.8em;
    margin-left: 0.3em;
  }
</style>
