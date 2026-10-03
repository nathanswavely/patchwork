<script>
  // The page a sign-in email's link opens
  // (docs/adr/2026-09-28-a-link-knows-where-it-was-asked-for.md). Loading it
  // spends nothing: mail scanners fetch links before a person does. The page
  // posts the token once, and the server answers by whether this browser is
  // the one that asked. If it is, this signs in. If not, it shows the code to
  // enter where sign-in was asked for, with a button to sign in here instead.
  import { api } from '../lib/api.js';
  import { login } from '../stores/auth.svelte.js';
  import { getParams, navigate, isSafeRedirectPath } from '../stores/router.svelte.js';

  let token = $derived(getParams().token || '');

  let phase = $state('loading'); // loading | code | error
  let email = $state('');
  let code = $state('');
  let signingInHere = $state(false);
  let hereError = $state('');

  let started = false;
  $effect(() => {
    if (started || !token) return;
    started = true;
    open(false);
  });

  async function open(here) {
    let res;
    try {
      res = await api('auth/magic-link/open', { method: 'POST', body: { token, here } });
    } catch (e) {
      if (here) {
        hereError = 'This link has expired or has already been used.';
        return;
      }
      phase = 'error';
      return;
    }

    if (res?.status === 'code') {
      email = res.email || '';
      code = res.code || '';
      phase = 'code';
      return;
    }

    // Login.svelte keeps the destination across the email round-trip.
    let pending = null;
    try {
      pending = localStorage.getItem('patchwork_auth_redirect');
      localStorage.removeItem('patchwork_auth_redirect');
    } catch { /* storage may be unavailable */ }
    const dest = isSafeRedirectPath(pending) ? pending : '/dashboard';

    if (res?.status === 'username_required') {
      // A first sign-in: the username is chosen, never derived (docs/adr/013).
      navigate(
        '/signup/complete?token=' + encodeURIComponent(res.signup_token) +
          (dest !== '/dashboard' ? '&redirect=' + encodeURIComponent(dest) : '')
      );
      return;
    }

    await login();
    navigate(dest);
  }

  async function signInHere() {
    hereError = '';
    signingInHere = true;
    try {
      await open(true);
    } finally {
      signingInHere = false;
    }
  }

  let spacedCode = $derived(code.length === 6 ? code.slice(0, 3) + ' ' + code.slice(3) : code);
</script>

<div class="page-fade">
  <div class="link-page">
    {#if phase === 'loading'}
      <p class="muted">Signing in...</p>

    {:else if phase === 'code'}
      {#if code}
        <h1>Your sign-in code</h1>
        <p>Enter this code where you asked to sign in{#if email}{' as '}<strong>{email}</strong>{/if}.</p>
        <p class="code" aria-label="Sign-in code {code.split('').join(' ')}">{spacedCode}</p>
        <p class="muted">It works once and expires 15 minutes after you asked. Do not share it with anyone.</p>
      {:else}
        <h1>Sign in</h1>
        <p>This link was sent before sign-in codes were shown here.</p>
      {/if}

      <div class="here">
        <button class="btn btn-secondary" onclick={signInHere} disabled={signingInHere}>
          {signingInHere ? 'Signing in...' : 'Sign in on this device'}
        </button>
        {#if code}
          <p class="muted small">The code stops working if you sign in here.</p>
        {/if}
        {#if hereError}
          <p class="error-text">{hereError}</p>
        {/if}
      </div>

    {:else}
      <h1>This link no longer works</h1>
      <p>This sign-in link has expired or has already been used.</p>
      <a
        href="/login?mode=signin"
        class="btn btn-secondary"
        onclick={(e) => { e.preventDefault(); navigate('/login?mode=signin'); }}
      >Get a new link</a>
    {/if}
  </div>
</div>

<style>
  .link-page {
    padding-top: 4rem;
    padding-bottom: 3rem;
  }

  h1 {
    font-size: 1.6rem;
    font-weight: 700;
    margin-bottom: 0.4rem;
  }

  p {
    font-size: 0.92rem;
    line-height: 1.5;
    margin-bottom: 0.5rem;
  }

  .code {
    font-size: 2.4rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    font-variant-numeric: tabular-nums;
    margin: 1.25rem 0;
    user-select: all;
  }

  .here {
    margin-top: 2rem;
    padding-top: 1.5rem;
    border-top: 1px solid var(--color-border);
  }

  .here .small {
    font-size: 0.82rem;
    margin-top: 0.5rem;
  }
</style>
