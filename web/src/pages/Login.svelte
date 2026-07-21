<script>
  import { api } from '../lib/api.js';
  import { login } from '../stores/auth.svelte.js';
  import { navigate, getQuery } from '../stores/router.svelte.js';
  import { getInstanceName } from '../stores/quilt.svelte.js';
  import {
    prepareRequestOptions,
    serializeAssertionResponse,
  } from '../lib/webauthn.js';

  let instanceName = $derived(getInstanceName());

  // Support redirect after auth and owner-specific context.
  let query = $derived(getQuery());
  let redirectTo = $derived(query.get('redirect') || '/');
  let isOwnerContext = $derived(query.get('context') === 'owner');

  let email = $state('');
  let emailSent = $state(false);
  let emailError = $state('');
  let emailLoading = $state(false);

  let passKeyError = $state('');
  let passKeyLoading = $state(false);

  let showSignIn = $state(false);
  let showRecovery = $state(false);
  let recoveryUsername = $state('');
  let recoveryCode = $state('');
  let recoveryError = $state('');
  let recoveryLoading = $state(false);

  async function handleEmailSubmit() {
    emailError = '';
    emailLoading = true;
    // Persist redirect so it survives the magic link round-trip.
    if (redirectTo && redirectTo !== '/') {
      localStorage.setItem('patchwork_auth_redirect', redirectTo);
    }
    try {
      await api('auth/magic-link', {
        method: 'POST',
        body: { email },
      });
      emailSent = true;
    } catch (e) {
      emailError = e.message || 'Failed to send link';
    } finally {
      emailLoading = false;
    }
  }

  async function handleRecoveryLogin() {
    recoveryError = '';
    recoveryLoading = true;
    try {
      await api('auth/recovery', {
        method: 'POST',
        body: { username: recoveryUsername, code: recoveryCode },
      });
      await login();
      // Land on security settings: the next moves are enrolling a
      // replacement passkey and checking how many codes are left.
      navigate('/settings/security');
    } catch (e) {
      recoveryError = e.message || 'Recovery failed';
    } finally {
      recoveryLoading = false;
    }
  }

  async function handlePasskeyLogin() {
    passKeyError = '';
    passKeyLoading = true;
    try {
      const beginData = await api('auth/webauthn/login/begin', {
        method: 'POST',
      });
      const credentialOptions = prepareRequestOptions(beginData);
      const credential = await navigator.credentials.get(credentialOptions);
      const serialized = serializeAssertionResponse(credential);
      await api('auth/webauthn/login/finish', {
        method: 'POST',
        body: serialized,
      });
      await login();
      navigate(redirectTo);
    } catch (e) {
      passKeyError = e.message || 'Passkey login failed';
    } finally {
      passKeyLoading = false;
    }
  }
</script>

<div class="page-fade">
  <div class="login-page">

    {#if emailSent}
      <!-- Email sent confirmation -->
      <div class="sent-state">
        <h1>Check your email</h1>
        <p>We sent a sign-in link to <strong>{email}</strong>.</p>
        <p class="muted">It should arrive within a minute or two. Click the link to continue.</p>
        <button class="btn-link" onclick={() => { emailSent = false; email = ''; }}>
          Use a different email
        </button>
      </div>

    {:else if !showSignIn}
      <!-- Primary: Sign up / Join -->
      {#if isOwnerContext}
        <h1>Create your account</h1>
        <p class="subtitle">Set up your account to claim and manage your patch on {instanceName}.</p>
      {:else}
        <h1>Join {instanceName}</h1>
        <p class="subtitle">Enter your email and we'll send you a sign-in link. No password needed.</p>
      {/if}

      <form onsubmit={(e) => { e.preventDefault(); handleEmailSubmit(); }}>
        <label for="email">Email address</label>
        <input
          id="email"
          type="email"
          bind:value={email}
          placeholder="you@example.com"
          required
          disabled={emailLoading}
        />
        <button type="submit" class="btn btn-primary submit-btn" disabled={emailLoading || !email.trim()}>
          {emailLoading ? 'Sending...' : 'Continue with email'}
        </button>
      </form>
      {#if emailError}
        <p class="error-text">{emailError}</p>
      {/if}

      <div class="signin-link">
        <span class="muted">Already have an account?</span>
        <button class="btn-link" onclick={() => showSignIn = true}>Sign in</button>
      </div>

    {:else}
      <!-- Secondary: Returning user sign-in -->
      <h1>Welcome back</h1>
      <p class="subtitle">Sign in to {instanceName}.</p>

      <div class="signin-options">
        <div class="signin-option">
          <h2>Passkey</h2>
          <p class="muted">The quickest way in. Your key stays on your device.</p>
          <button
            class="btn btn-primary passkey-btn"
            onclick={handlePasskeyLogin}
            disabled={passKeyLoading}
          >
            {passKeyLoading ? 'Authenticating...' : 'Sign in with passkey'}
          </button>
          {#if passKeyError}
            <p class="error-text">{passKeyError}</p>
          {/if}
        </div>

        <div class="divider">
          <span>or</span>
        </div>

        <div class="signin-option">
          <h2>Email</h2>
          <p class="muted">On a device without your passkey? We'll send you a sign-in link.</p>
          <form onsubmit={(e) => { e.preventDefault(); handleEmailSubmit(); }}>
            <input
              type="email"
              bind:value={email}
              placeholder="you@example.com"
              required
              disabled={emailLoading}
            />
            <button type="submit" class="btn btn-secondary submit-btn" disabled={emailLoading || !email.trim()}>
              {emailLoading ? 'Sending...' : 'Send sign-in link'}
            </button>
          </form>
          {#if emailError}
            <p class="error-text">{emailError}</p>
          {/if}
        </div>
      </div>

      {#if showRecovery}
        <div class="recovery-option">
          <h2>Recovery code</h2>
          <p class="muted">
            Enter your username and one of your saved recovery codes. Each
            code works once.
          </p>
          <form onsubmit={(e) => { e.preventDefault(); handleRecoveryLogin(); }}>
            <input
              type="text"
              bind:value={recoveryUsername}
              placeholder="username"
              autocapitalize="none"
              autocorrect="off"
              required
              disabled={recoveryLoading}
            />
            <input
              type="text"
              bind:value={recoveryCode}
              placeholder="xxxx-xxxx-xxxx"
              autocapitalize="none"
              autocorrect="off"
              required
              disabled={recoveryLoading}
            />
            <button
              type="submit"
              class="btn btn-secondary submit-btn"
              disabled={recoveryLoading || !recoveryUsername.trim() || !recoveryCode.trim()}
            >
              {recoveryLoading ? 'Checking...' : 'Sign in with recovery code'}
            </button>
          </form>
          {#if recoveryError}
            <p class="error-text">{recoveryError}</p>
          {/if}
        </div>
      {:else}
        <div class="recovery-link">
          <button class="btn-link muted-link" onclick={() => showRecovery = true}>
            Lost your passkey? Use a recovery code
          </button>
        </div>
      {/if}

      <div class="signin-link">
        <span class="muted">New here?</span>
        <button class="btn-link" onclick={() => showSignIn = false}>Create an account</button>
      </div>
    {/if}

  </div>
</div>

<style>
  .login-page {
    padding-top: 4rem;
    padding-bottom: 3rem;
  }

  h1 {
    font-size: 1.6rem;
    font-weight: 700;
    margin-bottom: 0.4rem;
  }

  .subtitle {
    font-size: 0.92rem;
    color: var(--color-text-muted);
    line-height: 1.5;
    margin-bottom: 2rem;
    max-width: 380px;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  label {
    font-size: 0.82rem;
    font-weight: 500;
    color: var(--color-text-muted);
  }

  .submit-btn {
    margin-top: 0.25rem;
  }

  .sent-state {
    padding: 2rem 0;
  }

  .sent-state p {
    font-size: 0.92rem;
    line-height: 1.5;
    margin-bottom: 0.5rem;
  }

  .signin-link {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin-top: 2rem;
    padding-top: 1.5rem;
    border-top: 1px solid var(--color-border);
    font-size: 0.88rem;
  }

  .btn-link {
    border: none;
    background: none;
    color: var(--color-primary);
    font-size: inherit;
    font-weight: 500;
    cursor: pointer;
    padding: 0;
  }

  .btn-link:hover {
    text-decoration: underline;
  }

  /* Sign-in options */
  .signin-options {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
  }

  .signin-option h2 {
    font-size: 1rem;
    font-weight: 600;
    margin-bottom: 0.3rem;
  }

  .signin-option .muted {
    font-size: 0.85rem;
    margin-bottom: 0.75rem;
  }

  .divider {
    display: flex;
    align-items: center;
    gap: 1rem;
    color: var(--color-text-muted);
    font-size: 0.8rem;
  }

  .divider::before,
  .divider::after {
    content: '';
    flex: 1;
    height: 1px;
    background: var(--color-border);
  }

  .passkey-btn {
    width: 100%;
    justify-content: center;
  }

  .recovery-link {
    margin-top: 1.25rem;
  }

  .muted-link {
    color: var(--color-text-muted);
    font-size: 0.82rem;
    font-weight: 400;
  }

  .recovery-option {
    margin-top: 1.5rem;
    padding-top: 1.5rem;
    border-top: 1px solid var(--color-border);
  }

  .recovery-option h2 {
    font-size: 1rem;
    font-weight: 600;
    margin-bottom: 0.3rem;
  }

  .recovery-option .muted {
    font-size: 0.85rem;
    margin-bottom: 0.75rem;
  }

  @media (max-width: 640px) {
    .submit-btn,
    .passkey-btn {
      min-height: 44px;
    }
  }
</style>
