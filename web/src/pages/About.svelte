<script>
  /**
   * The About page (CONTEXT.md "About page", docs/adr/040): public
   * orientation at /about — what this quilt is and how a Patchwork works.
   * Orientation, not disclosure: it hands off to the Label (via LabelGist)
   * for who runs the quilt and what it costs, rather than restating it.
   *
   * Reached from the "What is Patchwork?" affordance in the global bar and
   * the intro card, so its most likely first reader is anonymous.
   *
   * The prose here ships in the binary and is the same on every quilt, so
   * it speaks for the project rather than for any one instance; the only
   * instance-authored text on the page is the description slot under "What
   * is this?" and the Label gist. Two consequences: nothing may promise
   * what a given instance does (a sweep, a review process), and nothing may
   * assert what the platform does not enforce (docs/adr/049).
   *
   * "Where do patches come from?" exists because no other surface carries a
   * listing's provenance: a visitor meeting an unclaimed patch cold reads
   * the quilt as a roster of groups that opted in, and concludes the quilt
   * vouches for every tile on it. The claims it makes are load-bearing —
   * unclaimed patches carry no lining and no members (docs/adr/039),
   * claiming adopts the lining at setup (docs/adr/030), dissent brands
   * rather than bans (docs/adr/037), and export is whole-instance and
   * admin-only behind a step-up gate (docs/adr/017).
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
  function goGovernance(e) {
    e.preventDefault();
    navigate('/governance');
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
    <h2>What is this?</h2>
    <p>
      A Patchwork is a hyper-local-by-design platform built to enable
      discovery, connection, and community building without relying on
      billionaire owned, algorithm powered, attention-economy-buttressed
      apps and social media sites. Patchwork is an open source codebase
      that can be hosted by anyone. No ads and no monetization strategy.
    </p>
    <!-- The instance's own words, and the only place on this page a quilt
         speaks for itself. Attributed rather than blended into the prose
         around it: the paragraph above is the project's claim, this is the
         instance's, and a reader should be able to tell which is which.
         Renders nothing when unset. -->
    {#if instanceDescription}
      <p class="instance-intro">This instance describes itself as:</p>
      <p class="instance-desc">{instanceDescription}</p>
    {/if}
  </section>

  <section class="about-section">
    <h2>How does it work?</h2>
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
      <li>Follow a patch to see events and public notifications from it in your own personal quilt.</li>
      <li>Join a patch as a member to be involved in non-public activities.</li>
      <li>Create or manage a patch to update members, document policies, and even run internal elections and governance.</li>
    </ul>
    <p>
      Patches are arranged on the quilt near other patches that share
      members, followers, and events. Patchwork also has a robust
      governance system that allows patches to elect leaders, vote on
      policies, and attest to shared values. If you're interested in
      transparent, open-source governance through Patchwork,
      <a href="/governance" onclick={goGovernance}>read more here</a>.
    </p>
  </section>

  <section class="about-section">
    <h2>Where do patches come from?</h2>
    <p>
      Patches are created either by their owner or as "unclaimed" by an
      admin or a community suggestion. Unclaimed patches can be acquired
      and verified by their owners. However, any claimed patch must make a
      choice: adopt <a href="/lining" onclick={goLining}>the lining</a>,
      or adopt and then publicly dissent. The lining is the "baseline"
      commitment that Patchwork expects, and any dissent or modifications
      thereof will not result in a ban, but instead a brand. Patchwork was
      designed to be a community building tool, and therefore has
      intentional biases towards fostering community according to values
      held by the contributors. Neighbors with differing values are
      welcome to use the tools, as they are welcome to participate in
      society and use shared public infrastructure, but the admin(s) will
      take steps to enforce rules or remove individuals and entities if
      significant breach occurs.
    </p>
    <p>
      Patch presence on this quilt does not indicate an endorsement by this
      Patchwork or the people who run it. It only means the person, place,
      or group exists and that somebody recorded it, like a modern phone
      book. Nothing reaches your personal quilt except the patches you
      choose to follow, and anything that has no business being here at all
      can be reported to the admins.
    </p>
  </section>

  <section class="about-section">
    <h2>What makes Patchwork different</h2>
    <ul>
      <li>No ads, and no data collection beyond what the platform needs to function.</li>
      <li>No algorithm that decides what you should see.</li>
      <li>Run by community members.</li>
      <li>
        Creating a patch requires adopting
        <a href="/lining" onclick={goLining}>the lining</a>, and
        amendments to it for your community are public.
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

  /* The instance speaking in its own voice, marked as a quotation so it
     never reads as more of the project's prose. Both selectors carry the
     section class so they outrank `.about-section p`'s margins rather than
     fighting them with !important. */
  .about-section .instance-intro {
    margin-bottom: 6px;
    color: var(--color-text-muted);
  }

  .about-section .instance-desc {
    margin: 0;
    padding-left: 12px;
    border-left: 3px solid var(--color-border);
    line-height: 1.6;
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
