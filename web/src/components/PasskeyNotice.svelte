<script>
  /**
   * Tells an admin that an action needs a passkey they don't have yet
   * (docs/adr/017).
   *
   * Shown up front, next to the action, not raised as an error once they
   * click. Discovering the requirement at the moment you are trying to
   * export your data is the failure mode this exists to avoid.
   */
  let { show = false, action = 'this action' } = $props();
</script>

{#if show}
  <p class="passkey-notice" role="status">
    <strong>Passkey needed.</strong>
    To {action} you'll be asked to confirm with a passkey, and you don't have
    one enrolled yet. <a href="/settings/security">Add a passkey</a> first.
    It takes a moment and uses the sign-in you already have.
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
