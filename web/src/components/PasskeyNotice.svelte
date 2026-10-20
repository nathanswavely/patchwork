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
  import { stepUpStatus } from '../lib/stepUp.js';

  let { show = false, action = 'this action' } = $props();

  // The caller knows only that there is no passkey. Two other things mean
  // the person can confirm anyway (F-077): a recovery code this sign-in can
  // spend, or a confirmation window already open. The component asks for
  // itself, so no call site has to, and stays quiet until the answer is in
  // rather than flashing a warning it then takes back.
  let status = $state(null);
  $effect(() => {
    if (!show || status) return;
    let live = true;
    stepUpStatus().then((s) => { if (live) status = s || {}; });
    return () => { live = false; };
  });

  let canConfirmAnyway = $derived(
    Number(status?.recovery_ready) > 0 || status?.active === true
  );
</script>

{#if show && status && !canConfirmAnyway}
  <p class="lt-banner lt-tone-warning passkey-notice" role="status">
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
  }

  .passkey-notice a {
    text-decoration: underline;
  }
</style>
