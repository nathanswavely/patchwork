<script>
  /**
   * Usage: daily page views and visitors, counted on the server
   * (docs/adr/2026-09-18-counting-visitors-without-watching-anyone.md).
   *
   * The page is a gauge for the people running the machine, and it says
   * on its face what the counter keeps and what it refuses to. The switch
   * that turns counting on lives here rather than in Quilt settings, so
   * the numbers and the decision to have them are never on different
   * pages. The privacy policy reads the same switch.
   */
  import { api } from '../lib/api.js';
  import { showToast } from '../stores/toast.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';

  let loading = $state(true);
  let error = $state('');
  let data = $state(null);
  let days = $state(30);
  let toggling = $state(false);
  let clearing = $state(false);

  const WINDOWS = [7, 30, 90, 365];

  $effect(() => { load(days); });

  async function load(n) {
    loading = true;
    error = '';
    try {
      data = await api(`admin/usage?days=${n}`);
    } catch (e) {
      error = e.message;
    }
    loading = false;
  }

  async function setEnabled(value) {
    toggling = true;
    try {
      await api('admin/settings', { method: 'PATCH', body: { usage_stats: value } });
      showToast(value ? 'Counting on. The privacy policy now says so.' : 'Counting off. The privacy policy now says so.', 'success');
      await load(days);
    } catch (e) {
      showToast(e.message || 'Failed to save', 'error');
    }
    toggling = false;
  }

  async function clearCounts() {
    if (!confirm('Delete every daily count this quilt has kept? This cannot be undone.')) return;
    clearing = true;
    try {
      await api('admin/usage', { method: 'DELETE' });
      showToast('Counts cleared', 'success');
      await load(days);
    } catch (e) {
      showToast(e.message || 'Failed to clear', 'error');
    }
    clearing = false;
  }

  let maxViews = $derived(data ? Math.max(1, ...data.series.map((d) => d.views)) : 1);
  let maxPath = $derived(data && data.paths.length ? data.paths[0].views : 1);
  let quiet = $derived(data ? data.views === 0 : true);

  function shortDay(day) {
    const [, m, d] = day.split('-');
    return `${Number(m)}/${Number(d)}`;
  }
</script>

<div class="admin-usage page-fade">
  <header class="page-head">
    <h1>Usage</h1>
    <p class="muted">
      How much this quilt is being read, counted on the server as daily totals. There is no script in the page, no cookie, and nothing kept that names a person, an address, or a moment.
    </p>
  </header>

  {#if loading && !data}
    <Skeleton lines={8} />
  {:else if error}
    <ErrorState message={error} retry={() => load(days)} />
  {:else if data}
    <section class="card switch-card">
      <div class="switch-row">
        <div>
          <h2>{data.enabled ? 'Counting is on' : 'Counting is off'}</h2>
          <p class="muted">
            {#if data.enabled}
              Each page load adds one to that kind of page for today. A browser is counted once a day, from a hash of its address and type under a secret the server draws at random, keeps in memory, and replaces at midnight. Totals are kept for {data.retention_months} months. The privacy policy states all of this.
            {:else}
              Nothing is being counted. Turning this on counts page loads per kind of page per day and distinct browsers per day, on the server, and the privacy policy changes to say so. It never counts API calls, assets, or anyone a browser announces as software.
            {/if}
          </p>
        </div>
        <button
          class="btn"
          class:btn-primary={!data.enabled}
          onclick={() => setEnabled(!data.enabled)}
          disabled={toggling}
        >
          {toggling ? 'Saving…' : data.enabled ? 'Turn off' : 'Turn on'}
        </button>
      </div>
    </section>

    <div class="range-row">
      <div class="range-tabs" role="tablist" aria-label="Window">
        {#each WINDOWS as n (n)}
          <button
            role="tab"
            aria-selected={days === n}
            class="range-tab"
            class:active={days === n}
            onclick={() => { days = n; }}
          >
            {n === 365 ? 'Year' : `${n} days`}
          </button>
        {/each}
      </div>
      <span class="muted since">Since {data.since}, in UTC days</span>
    </div>

    <section class="tiles">
      <div class="tile">
        <span class="tile-n">{data.views.toLocaleString()}</span>
        <span class="tile-l">page loads</span>
      </div>
      <div class="tile">
        <span class="tile-n">{data.peak_visitors.toLocaleString()}</span>
        <span class="tile-l">visitors on the busiest day</span>
      </div>
      <div class="tile">
        <span class="tile-n">{data.activity.accounts.toLocaleString()}</span>
        <span class="tile-l">accounts created</span>
      </div>
      <div class="tile">
        <span class="tile-n">{(data.activity.joins + data.activity.follows).toLocaleString()}</span>
        <span class="tile-l">joins and follows</span>
      </div>
    </section>

    <section class="card">
      <h2>Page loads by day</h2>
      {#if quiet}
        <p class="muted">
          {data.enabled ? 'Nothing counted in this window yet. Counts are written once a minute.' : 'Nothing counted in this window. Counting is off.'}
        </p>
      {:else}
        <div class="chart" role="img" aria-label="Page loads per day">
          {#each data.series as d (d.day)}
            <div class="bar-col" title={`${d.day}: ${d.views} loads, ${d.visitors} visitors`}>
              <div class="bar" style={`height:${Math.round((d.views / maxViews) * 100)}%`}></div>
            </div>
          {/each}
        </div>
        <div class="chart-axis muted">
          <span>{shortDay(data.series[0].day)}</span>
          <span>{shortDay(data.series[data.series.length - 1].day)}</span>
        </div>
        <p class="muted hint">Visitors are distinct within one day only, so they are not added across days. Hover a bar for that day's two numbers.</p>
      {/if}
    </section>

    <div class="two-col">
      <section class="card">
        <h2>Pages</h2>
        {#if data.paths.length === 0}
          <p class="muted">Nothing counted in this window.</p>
        {:else}
          <table class="paths">
            <tbody>
              {#each data.paths as p (p.path)}
                <tr>
                  <td class="path"><code>{p.path}</code></td>
                  <td class="n">{p.views.toLocaleString()}</td>
                  <td class="meter"><div class="meter-fill" style={`width:${Math.round((p.views / maxPath) * 100)}%`}></div></td>
                </tr>
              {/each}
            </tbody>
          </table>
          <p class="muted hint">A page is counted by its shape. Which patch or person a load was for is never kept.</p>
        {/if}
      </section>

      <section class="card">
        <h2>Activity</h2>
        <p class="muted">What people did in the same window, from the records the quilt keeps anyway. These are the same whether counting is on or off.</p>
        <dl class="activity">
          <div><dt>Accounts created</dt><dd>{data.activity.accounts.toLocaleString()}</dd></div>
          <div><dt>Patches joined</dt><dd>{data.activity.joins.toLocaleString()}</dd></div>
          <div><dt>Patches followed</dt><dd>{data.activity.follows.toLocaleString()}</dd></div>
          <div><dt>Events posted</dt><dd>{data.activity.events.toLocaleString()}</dd></div>
          <div><dt>Proposals opened</dt><dd>{data.activity.proposals.toLocaleString()}</dd></div>
        </dl>
      </section>
    </div>

    <section class="card clear-card">
      <div>
        <h2>Clear counts</h2>
        <p class="muted">Deletes every daily total this quilt has kept. Counting continues if it is on. The deletion is recorded in the audit log.</p>
      </div>
      <button class="btn btn-danger" onclick={clearCounts} disabled={clearing}>
        {clearing ? 'Clearing…' : 'Clear all counts'}
      </button>
    </section>
  {/if}
</div>

<style>
  .admin-usage {
    display: flex;
    flex-direction: column;
    gap: var(--space-lg, 1.5rem);
  }

  .card {
    background: var(--color-surface, #fff);
    border: 1px solid var(--color-border, #e3e0da);
    border-radius: var(--radius-md, 8px);
    padding: 1.25rem;
  }

  .card h2 {
    font-size: 1rem;
    margin: 0 0 0.5rem;
  }

  .switch-row,
  .clear-card {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1.5rem;
  }

  .switch-row p,
  .clear-card p {
    margin: 0;
    max-width: 62ch;
  }

  .switch-row .btn,
  .clear-card .btn {
    flex-shrink: 0;
  }

  .range-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    flex-wrap: wrap;
  }

  .range-tabs {
    display: flex;
    gap: 0.25rem;
  }

  .range-tab {
    border: 1px solid var(--color-border, #e3e0da);
    background: transparent;
    padding: 0.35rem 0.75rem;
    border-radius: var(--radius-sm, 6px);
    cursor: pointer;
    font: inherit;
    color: inherit;
  }

  .range-tab.active {
    background: var(--color-primary, #333);
    color: var(--color-on-primary, #fff);
    border-color: var(--color-primary, #333);
  }

  .since {
    font-size: 0.85rem;
  }

  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: 0.75rem;
  }

  .tile {
    background: var(--color-surface, #fff);
    border: 1px solid var(--color-border, #e3e0da);
    border-radius: var(--radius-md, 8px);
    padding: 1rem;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .tile-n {
    font-size: 1.6rem;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .tile-l {
    font-size: 0.85rem;
    color: var(--color-text-muted, #666);
  }

  .chart {
    display: flex;
    align-items: flex-end;
    gap: 2px;
    height: 160px;
    padding-top: 0.5rem;
  }

  .bar-col {
    flex: 1 1 0;
    min-width: 0;
    height: 100%;
    display: flex;
    align-items: flex-end;
  }

  .bar {
    width: 100%;
    min-height: 2px;
    background: var(--color-primary, #333);
    border-radius: 2px 2px 0 0;
    opacity: 0.85;
  }

  .chart-axis {
    display: flex;
    justify-content: space-between;
    font-size: 0.8rem;
    margin-top: 0.25rem;
  }

  .hint {
    font-size: 0.85rem;
    margin: 0.75rem 0 0;
  }

  .two-col {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-lg, 1.5rem);
  }

  @media (max-width: 720px) {
    .two-col {
      grid-template-columns: 1fr;
    }

    .switch-row,
    .clear-card {
      flex-direction: column;
    }
  }

  .paths {
    width: 100%;
    border-collapse: collapse;
  }

  .paths td {
    padding: 0.3rem 0.5rem 0.3rem 0;
    vertical-align: middle;
  }

  .paths .path code {
    font-size: 0.85rem;
  }

  .paths .n {
    text-align: right;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .paths .meter {
    width: 30%;
  }

  .meter-fill {
    height: 8px;
    background: var(--color-primary, #333);
    opacity: 0.35;
    border-radius: 4px;
  }

  .activity {
    margin: 0.75rem 0 0;
    display: grid;
    gap: 0.4rem;
  }

  .activity div {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
  }

  .activity dt {
    color: var(--color-text-muted, #666);
  }

  .activity dd {
    margin: 0;
    font-variant-numeric: tabular-nums;
  }
</style>
