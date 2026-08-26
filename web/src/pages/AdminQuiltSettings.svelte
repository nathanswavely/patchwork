<script>
  /**
   * Quilt settings (docs/adr/014): the instance's community identity —
   * rename, description, quilt icon — plus data export and the danger
   * zone. Deployment concerns (domain, SMTP, federation) stay in
   * patchwork.yaml and are shown read-only here.
   */
  import * as d3 from 'd3';
  import { api } from '../lib/api.js';
  import { withStepUp, stepUpStatus, PasskeyRequiredError } from '../lib/stepUp.js';
  import PasskeyNotice from '../components/PasskeyNotice.svelte';
  import { showToast } from '../stores/toast.svelte.js';
  import { applyIdentityChange } from '../stores/quilt.svelte.js';
  import { renderDraftBlock } from '../lib/quiltBlocks.js';
  import BlockDrafter from '../components/BlockDrafter.svelte';
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

  // Where this quilt keeps time (docs/adr/045): the rung an event's zone
  // falls through to when neither the event nor its patch names one. Set
  // here as well as in patchwork.yaml because a wrong zone shows up as
  // every event being hours off, and that fix should not need a redeploy.
  let timezone = $state('');
  let timezoneConfigured = $state('');
  let savingTimezone = $state(false);

  function zoneIsKnown(tz) {
    if (!tz) return true; // empty clears the override
    try {
      new Intl.DateTimeFormat('en-US', { timeZone: tz });
      return true;
    } catch {
      return false;
    }
  }

  async function saveTimezone() {
    const tz = timezone.trim();
    if (!zoneIsKnown(tz)) {
      showToast('That is not a timezone this quilt knows', 'error');
      return;
    }
    savingTimezone = true;
    try {
      const saved = await api('admin/settings', { method: 'PATCH', body: { timezone: tz } });
      timezone = saved?.timezone || tz || timezoneConfigured;
      showToast(tz ? 'Timezone saved' : 'Timezone reset to the configured default', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
    }
    savingTimezone = false;
  }

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
  let resetting = $state('');

  // Icon (docs/adr/043): drafted in the block drafter, like a patch tile.
  let iconDraft = $state({ grid: 3, seams: [], colors: {} });
  let iconBundle = $state(['#039BE6', '#F2EEE4']);
  let iconSeed = $state('');
  let savingIcon = $state(false);

  let iconDirty = $derived(JSON.stringify(currentIconDesign()) !== iconSeed);

  function currentIconDesign() {
    return { block: iconDraft, bundle: iconBundle };
  }

  // JSON round-trip, not structuredClone: what comes back from the API is
  // already wrapped in state proxies, which structuredClone refuses.
  function seedIcon(icon) {
    iconDraft = JSON.parse(JSON.stringify({ grid: 3, seams: [], colors: {}, ...(icon?.design?.block || {}) }));
    if (icon?.design?.bundle?.length) iconBundle = [...icon.design.bundle];
    iconSeed = JSON.stringify(currentIconDesign());
  }

  // Danger zone
  let confirmName = $state('');
  let wiping = $state(false);
  let wipeArmed = $state(false);

  // Export and wipe both need a fresh passkey confirmation (docs/adr/017).
  // Checked on load so someone without a passkey is told here, rather than
  // finding out at the moment they are trying to get their data out.
  let hasPasskey = $state(true);
  let exporting = $state(false);

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
      timezone = data.timezone || '';
      timezoneConfigured = data.timezone_configured || '';
      seedIcon(data.icon);
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
      applyIdentity(res);
      showToast('Quilt identity saved', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
    }
    savingIdentity = false;
  }

  /**
   * Drop one override and let patchwork.yaml speak for that field again
   * (explicit null on the endpoint). Per field, not per form: an admin who
   * wants the deployment's description back rarely wants to lose the name
   * they chose. The fields are only reachable when actually overridden, so
   * there is no "reset" that resets nothing.
   */
  async function resetField(field) {
    resetting = field;
    try {
      const res = await api('admin/settings', { method: 'PATCH', body: { [field]: null } });
      applyIdentity(res);
      showToast(field === 'name' ? 'Quilt name reset to the deployment default' : 'Description reset to the deployment default', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to reset', 'error');
    }
    resetting = '';
  }

  // The server owns what is effective after a write, including which fields
  // still carry an override, so the form re-seeds from the response rather
  // than assuming the value it just sent won.
  function applyIdentity(res) {
    data = {
      ...data,
      name: res.name,
      description: res.description,
      name_overridden: res.name_overridden,
      description_overridden: res.description_overridden,
    };
    name = res.name;
    description = res.description;
    applyIdentityChange({ name: res.name });
  }

  // The fabrics the icon draws with. Slot zero is the quilt's own color;
  // the last fabric is the ground, matching what the server renders.
  let iconPalette = $derived.by(() => {
    const b = iconBundle.length ? iconBundle : ['#039BE6', '#F2EEE4'];
    return { primary: b[0], secondary: b[1] || b[0], bg: b[b.length - 1], slots: [...b] };
  });

  const ICON_PREVIEW = 64;
  const STARTER_THUMB = 44;

  let previewEl = $state(null);
  let starterEls = $state({});

  function drawIcon(svgEl, size, block, palette) {
    const svg = d3.select(svgEl);
    svg.selectAll('*').remove();
    renderDraftBlock(svg.append('g'), size, block, palette);
  }

  $effect(() => {
    if (!previewEl) return;
    void JSON.stringify(iconDraft);
    void iconPalette;
    drawIcon(previewEl, ICON_PREVIEW, iconDraft, iconPalette);
  });

  $effect(() => {
    const p = iconPalette;
    for (const s of data?.icon_starters || []) {
      if (starterEls[s.key]) drawIcon(starterEls[s.key], STARTER_THUMB, s.block, p);
    }
  });

  function startFrom(starter) {
    iconDraft = JSON.parse(JSON.stringify({ seams: [], colors: {}, ...starter.block }));
  }

  async function saveIcon() {
    savingIcon = true;
    try {
      const res = await api('admin/settings', {
        method: 'PATCH',
        body: { icon_design: currentIconDesign() },
      });
      data = { ...data, icon: res.icon };
      iconSeed = JSON.stringify(currentIconDesign());
      applyIdentityChange();
      showToast('Quilt icon saved', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to save the icon', 'error');
    }
    savingIcon = false;
  }

  async function resetIcon() {
    savingIcon = true;
    try {
      const res = await api('admin/settings', { method: 'PATCH', body: { icon_design: null } });
      data = { ...data, icon: res.icon };
      seedIcon(res.icon);
      applyIdentityChange();
      showToast('Quilt icon reset', 'info');
    } catch (e) {
      showToast(e.message || 'Failed to reset the icon', 'error');
    }
    savingIcon = false;
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
      <div class="settings-card">
        <label class="field">
          <span class="field-label">Quilt name</span>
          <input type="text" bind:value={name} maxlength="100" />
          {#if data.name_overridden}
            <span class="field-hint">
              Set here in the admin panel. Overrides the name in patchwork.yaml.
              <button class="link-btn" onclick={() => resetField('name')} disabled={!!resetting}>
                {resetting === 'name' ? 'Resetting…' : 'Reset to default'}
              </button>
            </span>
          {/if}
        </label>
        <label class="field">
          <span class="field-label">Description</span>
          <textarea bind:value={description} rows="3" maxlength="500"></textarea>
          {#if data.description_overridden}
            <span class="field-hint">
              Set here in the admin panel. Overrides the description in patchwork.yaml.
              <button class="link-btn" onclick={() => resetField('description')} disabled={!!resetting}>
                {resetting === 'description' ? 'Resetting…' : 'Reset to default'}
              </button>
            </span>
          {/if}
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
      <div class="settings-card">
        <p class="section-desc">
          Represents this quilt in the quilt switcher and in other people's Connected Quilts.
          Draft it here, in the same drafter patches use for their tiles.
        </p>

        <div class="icon-current">
          <svg
            bind:this={previewEl}
            class="icon-preview"
            viewBox="0 0 {ICON_PREVIEW} {ICON_PREVIEW}"
            width={ICON_PREVIEW}
            height={ICON_PREVIEW}
            role="img"
            aria-label="Quilt icon preview"
          ></svg>
          <div class="icon-meta">
            <span class="icon-kind">
              {#if data.icon.chosen}
                Drafted for this quilt.
              {:else}
                Assigned from the quilt's name. Draft your own below.
              {/if}
            </span>
            <div class="icon-actions">
              <button class="btn btn-primary btn-sm" onclick={saveIcon} disabled={savingIcon || !iconDirty}>
                {savingIcon ? 'Saving…' : 'Save icon'}
              </button>
              {#if data.icon.chosen}
                <button class="btn btn-secondary btn-sm" onclick={resetIcon} disabled={savingIcon}>
                  Reset icon
                </button>
              {/if}
            </div>
          </div>
        </div>

        <div class="starter-picker">
          <span class="field-label">Start from a block</span>
          <div class="starter-grid">
            {#each data.icon_starters as starter (starter.key)}
              <button class="starter-option" onclick={() => startFrom(starter)} title={starter.name}>
                <svg
                  bind:this={starterEls[starter.key]}
                  viewBox="0 0 {STARTER_THUMB} {STARTER_THUMB}"
                  width={STARTER_THUMB}
                  height={STARTER_THUMB}
                  role="img"
                  aria-label={starter.name}
                ></svg>
                <span class="starter-name">{starter.name}</span>
              </button>
            {/each}
          </div>
          <span class="field-hint">Starting over replaces what's on the canvas. Nothing is saved until you save.</span>
        </div>

        <BlockDrafter bind:draft={iconDraft} bind:bundle={iconBundle} previewLabel="in the switcher" />
      </div>
    </section>

    <!-- ===== Data export ===== -->
    <section class="section">
      <h2>Timekeeping</h2>
      <p class="section-desc">
        Where this quilt keeps time. An event shows the wall clock of the place
        it happens, so this is what a patch falls back to when it has not named
        a timezone of its own, and what a calendar feed's zoneless times mean
        when it is imported. Everyone reading from elsewhere sees the event's
        own clock, with the zone named next to it.
      </p>
      <div class="settings-card">
        <label class="field-label" for="quilt-timezone">Timezone</label>
        <input
          id="quilt-timezone"
          type="text"
          bind:value={timezone}
          disabled={savingTimezone}
          placeholder={timezoneConfigured || 'America/New_York'}
        />
        <p class="section-desc">
          An IANA name, like America/New_York or Europe/Berlin.
          {#if timezoneConfigured}
            Clear it to go back to {timezoneConfigured.replace(/_/g, ' ')} from patchwork.yaml.
          {/if}
          {#if !zoneIsKnown(timezone.trim())}
            <span class="zone-invalid">Not a timezone this quilt knows.</span>
          {/if}
        </p>
        <button class="btn btn-primary" onclick={saveTimezone} disabled={savingTimezone}>
          {savingTimezone ? 'Saving...' : 'Save timezone'}
        </button>
      </div>
    </section>

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
      <div class="settings-card">
        <!-- docs/adr/060: ap_followers and ap_id stay behind, so the community
             travels and its audience does not. Kept outside the <p>: the copy
             ledger blanks comments when extracting but matches raw source when
             writing back, so a comment inside a text run makes that string
             permanently unwritable (tools/copy-ledger/writeback.js). -->
        <p class="section-desc">
          Download this quilt's portable community data as a zip: patches, people, memberships, events, proposals with votes, and governance records. Credentials, sessions, and federation keys deliberately stay behind, as do followers from other sites, since the addresses they follow belong to this quilt rather than to the patches. For a full backup of the deployment itself, back up the server's data directory.
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
  .zone-invalid {
    color: var(--color-danger, #e05252);
  }

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
    max-width: var(--pw-measure);
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

  /* Reset sits inside the hint that explains the override, at the hint's
     weight: it undoes a setting rather than saving one, so it never
     competes with Save Identity. */
  .link-btn {
    padding: 0;
    border: none;
    background: none;
    font: inherit;
    color: var(--color-primary);
    cursor: pointer;
    text-decoration: underline;
  }

  .link-btn:disabled {
    color: var(--color-text-muted);
    cursor: default;
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
    border: 2px solid var(--lt-thread, var(--color-border));
    border-radius: 4px;
    flex-shrink: 0;
    display: block;
  }

  .icon-meta {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.5rem;
  }

  .icon-kind {
    font-size: 0.85rem;
    color: var(--color-text-muted);
  }

  .icon-actions {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .starter-picker {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .starter-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(84px, 1fr));
    gap: 0.5rem;
  }

  .starter-option {
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

  .starter-option:hover {
    border-color: var(--color-primary);
  }

  .starter-option svg {
    border-radius: 2px;
  }

  .starter-name {
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
