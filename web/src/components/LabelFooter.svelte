<script>
  /**
   * The Label's shell affordance (docs/adr/023): one component, two
   * densities — never two footers.
   *
   *  - variant="overlay": the attribution strip over the quilt/map, in the
   *    register of a map's terms line — thin, translucent, bottom-right so
   *    it never fights the floating rail. Desktop only; on mobile the rail
   *    owns the bottom edge and SocialShell shows an info button instead.
   *  - variant="page": an ordinary footer at scroll end, every screen size.
   *
   * The Label parts render nothing until a Label is published — an empty
   * disclosure is not worth chrome. The legal links (docs/adr/028) render
   * regardless: a privacy policy always exists (a default ships with the
   * software) and must be findable without an account. Absent in
   * workspaces and the admin panel by construction: only SocialShell
   * mounts this.
   */
  import { navigate } from '../stores/router.svelte.js';
  import { getInstanceName } from '../stores/quilt.svelte.js';
  import { getLabel, loadLabel } from '../stores/label.svelte.js';

  let { variant = 'page' } = $props();

  let instanceName = $derived(getInstanceName());
  let label = $derived(getLabel());

  $effect(() => { loadLabel(); });

  let stewards = $derived(label?.stewards || []);
  let stewardLine = $derived.by(() => {
    if (stewards.length === 0) return '';
    const first = `@${stewards[0].username}`;
    return stewards.length === 1 ? first : `${first} +${stewards.length - 1}`;
  });

  function goLabel(e) {
    e.preventDefault();
    navigate('/label');
  }
  function goTo(e, href) {
    e.preventDefault();
    navigate(href);
  }
</script>

{#if variant === 'overlay'}
  <div class="label-strip">
    <span class="strip-name">{instanceName}</span>
    {#if label?.published}
      <span class="strip-sep">&middot;</span>
      <span>stewarded by {stewardLine}</span>
      <span class="strip-sep">&middot;</span>
      <a href="/label" onclick={goLabel}>The Label</a>
    {/if}
    <span class="strip-sep">&middot;</span>
    <a href="/privacy" onclick={(e) => goTo(e, '/privacy')}>Privacy</a>
    <span class="strip-sep">&middot;</span>
    <a href="/terms" onclick={(e) => goTo(e, '/terms')}>Terms</a>
    {#if label?.published}
      <span class="strip-sep">&middot;</span>
      <a href="/label" onclick={goLabel} class="strip-seamrip">yours to seamrip</a>
    {/if}
  </div>
{:else}
  <footer class="label-footer">
    {#if label?.published}
      <span>{instanceName} is stewarded by {stewardLine}</span>
    {:else}
      <span>{instanceName}</span>
    {/if}
    <span class="footer-links">
      {#if label?.published}
        <a href="/label" onclick={goLabel}>The Label</a>
        <span class="strip-sep">&middot;</span>
      {/if}
      <a href="/privacy" onclick={(e) => goTo(e, '/privacy')}>Privacy</a>
      <span class="strip-sep">&middot;</span>
      <a href="/terms" onclick={(e) => goTo(e, '/terms')}>Terms</a>
      {#if label?.published}
        <span class="strip-sep">&middot;</span>
        <a href="/label" onclick={goLabel}>yours to seamrip</a>
      {/if}
    </span>
  </footer>
{/if}

<style>
  /* Overlay: attribution-register, not navigation (docs/adr/023). Bottom-
     right so the floating rail keeps the left edge. */
  .label-strip {
    position: fixed;
    right: 0;
    bottom: 0;
    z-index: 50; /* under the rail (55) and bar (60) */
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 3px 10px;
    font-size: 0.7rem;
    color: var(--color-text);
    background: var(--color-glass);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border-top-left-radius: 6px;
    border-top: 1px solid var(--color-border);
    border-left: 1px solid var(--color-border);
    opacity: 0.85;
  }
  .label-strip:hover {
    opacity: 1;
  }
  .label-strip a {
    color: var(--color-text);
    text-decoration: underline;
    text-underline-offset: 2px;
  }
  .strip-name {
    font-weight: 600;
  }
  .strip-sep {
    opacity: 0.5;
  }

  /* The strip is desktop-only: on mobile the rail owns the bottom edge
     and the info button (SocialShell) stands in. Deliberate — do not
     "fix" this back into a stacked strip (docs/adr/023). */
  @media (max-width: 768px) {
    .label-strip {
      display: none;
    }
  }

  /* Page density: an ordinary footer at scroll end. */
  .label-footer {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin-top: 48px;
    padding: 16px 0 8px;
    border-top: 1px solid var(--color-border);
    font-size: 0.8rem;
    color: var(--color-text);
    opacity: 0.75;
  }
  .label-footer a {
    color: var(--color-text);
  }
  .footer-links {
    display: flex;
    gap: 6px;
    align-items: center;
  }
</style>
