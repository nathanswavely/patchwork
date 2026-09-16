<script>
  /**
   * The Display menu (docs/adr/112, CONTEXT.md "Display menu"): the two
   * standing choices a reader makes about how Patchwork looks to them.
   *
   * One component, two homes. Signed in it sits in the account dropdown;
   * signed out it sits in a dropdown hung off that same slot in the bar, so
   * the control does not move when somebody joins. Both are the same rows in
   * the same order, which is the whole point of sharing the component rather
   * than writing the menu twice.
   *
   * Both preferences are per browser (theme.svelte.js, colors.svelte.js) and
   * neither is on the account — the signed-out home could not reach an
   * account setting, and a reader with no account is exactly who the second
   * row is for.
   */
  import { getThemePreference, setTheme } from '../stores/theme.svelte.js';
  import { getColorMode, setColorMode } from '../stores/colors.svelte.js';

  const THEMES = [
    { value: 'light', label: 'Light' },
    { value: 'dark', label: 'Dark' },
    { value: 'system', label: 'System' },
  ];

  // "Default" rather than "Full" because no instance setting moves the
  // default — docs/adr/112 decision 3. If that ever became configurable the
  // word would be a lie on the first fork that flipped it.
  const COLORS = [
    { value: 'default', label: 'Default' },
    { value: 'muted', label: 'Muted' },
  ];
</script>

<div class="display-menu">
  <div class="display-row">
    <span class="display-label" id="display-theme-label">Theme</span>
    <div class="segmented" role="group" aria-labelledby="display-theme-label">
      {#each THEMES as t (t.value)}
        <button
          type="button"
          class:selected={getThemePreference() === t.value}
          aria-pressed={getThemePreference() === t.value}
          onclick={() => setTheme(t.value)}
        >{t.label}</button>
      {/each}
    </div>
  </div>

  <div class="display-row">
    <span class="display-label" id="display-colors-label">Colors</span>
    <div class="segmented" role="group" aria-labelledby="display-colors-label">
      {#each COLORS as c (c.value)}
        <button
          type="button"
          class:selected={getColorMode() === c.value}
          aria-pressed={getColorMode() === c.value}
          onclick={() => setColorMode(c.value)}
        >{c.label}</button>
      {/each}
    </div>
  </div>
</div>

<style>
  .display-menu {
    padding: 0.5rem 0.75rem 0.4rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .display-row {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .display-label {
    font-size: 0.72rem;
    font-weight: 600;
    letter-spacing: 0.02em;
    text-transform: uppercase;
    color: var(--color-text-muted);
  }

  .segmented {
    display: flex;
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
  }

  /* These buttons sit inside the account dropdown, whose own rule makes
     every descendant button a full-width block row. Reassert the segment
     shape here so the two menus cannot disagree about it. */
  .segmented button {
    flex: 1;
    display: block;
    width: auto;
    padding: 0.3rem 0.4rem;
    font-size: 0.78rem;
    text-align: center;
    background: transparent;
    border: none;
    border-right: 1px solid var(--color-border);
    color: var(--color-text-muted);
    cursor: pointer;
    white-space: nowrap;
  }

  .segmented button:last-child {
    border-right: none;
  }

  .segmented button:hover {
    background: var(--color-surface-hover, rgba(127, 127, 127, 0.12));
    color: var(--color-text);
  }

  .segmented button.selected {
    background: var(--color-accent);
    color: var(--color-bg);
    font-weight: 600;
  }
</style>
