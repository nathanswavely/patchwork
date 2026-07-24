<script>
  // Connected quilts are account-backed (docs/adr/024) — they follow the
  // person to any device, alongside their remote follows.
  import { navigate } from '../stores/router.svelte.js';
  import {
    getConnectedQuilts,
    connectQuilt,
    disconnectQuilt,
    normalizeOrigin,
  } from '../stores/multiQuilt.svelte.js';

  // Paste-a-link follow path (docs/adr/024): browsing other quilts
  // happens on their own site, so following starts from a copied link.
  let followUrl = $state('');
  let followError = $state('');

  function handleFollowLink() {
    followError = '';
    const m = followUrl.trim().match(/^https?:\/\/([^/]+)\/patches\/([a-z0-9-]+)\/?$/);
    if (!m) {
      followError = 'That doesn’t look like a patch link — expected https://their-quilt/patches/patch-name.';
      return;
    }
    const [, host, slug] = m;
    if (host === window.location.host) {
      navigate(`/patches/${slug}`);
    } else {
      navigate(`/quilts/${host}/patches/${slug}`);
    }
  }

  let newUrl = $state('');
  let error = $state('');
  let validating = $state(false);
  let validatedName = $state('');
  let testResults = $state({});

  async function validateInstance(url) {
    const res = await fetch(`${normalizeOrigin(url)}/api/v1/instance`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();
    if (!data.name) throw new Error('Invalid instance response');
    return data;
  }

  async function handleAdd() {
    if (!newUrl.trim()) return;
    error = '';
    validatedName = '';
    validating = true;

    try {
      const instance = await validateInstance(newUrl.trim());
      await connectQuilt(newUrl.trim(), instance.name || '');
      validatedName = instance.name;
      if (instance.multi_quilt === false) {
        validatedName += ' — note: this quilt doesn’t allow browsing from other quilts, so it will open on their site';
      }
      newUrl = '';
    } catch (e) {
      error = `Could not reach instance: ${e.message}`;
    } finally {
      validating = false;
    }
  }

  async function handleRemove(quilt) {
    error = '';
    try {
      await disconnectQuilt(quilt.id);
    } catch (e) {
      error = e.data?.error || 'Failed to remove quilt.';
    }
  }

  async function testConnection(url) {
    testResults = { ...testResults, [url]: { loading: true } };
    try {
      const instance = await validateInstance(url);
      testResults = { ...testResults, [url]: {
        loading: false,
        ok: true,
        name: instance.name,
        stats: instance.stats,
      }};
    } catch (e) {
      testResults = { ...testResults, [url]: {
        loading: false,
        ok: false,
        error: e.message,
      }};
    }
  }
</script>

<div class="page-fade">
  <div class="container-narrow">
    <h1>Connected Quilts</h1>
    <p class="page-desc">Connect other Patchworks to keep them a click away in the quilt switcher — each opens on its own site. Your connections are part of your account, so they follow you to any device.</p>

    <div class="follow-link card">
      <h2>Follow a patch from another quilt</h2>
      <p class="muted follow-link-desc">Found a patch while visiting another quilt? Paste its link (or drop it in the search bar) and it opens here, with a Follow button. Followed patches join your My Quilt.</p>
      <form class="add-form" onsubmit={(e) => { e.preventDefault(); handleFollowLink(); }}>
        <input
          type="url"
          bind:value={followUrl}
          placeholder="https://their-quilt.example.com/patches/gallery-row"
        />
        <button type="submit" class="btn btn-secondary" disabled={!followUrl.trim()}>Open patch</button>
      </form>
      {#if followError}
        <p class="error-text">{followError}</p>
      {/if}
    </div>

    <form class="add-form" onsubmit={(e) => { e.preventDefault(); handleAdd(); }}>
      <input
        type="url"
        bind:value={newUrl}
        placeholder="https://other-patchwork.example.com"
        disabled={validating}
      />
      <button type="submit" class="btn btn-primary" disabled={validating || !newUrl.trim()}>
        {validating ? 'Checking...' : 'Add Quilt'}
      </button>
    </form>

    {#if error}
      <p class="error-text">{error}</p>
    {/if}
    {#if validatedName}
      <p class="success-text">Added "{validatedName}"</p>
    {/if}

    <div class="quilt-list">
      {#each getConnectedQuilts() as quilt (quilt.id)}
        <div class="quilt-item card">
          <!-- Quilt icon (docs/adr/014): plain img, no CORS needed; older
               instances without the endpoint just hide the slot. -->
          <img
            class="quilt-icon"
            src={`${quilt.url}/api/v1/instance/icon`}
            alt=""
            width="32"
            height="32"
            loading="lazy"
            onerror={(e) => { e.target.style.display = 'none'; }}
          />
          <div class="quilt-info">
            {#if quilt.name}
              <span class="quilt-name">{quilt.name}</span>
            {/if}
            <span class="quilt-url">{quilt.url}</span>
            {#if testResults[quilt.url]}
              {#if testResults[quilt.url].loading}
                <span class="muted" style="font-size: 0.8rem;">Testing...</span>
              {:else if testResults[quilt.url].ok}
                <span class="status-ok">
                  {testResults[quilt.url].name}
                  {#if testResults[quilt.url].stats}
                    &middot; {testResults[quilt.url].stats.node_count || 0} patches
                  {/if}
                </span>
              {:else}
                <span class="status-err">Unreachable: {testResults[quilt.url].error}</span>
              {/if}
            {/if}
          </div>
          <div class="quilt-actions">
            <button class="btn btn-secondary btn-sm" onclick={() => testConnection(quilt.url)}>
              Test
            </button>
            <button class="btn btn-danger btn-sm" onclick={() => handleRemove(quilt)}>
              Remove
            </button>
          </div>
        </div>
      {:else}
        <div class="empty-state">
          <p>No quilts followed yet.</p>
          <p class="muted" style="font-size: 0.85rem;">Add a Patchwork instance URL above to start exploring.</p>
        </div>
      {/each}
    </div>
  </div>
</div>

<style>
  h1 {
    margin-bottom: 0.25rem;
  }

  .page-desc {
    color: var(--color-text-muted);
    margin-bottom: 1.5rem;
    line-height: 1.5;
  }

  .add-form {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 1rem;
  }

  .follow-link {
    margin-bottom: 1.5rem;
  }

  .follow-link h2 {
    font-size: 1rem;
    margin: 0 0 0.3rem;
  }

  .follow-link-desc {
    font-size: 0.85rem;
    margin-bottom: 0.75rem;
    line-height: 1.45;
  }

  .follow-link .add-form {
    margin-bottom: 0;
  }

  .error-text {
    color: var(--color-error);
    font-size: 0.85rem;
  }

  .add-form input {
    flex: 1;
  }

  .quilt-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    margin-top: 1.5rem;
  }

  .quilt-item {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
  }

  .quilt-icon {
    width: 32px;
    height: 32px;
    flex-shrink: 0;
    object-fit: cover;
    border: 1px solid var(--color-border);
    background: var(--color-bg);
  }

  .quilt-info {
    flex: 1;
    min-width: 0;
  }

  .quilt-url {
    display: block;
    font-family: monospace;
    font-size: 0.85rem;
    color: var(--color-text);
    word-break: break-all;
  }

  .quilt-name {
    display: block;
    font-weight: 700;
    font-size: 0.9rem;
  }

  .quilt-actions {
    display: flex;
    gap: 0.35rem;
    flex-shrink: 0;
  }

  .status-ok {
    display: block;
    font-size: 0.8rem;
    color: var(--color-success);
    margin-top: 0.25rem;
  }

  .status-err {
    display: block;
    font-size: 0.8rem;
    color: var(--color-error);
    margin-top: 0.25rem;
  }

  .empty-state {
    text-align: center;
    padding: 2rem 1rem;
    color: var(--color-text-muted);
  }

  .empty-state p:first-child {
    font-size: 1rem;
    margin-bottom: 0.25rem;
  }

  @media (max-width: 640px) {
    .add-form {
      flex-direction: column;
    }

    .quilt-item {
      flex-direction: column;
      gap: 0.5rem;
    }

    .quilt-actions {
      align-self: flex-end;
    }
  }
</style>
