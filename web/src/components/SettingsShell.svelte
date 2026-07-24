<script>
  import { navigate, getPath } from '../stores/router.svelte.js';

  let {
    title = 'Settings',
    sections = [],
    children,
  } = $props();

  let currentPath = $derived(getPath());

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

<div class="settings-shell">
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
          </a>
        </li>
      {/each}
    </ul>
  </nav>
  <div class="settings-content">
    {@render children()}
  </div>
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
    display: block;
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

  .settings-content {
    flex: 1;
    min-width: 0;
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
  }
</style>
