<script>
  /**
   * Quilt settings (docs/adr/014): the instance's community identity —
   * rename, description, quilt icon — plus data export and the danger
   * zone. Deployment concerns (domain, SMTP, federation) stay in
   * patchwork.yaml and are shown read-only here.
   */
  import { api } from '../lib/api.js';
  import { withStepUp, stepUpStatus, PasskeyRequiredError } from '../lib/stepUp.js';
  import PasskeyNotice from '../components/PasskeyNotice.svelte';
  import { showToast } from '../stores/toast.svelte.js';
  import { applyIdentityChange } from '../stores/quilt.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';

  let loading = $state(true);
  let error = $state('');
  let data = $state(null);

  // Identity form
  let name = $state('');
  let description = $state('');
  // Quilt policy (docs/adr/037): hide amended-lining patches from discovery
  // for everyone. Personal settings can hide more, never reveal what this hides.
  let hideAmendedLinings = $state(false);
  let savingPolicy = $state(false);

  async function savePolicy(value) {
    savingPolicy = true;
    try {
      await api('admin/settings', { method: 'PATCH', body: { hide_amended_linings: value } });
      hideAmendedLinings = value;
      showToast('Policy saved', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
    }
    savingPolicy = false;
  }
  let savingIdentity = $state(false);

  // Icon
  let iconBust = $state(0);
  let uploading = $state(false);
  let iconError = $state('');
  let fileInput = $state(null);

  // Danger zone
  let confirmName = $state('');
  let wiping = $state(false);
  let wipeArmed = $state(false);

  // Export and wipe both need a fresh passkey confirmation (docs/adr/017).
  // Checked on load so someone without a passkey is told here, rather than
  // finding out at the moment they are trying to get their data out.
  let hasPasskey = $state(true);
  let exporting = $state(false);

  let iconSrc = $derived(`/api/v1/instance/icon${iconBust ? `?t=${iconBust}` : ''}`);

  $effect(() => {
    load();
    stepUpStatus().then((s) => { hasPasskey = s.has_passkey !== false; });
  });

  async function load() {
    loading = true;
    error = '';
    try {
      data = await api('admin/settings');
      name = data.name;
      description = data.description;
      hideAmendedLinings = !!data.hide_amended_linings;
    } catch (e) {
      error = e.message;
    }
    loading = false;
  }

  async function saveIdentity() {
    savingIdentity = true;
    try {
      const res = await api('admin/settings', {
        method: 'PATCH',
        body: { name: name.trim(), description },
      });
      data = { ...data, name: res.name, name_overridden: true, description_overridden: true };
      applyIdentityChange({ name: res.name });
      showToast('Quilt identity saved', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
    }
    savingIdentity = false;
  }

  function checkImageDimensions(file) {
    return new Promise((resolve, reject) => {
      const url = URL.createObjectURL(file);
      const img = new Image();
      img.onload = () => {
        URL.revokeObjectURL(url);
        resolve({ width: img.naturalWidth, height: img.naturalHeight });
      };
      img.onerror = () => {
        URL.revokeObjectURL(url);
        reject(new Error('Could not read the image.'));
      };
      img.src = url;
    });
  }

  async function handleFile(e) {
    const file = e.target.files?.[0];
    if (!file) return;
    iconError = '';

    const c = data.icon_constraints;
    if (!c.formats.includes(file.type)) {
      iconError = 'Icon must be a PNG or JPEG image.';
      return;
    }
    if (file.size > c.max_bytes) {
      iconError = `Icon must be ${Math.round(c.max_bytes / 1024)} KB or smaller.`;
      return;
    }
    try {
      const { width, height } = await checkImageDimensions(file);
      if (width !== height) {
        iconError = `Icon must be square (this one is ${width}×${height}).`;
        return;
      }
      if (width < c.min_px || width > c.max_px) {
        iconError = `Icon must be between ${c.min_px} and ${c.max_px} pixels.`;
        return;
      }
    } catch (err) {
      iconError = err.message;
      return;
    }

    uploading = true;
    try {
      const res = await fetch('/api/v1/admin/settings/icon', {
        method: 'PUT',
        headers: { 'Content-Type': file.type, 'X-Patchwork-Request': 'true' },
        body: file,
        credentials: 'same-origin',
      });
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        throw new Error(body?.error || `Upload failed (${res.status})`);
      }
      data = { ...data, icon: { kind: 'upload', mime: file.type } };
      iconBust = Date.now();
      applyIdentityChange();
      showToast('Quilt icon updated', 'success');
    } catch (err) {
      iconError = err.message;
    }
    uploading = false;
    if (fileInput) fileInput.value = '';
  }

  async function removeUpload() {
    try {
      await api('admin/settings/icon', { method: 'DELETE' });
      await load();
      iconBust = Date.now();
      applyIdentityChange();
      showToast('Uploaded icon removed', 'info');
    } catch (e) {
      showToast(e.message || 'Failed to remove icon', 'error');
    }
  }

  async function chooseDefault(key) {
    try {
      const res = await api('admin/settings', {
        method: 'PATCH',
        body: { icon_default: key },
      });
      data = { ...data, icon: res.icon };
      iconBust = Date.now();
      applyIdentityChange();
      showToast('Default icon chosen', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to set icon', 'error');
    }
  }

  /**
   * The export is gated, so it can't be a plain download link — a navigation
   * would just hit the 403. Fetch it, confirming with a passkey if asked, and
   * hand the browser the blob.
   */
  async function downloadExport() {
    exporting = true;
    try {
      const blob = await withStepUp(async () => {
        const res = await fetch('/api/v1/admin/export', { credentials: 'same-origin' });
        if (!res.ok) {
          const body = await res.json().catch(() => null);
          const err = new Error(body?.error || `Export failed (${res.status})`);
          err.status = res.status;
          err.data = body;
          throw err;
        }
        return res.blob();
      });

      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'patchwork-export.zip';
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      if (e instanceof PasskeyRequiredError) hasPasskey = false;
      showToast(e.message || 'Export failed', 'error');
    }
    exporting = false;
  }

  async function wipeQuilt() {
    wiping = true;
    try {
      await withStepUp(() => api('admin/wipe', {
        method: 'POST',
        body: { confirm_name: confirmName },
      }));
      // Everything is gone, including this session. Hard reload to first-run.
      localStorage.clear();
      window.location.href = '/';
    } catch (e) {
      if (e instanceof PasskeyRequiredError) hasPasskey = false;
      showToast(e.message || 'Wipe failed', 'error');
      wiping = false;
      wipeArmed = false;
    }
  }
</script>

<div class="page-fade">
  <div class="page-header">
    <h1>Quilt Settings</h1>
    <p class="muted">How this quilt presents itself, here and on other quilts.</p>
  </div>

  {#if loading}
    <Skeleton lines={6} />
  {:else if error}
    <ErrorState message={error} retry={load} />
  {:else if data}
    <!-- ===== Identity ===== -->
    <section class="section">
      <h2>Identity</h2>
      <div class="card settings-card">
        <label class="field">
          <span class="field-label">Quilt name</span>
          <input type="text" bind:value={name} maxlength="100" />
          {#if data.name_overridden}
            <span class="field-hint">Set here in the admin panel. Overrides the name in patchwork.yaml.</span>
          {/if}
        </label>
        <label class="field">
          <span class="field-label">Description</span>
          <textarea bind:value={description} rows="3" maxlength="500"></textarea>
        </label>
        <div class="field">
          <span class="field-label">Domain</span>
          <span class="field-static">{data.domain || '(not set)'}</span>
          <span class="field-hint">The domain is deployment configuration (patchwork.yaml) and cannot be changed here. Federation identities minted with it are permanent.</span>
        </div>
        <div class="field-actions">
          <button class="btn btn-primary" onclick={saveIdentity} disabled={savingIdentity || !name.trim()}>
            {savingIdentity ? 'Saving…' : 'Save Identity'}
          </button>
        </div>
      </div>
    </section>

    <!-- ===== Quilt icon ===== -->
    <section class="section">
      <h2>Quilt Icon</h2>
      <div class="card settings-card">
        <p class="section-desc">
          Represents this quilt in the quilt switcher and in other people's Connected Quilts.
          Square PNG or JPEG, {data.icon_constraints.min_px}&ndash;{data.icon_constraints.max_px} px,
          up to {Math.round(data.icon_constraints.max_bytes / 1024)} KB. 256&times;256 is plenty. It's shown small.
        </p>

        <div class="icon-current">
          <img class="icon-preview" src={iconSrc} alt="Current quilt icon" width="64" height="64" />
          <div class="icon-meta">
            {#if data.icon.kind === 'upload'}
              <span class="icon-kind">Uploaded image</span>
              <button class="btn btn-secondary btn-sm" onclick={removeUpload}>Remove upload</button>
            {:else if data.icon.chosen}
              <span class="icon-kind">Default block: {data.icon.default_key}</span>
            {:else}
              <span class="icon-kind">Assigned block: {data.icon.default_key} (pick one below or upload your own)</span>
            {/if}
          </div>
        </div>

        <div class="icon-upload">
          <label class="btn btn-secondary upload-btn" class:disabled={uploading}>
            {uploading ? 'Uploading…' : 'Upload icon…'}
            <input
              bind:this={fileInput}
              type="file"
              accept="image/png,image/jpeg"
              onchange={handleFile}
              disabled={uploading}
              hidden
            />
          </label>
          {#if iconError}
            <span class="error-text">{iconError}</span>
          {/if}
        </div>

        <div class="default-picker">
          <span class="field-label">Or choose a default block</span>
          <div class="default-grid">
            {#each data.default_icons as key (key)}
              <button
                class="default-option"
                class:active={data.icon.kind === 'default' && data.icon.default_key === key}
                onclick={() => chooseDefault(key)}
                title={key}
              >
                <img src={`/api/v1/instance/icon?block=${key}`} alt={key} width="48" height="48" />
                <span class="default-name">{key}</span>
              </button>
            {/each}
          </div>
        </div>
      </div>
    </section>

    <!-- ===== Data export ===== -->
    <section class="section">
      <h2>The Lining</h2>
      <p class="section-desc">
        Every patch starts with the lining and can amend its copy by proposal.
        Amended patches always wear a public badge; this policy also keeps them
        out of the quilt, search, the map, and public feeds for everyone.
        Direct links still work.
      </p>
      <label class="policy-toggle">
        <input
          type="checkbox"
          checked={hideAmendedLinings}
          disabled={savingPolicy}
          onchange={(e) => savePolicy(e.target.checked)}
        />
        <span>Hide patches that amended the lining</span>
      </label>
    </section>

    <section class="section">
      <h2>Data Export (Seamrip)</h2>
      <div class="card settings-card">
        <p class="section-desc">
          Download this quilt's portable community data as a zip: patches, people,
          memberships, events, proposals with votes, and governance records.
          Credentials, sessions, and federation keys deliberately stay behind.
          For a full backup of the deployment itself, back up the server's data
          directory. That's an ops practice, not this export.
        </p>
        <PasskeyNotice show={!hasPasskey} action="export this quilt's data" />
        <button class="btn btn-primary" onclick={downloadExport} disabled={exporting}>
          {exporting ? 'Preparing…' : 'Download Export'}
        </button>
      </div>
    </section>

    <!-- ===== Danger zone ===== -->
    <section class="section">
      <h2 class="danger-heading">Danger Zone</h2>
      <div class="danger-card">
        <h3>Wipe this quilt</h3>
        <PasskeyNotice show={!hasPasskey} action="wipe this quilt" />
        <p class="danger-warning">
          Erases <strong>all community data</strong>: every patch, person, event,
          proposal, and governance record. The deployment returns to first-run.
          The server, domain, and configuration survive. The community's data
          does not. This cannot be undone. Download an export first.
        </p>
        <label class="field">
          <span class="field-label">Type the quilt name to confirm: <strong>{data.name}</strong></span>
          <input
            type="text"
            bind:value={confirmName}
            placeholder={data.name}
            autocomplete="off"
            spellcheck="false"
          />
        </label>
        {#if !wipeArmed}
          <button
            class="btn btn-danger"
            disabled={confirmName !== data.name}
            onclick={() => { wipeArmed = true; }}
          >
            Wipe Quilt Data…
          </button>
        {:else}
          <div class="wipe-confirm">
            <span class="danger-warning">Really erase everything? You will be signed out.</span>
            <button class="btn btn-danger" onclick={wipeQuilt} disabled={wiping}>
              {wiping ? 'Wiping…' : 'Yes, erase all community data'}
            </button>
            <button class="btn btn-secondary" onclick={() => { wipeArmed = false; }} disabled={wiping}>
              Cancel
            </button>
          </div>
        {/if}
      </div>
    </section>
  {/if}
</div>

<style>
  .page-header {
    padding: 1.5rem 0 1rem;
  }

  .page-header .muted {
    margin-top: 0.25rem;
    color: var(--color-text-muted);
    font-size: 0.9rem;
  }

  .section {
    margin-bottom: 2rem;
    max-width: 640px;
  }

  .section h2 {
    font-size: 1.1rem;
    margin-bottom: 0.75rem;
  }

  .settings-card {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .policy-toggle {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.9rem;
    cursor: pointer;
  }

  .section-desc {
    font-size: 0.88rem;
    color: var(--color-text-muted);
    line-height: 1.5;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .field-label {
    font-size: 0.82rem;
    font-weight: 600;
  }

  .field-hint {
    font-size: 0.78rem;
    color: var(--color-text-muted);
    line-height: 1.4;
  }

  .field-static {
    font-family: monospace;
    font-size: 0.88rem;
  }

  .field-actions {
    display: flex;
    justify-content: flex-end;
  }

  .icon-current {
    display: flex;
    align-items: center;
    gap: 1rem;
  }

  .icon-preview {
    width: 64px;
    height: 64px;
    border: 1px solid var(--color-border);
    background: var(--color-bg);
    object-fit: cover;
    flex-shrink: 0;
  }

  .icon-meta {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.4rem;
  }

  .icon-kind {
    font-size: 0.85rem;
    color: var(--color-text-muted);
  }

  .icon-upload {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .upload-btn {
    cursor: pointer;
  }

  .upload-btn.disabled {
    opacity: 0.6;
    pointer-events: none;
  }

  .error-text {
    color: var(--color-error);
    font-size: 0.82rem;
  }

  .default-picker {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .default-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(84px, 1fr));
    gap: 0.5rem;
  }

  .default-option {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.3rem;
    padding: 0.6rem 0.25rem;
    background: var(--color-bg);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    cursor: pointer;
    transition: border-color 120ms ease;
  }

  .default-option:hover {
    border-color: var(--color-primary);
  }

  .default-option.active {
    border-color: var(--color-primary);
    box-shadow: 0 0 0 1px var(--color-primary);
  }

  .default-option img {
    width: 48px;
    height: 48px;
  }

  .default-name {
    font-size: 0.7rem;
    color: var(--color-text-muted);
    word-break: break-word;
    text-align: center;
  }

  .danger-heading {
    color: var(--color-error);
  }

  .danger-card {
    border: 1px solid var(--color-error);
    border-radius: 6px;
    padding: 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .danger-card h3 {
    font-size: 0.95rem;
  }

  .danger-warning {
    font-size: 0.88rem;
    color: var(--color-text-muted);
    line-height: 1.5;
  }

  .wipe-confirm {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
  }
</style>
