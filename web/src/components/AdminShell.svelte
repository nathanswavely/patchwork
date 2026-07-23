<script>
  /**
   * The admin panel: instance administration as a full-screen takeover
   * (docs/adr/005) — global bar with the Administration crumb and admin
   * finder, sections as the tab row, full-width content. No discovery
   * chrome.
   */
  import { navigate, getPath } from '../stores/router.svelte.js';
  import { adminFinderProvider } from '../lib/finderProviders.js';
  import GlobalBar from './GlobalBar.svelte';
  import ContextCrumb from './ContextCrumb.svelte';
  import WorkspaceSearch from './WorkspaceSearch.svelte';
  import { Gauge, Users, Flag, Tray, HandPalm, ListMagnifyingGlass, SquaresFour, Tag, Graph, IdentificationCard, Scales, CalendarBlank, Archive } from 'phosphor-svelte';

  let { children } = $props();

  const sections = [
    { label: 'Overview', href: '/admin', icon: Gauge },
    { label: 'Quilt', href: '/admin/quilt', icon: SquaresFour },
    { label: 'Neighbors', href: '/admin/neighbors', icon: Graph },
    { label: 'Label', href: '/admin/label', icon: IdentificationCard },
    { label: 'Legal', href: '/admin/legal', icon: Scales },
    { label: 'Tags', href: '/admin/tags', icon: Tag },
    { label: 'Users', href: '/admin/users', icon: Users },
    { label: 'Reports', href: '/admin/reports', icon: Flag },
    { label: 'Submissions', href: '/admin/submissions', icon: Tray },
    { label: 'Event submissions', href: '/admin/event-submissions', icon: CalendarBlank },
    { label: 'Claims', href: '/admin/claims', icon: HandPalm },
    { label: 'Archived', href: '/admin/archived', icon: Archive },
    { label: 'Audit Log', href: '/admin/audit', icon: ListMagnifyingGlass },
  ];

  let path = $derived(getPath());

  function isActive(href) {
    return href === '/admin' ? path === '/admin' : path.startsWith(href);
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
      {#each sections as section (section.href)}
        {@const Icon = section.icon}
        <a
          href={section.href}
          class="workspace-tab"
          class:active={isActive(section.href)}
          onclick={(e) => handleNav(e, section.href)}
        >
          <span class="tab-icon"><Icon size={16} weight="duotone" /></span>
          {section.label}
        </a>
      {/each}
    </nav>
  </div>

  <div class="workspace-body work-content">
    {@render children()}
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
</style>
