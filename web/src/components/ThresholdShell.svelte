<script>
  /**
   * Threshold shell (docs/adr/040): the minimal shell for the four
   * standalone auth/onboarding routes (/welcome, /login, /invite/:token,
   * /signup/complete). These are the doorway before a person has an
   * account, or the last step of getting one — not a workspace, not
   * discovery. The bar carries only identity and an exit: the quilt mark
   * and the instance name, both leading home. No search, no notification
   * bell, no user menu, no Log In link — those belong to the global bar
   * (docs/adr/005), which these flows deliberately don't have.
   *
   * The mark/name pairing reuses ContextCrumb exactly as the workspace and
   * admin shells do, with `href="/"` so both halves lead to the same exit
   * (there is no "context root" here distinct from home).
   */
  import ContextCrumb from './ContextCrumb.svelte';
  import { getInstanceName } from '../stores/quilt.svelte.js';

  let { children } = $props();
</script>

<div class="threshold-shell">
  <header class="threshold-bar">
    <ContextCrumb label={getInstanceName()} href="/" />
  </header>

  <main class="threshold-main">
    {@render children()}
  </main>
</div>

<style>
  .threshold-shell {
    min-height: 100vh;
  }

  /* Same geometry as the global bar (docs/adr/005): fixed, 56px, seam at
     the bottom since there is no secondary nav row to carry it here. */
  .threshold-bar {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    height: 56px;
    z-index: 60;
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 0 16px 0 8px;
    background: var(--color-surface);
    border-bottom: 1px solid var(--color-border);
  }

  @media (max-width: 768px) {
    .threshold-bar {
      padding: 0 16px 0 12px;
      background: color-mix(in srgb, var(--color-surface) 72%, transparent);
      backdrop-filter: blur(14px);
      -webkit-backdrop-filter: blur(14px);
    }
  }

  /* The gutter belongs to the shell, not the pages inside it (docs/adr/038).
     No bottom padding or forced min-height beyond clearing the bar — each
     page already owns its own vertical rhythm (padding-top, a pinned CTA,
     etc.), and doubling that here is exactly the "double-scrolling" the
     wrapping guards against. */
  .threshold-main {
    max-width: var(--pw-measure);
    margin: 0 auto;
    padding: 56px var(--pw-gutter) 0;
  }
</style>
