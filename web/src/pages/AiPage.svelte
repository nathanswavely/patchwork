<script>
  /**
   * The AI page (docs/adr/2026-10-20-the-site-says-where-ai-is.md): where AI
   * is and is not in this software. Public and readable logged out, like the
   * legal documents.
   *
   * Two parts, owned by two parties. The project part is shipped text the
   * server fills in from the code it describes; nobody on an instance edits
   * it. The stewards' part is what this quilt's stewards wrote, and may be
   * empty.
   *
   * Every string in this file must be decided by a person in the copy
   * ledger before the page ships (TestAIPageWordsAreDecided).
   */
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { getInstanceName } from '../stores/quilt.svelte.js';
  import MarkdownRenderer from '../components/MarkdownRenderer.svelte';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';

  let instanceName = $derived(getInstanceName());
  let loading = $state(true);
  let error = $state('');
  let data = $state(null);

  $effect(() => {
    api('ai')
      .then((d) => { data = d; })
      .catch((e) => { error = e.message || 'Failed to load'; })
      .finally(() => { loading = false; });
  });

  function go(e, href) {
    e.preventDefault();
    navigate(href);
  }
</script>

<svelte:head>
  <title>Where AI is &mdash; {instanceName}</title>
</svelte:head>

<div class="ai-page lt-fade-in">
  {#if loading}
    <Skeleton lines={10} />
  {:else if error}
    <ErrorState message={error} />
  {:else}
    <header class="ai-header">
      <p class="ai-kicker">The fine print</p>
      <h1>Where AI is</h1>
    </header>

    <section class="ai-prose">
      <MarkdownRenderer content={data.project_markdown} />
    </section>

    <section class="ai-stewards">
      <h2>From the stewards of {instanceName}</h2>
      {#if data.stewards_markdown}
        <MarkdownRenderer content={data.stewards_markdown} />
      {:else}
        <p class="lt-muted">The stewards of {instanceName} have not added anything here yet.</p>
      {/if}
    </section>

    <footer class="ai-cross lt-muted">
      Also see the
      <a href="/privacy" onclick={(e) => go(e, '/privacy')}>Privacy Policy</a>.
      <a href="/label" onclick={(e) => go(e, '/label')}>The Label</a> says
      who runs {instanceName}.
    </footer>
  {/if}
</div>

<style>
  /* No horizontal padding: .social-main owns the gutter (docs/adr/038). */
  .ai-page {
    max-width: var(--pw-measure);
    margin: 0 auto;
    padding: 32px 0 64px;
  }

  .ai-header {
    margin-bottom: 20px;
  }
  .ai-kicker {
    font-size: 0.75rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--color-primary);
    margin: 0 0 4px;
  }
  .ai-header h1 {
    margin: 0;
    font-size: 1.6rem;
  }

  .ai-prose,
  .ai-stewards {
    margin-bottom: 28px;
  }

  .ai-stewards h2 {
    font-size: 1.15rem;
    margin: 0 0 8px;
  }

  .ai-cross {
    border-top: 1px dashed var(--color-border);
    padding-top: 16px;
    font-size: 0.85rem;
    line-height: 1.6;
  }
</style>
