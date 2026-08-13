<script>
  /**
   * The About page (CONTEXT.md "About page", docs/adr/040): public
   * orientation at /about — what this quilt is and how a Patchwork works.
   * Orientation, not disclosure: it hands off to the Label (via LabelGist)
   * for who runs the quilt and what it costs, rather than restating it.
   *
   * Reached from the "What is Patchwork?" affordance in the global bar and
   * the intro card, so its most likely first reader is anonymous.
   */
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn, isAuthChecked } from '../stores/auth.svelte.js';
  import {
    getInstanceName, getInstanceDescription, getInstanceIconUrl,
    getInstanceStats, isInstanceLoaded,
  } from '../stores/quilt.svelte.js';
  import QuiltCanvas from '../components/QuiltCanvas.svelte';
  import LabelGist from '../components/LabelGist.svelte';

  // The canonical project repo (README.md's clone instructions point at
  // "<this-repo>" rather than a fixed URL; the go.mod module path is the
  // closest thing to a canonical address, so that's the fallback).
  const REPO_URL = 'https://github.com/patchwork-toolkit/patchwork';

  let instanceName = $derived(getInstanceName());
  let instanceDescription = $derived(getInstanceDescription());
  let statsLoaded = $derived(isInstanceLoaded());
  let hasPatches = $derived((getInstanceStats().node_count || 0) > 0);

  function goLogin(e) {
    e.preventDefault();
    navigate('/login');
  }
  function goLining(e) {
    e.preventDefault();
    navigate('/lining');
  }
</script>

<svelte:head>
  <title>About &mdash; {instanceName}</title>
</svelte:head>

<div class="about-page page-fade">
  <!-- Hero: a live miniature of this instance's real quilt (docs/adr/040)
       — no fake imagery standing in for identity. Falls back to the
       instance's own icon when there's nothing to place yet. -->
  <div class="about-hero">
    {#if statsLoaded && hasPatches}
      <div class="hero-quilt">
        <QuiltCanvas interactive={false} showLabels={false} />
      </div>
    {:else if statsLoaded}
      <div class="hero-quilt hero-fallback">
        {#if getInstanceIconUrl()}
          <img src={getInstanceIconUrl()} alt="" width="64" height="64" />
        {:else}
          <svg width="64" height="64" viewBox="0 0 24 24" fill="none">
            <rect x="2" y="2" width="9" height="9" rx="1" stroke="currentColor" stroke-width="1.5"/>
            <rect x="13" y="2" width="9" height="9" rx="1" stroke="currentColor" stroke-width="1.5"/>
            <rect x="2" y="13" width="9" height="9" rx="1" stroke="currentColor" stroke-width="1.5"/>
            <rect x="13" y="13" width="9" height="9" rx="1" stroke="currentColor" stroke-width="1.5"/>
          </svg>
        {/if}
      </div>
    {:else}
      <div class="hero-quilt hero-loading"></div>
    {/if}
    <h1>What is {instanceName}?</h1>
  </div>

  <section class="about-section">
    <h2>What this is</h2>
    <p>
      <strong>{instanceName}</strong> is a Patchwork: a home the
      communities here run for themselves, on their own machine.
      {#if instanceDescription}{instanceDescription}{/if}
    </p>
  </section>

  <section class="about-section">
    <h2>How it works</h2>
    <p>
      Every group or entity here is represented by patch. Patches are equals: none is a subcategory of another, and none owns another.
    </p>
    <p>
      You can follow any public patch with one click. Joining is for people who want to participate: members vote on proposals, appear on the member list, and help run the patch.
    </p>
    <p>
      The quilt places patches near the other patches they share people with. When two tiles sit close, it's because real people move between those groups.
    </p>
    <p>
      Events come from the patches you follow or join. Your feed is built
      from those connections and nothing else.
    </p>
  </section>

  <section class="about-section">
    <h2>What makes it different</h2>
    <ul>
      <li>No ads, and nothing here is for sale.</li>
      <li>No algorithm decides what you see. The quilt and your feed follow the connections you actually have.</li>
      <li>Run by named people in this community, with the costs stated in the open.</li>
      <li>
        Every patch starts from <a href="/lining" onclick={goLining}>the lining</a>,
        a shared community-standards baseline. Amendments to it are public.
      </li>
      <li>
        "Leaving" is totally fine and affordances for it are built in. It's called a "Seamrip." A community can export its data and stand up again under new stewards The
        <a href="/label">Label</a> says where the door is.
      </li>
    </ul>
  </section>

  <LabelGist />

  {#if isAuthChecked() && !isLoggedIn()}
    <section class="about-join">
      <a href="/login" class="btn btn-primary" onclick={goLogin}>Join {instanceName}</a>
    </section>
  {/if}

  <footer class="about-footer">
    <a href={REPO_URL} target="_blank" rel="noopener">
      Patchwork is open source. Start your own quilt &rarr;
    </a>
  </footer>
</div>

<style>
  .about-page {
    max-width: var(--pw-measure);
    margin: 0 auto;
    padding: 32px 0 64px;
  }

  .about-hero {
    text-align: center;
    margin-bottom: 32px;
  }

  .hero-quilt {
    position: relative;
    height: 200px;
    border-radius: var(--radius);
    overflow: hidden;
    background: var(--color-bg);
    margin-bottom: 16px;
  }

  .hero-fallback,
  .hero-loading {
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--color-primary);
  }

  .hero-fallback img {
    object-fit: cover;
  }

  .about-hero h1 {
    margin: 0;
    font-size: 1.5rem;
  }

  .about-section {
    margin-bottom: 28px;
  }

  .about-section h2 {
    font-size: 1.05rem;
    margin: 0 0 10px;
  }

  .about-section p {
    margin: 0 0 10px;
    line-height: 1.6;
  }

  .about-section ul {
    margin: 0;
    padding-left: 1.25rem;
    line-height: 1.6;
  }

  .about-section li {
    margin-bottom: 8px;
  }

  .about-section a {
    color: var(--color-primary);
  }

  .about-join {
    margin: 28px 0;
    text-align: center;
  }

  .about-footer {
    margin-top: 40px;
    padding-top: 16px;
    border-top: 1px solid var(--color-border);
    font-size: 0.85rem;
    text-align: center;
  }

  .about-footer a {
    color: var(--color-text-muted);
  }
</style>
