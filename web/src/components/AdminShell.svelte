<script>
  /**
   * The admin panel: instance administration as a full-screen takeover
   * (docs/adr/005) — global bar with the Administration crumb and admin
   * finder, five tabs as the tab row (docs/adr/2026-09-17-an-admin-tab-answers-one-question.md), full-width content.
   * No discovery chrome.
   *
   * Review and Settings carry a sidebar, the same SettingsShell a patch's
   * Settings and Governance use, so the panel reads like a workspace
   * rather than a row of fifteen tabs. The tab and section lists come
   * from lib/adminPanel.js; this file only draws them.
   *
   * On a narrow screen there is no tab row: the panel drills down, one
   * level on screen at a time. /admin carries the five tabs as a list above
   * the Overview, Review and Settings are lists of their sections, and every
   * page below the root has one back link up a level.
   */
  import { navigate, getPath } from '../stores/router.svelte.js';
  import { api } from '../lib/api.js';
  import { adminTabs, adminTabForPath } from '../lib/adminPanel.js';
  import { adminFinderProvider } from '../lib/finderProviders.js';
  import GlobalBar from './GlobalBar.svelte';
  import ContextCrumb from './ContextCrumb.svelte';
  import WorkspaceSearch from './WorkspaceSearch.svelte';
  import SettingsShell from './SettingsShell.svelte';
  import { ltTabs } from '../lib/lt.js';
  import { Gauge, Tray, Users, GearSix, ListMagnifyingGlass, CaretLeft, CaretRight } from 'phosphor-svelte';

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

  // The tabs the narrow-screen root lists. Overview is the root itself.
  let menuTabs = $derived(tabs.filter((t) => t.id !== 'overview'));

  function tabHref(tab) {
    return tab.sections ? tab.sections[0].href : tab.href;
  }

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
    <nav class="lt-tabs workspace-tabs" use:ltTabs>
      {#each tabs as tab (tab.id)}
        {@const Icon = tab.icon}
        {@const href = tabHref(tab)}
        <a
          {href}
          class="lt-tab workspace-tab"
          aria-current={isActive(tab) ? 'page' : undefined}
          onclick={(e) => handleNav(e, href)}
        >
          <span class="tab-icon"><Icon size={16} weight="duotone" /></span>
          {tab.label}
          {#if tab.id === 'review' && reviewTotal > 0}
            <span class="lt-count tab-count" aria-label="{reviewTotal} waiting">{reviewTotal}</span>
          {/if}
        </a>
      {/each}
    </nav>
  </div>

  <div class="workspace-body work-content">
    {#if activeId === 'overview'}
      <nav class="admin-menu" aria-label="Administration">
        {#each menuTabs as tab (tab.id)}
          {@const Icon = tab.icon}
          <a href={tab.href} class="lt-list-row admin-menu-link" onclick={(e) => handleNav(e, tab.href)}>
            <span class="tab-icon"><Icon size={20} weight="duotone" /></span>
            <span class="admin-menu-label">{tab.label}</span>
            {#if tab.id === 'review' && reviewTotal > 0}
              <span class="lt-count tab-count" aria-label="{reviewTotal} waiting">{reviewTotal}</span>
            {/if}
            <span class="admin-menu-caret"><CaretRight size={14} weight="bold" /></span>
          </a>
        {/each}
      </nav>
    {:else if !activeTab?.sections}
      <a href="/admin" class="lt-split-back admin-back" onclick={(e) => handleNav(e, '/admin')}>
        <CaretLeft size={14} weight="bold" />
        Administration
      </a>
    {/if}

    {#if activeTab?.sections}
      <SettingsShell
        title={activeTab.label}
        sections={sidebarSections}
        indexHref={activeTab.href}
        parent={{ href: '/admin', label: 'Administration' }}
      >
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
    /* Stick under the fixed global bar, not at the viewport's top: at
       top: 0 the bar (z-index 60) covers the row the moment the page
       scrolls. */
    top: 56px;
    margin-top: 56px; /* clear the fixed global bar */
    z-index: 50;
    display: flex;
    align-items: center;
    padding: 0 16px;
    background: var(--color-surface);
    border-bottom: 1px solid var(--color-border);
  }

  /* Liquid Textile's tab row (ltTabs); the nav under it draws the rule. */
  .workspace-tabs {
    border-bottom: 0;
  }

  .tab-icon {
    display: flex;
    flex-shrink: 0;
    color: var(--color-text-muted);
  }

  .workspace-tab[aria-current="page"] .tab-icon {
    color: var(--color-accent);
  }

  /* The narrow screen's navigation. Neither exists on a wide one: the
     back link is Liquid Textile's split back link, hidden until here. */
  .admin-menu {
    display: none;
  }

  @media (max-width: 640px) {
    .workspace-nav {
      display: none;
    }

    .workspace-body {
      margin-top: 56px; /* the tab row cleared the global bar before */
    }

    .admin-menu {
      display: flex;
      flex-direction: column;
      margin-bottom: 1.5rem;
      border-top: 1px solid var(--color-border);
    }

    .admin-back {
      display: inline-flex;
    }
  }

  /* Liquid Textile's phone menu row, inset and sized like the sections
     list a tab drills into (SettingsShell), which sits in the same page. */
  .admin-menu-link {
    padding: 0 0.25rem;
    font-size: 1rem;
  }

  .admin-menu-label {
    flex: 1;
  }

  .admin-menu-caret {
    display: flex;
    color: var(--color-text-muted);
  }
</style>
