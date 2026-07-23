<script>
  let {
    checked = false,
    disabled = false,
    label = '', // aria-label; pass when there is no visible label text
    onchange = () => {},
  } = $props();
</script>

<label class="switch" class:disabled>
  <input type="checkbox" {checked} {disabled} {onchange} aria-label={label || undefined} />
  <span class="track"><span class="thumb"></span></span>
</label>

<style>
  .switch {
    display: inline-flex;
    flex-shrink: 0;
    cursor: pointer;
  }

  .switch.disabled {
    cursor: not-allowed;
    opacity: 0.5;
  }

  /* Hidden but focusable — display:none would drop it from the tab order. */
  .switch input {
    position: absolute;
    opacity: 0;
    width: 0;
    height: 0;
  }

  .track {
    display: block;
    width: 38px;
    height: 22px;
    border-radius: 11px;
    background: var(--color-text-muted);
    position: relative;
    transition: background 150ms ease;
  }

  .switch input:checked + .track {
    background: var(--color-primary);
  }

  .switch input:focus-visible + .track {
    outline: 2px solid var(--color-primary);
    outline-offset: 2px;
  }

  .thumb {
    position: absolute;
    top: 3px;
    left: 3px;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: var(--color-surface);
    transition: transform 150ms ease;
  }

  .switch input:checked + .track .thumb {
    transform: translateX(16px);
  }
</style>
