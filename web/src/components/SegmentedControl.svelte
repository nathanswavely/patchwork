<script>
  /**
   * A few mutually exclusive choices side by side: Liquid Textile's
   * segmented control (ltSegmented in lib/lt.js). It is a radio group with
   * one tab stop; the arrows move and choose, and aria-checked marks the
   * choice. The page owns the value.
   */
  import { ltSegmented } from '../lib/lt.js';

  let {
    options = [], // [{ value, label, title? }]
    value,
    label = '', // aria-label for the group
    onchange = () => {},
  } = $props();

  // A segment carries its value as text (data-value); hand the page back
  // its own value, which may be a number.
  function pick(v) {
    const opt = options.find((o) => String(o.value) === v);
    if (opt) onchange(opt.value);
  }
</script>

<div
  class="lt-segmented"
  aria-label={label || undefined}
  use:ltSegmented={{ value: String(value), onChange: pick }}
>
  {#each options as opt (opt.value)}
    <button type="button" class="lt-segment" data-value={String(opt.value)} title={opt.title}>{opt.label}</button>
  {/each}
</div>
