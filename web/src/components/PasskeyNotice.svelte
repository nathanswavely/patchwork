<script>
  /**
   * Tells an admin that an action needs a confirmation they can't yet give
   * (docs/adr/017, docs/adr/099).
   *
   * Shown up front, next to the action, not raised as an error once they
   * click. Discovering the requirement at the moment you are trying to
   * export your data is the failure mode this exists to avoid.
   *
   * It names both proofs. "Add a passkey first" was a dead end for the one
   * person who most needs to read this — the device that cannot make one.
   */
  let { show = false, action = 'this action' } = $props();
</script>

{#if show}
  <p class="passkey-notice" role="status">
    <strong>Confirmation needed.</strong>
    To {action} you'll be asked to confirm, and you don't have a passkey
    enrolled yet. <a href="/settings/security">Add a passkey</a> first — it
    takes a moment and uses the sign-in you already have. If this device
    can't make one, a recovery code confirms too, once you've signed in again
    with one.
  </p>
{/if}

<style>
  .passkey-notice {
    margin: 0 0 1rem;
    padding: 0.75rem 1rem;
    border: 1px solid var(--color-warning, #b8860b);
    border-radius: var(--radius-sm, 4px);
    background: var(--color-warning-bg, rgba(184, 134, 11, 0.08));
    font-size: 0.9rem;
    line-height: 1.5;
  }

  .passkey-notice a {
    text-decoration: underline;
  }
</style>
