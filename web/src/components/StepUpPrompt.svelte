<script>
  /**
   * The one dialog that asks someone to confirm an irreversible action
   * (docs/adr/017, docs/adr/099).
   *
   * Mounted once by App.svelte and registered as the module-level prompt
   * provider in lib/stepUp.js, so every surface that wraps an action in
   * withStepUp gets it without knowing it exists. It resolves with the
   * recovery code that opened the window, 'passkey' when a passkey opened
   * it, or null when the person walked away.
   */
  import Modal from './Modal.svelte';
  import {
    setStepUpPrompt,
    stepUp,
    stepUpStatus,
    stepUpWithRecoveryCode,
  } from '../lib/stepUp.js';
  import { showToast } from '../stores/toast.svelte.js';

  let open = $state(false);
  let hasPasskey = $state(false);
  let recoveryReady = $state(0);
  let passkeyNote = $state('');
  let code = $state('');
  let error = $state('');
  let guidance = $state('');
  let busy = $state(false);
  let lowCodesLeft = $state(null); // non-null once a code was spent and few remain
  let fieldEl = $state(null);

  let resolver = null;

  function finish(value) {
    const resolve = resolver;
    resolver = null;
    open = false;
    if (resolve) resolve(value);
  }

  // Dismissal is a real answer: the caller surfaces the error it already had.
  function dismiss() {
    if (busy) return;
    if (lowCodesLeft !== null) {
      finish(lowCodesLeft.usedCode);
      return;
    }
    finish(null);
  }

  function ask(context = {}) {
    // A second caller while one dialog is up: the first is answered "no"
    // rather than left with a promise nothing will ever settle.
    if (resolver) finish(null);

    hasPasskey = context.hasPasskey !== false;
    passkeyNote = context.reason || '';
    recoveryReady = 0;
    code = '';
    error = '';
    guidance = '';
    lowCodesLeft = null;
    busy = false;
    open = true;

    // What this session could actually confirm with, so the dialog can say
    // so rather than let someone hunt for a code that will be refused.
    stepUpStatus().then((s) => {
      recoveryReady = Number(s?.recovery_ready) || 0;
      if (context.hasPasskey === undefined) hasPasskey = s?.has_passkey !== false;
    });

    return new Promise((resolve) => {
      resolver = resolve;
    });
  }

  $effect(() => {
    setStepUpPrompt(ask);
    return () => setStepUpPrompt(null);
  });

  // The recovery-code field is what the dialog is for; land the cursor
  // there. Deferred a frame because Modal moves focus to its own close
  // button when it opens, and whichever of the two effects runs second wins.
  $effect(() => {
    if (!open || !fieldEl || lowCodesLeft !== null) return;
    const el = fieldEl;
    const frame = requestAnimationFrame(() => el.focus());
    return () => cancelAnimationFrame(frame);
  });

  async function tryPasskey() {
    busy = true;
    error = '';
    guidance = '';
    try {
      await stepUp();
      finish('passkey');
    } catch (e) {
      error = e?.message || 'That passkey check did not go through.';
      guidance = 'You can enter a recovery code instead.';
    } finally {
      busy = false;
    }
  }

  async function submitCode(e) {
    e?.preventDefault?.();
    const entered = code.trim();
    if (!entered || busy) return;
    busy = true;
    error = '';
    guidance = '';
    try {
      const res = await stepUpWithRecoveryCode(entered);
      const remaining = Number(res?.codes_remaining) || 0;
      if (remaining <= 2) {
        // Worth stopping for: at zero there is no way to confirm anything
        // until a new set is made, and making one costs a sign-out before
        // it counts.
        lowCodesLeft = { remaining, usedCode: entered };
        busy = false;
        return;
      }
      showToast(`Confirmed. ${remaining} recovery codes left.`, 'success');
      finish(entered);
    } catch (err) {
      switch (err?.code) {
        case 'no_recovery_codes':
          error = 'This account has no recovery codes.';
          guidance = 'Make a set at Settings → Security. You will need to sign in again with one of them before a code can confirm anything.';
          break;
        case 'recovery_codes_too_new':
          error = 'Every one of your recovery codes was made during this sign-in, so none of them can confirm anything yet.';
          guidance = 'Sign out and sign back in using one of your codes. That signs you in and confirms you at once, and the rest of the set works from then on.';
          break;
        case 'rate_limited':
          error = 'Too many attempts. Wait a couple of minutes and try again.';
          guidance = '';
          break;
        default:
          error = 'That recovery code is not right, or it has already been used.';
          guidance = 'Each code works once. Try another from your set.';
          break;
      }
      busy = false;
    }
  }
</script>

<Modal {open} onClose={dismiss} label="Confirm this action">
  {#if lowCodesLeft !== null}
    <h3 class="su-title">Confirmed</h3>
    <p class="su-low">
      {lowCodesLeft.remaining === 0
        ? 'That was your last recovery code. Until you make a new set you have no way to confirm anything that asks for it.'
        : `${lowCodesLeft.remaining} recovery ${lowCodesLeft.remaining === 1 ? 'code' : 'codes'} left. When they run out you have no way to confirm anything that asks for one.`}
    </p>
    <p class="su-help">
      Make a new set at Settings → Security, then sign out and sign back in
      using one of them — a code only confirms things once it is older than
      your sign-in.
    </p>
    <div class="su-actions">
      <button class="btn btn-primary" onclick={() => finish(lowCodesLeft.usedCode)}>
        Continue
      </button>
    </div>
  {:else}
    <h3 class="su-title">Confirm it's you</h3>
    <p class="su-help">
      This action can't be undone, so Patchwork asks for a fresh proof it's
      you — not just the session you're already signed in with.
    </p>

    {#if passkeyNote}
      <p class="su-note">{passkeyNote}</p>
    {/if}

    {#if hasPasskey}
      <div class="su-actions su-passkey">
        <button class="btn btn-primary" onclick={tryPasskey} disabled={busy}>
          {busy ? 'Waiting...' : 'Use a passkey'}
        </button>
      </div>
      <p class="su-or">or</p>
    {/if}

    <form onsubmit={submitCode}>
      <label class="su-label" for="stepup-code">Recovery code</label>
      <input
        id="stepup-code"
        class="su-input"
        type="text"
        bind:value={code}
        bind:this={fieldEl}
        autocomplete="one-time-code"
        spellcheck="false"
        placeholder="xxxx-xxxx-xxxx"
        disabled={busy}
      />
      {#if recoveryReady > 0}
        <p class="su-help su-ready">
          {recoveryReady} of your recovery codes can confirm this. Each one
          works once.
        </p>
      {:else}
        <p class="su-help su-ready">
          A recovery code has to be older than this sign-in to confirm
          anything. If you just made a set, sign out and sign back in with one
          of the codes; if you have none, make a set at Settings → Security.
        </p>
      {/if}

      {#if error}
        <p class="su-error" role="alert">{error}</p>
      {/if}
      {#if guidance}
        <p class="su-help">{guidance}</p>
      {/if}

      <div class="su-actions">
        <button class="btn btn-primary" type="submit" disabled={busy || !code.trim()}>
          {busy ? 'Checking...' : 'Confirm'}
        </button>
        <button class="btn btn-secondary" type="button" onclick={dismiss} disabled={busy}>
          Cancel
        </button>
      </div>
    </form>
  {/if}
</Modal>

<style>
  .su-title {
    margin: 0 0 0.5rem;
    font-size: 1.05rem;
  }

  .su-help {
    margin: 0 0 0.75rem;
    font-size: 0.85rem;
    line-height: 1.5;
    color: var(--color-text-muted);
  }

  .su-ready {
    margin-top: 0.4rem;
  }

  .su-note {
    margin: 0 0 0.75rem;
    font-size: 0.85rem;
    line-height: 1.5;
  }

  .su-or {
    margin: 0 0 0.75rem;
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .su-label {
    display: block;
    font-size: 0.8rem;
    font-weight: 600;
    margin-bottom: 0.3rem;
  }

  /* Border, background and focus ring come from app.css; only the width
     and the code face are this field's own. */
  .su-input {
    width: 100%;
    font-family: var(--font-mono, monospace);
  }

  .su-error {
    margin: 0.6rem 0 0.4rem;
    font-size: 0.85rem;
    line-height: 1.5;
    color: var(--color-error);
  }

  .su-low {
    margin: 0 0 0.75rem;
    font-size: 0.9rem;
    line-height: 1.5;
  }

  .su-actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 1rem;
  }

  .su-passkey {
    margin-top: 0.5rem;
  }
</style>
