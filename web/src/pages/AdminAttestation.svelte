<script>
  /**
   * Proving the admin role to an outside party (docs/adr/087).
   *
   * A hosting provider wants to re-point a billing contact; a directory
   * wants to check who submitted a listing. They hand over a nonce, an
   * admin signs it here with the quilt's own key, and they check the blob
   * against the key served at this domain.
   *
   * The page has one job beyond the mechanics: be plain about what the
   * result does and does not say. It proves the role, not the person —
   * naming who the admins are is what ADR 023 refused, and a page that let
   * anyone read this as identity would smuggle that back in.
   */
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { withStepUp, stepUpStatus, PasskeyRequiredError } from '../lib/stepUp.js';
  import PasskeyNotice from '../components/PasskeyNotice.svelte';

  let nonce = $state('');
  let signing = $state(false);
  let error = $state('');
  let result = $state(null);

  // Checked on load so an admin without a passkey learns it here, rather
  // than at the moment somebody is waiting on them for a blob.
  let hasPasskey = $state(true);

  $effect(() => {
    stepUpStatus().then((s) => { hasPasskey = s.has_passkey !== false; });
  });

  let expiresAt = $derived(
    result?.statement?.expires_at
      ? new Date(result.statement.expires_at).toLocaleString()
      : ''
  );

  async function sign() {
    if (!nonce.trim() || signing) return;
    signing = true;
    error = '';
    try {
      result = await withStepUp(() =>
        api('admin/attestation', { method: 'POST', body: { nonce: nonce.trim() } })
      );
    } catch (e) {
      if (e instanceof PasskeyRequiredError) hasPasskey = false;
      result = null;
      error = e.data?.error || e.message || 'Signing failed.';
    } finally {
      signing = false;
    }
  }

  async function copyBlob() {
    try {
      await navigator.clipboard.writeText(result.attestation);
      showToast('Copied');
    } catch {
      showToast('Copy failed. Select the text instead.', 'error');
    }
  }
</script>

<div class="admin-page">
  <h1>Prove You Administer This Quilt</h1>
  <p class="page-desc">
    Sometimes an outside party — a hosting provider re-pointing a billing
    contact, a directory checking a submission — needs to know that whoever
    is writing to them really administers this quilt. Ask them for a nonce:
    any string they choose, which is how they know the answer was made for
    them and not copied from somewhere else. Sign it here, send them the
    result, and they can check it against this quilt's public key without an
    account and without asking us anything.
  </p>
  <p class="page-desc">
    <strong>What it proves:</strong> an admin of this quilt signed that exact
    string at that exact time, and the signature stands for fifteen minutes.
    <strong>What it does not prove:</strong> who you are. The signed statement
    names this domain and nothing else — no username, no email, no account.
    Anyone with admin here could have produced it, and that is deliberate:
    this quilt does not publish who its admins are.
  </p>

  <PasskeyNotice show={!hasPasskey} action="sign an attestation" />

  <form class="sign-form" onsubmit={(e) => { e.preventDefault(); sign(); }}>
    <label for="nonce">Their nonce</label>
    <div class="field-row">
      <input
        id="nonce"
        type="text"
        bind:value={nonce}
        placeholder="the string they gave you"
        maxlength="64"
        autocomplete="off"
        disabled={signing}
      />
      <button type="submit" class="btn btn-primary" disabled={signing || !nonce.trim()}>
        {signing ? 'Signing…' : 'Sign'}
      </button>
    </div>
    <p class="hint">
      8 to 64 printable characters, on one line. Paste it exactly as they sent
      it — a changed character is a different nonce, and their check will fail.
    </p>
  </form>

  {#if error}
    <p class="error-text">{error}</p>
  {/if}

  {#if result}
    <section class="result card">
      <div class="result-head">
        <h2>Send them this</h2>
        <button class="btn btn-secondary btn-sm" onclick={copyBlob}>Copy</button>
      </div>
      <code class="blob">{result.attestation}</code>
      <dl class="facts">
        <div><dt>Good until</dt><dd>{expiresAt}</dd></div>
        <div><dt>Signature</dt><dd>{result.algorithm}</dd></div>
        <div><dt>They check it against</dt><dd class="url">{result.key_url}</dd></div>
      </dl>
      <p class="hint">
        Tell them to fetch the key from this quilt's own address, not from a
        link in the message — a signature only means something when the key
        came from the domain they care about.
      </p>
    </section>
  {/if}
</div>

<style>
  .admin-page {
    max-width: var(--pw-measure);
  }

  h1 {
    margin-bottom: 0.25rem;
  }

  .page-desc {
    color: var(--color-text-muted);
    margin-bottom: 1rem;
    line-height: 1.55;
  }

  .sign-form {
    margin-top: 1.5rem;
  }

  .sign-form label {
    display: block;
    font-weight: 600;
    font-size: 0.88rem;
    margin-bottom: 0.35rem;
  }

  .field-row {
    display: flex;
    gap: 0.5rem;
  }

  .field-row input {
    flex: 1;
    font-family: monospace;
  }

  .hint {
    margin: 0.5rem 0 0;
    font-size: 0.82rem;
    color: var(--color-text-muted);
    line-height: 1.5;
  }

  .result {
    margin-top: 1.5rem;
  }

  .result-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    margin-bottom: 0.75rem;
  }

  .result-head h2 {
    margin: 0;
    font-size: 1rem;
  }

  .blob {
    display: block;
    padding: 0.75rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm, 4px);
    background: var(--color-bg);
    font-family: monospace;
    font-size: 0.78rem;
    line-height: 1.5;
    word-break: break-all;
    user-select: all;
  }

  .facts {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 1.5rem;
    margin: 0.9rem 0 0;
  }

  .facts div {
    display: flex;
    gap: 0.4rem;
    font-size: 0.82rem;
  }

  .facts dt {
    color: var(--color-text-muted);
  }

  .facts dd {
    margin: 0;
    font-weight: 600;
  }

  .facts dd.url {
    font-family: monospace;
    font-weight: 400;
    word-break: break-all;
  }

  .error-text {
    margin-top: 0.75rem;
    color: var(--color-error);
    font-size: 0.85rem;
  }
</style>
