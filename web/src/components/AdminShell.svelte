<script>
  /**
   * The admin panel: instance administration as a full-screen takeover
   * (docs/adr/005) — global bar with the Administration crumb and admin
   * finder, five tabs as the tab row (docs/adr/118), full-width content.
   * No discovery chrome.
   *
   * Review and Settings carry a sidebar, the same SettingsShell a patch's
   * Settings and Governance use, so the panel reads like a workspace
   * rather than a row of fifteen tabs. The tab and section lists come
   * from lib/adminPanel.js; this file only draws them.
   */
  import { navigate, getPath } from '../stores/router.svelte.js';
  import { api } from '../lib/api.js';
  import { adminTabs, adminTabForPath } from '../lib/adminPanel.js';
  import { adminFinderProvider } from '../lib/finderProviders.js';
  import GlobalBar from './GlobalBar.svelte';
  import ContextCrumb from './ContextCrumb.svelte';
  import WorkspaceSearch from './WorkspaceSearch.svelte';
  import SettingsShell from './SettingsShell.svelte';
  import { Gauge, Tray, Users, GearSix, ListMagnifyingGlass } from 'phosphor-svelte';

  let { children } = $props();

  const ICONS = {
    overview: Gauge,
    review: Tray,
    users: Users,
    settings: GearSix,
    audit: ListMagnifyingGlass,
  };

  const tabs = adminTabs().map((t) => ({ ...t, icon: ICONS[t.id] }));

  let path = $derived(getPath());
  let activeId = $derived(adminTabForPath(path));
  let activeTab = $derived(tabs.find((t) => t.id === activeId) || null);

  // What is waiting, by queue, from the same endpoint the Overview reads —
  // so the Review badge, the sidebar counts and the Overview's inbox never
  // disagree. Re-read on every move within the panel: a decision on one
  // queue page changes the number, and one small request per navigation
  // is cheaper than five pages each reporting back.
  let inboxCounts = $state({});
  $effect(() => {
    void path;
    api('admin/overview')
      .then((o) => {
        const counts = {};
        for (const q of o?.inbox || []) counts[q.queue] = q.count;
        inboxCounts = counts;
      })
      .catch(() => {});
  });

  let reviewTotal = $derived(
    Object.values(inboxCounts).reduce((sum, n) => sum + n, 0)
  );

  // The sidebar sections for the active tab, each carrying its own count
  // where it has a queue. SettingsShell renders a count when one is given.
  let sidebarSections = $derived(
    (activeTab?.sections || []).map((s) => ({
      label: s.label,
      href: s.href,
      count: s.queue ? inboxCounts[s.queue] || 0 : undefined,
    }))
  );

  function isActive(tab) {
    return tab.id === activeId;
  }

  function handleNav(e, href) {
    e.preventDefault();
    navigate(href);
  }

  const finderProvider = adminFinderProvider();
</script>

<div class="workspace">
  <GlobalBar>
    {#snippet leading()}
      <ContextCrumb label="Administration" href="/admin" />
    {/snippet}
    {#snippet search()}
      <WorkspaceSearch placeholder="Search administration…" provider={finderProvider} />
    {/snippet}
  </GlobalBar>

  <div class="workspace-nav">
    <nav class="workspace-tabs">
      {#each tabs as tab (tab.id)}
        {@const Icon = tab.icon}
        {@const href = tab.sections ? tab.sections[0].href : tab.href}
        <a
          {href}
          class="workspace-tab"
          class:active={isActive(tab)}
          onclick={(e) => handleNav(e, href)}
        >
          <span class="tab-icon"><Icon size={16} weight="duotone" /></span>
          {tab.label}
          {#if tab.id === 'review' && reviewTotal > 0}
            <span class="tab-count" aria-label="{reviewTotal} waiting">{reviewTotal}</span>
          {/if}
        </a>
      {/each}
    </nav>
  </div>

  <div class="workspace-body work-content">
    {#if activeTab?.sections}
      <SettingsShell title={activeTab.label} sections={sidebarSections}>
        {#snippet children()}
          {@render children()}
        {/snippet}
      </SettingsShell>
    {:else}
      {@render children()}
    {/if}
  </div>
</div>

<style>
  .workspace {
    min-height: 100vh;
  }

  .workspace-nav {
    position: sticky;
    top: 0;
    margin-top: 56px; /* clear the fixed global bar */
    z-index: 50;
    display: flex;
    align-items: center;
    padding: 0 16px;
    background: var(--color-surface);
    border-bottom: 1px solid var(--color-border);
  }

  .workspace-tabs {
    display: flex;
    align-items: stretch;
    gap: 4px;
    overflow-x: auto;
    scrollbar-width: none;
  }

  .workspace-tabs::-webkit-scrollbar {
    display: none;
  }

  .workspace-tab {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 12px 12px;
    font-size: 0.88rem;
    font-weight: 500;
    color: var(--color-text-muted);
    text-decoration: none;
    white-space: nowrap;
    border-bottom: 2px solid transparent;
    transition: color 120ms ease;
  }

  .workspace-tab:hover {
    color: var(--color-text);
    text-decoration: none;
  }

  .workspace-tab.active {
    color: var(--color-text);
    font-weight: 600;
    border-bottom-color: var(--color-accent);
  }

  .tab-icon {
    display: flex;
    flex-shrink: 0;
    color: var(--color-text-muted);
  }

  .workspace-tab.active .tab-icon {
    color: var(--color-accent);
  }

  .tab-count {
    min-width: 1.25rem;
    padding: 0 0.35rem;
    border-radius: 999px;
    background: var(--color-accent);
    color: var(--color-surface);
    font-size: 0.72rem;
    font-weight: 600;
    line-height: 1.25rem;
    text-align: center;
  }
</style>
