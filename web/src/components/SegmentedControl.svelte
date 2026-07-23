<script>
  let {
    options = [], // [{ value, label, title? }]
    value,
    label = '', // aria-label for the group
    onchange = () => {},
  } = $props();
</script>

<div class="seg" role="group" aria-label={label || undefined}>
  {#each options as opt (opt.value)}
    <button
      type="button"
      class:active={value === opt.value}
      aria-pressed={value === opt.value}
      title={opt.title}
      onclick={() => onchange(opt.value)}
    >{opt.label}</button>
  {/each}
</div>

<style>
  .seg {
    display: inline-flex;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
  }

  .seg button {
    padding: 0.3rem 0.75rem;
    border: none;
    background: var(--color-surface);
    font-size: 0.8rem;
    font-weight: 500;
    color: var(--color-text-muted);
    cursor: pointer;
  }

  .seg button + button {
    border-left: 1px solid var(--color-border);
  }

  .seg button.active {
    background: var(--color-primary);
    color: var(--color-btn-on-primary);
  }
</style>
