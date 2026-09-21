<script>
  /**
   * The apps this quilt vouches for
   * (docs/adr/2026-09-20-an-instance-vouches-for-an-app.md).
   *
   * Listing an app publishes its identifier at the address Apple and Google
   * fetch, and from then on that app may ask a phone for this quilt's
   * passkeys. That is a grant outward, so the add runs through the same
   * step-up flow the export and the attestation run through (docs/adr/017).
   * Removing is a plain admin act: taking trust back is the safe direction.
   */
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { withStepUp, stepUpStatus, PasskeyRequiredError } from '../lib/stepUp.js';
  import PasskeyNotice from '../components/PasskeyNotice.svelte';

  let apps = $state([]);
  let urls = $state({ apple: '', android: '' });
  let loading = $state(true);
  let hasPasskey = $state(true);

  let platform = $state('apple');
  let identifier = $state('');
  let fingerprints = $state('');
  let label = $state('');
  let busy = $state(false);
  let error = $state('');

  const PLACEHOLDERS = {
    apple: 'ABCDE12345.org.example.app',
    android: 'org.example.app',
  };

  async function load() {
    loading = true;
    try {
      const data = await api('admin/native-apps');
      apps = data.native_apps || [];
      urls = data.urls || { apple: '', android: '' };
    } catch {
      apps = [];
    } finally {
      loading = false;
    }
  }

  async function handleAdd() {
    if (!identifier.trim() || busy) return;
    busy = true;
    error = '';
    try {
      const body = {
        platform,
        identifier: identifier.trim(),
        label: label.trim(),
      };
      if (platform === 'android') {
        body.fingerprints = fingerprints
          .split('\n')
          .map((line) => line.trim())
          .filter(Boolean);
      }
      await withStepUp(() => api('admin/native-apps', { method: 'POST', body }));
      showToast('App listed');
      identifier = '';
      fingerprints = '';
      label = '';
      await load();
    } catch (e) {
      if (e instanceof PasskeyRequiredError) hasPasskey = false;
      error = e.data?.error || e.message || 'Could not list that app.';
    } finally {
      busy = false;
    }
  }

  async function handleRemove(app) {
    try {
      await api(`admin/native-apps/${app.id}`, { method: 'DELETE' });
      showToast('App removed');
      apps = apps.filter((a) => a.id !== app.id);
    } catch (e) {
      showToast(e.data?.error || 'Could not remove that app', 'error');
    }
  }

  function platformLabel(value) {
    return value === 'android' ? 'Android' : 'Apple';
  }

  function addedOn(stamp) {
    if (!stamp) return '';
    const d = new Date(stamp);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString();
  }

  $effect(() => { load(); });
  $effect(() => { stepUpStatus().then((s) => { hasPasskey = s.has_passkey !== false; }); });
</script>

<div class="admin-page">
  <h1>Apps this quilt vouches for</h1>
  <p class="page-desc">
    A native app can hold this quilt's passkeys only if approved by this quilt. Listing an app publishes its identifier at the address the app stores check.
  </p>

  <div class="urls">
    <p class="urls-title">What gets published</p>
    <code>{urls.apple || '/.well-known/apple-app-site-association'}</code>
    <code>{urls.android || '/.well-known/assetlinks.json'}</code>
    <p class="muted">
      Each address answers "not found" until you list an app for that
      platform. Apple caches the file for up to a day, so a change can take
      that long to reach a phone.
    </p>
  </div>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if apps.length === 0}
    <div class="empty-state">
      <p>No apps listed.</p>
      <p class="muted">This quilt vouches for nobody, and publishes neither file.</p>
    </div>
  {:else}
    <div class="app-list">
      {#each apps as app (app.id)}
        <div class="app-item card">
          <div class="app-info">
            <span class="app-platform">{platformLabel(app.platform)}</span>
            {#if app.label}<span class="app-label">{app.label}</span>{/if}
            <code class="app-id">{app.identifier}</code>
            {#if app.fingerprints?.length}
              <ul class="prints">
                {#each app.fingerprints as print (print)}
                  <li><code>{print}</code></li>
                {/each}
              </ul>
            {/if}
            {#if addedOn(app.created_at)}
              <span class="muted added">Listed {addedOn(app.created_at)}</span>
            {/if}
          </div>
          <button class="btn btn-danger btn-sm" onclick={() => handleRemove(app)}>Remove</button>
        </div>
      {/each}
    </div>
  {/if}

  <h2>List an app</h2>
  <PasskeyNotice show={!hasPasskey} action="list an app" />
  <form class="add-form" onsubmit={(e) => { e.preventDefault(); handleAdd(); }}>
    <label>
      Platform
      <select bind:value={platform} disabled={busy}>
        <option value="apple">Apple</option>
        <option value="android">Android</option>
      </select>
    </label>

    <label>
      Identifier
      <input
        type="text"
        bind:value={identifier}
        placeholder={PLACEHOLDERS[platform]}
        disabled={busy}
      />
    </label>

    {#if platform === 'android'}
      <label>
        Signing-certificate fingerprints
        <textarea
          bind:value={fingerprints}
          rows="4"
          placeholder="AA:BB:CC:… — one per line"
          disabled={busy}
        ></textarea>
      </label>
    {/if}

    <label>
      Label
      <input
        type="text"
        bind:value={label}
        maxlength="64"
        placeholder="What you call this app"
        disabled={busy}
      />
    </label>

    <button type="submit" class="btn btn-primary" disabled={busy || !identifier.trim()}>
      {busy ? 'Listing…' : 'Add'}
    </button>
  </form>
  {#if error}
    <p class="error-text">{error}</p>
  {/if}
</div>

<style>
  .admin-page {
    max-width: var(--pw-measure);
  }

  h1 {
    margin-bottom: 0.25rem;
  }

  h2 {
    margin-top: 2rem;
    font-size: 1.05rem;
  }

  .page-desc {
    color: var(--color-text-muted);
    margin-bottom: 1.25rem;
    line-height: 1.5;
  }

  .urls {
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm, 4px);
    padding: 0.9rem 1rem;
    margin-bottom: 1.5rem;
  }

  .urls-title {
    margin: 0 0 0.5rem;
    font-weight: 700;
    font-size: 0.85rem;
  }

  .urls code {
    display: block;
    font-size: 0.82rem;
    word-break: break-all;
    margin-bottom: 0.35rem;
  }

  .urls .muted {
    margin: 0.6rem 0 0;
    font-size: 0.85rem;
    line-height: 1.5;
  }

  .app-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .app-item {
    display: flex;
    align-items: flex-start;
    gap: 0.9rem;
  }

  .app-info {
    flex: 1;
    min-width: 0;
  }

  .app-platform {
    display: inline-block;
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
  }

  .app-label {
    display: block;
    font-weight: 700;
    font-size: 0.9rem;
  }

  .app-id {
    display: block;
    font-size: 0.82rem;
    word-break: break-all;
  }

  .prints {
    list-style: none;
    padding: 0;
    margin: 0.4rem 0 0;
  }

  .prints code {
    font-size: 0.72rem;
    color: var(--color-text-muted);
    word-break: break-all;
  }

  .added {
    display: block;
    font-size: 0.78rem;
    margin-top: 0.35rem;
  }

  .add-form {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    margin-top: 0.5rem;
    align-items: flex-start;
  }

  .add-form label {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    font-size: 0.85rem;
    font-weight: 600;
    width: 100%;
  }

  .add-form input,
  .add-form select,
  .add-form textarea {
    font-weight: 400;
    width: 100%;
  }

  .add-form textarea {
    font-family: monospace;
    font-size: 0.8rem;
  }

  .empty-state {
    text-align: center;
    padding: 2rem 1rem;
    color: var(--color-text-muted);
  }

  .error-text {
    color: var(--color-error);
    font-size: 0.85rem;
  }
</style>
