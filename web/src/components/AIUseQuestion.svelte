<script>
  /**
   * The AI question (docs/adr/2026-10-20-a-post-says-whether-ai-made-it.md,
   * decision 2): three answers and none preselected, because an unchecked
   * box would be an answer by silence (docs/adr/082). Renders nothing when
   * the quilt has the question off, and marks itself required when the
   * quilt requires it. The form checks completeness before submitting
   * (aiAnswerMissing in lib/aiUse.js).
   */
  import { getAiDisclosure } from '../stores/quilt.svelte.js';
  import { AI_USE_OPTIONS } from '../lib/aiUse.js';

  let {
    value = $bindable(null),
    legend = 'Did you use AI to write this?',
    disabled = false,
    // Called with the picked answer, for a host that saves on pick rather
    // than with a form.
    onchange = null,
  } = $props();

  const name = $props.id();
  let mode = $derived(getAiDisclosure());
</script>

{#if mode !== 'off'}
  <fieldset class="ai-use">
    <legend class="lt-label">
      <span>{legend}{#if mode === 'required'}{' '}<span class="lt-required">*</span>{/if}</span>
    </legend>
    {#each AI_USE_OPTIONS as opt (opt.value)}
      <label class="lt-check">
        <input
          type="radio"
          {name}
          value={opt.value}
          bind:group={value}
          {disabled}
          required={mode === 'required'}
          onchange={() => onchange?.(opt.value)}
        />
        <span>{opt.label}</span>
      </label>
    {/each}
  </fieldset>
{/if}

<style>
  .ai-use {
    border: none;
    padding: 0;
    margin: 0;
    min-width: 0;
  }

  .ai-use legend {
    padding: 0;
    margin-bottom: 0.15rem;
  }

  .ai-use .lt-check {
    min-height: 2rem;
  }
</style>
