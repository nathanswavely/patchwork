<script>
  import { api } from '../lib/api.js';
  import { getUser } from '../stores/auth.svelte.js';
  import {
    prepareCreationOptions,
    serializeCreationResponse,
  } from '../lib/webauthn.js';
  import Skeleton from '../components/Skeleton.svelte';
  import { showToast } from '../stores/toast.svelte.js';
  import ConfirmAction from '../components/ConfirmAction.svelte';
  import Modal from '../components/Modal.svelte';

  const MAX_NAME = 64;

  let user = $derived(getUser());
  let credentials = $state([]);
  let loadingCreds = $state(false);
  let addingPasskey = $state(false);
  let passkeyError = $state('');
  let renamingId = $state(null);
  let renameValue = $state('');

  // Naming happens *after* the credential exists, not before. Two reasons:
  // a field sitting above the button described an object that did not exist
  // yet and read as an editor for the list above it, and collecting the name
  // mid-ceremony would put a person's typing between the challenge and the
  // finish call — the challenge has a TTL, and a slow typist would lose the
  // whole enrollment. So the passkey is saved with a guessed name and the
  // modal just renames it. Dismissing the modal costs nothing.
  let namingId = $state(null);
  let namingValue = $state('');
  // The name the credential is already stored under. Held separately from the
  // editable value so the dismiss button can name what dismissing actually
  // keeps, rather than echoing whatever half-typed text is in the box.
  let namingDefault = $state('');
  let savingName = $state(false);

  let recoveryStatus = $state(null); // { total, remaining }
  let newCodes = $state([]);
  let generatingCodes = $state(false);
  let codesCopied = $state(false);

  $effect(() => {
    if (user) {
      loadCredentials();
      loadRecoveryStatus();
    }
  });

  async function loadRecoveryStatus() {
    try {
      recoveryStatus = await api('auth/recovery-codes');
    } catch {
      recoveryStatus = null;
    }
  }

  async function generateRecoveryCodes() {
    generatingCodes = true;
    codesCopied = false;
    try {
      const data = await api('auth/recovery-codes', { method: 'POST' });
      newCodes = data.codes || [];
      await loadRecoveryStatus();
    } catch (e) {
      showToast(e.message || 'Failed to generate codes', 'error');
    } finally {
      generatingCodes = false;
    }
  }

  async function copyCodes() {
    try {
      await navigator.clipboard.writeText(newCodes.join('\n'));
      codesCopied = true;
      showToast('Codes copied', 'success');
    } catch {
      showToast('Copy failed — select and copy them by hand', 'error');
    }
  }

  // A guess at the device in hand, so the field is never a blank stare. The
  // person can overwrite it; the point is that "Passkey", "Passkey",
  // "Passkey" is nobody's idea of a useful list.
  function suggestName() {
    const ua = navigator.userAgent;
    let device = 'This device';
    if (/iPhone/.test(ua)) device = 'iPhone';
    else if (/iPad/.test(ua)) device = 'iPad';
    else if (/Android/.test(ua)) device = 'Android phone';
    else if (/Mac OS X/.test(ua)) device = 'Mac';
    else if (/Windows/.test(ua)) device = 'Windows PC';
    else if (/Linux/.test(ua)) device = 'Linux PC';

    const taken = new Set(credentials.map((c) => c.name));
    if (!taken.has(device)) return device;
    for (let i = 2; i < 20; i++) {
      if (!taken.has(`${device} ${i}`)) return `${device} ${i}`;
    }
    return device;
  }

  async function loadCredentials() {
    loadingCreds = true;
    try {
      const data = await api('auth/credentials');
      credentials = Array.isArray(data) ? data : data.credentials || [];
    } catch {
      credentials = [];
    } finally {
      loadingCreds = false;
    }
  }

  async function addPasskey() {
    passkeyError = '';
    addingPasskey = true;
    try {
      const beginData = await api('auth/webauthn/register/begin', {
        method: 'POST',
      });
      const credentialOptions = prepareCreationOptions(beginData);
      const credential = await navigator.credentials.create(credentialOptions);
      const serialized = serializeCreationResponse(credential);

      // Enroll with the guess. The name is settled in the modal below, which
      // is a plain rename against a credential that already exists — so the
      // ceremony is never waiting on anyone to finish typing.
      const created = await api('auth/webauthn/register/finish', {
        method: 'POST',
        body: { ...serialized, name: suggestName() },
      });

      await loadCredentials();
      showToast('Passkey added', 'success');

      if (created?.id) {
        namingId = created.id;
        namingValue = created.name || '';
        namingDefault = created.name || 'Passkey';
      }
    } catch (e) {
      passkeyError = e.message || 'Failed to add passkey';
      showToast('Something went wrong. Please try again.', 'error');
    } finally {
      addingPasskey = false;
    }
  }

  function closeNaming() {
    namingId = null;
    namingValue = '';
    namingDefault = '';
    savingName = false;
  }

  // Saves the name the person typed. The passkey is already enrolled and
  // usable either way, so a failure here is a rename that did not take — not
  // a lost credential. Say that plainly rather than implying the worse thing.
  async function saveNewName() {
    const name = namingValue.trim();
    if (!name) {
      closeNaming();
      return;
    }
    savingName = true;
    try {
      const updated = await api(`auth/credentials/${namingId}`, {
        method: 'PATCH',
        body: { name },
      });
      credentials = credentials.map((c) =>
        c.id === namingId ? { ...c, name: updated.name } : c,
      );
      closeNaming();
    } catch (e) {
      savingName = false;
      showToast(e.message || 'Could not rename — your passkey still works.', 'error');
    }
  }

  // Select-all on focus so the guess is one keystroke from being replaced,
  // but still there to keep if it is right.
  function selectAll(node) {
    node.focus();
    node.select();
  }

  function startRename(cred) {
    renamingId = cred.id;
    renameValue = cred.name || '';
  }

  function cancelRename() {
    renamingId = null;
    renameValue = '';
  }

  async function saveRename(id) {
    const name = renameValue.trim();
    try {
      const updated = await api(`auth/credentials/${id}`, {
        method: 'PATCH',
        body: { name },
      });
      credentials = credentials.map((c) =>
        c.id === id ? { ...c, name: updated.name } : c,
      );
      cancelRename();
      showToast('Passkey renamed', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to rename', 'error');
    }
  }

  async function revokeCredential(id) {
    try {
      await api(`auth/credentials/${id}`, { method: 'DELETE' });
      credentials = credentials.filter((c) => c.id !== id);
      showToast('Passkey revoked', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to revoke', 'error');
    }
  }
</script>

<div class="page-fade">
  <section class="card">
    <h2>Passkeys</h2>
    <p class="muted" style="margin-bottom: 1rem;">
      Passkeys sign you in without a password or email.
    </p>

    {#if loadingCreds}
      <Skeleton lines={2} height="0.9rem" />
    {:else if credentials.length === 0}
      <p class="muted">No passkeys enrolled yet.</p>
    {:else}
      <ul class="cred-list">
        {#each credentials as cred (cred.id)}
          <li>
            {#if renamingId === cred.id}
              <form
                class="rename-form"
                onsubmit={(e) => {
                  e.preventDefault();
                  saveRename(cred.id);
                }}
              >
                <!-- svelte-ignore a11y_autofocus -->
                <input
                  type="text"
                  bind:value={renameValue}
                  maxlength={MAX_NAME}
                  aria-label="Passkey name"
                  autofocus
                />
                <button class="btn btn-secondary" type="submit">Save</button>
                <button class="btn btn-ghost" type="button" onclick={cancelRename}>
                  Cancel
                </button>
              </form>
            {:else}
              <div class="cred-info">
                <span class="cred-name">{cred.name || 'Passkey'}</span>
                {#if cred.created_at}
                  <small class="muted">
                    Added {new Date(cred.created_at).toLocaleDateString()}
                  </small>
                {/if}
              </div>
              <div class="cred-actions">
                <button class="btn btn-ghost" onclick={() => startRename(cred)}>
                  Rename
                </button>
                <ConfirmAction
                  label="Revoke"
                  confirmLabel="Yes, revoke passkey"
                  variant="danger"
                  onConfirm={() => revokeCredential(cred.id)}
                />
              </div>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}

    <div class="add-passkey">
      <button class="btn btn-secondary" onclick={addPasskey} disabled={addingPasskey}>
        {addingPasskey ? 'Enrolling...' : 'Add Passkey'}
      </button>
      {#if passkeyError}
        <p class="error-text">{passkeyError}</p>
      {/if}
    </div>

    {#if credentials.length === 1}
      <p class="muted second-passkey-nudge">
        One passkey is one lost phone away from a locked account. Add a
        second on another device.
      </p>
    {/if}
  </section>

  <section class="card">
    <h2>Recovery codes</h2>
    <p class="muted" style="margin-bottom: 1rem;">
      If you lose your passkey, a recovery code signs you in. Each code works
      once.
    </p>

    {#if newCodes.length > 0}
      <div class="codes-reveal">
        <p class="codes-warning">
          Write these down or save them somewhere safe. This is the only time
          they're shown.
        </p>
        <ul class="codes-list">
          {#each newCodes as code (code)}
            <li><code>{code}</code></li>
          {/each}
        </ul>
        <div class="codes-actions">
          <button class="btn btn-secondary" onclick={copyCodes}>
            {codesCopied ? 'Copied' : 'Copy all'}
          </button>
          <button class="btn-link" onclick={() => { newCodes = []; }}>
            Done — I've saved them
          </button>
        </div>
      </div>
    {:else}
      {#if recoveryStatus && recoveryStatus.total > 0}
        <p class="muted">
          {recoveryStatus.remaining} of {recoveryStatus.total} codes left.
        </p>
        {#if recoveryStatus.remaining <= 3}
          <p class="codes-warning">Running low — generate a fresh set.</p>
        {/if}
        <div style="margin-top: 1rem;">
          <ConfirmAction
            label={generatingCodes ? 'Generating...' : 'Generate new codes'}
            confirmLabel="Yes, replace my codes"
            onConfirm={generateRecoveryCodes}
          />
          <p class="muted" style="margin-top: 0.5rem; font-size: 0.8rem;">
            New codes replace the old set — anything written down stops
            working.
          </p>
        </div>
      {:else}
        <p class="muted">No recovery codes yet.</p>
        <div style="margin-top: 1rem;">
          <button class="btn btn-secondary" onclick={generateRecoveryCodes} disabled={generatingCodes}>
            {generatingCodes ? 'Generating...' : 'Generate recovery codes'}
          </button>
        </div>
      {/if}
    {/if}
  </section>
</div>

<Modal open={namingId !== null} label="Name this passkey" onClose={closeNaming}>
  <h3 class="naming-title">Name this passkey</h3>
  <p class="muted naming-help">
    It's saved and ready to use. A name helps you tell it apart from your
    others later.
  </p>
  <form
    class="naming-form"
    onsubmit={(e) => {
      e.preventDefault();
      saveNewName();
    }}
  >
    <input
      type="text"
      bind:value={namingValue}
      maxlength={MAX_NAME}
      aria-label="Passkey name"
      placeholder="Passkey"
      use:selectAll
    />
    <div class="naming-actions">
      <button class="btn btn-primary" type="submit" disabled={savingName}>
        {savingName ? 'Saving...' : 'Save name'}
      </button>
      <button class="btn btn-ghost" type="button" onclick={closeNaming}>
        Keep "{namingDefault}"
      </button>
    </div>
  </form>
</Modal>

<style>
  h2 {
    font-size: 1.1rem;
    margin-bottom: 1rem;
  }

  .cred-list {
    list-style: none;
    padding: 0;
  }

  .cred-list li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.75rem 0;
    border-bottom: 1px solid var(--color-border);
  }

  .cred-list li:last-child {
    border-bottom: none;
  }

  .cred-info {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .cred-name {
    font-weight: 500;
  }

  .cred-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .rename-form {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    flex-wrap: wrap;
  }

  .rename-form input {
    flex: 1 1 12rem;
  }

  .second-passkey-nudge {
    margin-top: 0.75rem;
    font-size: 0.82rem;
  }

  .codes-warning {
    color: var(--color-warning, #b45309);
    font-size: 0.85rem;
    font-weight: 500;
    margin-bottom: 0.75rem;
  }

  .codes-list {
    list-style: none;
    padding: 0.75rem 1rem;
    margin: 0 0 1rem;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(9rem, 1fr));
    gap: 0.4rem 1.5rem;
    background: var(--color-bg-subtle, rgba(0, 0, 0, 0.04));
    border: 1px solid var(--color-border);
    border-radius: 6px;
  }

  .codes-list code {
    font-size: 0.9rem;
    letter-spacing: 0.03em;
  }

  .codes-actions {
    display: flex;
    align-items: center;
    gap: 1rem;
  }

  .btn-link {
    border: none;
    background: none;
    color: var(--color-primary);
    font-size: 0.88rem;
    font-weight: 500;
    cursor: pointer;
    padding: 0;
  }

  .btn-link:hover {
    text-decoration: underline;
  }

  .add-passkey {
    margin-top: 1rem;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.4rem;
  }

  .naming-title {
    font-size: 1.05rem;
    margin-bottom: 0.35rem;
  }

  .naming-help {
    margin-bottom: 1rem;
  }

  .naming-form {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .naming-form input {
    width: 100%;
  }

  .naming-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
  }
</style>
