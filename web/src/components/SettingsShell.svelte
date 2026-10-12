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
  import { getContext } from 'svelte';
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

  let back = $derived(
    indexHref ? (atIndex ? parent : { href: indexHref, label: title }) : null
  );

  // Inside a patch workspace the back link belongs on the workspace's own
  // row, beside the relationship cluster (PatchShell). Where there is no such
  // row (the admin panel) the shell draws it itself.
  const workspaceUp = getContext('workspaceUp');

  $effect(() => {
    if (!workspaceUp) return;
    workspaceUp.set(back);
    return () => workspaceUp.set(null);
  });

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

<!-- Liquid Textile's side navigation, the current section marked
     aria-current. A shell given indexHref is Liquid Textile's split: on a
     narrow screen it shows one level at a time, by data-at. -->
<div
  class="settings-shell"
  class:lt-split={!!indexHref}
  data-at={indexHref ? (atIndex ? 'index' : 'detail') : undefined}
>
  {#if indexHref && !workspaceUp}
    {#if back}
      <a href={back.href} class="lt-split-back settings-back" onclick={(e) => handleClick(e, back.href)}>
        <CaretLeft size={14} weight="bold" />
        {back.label}
      </a>
    {/if}
  {/if}
  <nav class="settings-sidebar" class:lt-split-nav={!!indexHref}>
    <h2 class="settings-title">{title}</h2>
    <ul class="lt-sidenav settings-nav">
      {#each sections as section}
        <li>
          <a
            href={section.href}
            class="lt-sidenav-link settings-nav-link"
            aria-current={isActive(section.href) ? 'page' : undefined}
            onclick={(e) => handleClick(e, section.href)}
          >
            {section.label}
            {#if section.count > 0}
              <span class="lt-count settings-nav-count">{section.count}</span>
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
    min-height: 400px;
  }

  /* A shell that does not drill: the list beside the page, at the split's
     measure so every settings surface lines up. */
  .settings-shell:not(.lt-split) {
    display: flex;
    gap: 2rem;
  }

  .settings-shell:not(.lt-split) .settings-sidebar {
    flex: 0 0 13rem;
  }

  .settings-sidebar {
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

  /* How many items a section is holding for the reader. Only the admin
     panel's Review sidebar passes one; a section without a count renders
     exactly as before. */

  .settings-content {
    flex: 1;
    min-width: 0;
  }

  /* The shell owns the rhythm between a settings page's cards. No page
     defines it (each renders a bare section.lt-card stack, usually inside its
     own .lt-fade-in wrapper), so without this the cards butt together on
     every settings surface. Adjacent-sibling so it works at whatever depth
     a page stacks its cards, and only between them — never above the
     first. :global because the cards come from the consuming page's
     snippet. */
  .settings-content :global(.lt-card + .lt-card) {
    margin-top: 1.25rem;
  }

  @media (max-width: 640px) {
    /* A shell that does not drill keeps its scrolling row of sections. */
    .settings-shell:not(.lt-split) {
      flex-direction: column;
      gap: 0;
    }

    .settings-shell:not(.lt-split) .settings-sidebar {
      flex: none;
      padding-bottom: 1rem;
      border-bottom: 1px solid var(--color-border);
      margin-bottom: 1rem;
    }

    .settings-shell:not(.lt-split) .settings-nav {
      flex-direction: row;
      gap: 0;
      overflow-x: auto;
      -webkit-overflow-scrolling: touch;
    }

    .settings-shell:not(.lt-split) .settings-nav-link {
      white-space: nowrap;
    }

    /* Drill-down. The split hides the list on a section's page and shows
       the back link there; the index, the list as a page of its own, goes
       back to the shell's parent as well. */
    .lt-split[data-at="index"] .settings-back {
      display: inline-flex;
    }

    .lt-split .settings-sidebar {
      padding-top: 0;
    }

    .lt-split .settings-title {
      font-size: 1.6rem;
      text-transform: none;
      letter-spacing: normal;
      color: var(--color-text);
      padding: 0;
      margin-bottom: 1rem;
    }

    .lt-split .settings-nav-count {
      margin-left: auto;
    }

    .lt-split .settings-nav-caret {
      display: flex;
    }
  }

  /* Only a drill-down shell on a narrow screen has these. */
  .settings-nav-caret {
    display: none;
    color: var(--color-text-muted);
  }
</style>
