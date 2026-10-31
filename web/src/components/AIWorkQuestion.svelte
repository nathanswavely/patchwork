<script>
  /**
   * The patch-level declaration (docs/adr/2026-10-20-a-post-says-whether-ai-made-it.md,
   * decision 6): whether the work a patch makes relies heavily on AI. Yes
   * puts a public badge on the patch. Two answers and neither preselected,
   * so an unanswered question stays null rather than reading as No.
   * `onchange` lets a host save on pick instead of binding.
   */
  let {
    value = $bindable(null),
    disabled = false,
    onchange = null,
  } = $props();

  const name = $props.id();

  function pick(v) {
    value = v;
    onchange?.(v);
  }
</script>

<fieldset class="ai-work">
  <legend class="lt-label">
    <span>Does the work this patch makes or publishes rely heavily on AI-generated content?</span>
  </legend>
  <p class="lt-hint">
    For example: a newsletter written by a model, generated music or art, or
    listings a model writes. Yes shows an "AI work" badge on this patch.
  </p>
  <div class="ai-work-options">
    <label class="lt-check">
      <input type="radio" {name} checked={value === true} {disabled} onchange={() => pick(true)} />
      <span>Yes</span>
    </label>
    <label class="lt-check">
      <input type="radio" {name} checked={value === false} {disabled} onchange={() => pick(false)} />
      <span>No</span>
    </label>
  </div>
</fieldset>

<style>
  .ai-work {
    border: none;
    padding: 0;
    margin: 0;
    min-width: 0;
  }

  .ai-work legend {
    padding: 0;
    margin-bottom: 0.25rem;
  }

  .ai-work-options {
    display: flex;
    gap: 1.25rem;
  }
</style>
