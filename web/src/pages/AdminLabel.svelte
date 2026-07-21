<script>
  /**
   * Admin editing for the Label (docs/adr/023). The disclosure is authored
   * prose plus structured slots only where structure earns its keep: cost
   * line items, support and feedback links, the steward roster.
   *
   * The floor: a Label cannot publish with zero listed stewards. Adding
   * yourself lists you (you're consenting in the act); adding anyone else
   * invites them — only they can accept.
   */
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import { getUser } from '../stores/auth.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';
  import { Plus, Trash } from 'phosphor-svelte';

  let loading = $state(true);
  let error = $state('');

  let published = $state(false);
  let prose = $state('');
  let supportUrl = $state('');
  let feedbackUrl = $state('');
  let currency = $state('USD');
  let seamrippedFromName = $state('');
  let seamrippedFromUrl = $state('');
  let stewards = $state([]);
  // Cost rows are edited in display units; converted to integer minor
  // units on save (the backend never sees floats-as-money).
  let costs = $state([]);

  let saving = $state(false);
  let savingCosts = $state(false);
  let newStewardUsername = $state('');

  let me = $derived(getUser());
  let listedCount = $derived(stewards.filter((s) => s.listed).length);

  $effect(() => { load(); });

  async function load() {
    loading = true;
    error = '';
    try {
      const data = await api('admin/label');
      published = data.published;
      prose = data.prose;
      supportUrl = data.support_url;
      feedbackUrl = data.feedback_url;
      currency = data.currency;
      seamrippedFromName = data.seamripped_from_name;
      seamrippedFromUrl = data.seamripped_from_url;
      stewards = data.stewards;
      costs = (data.cost_items || []).map((c) => ({
        service: c.service,
        purpose: c.purpose,
        why: c.why,
        amount: (c.amount_minor / 100).toFixed(2),
        period: c.period,
        stated_on: c.stated_on,
      }));
    } catch (e) {
      error = e.message;
    }
    loading = false;
  }

  async function saveContent() {
    saving = true;
    try {
      await api('admin/label', {
        method: 'PATCH',
        body: {
          prose,
          support_url: supportUrl,
          feedback_url: feedbackUrl,
          currency,
          seamripped_from_name: seamrippedFromName,
          seamripped_from_url: seamrippedFromUrl,
        },
      });
      showToast('Label saved', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
    }
    saving = false;
  }

  function addCostRow() {
    const today = new Date().toISOString().slice(0, 10);
    costs = [...costs, { service: '', purpose: '', why: '', amount: '', period: 'monthly', stated_on: today }];
  }

  function removeCostRow(i) {
    costs = costs.filter((_, idx) => idx !== i);
  }

  async function saveCosts() {
    const items = [];
    for (const c of costs) {
      const amount = Math.round(parseFloat(c.amount) * 100);
      if (!c.service.trim()) {
        showToast('Every cost item needs a service name', 'error');
        return;
      }
      if (!Number.isFinite(amount) || amount < 0) {
        showToast(`"${c.service}" needs a valid amount`, 'error');
        return;
      }
      items.push({
        service: c.service.trim(),
        purpose: c.purpose.trim(),
        why: c.why.trim(),
        amount_minor: amount,
        period: c.period,
        stated_on: c.stated_on,
      });
    }
    savingCosts = true;
    try {
      await api('admin/label/costs', { method: 'PUT', body: { items } });
      showToast('Costs saved', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to save costs', 'error');
    }
    savingCosts = false;
  }

  async function addSteward() {
    const username = newStewardUsername.trim().replace(/^@/, '');
    if (!username) return;
    try {
      await api('admin/label/stewards', { method: 'POST', body: { username } });
      newStewardUsername = '';
      if (username === me?.username) {
        showToast('You are listed as a steward', 'success');
      } else {
        showToast(`Invited @${username}. The listing stays hidden until they accept`, 'info');
      }
      await load();
    } catch (e) {
      showToast(e.message || 'Failed to add steward', 'error');
    }
  }

  async function removeSteward(s) {
    try {
      await api(`admin/label/stewards/${s.id}`, { method: 'DELETE' });
      await load();
    } catch (e) {
      showToast(e.message || 'Failed to remove steward', 'error');
    }
  }

  async function setPublished(next) {
    try {
      await api('admin/label', { method: 'PATCH', body: { published: next } });
      published = next;
      showToast(next ? 'The Label is public at /label' : 'Label unpublished', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to update', 'error');
    }
  }
</script>

<div class="page-fade">
  <div class="page-header">
    <h1>The Label</h1>
    <p class="muted">
      The page that tells people who runs this quilt and what it costs to
      keep up. Publishes to /label, readable without an account.
    </p>
  </div>

  {#if loading}
    <Skeleton lines={8} />
  {:else if error}
    <ErrorState message={error} retry={load} />
  {:else}
    <!-- ===== Publish state ===== -->
    <section class="section">
      <div class="card settings-card publish-card">
        <div>
          <strong>{published ? 'Published' : 'Not published'}</strong>
          <p class="muted small">
            {#if published}
              Anyone can read the Label, signed in or not.
            {:else if listedCount === 0}
              You need at least one listed steward before this can publish.
              Somebody has to put their name on it.
            {:else}
              Ready to publish whenever you are.
            {/if}
          </p>
        </div>
        {#if published}
          <button class="btn btn-secondary" onclick={() => setPublished(false)}>Unpublish</button>
        {:else}
          <button class="btn btn-primary" onclick={() => setPublished(true)} disabled={listedCount === 0}>
            Publish
          </button>
        {/if}
      </div>
    </section>

    <!-- ===== Stewards ===== -->
    <section class="section">
      <h2>Stewards</h2>
      <div class="card settings-card">
        <p class="section-desc">
          The people who answer for this quilt, by name. Add yourself and
          you appear right away. Add anyone else and they get an invitation
          instead, because a listing belongs to the person in it. A handle
          is plenty. No legal names.
        </p>
        {#each stewards as s (s.id)}
          <div class="steward-row">
            <span class="steward-who">
              <strong>{s.display_name || `@${s.username}`}</strong>
              <span class="muted small">@{s.username}</span>
            </span>
            <span class="steward-state muted small">
              {s.listed ? (s.blurb || 'Listed') : 'Invited — not yet accepted'}
            </span>
            <button class="icon-btn" title="Remove" onclick={() => removeSteward(s)}>
              <Trash size={16} />
            </button>
          </div>
        {/each}
        <div class="steward-add">
          <input
            type="text"
            placeholder="username"
            bind:value={newStewardUsername}
            onkeydown={(e) => e.key === 'Enter' && addSteward()}
          />
          <button class="btn btn-secondary" onclick={addSteward} disabled={!newStewardUsername.trim()}>
            <Plus size={14} /> Add steward
          </button>
        </div>
      </div>
    </section>

    <!-- ===== In your own words ===== -->
    <section class="section">
      <h2>In your own words</h2>
      <div class="card settings-card">
        <p class="section-desc">
          Introduce yourself. Say why this quilt exists and what you spend
          to keep it going, in whatever voice you'd actually use out loud.
          Markdown works.
        </p>
        <textarea class="prose-input" bind:value={prose} rows="10"
          placeholder="Hi, I run this quilt. Here's why…"></textarea>
        <div class="field-grid">
          <label class="field">
            <span class="field-label">Support link</span>
            <input type="url" bind:value={supportUrl} placeholder="https://ko-fi.com/… (optional)" />
          </label>
          <label class="field">
            <span class="field-label">Feedback link</span>
            <input type="url" bind:value={feedbackUrl} placeholder="https://… (optional)" />
          </label>
        </div>
        {#if seamrippedFromName}
          <div class="field">
            <span class="field-label">Seamripped from</span>
            <div class="seamrip-line">
              <span>{seamrippedFromName} {#if seamrippedFromUrl}<span class="muted small">({seamrippedFromUrl})</span>{/if}</span>
              <button class="btn btn-secondary btn-sm"
                onclick={() => { seamrippedFromName = ''; seamrippedFromUrl = ''; }}>
                Remove
              </button>
            </div>
            <span class="field-hint">
              Worth keeping if the history is friendly. Remove it if it
              isn't.
            </span>
          </div>
        {/if}
        <div class="field-actions">
          <button class="btn btn-primary" onclick={saveContent} disabled={saving}>
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </section>

    <!-- ===== Costs ===== -->
    <section class="section">
      <h2>What this runs on</h2>
      <div class="card settings-card">
        <p class="section-desc">
          List what you pay and when you last checked each number, and the
          page can show readers a monthly total and warn them when a figure
          gets old. Use the why field to explain your choices, like picking
          Hetzner over a hyperscaler. You can also skip this and talk costs
          in the prose, though then there's no total and no staleness
          warning.
        </p>
        <label class="field currency-field">
          <span class="field-label">Currency</span>
          <input type="text" bind:value={currency} maxlength="3" placeholder="USD" />
        </label>
        {#each costs as c, i}
          <div class="cost-row">
            <div class="cost-row-main">
              <input class="cost-service" type="text" placeholder="Service (e.g. Hetzner CX22)" bind:value={c.service} />
              <input class="cost-purpose" type="text" placeholder="What for (e.g. the server)" bind:value={c.purpose} />
              <input class="cost-amount" type="number" min="0" step="0.01" placeholder="0.00" bind:value={c.amount} />
              <select bind:value={c.period}>
                <option value="monthly">/month</option>
                <option value="yearly">/year</option>
              </select>
              <input class="cost-date" type="date" bind:value={c.stated_on} title="When this figure was stated or last reviewed" />
              <button class="icon-btn" title="Remove" onclick={() => removeCostRow(i)}>
                <Trash size={16} />
              </button>
            </div>
            <input class="cost-why" type="text"
              placeholder="Why this one? (cheap, EU-based, no contract)"
              bind:value={c.why} />
          </div>
        {/each}
        <div class="field-actions cost-actions">
          <button class="btn btn-secondary" onclick={addCostRow}><Plus size={14} /> Add line</button>
          <button class="btn btn-primary" onclick={saveCosts} disabled={savingCosts}>
            {savingCosts ? 'Saving…' : 'Save costs'}
          </button>
        </div>
      </div>
    </section>
  {/if}
</div>

<style>
  .section {
    margin-bottom: 28px;
  }
  .section h2 {
    font-size: 1.05rem;
    margin: 0 0 10px;
  }
  .card {
    border: 1px solid var(--color-border);
    border-radius: 10px;
    background: var(--color-surface);
    padding: 16px;
  }
  .section-desc {
    margin: 0 0 14px;
    font-size: 0.85rem;
    color: var(--color-text);
    opacity: 0.8;
  }
  .small {
    font-size: 0.8rem;
  }

  .publish-card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
  }
  .publish-card p {
    margin: 2px 0 0;
  }

  .steward-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 0;
    border-bottom: 1px solid var(--color-border);
  }
  .steward-who {
    display: flex;
    flex-direction: column;
  }
  .steward-state {
    margin-left: auto;
  }
  .steward-add {
    display: flex;
    gap: 8px;
    margin-top: 12px;
  }
  .steward-add input {
    flex: 1;
    max-width: 240px;
  }

  .prose-input {
    width: 100%;
    resize: vertical;
    margin-bottom: 14px;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-bottom: 12px;
  }
  .field-label {
    font-size: 0.8rem;
    font-weight: 600;
  }
  .field-hint {
    font-size: 0.75rem;
    opacity: 0.7;
  }
  .field-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
  @media (max-width: 640px) {
    .field-grid {
      grid-template-columns: 1fr;
    }
  }
  .field-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 8px;
  }
  .seamrip-line {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  .currency-field {
    max-width: 120px;
  }
  .cost-row {
    padding: 10px 0;
    border-top: 1px solid var(--color-border);
  }
  .cost-row-main {
    display: flex;
    gap: 6px;
    align-items: center;
    flex-wrap: wrap;
  }
  .cost-service { flex: 2 1 160px; }
  .cost-purpose { flex: 2 1 140px; }
  .cost-amount { flex: 0 1 90px; }
  .cost-date { flex: 0 1 140px; }
  .cost-why {
    width: 100%;
    margin-top: 6px;
    font-size: 0.85rem;
  }
  .cost-actions {
    justify-content: space-between;
  }

  .icon-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--color-text);
    opacity: 0.6;
    padding: 4px;
  }
  .icon-btn:hover {
    opacity: 1;
  }
</style>
