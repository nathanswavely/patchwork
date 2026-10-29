<script>
  /**
   * The AI page (docs/adr/2026-10-20-the-site-says-where-ai-is.md): where AI
   * is and is not in this software. Public and readable logged out, like the
   * legal documents.
   *
   * Two parts, owned by two parties. The project part is shipped text the
   * server fills in from the code it describes; nobody on an instance edits
   * it. The copy figures come from the ledger embedded in the binary. The
   * stewards' part is what this quilt's stewards wrote, and may be empty.
   *
   * Every string in this file must be recorded as a person's in the copy
   * ledger before the page ships (TestAIPageWordsAreAPersons).
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

  // Display names for internal/copystats VisitorPages, kept here so the
  // copy ledger sees them.
  const pageNames = {
    lining: 'The lining',
    charters: 'Charter templates',
    about: 'About',
    label: 'The Label',
    legal: 'Privacy and Terms',
    ai: 'This page',
  };

  let writing = $derived(data?.copy?.writing);
  let writingTotal = $derived(writing ? writing.human + writing.drafted + writing.unreviewed : 0);
  let iface = $derived(data?.copy?.interface);

  function pct(n, total) {
    if (!total) return '0%';
    return `${Math.round((n / total) * 100)}%`;
  }

  function num(n) {
    return (n || 0).toLocaleString();
  }

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

    <section class="ai-copy">
      <MarkdownRenderer content={data.copy_markdown} />

      <dl class="ai-figures">
        <div class="figure">
          <dt>Writing</dt>
          <dd>
            {num(writingTotal)} words: {pct(writing.human, writingTotal)} written by a person,
            {pct(writing.drafted, writingTotal)} drafted with AI and kept,
            {pct(writing.unreviewed, writingTotal)} not reviewed yet.
          </dd>
        </div>
        <div class="figure">
          <dt>Interface text</dt>
          <dd>{num(iface.reviewed)} of {num(iface.total)} strings checked by a person.</dd>
        </div>
      </dl>

      <div class="lt-table-wrap">
        <table class="lt-table">
          <caption class="lt-muted">Writing on the pages a visitor reads, in words.</caption>
          <thead>
            <tr>
              <th scope="col">Page</th>
              <th scope="col" class="n">Written by a person</th>
              <th scope="col" class="n">Drafted with AI, kept</th>
              <th scope="col" class="n">Not reviewed yet</th>
            </tr>
          </thead>
          <tbody>
            {#each data.copy.pages as p (p.id)}
              <tr>
                <th scope="row">{pageNames[p.id] || p.id}</th>
                <td class="n">{num(p.human)}</td>
                <td class="n">{num(p.drafted)}</td>
                <td class="n">{num(p.unreviewed)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      {#if data.copy.not_copy > 0}
        <p class="lt-muted not-copy">
          {num(data.copy.not_copy)} strings are marked as not copy, such as format strings and identifiers, and are left out of these figures.
        </p>
      {/if}
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
  .ai-copy,
  .ai-stewards {
    margin-bottom: 28px;
  }

  .ai-figures {
    display: grid;
    gap: 10px;
    margin: 16px 0;
  }
  .figure dt {
    font-weight: 700;
  }
  .figure dd {
    margin: 2px 0 0;
  }

  .lt-table caption {
    caption-side: top;
    text-align: left;
    font-size: 0.85rem;
    padding-bottom: 6px;
  }
  .n {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }

  .not-copy {
    font-size: 0.85rem;
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
