<script>
  import { api } from '../lib/api.js';
  import { login } from '../stores/auth.svelte.js';
  import { getParams, navigate } from '../stores/router.svelte.js';
  import {
    prepareCreationOptions,
    serializeCreationResponse,
  } from '../lib/webauthn.js';

  let token = $derived(getParams().token || '');
  let validating = $state(true);
  let valid = $state(false);
  let validationError = $state('');

  let username = $state('');
  let displayName = $state('');
  let submitError = $state('');
  let submitting = $state(false);

  let registered = $state(false);
  let enrollingPasskey = $state(false);
  let passkeyError = $state('');
  let passkeyDone = $state(false);

  // Chosen, not derived: same rules the server enforces (docs/adr/013).
  const USERNAME_PATTERN = '[a-z0-9][a-z0-9\\-]{1,28}[a-z0-9]';
  let usernameHint = $derived.by(() => {
    const u = username.trim().toLowerCase();
    if (!u) return '';
    if (u.length < 3) return 'At least 3 characters.';
    if (!new RegExp(`^${USERNAME_PATTERN}$`).test(u)) {
      return 'Lowercase letters, numbers, and hyphens only, starting and ending with a letter or number.';
    }
    return '';
  });

  $effect(() => {
    if (token) {
      validateToken();
    }
  });

  async function validateToken() {
    validating = true;
    try {
      await api(`auth/invite/${token}/validate`);
      valid = true;
    } catch (e) {
      validationError = e.message || 'Invalid or expired invite link';
    } finally {
      validating = false;
    }
  }

  async function handleSubmit() {
    submitError = '';
    submitting = true;
    try {
      await api('auth/invite', {
        method: 'POST',
        body: {
          token,
          username: username.trim().toLowerCase(),
          display_name: displayName.trim() || undefined,
        },
      });
      await login();
      registered = true;
    } catch (e) {
      submitError = e.message || 'Registration failed';
    } finally {
      submitting = false;
    }
  }

  async function enrollPasskey() {
    passkeyError = '';
    enrollingPasskey = true;
    try {
      const beginData = await api('auth/webauthn/register/begin', {
        method: 'POST',
      });

      const credentialOptions = prepareCreationOptions(beginData);
      const credential = await navigator.credentials.create(credentialOptions);
      const serialized = serializeCreationResponse(credential);

      await api('auth/webauthn/register/finish', {
        method: 'POST',
        body: serialized,
      });

      passkeyDone = true;
    } catch (e) {
      passkeyError = e.message || 'Passkey enrollment failed';
    } finally {
      enrollingPasskey = false;
    }
  }

  function goToDashboard() {
    navigate('/welcome');
  }
</script>

<div class="page-fade">
  <div class="invite-page">
    {#if validating}
      <div class="card">
        <p class="muted">Checking your invite...</p>
      </div>
    {:else if !valid}
      <div class="card">
        <h1>This invite didn't work</h1>
        <p class="error-text">{validationError}</p>
        <p class="muted" style="margin-top: 1rem;">
          This invite link may have expired or already been used.
          Ask your community admin for a new one.
        </p>
      </div>
    {:else if registered}
      <div class="card">
        <h1>You're in</h1>
        {#if passkeyDone}
          <p class="success-text">Passkey saved.</p>
          <p class="muted" style="margin-top: 0.5rem;">
            This device can sign you in from now on.
          </p>
          <button class="btn btn-primary" style="margin-top: 1.5rem;" onclick={goToDashboard}>
            Take a look around
          </button>
        {:else}
          <p>Create a passkey and this device signs you in with one tap.</p>
          <p class="muted" style="margin-top: 0.5rem;">
            The key stays on your device, and we never hold a password for
            you. Nothing to leak, and email can break without locking you out.
          </p>
          <button
            class="btn btn-primary"
            style="margin-top: 1rem;"
            onclick={enrollPasskey}
            disabled={enrollingPasskey}
          >
            {enrollingPasskey ? 'Waiting for your browser...' : 'Create a passkey'}
          </button>
          {#if passkeyError}
            <p class="error-text">{passkeyError}</p>
          {/if}
          <button class="btn btn-secondary" style="margin-top: 0.5rem;" onclick={goToDashboard}>
            Skip for now
          </button>
        {/if}
      </div>
    {:else}
      <div class="card">
        <h1>You're invited</h1>
        <p class="muted" style="margin-bottom: 1.5rem;">
          Someone here sent you this link. Pick a username and you're in.
        </p>

        <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
          <label for="username">Username <span class="required">*</span></label>
          <input
            id="username"
            type="text"
            bind:value={username}
            placeholder="Choose a username"
            pattern={USERNAME_PATTERN}
            minlength="3"
            maxlength="30"
            autocapitalize="off"
            autocorrect="off"
            spellcheck="false"
            required
            disabled={submitting}
          />
          {#if usernameHint}
            <p class="hint">{usernameHint}</p>
          {:else}
            <p class="hint muted">3-30 characters: lowercase letters, numbers, hyphens. This becomes your profile address and can't be changed later.</p>
          {/if}

          <label for="display-name">Display Name</label>
          <input
            id="display-name"
            type="text"
            bind:value={displayName}
            placeholder="How you'd like to be known (optional)"
            disabled={submitting}
          />

          <button
            type="submit"
            class="btn btn-primary"
            disabled={submitting || !username.trim()}
          >
            {submitting ? 'Creating Account...' : 'Create Account'}
          </button>

          <p class="agree-line muted">
            By creating an account you agree to the
            <a href="/terms" target="_blank" rel="noopener">User Agreement</a>
            and <a href="/privacy" target="_blank" rel="noopener">Privacy Policy</a>.
          </p>
        </form>

        {#if submitError}
          <p class="error-text">{submitError}</p>
        {/if}
      </div>
    {/if}
  </div>
</div>

<style>
  .invite-page {
    padding-top: 3rem;
    padding-bottom: 3rem;
  }

  .invite-page .card {
    max-width: 480px;
    margin: 0 auto;
  }

  .invite-page h1 {
    margin-bottom: 0.5rem;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  label {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--color-text-muted);
  }

  .required {
    color: var(--color-error);
  }

  .hint {
    font-size: 0.78rem;
    color: var(--color-text-muted);
    margin-top: -0.4rem;
  }

  .agree-line {
    font-size: 0.78rem;
    margin: 0;
    text-align: center;
  }
  .agree-line a {
    color: inherit;
    text-decoration: underline;
    text-underline-offset: 2px;
  }
</style>
