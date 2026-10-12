<script>
  /**
   * A two-step button for an act that cannot be taken back: Liquid
   * Textile's confirmAction() (lib/lt.js). The first click swaps the button
   * for a question, a confirm and a cancel, with focus on the confirm; it
   * swaps back after autoRevertMs, on Cancel, or on Escape. On a phone the
   * question is read aloud but not shown, so the row stays on one line.
   * The words are passed in here so the copy ledger owns them.
   */
  import { ltConfirm } from '../lib/lt.js';

  let {
    label = 'Action',
    confirmLabel = 'Confirm',
    variant = 'default', // 'danger' | 'warning' | 'default'
    onConfirm = async () => {},
    autoRevertMs = 5000,
    disabled = false,
  } = $props();

  // The confirm wears the act's colour; a plain act confirms in primary.
  let tone = $derived(variant === 'default' ? 'primary' : variant);
</script>

<button
  class="lt-btn lt-btn-sm"
  class:lt-btn-danger={variant === 'danger'}
  class:lt-btn-warning={variant === 'warning'}
  class:lt-btn-secondary={variant === 'default'}
  {disabled}
  use:ltConfirm={{
    onConfirm,
    prompt: 'Are you sure?',
    confirmLabel,
    cancelLabel: 'Cancel',
    tone,
    timeout: autoRevertMs,
  }}
>
  {label}
</button>
