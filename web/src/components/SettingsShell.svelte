<script>
  /**
   * A workspace's sidebar of sections beside the section's page.
   *
   * On a narrow screen a shell given `indexHref` drills down rather than
   * stacking: its index URL is the list of sections, full width, and a
   * section's page drops the list for one back link to it. One level of
   * navigation is on screen at a time. A shell without `indexHref` keeps
   * the scrolling row of sections it has always had there. `parent` is
   * where the index itself goes back to.
   */
  import { navigate, getPath } from '../stores/router.svelte.js';
  import { CaretLeft, CaretRight } from 'phosphor-svelte';

  let {
    title = 'Settings',
    sections = [],
    indexHref = null,
    parent = null,
    children,
  } = $props();

  let currentPath = $derived(getPath());
  let atIndex = $derived(
    !!indexHref && (currentPath === indexHref || currentPath === indexHref + '/')
  );

  function isActive(href) {
    if (currentPath === href) return true;
    // For prefix matching, only match if no other section is a more specific match
    if (currentPath.startsWith(href + '/')) {
      return !sections.some(s => s.href !== href && currentPath.startsWith(s.href) && s.href.length > href.length);
    }
    return false;
  }

  function handleClick(e, href) {
    e.preventDefault();
    navigate(href);
  }
</script>

<div class="settings-shell" class:drill={!!indexHref} class:at-index={atIndex}>
  {#if indexHref}
    {@const back = atIndex ? parent : { href: indexHref, label: title }}
    {#if back}
      <a href={back.href} class="settings-back" onclick={(e) => handleClick(e, back.href)}>
        <CaretLeft size={14} weight="bold" />
        {back.label}
      </a>
    {/if}
  {/if}
  <nav class="settings-sidebar">
    <h2 class="settings-title">{title}</h2>
    <ul class="settings-nav">
      {#each sections as section}
        <li>
          <a
            href={section.href}
            class="settings-nav-link"
            class:active={isActive(section.href)}
            onclick={(e) => handleClick(e, section.href)}
          >
            {section.label}
            {#if section.count > 0}
              <span class="settings-nav-count">{section.count}</span>
            {/if}
            {#if indexHref}
              <span class="settings-nav-caret"><CaretRight size={14} weight="bold" /></span>
            {/if}
          </a>
        </li>
      {/each}
    </ul>
  </nav>
  {#if !atIndex}
    <div class="settings-content">
      {@render children()}
    </div>
  {/if}
</div>

<style>
  .settings-shell {
    display: flex;
    gap: 2rem;
    min-height: 400px;
  }

  .settings-sidebar {
    flex-shrink: 0;
    width: 200px;
    padding-top: 0.5rem;
  }

  .settings-title {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-text-muted);
    padding: 0 0.5rem;
    margin-bottom: 0.5rem;
  }

  .settings-nav {
    list-style: none;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .settings-nav-link {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.45rem 0.6rem;
    font-size: 0.85rem;
    color: var(--color-text-muted);
    text-decoration: none;
    border-radius: 4px;
    transition: background 100ms ease, color 100ms ease;
  }

  .settings-nav-link:hover {
    background: var(--color-overlay);
    color: var(--color-text);
    text-decoration: none;
  }

  .settings-nav-link.active {
    background: var(--color-overlay);
    color: var(--color-text);
    font-weight: 500;
  }

  /* How many items a section is holding for the reader. Only the admin
     panel's Review sidebar passes one; a section without a count renders
     exactly as before. */
  .settings-nav-count {
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

  .settings-content {
    flex: 1;
    min-width: 0;
  }

  /* The shell owns the rhythm between a settings page's cards. No page
     defines it (each renders a bare section.card stack, usually inside its
     own .page-fade wrapper), so without this the cards butt together on
     every settings surface. Adjacent-sibling so it works at whatever depth
     a page stacks its cards, and only between them — never above the
     first. :global because the cards come from the consuming page's
     snippet. */
  .settings-content :global(.card + .card) {
    margin-top: 1.25rem;
  }

  @media (max-width: 640px) {
    .settings-shell {
      flex-direction: column;
      gap: 0;
    }

    .settings-sidebar {
      width: 100%;
      padding-bottom: 1rem;
      border-bottom: 1px solid var(--color-border);
      margin-bottom: 1rem;
    }

    .settings-nav {
      flex-direction: row;
      gap: 0;
      overflow-x: auto;
      -webkit-overflow-scrolling: touch;
    }

    .settings-nav-link {
      white-space: nowrap;
    }

    /* Drill-down. A section's page shows the back link and no list. */
    .drill .settings-back {
      display: inline-flex;
    }

    .drill:not(.at-index) .settings-sidebar {
      display: none;
    }

    /* The index is the list, as a page of its own. */
    .drill.at-index .settings-sidebar {
      padding-top: 0;
      border-bottom: none;
    }

    .drill.at-index .settings-title {
      font-size: 1.6rem;
      text-transform: none;
      letter-spacing: normal;
      color: var(--color-text);
      padding: 0;
      margin-bottom: 1rem;
    }

    .drill .settings-nav {
      flex-direction: column;
      overflow-x: visible;
      border-top: 1px solid var(--color-border);
    }

    .drill .settings-nav-link {
      min-height: 52px;
      padding: 0.75rem 0.25rem;
      font-size: 1rem;
      color: var(--color-text);
      border-radius: 0;
      border-bottom: 1px solid var(--color-border);
    }

    .drill .settings-nav-link.active {
      background: none;
      font-weight: 400;
    }

    .drill .settings-nav-count {
      margin-left: auto;
    }

    .drill .settings-nav-caret {
      display: flex;
    }
  }

  /* Only a drill-down shell on a narrow screen has these. */
  .settings-back {
    display: none;
    align-items: center;
    gap: 0.35rem;
    min-height: 44px;
    margin-bottom: 0.5rem;
    font-size: 0.9rem;
    color: var(--color-text-muted);
    text-decoration: none;
  }

  .settings-back:hover {
    color: var(--color-text);
    text-decoration: none;
  }

  .settings-nav-caret {
    display: none;
    color: var(--color-text-muted);
  }
</style>
