<script>
  let {
    checked = false,
    disabled = false,
    label = '', // aria-label; pass when there is no visible label text
    onchange = () => {},
  } = $props();
</script>

<!-- Liquid Textile's switch: the checkbox is the switch, so a screen reader
     announces it as one (role="switch"), and its touch target reaches
     --lt-target (44px on a coarse pointer) without moving the row. -->
<input
  type="checkbox"
  role="switch"
  class="lt-switch"
  {checked}
  {disabled}
  {onchange}
  aria-label={label || undefined}
/>
