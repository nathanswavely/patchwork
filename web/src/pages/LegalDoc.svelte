<script>
  /**
   * A legal document — the privacy policy or user agreement (docs/adr/028).
   * Public and readable logged out, like the Label: the person weighing
   * whether to join has no account yet.
   *
   * The server always has something to serve — a default ships with the
   * software and the admin can replace it — so this page never has an
   * empty state, only a loading and an error one.
   */
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { getInstanceName } from '../stores/quilt.svelte.js';
  import MarkdownRenderer from '../components/MarkdownRenderer.svelte';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';

  let { doc } = $props(); // 'privacy' | 'terms'

  let instanceName = $derived(getInstanceName());
  let loading = $state(true);
  let error = $state('');
  let data = $state(null);

  // Rerenders on doc change: /privacy and /terms share this component.
  $effect(() => {
    const which = doc;
    loading = true;
    error = '';
    api(`legal/${which}`)
      .then((d) => { data = d; })
      .catch((e) => { error = e.message || 'Failed to load'; })
      .finally(() => { loading = false; });
  });

  let updatedDate = $derived.by(() => {
    if (!data?.updated_at) return '';
    return data.updated_at.slice(0, 10);
  });

  let other = $derived(doc === 'privacy'
    ? { href: '/terms', label: 'User Agreement' }
    : { href: '/privacy', label: 'Privacy Policy' });

  function go(e, href) {
    e.preventDefault();
    navigate(href);
  }
</script>

<svelte:head>
  <title>{data?.title || 'Legal'} &mdash; {instanceName}</title>
</svelte:head>

<div class="legal-page page-fade">
  {#if loading}
    <Skeleton lines={10} />
  {:else if error}
    <ErrorState message={error} />
  {:else}
    <header class="legal-header">
      <p class="legal-kicker">The fine print</p>
      <h1>{data.title}</h1>
      {#if data.customized && updatedDate}
        <p class="legal-sub muted">Last updated {updatedDate}.</p>
      {/if}
    </header>

    <section class="legal-prose">
      <MarkdownRenderer content={data.markdown} />
    </section>

    <footer class="legal-cross muted">
      Also see the
      <a href={other.href} onclick={(e) => go(e, other.href)}>{other.label}</a>.
      <a href="/label" onclick={(e) => go(e, '/label')}>The Label</a> says
      who runs {instanceName} and what it costs to keep up.
    </footer>
  {/if}
</div>

<style>
  .legal-page {
    max-width: 640px;
    margin: 0 auto;
    padding: 32px 20px 64px;
  }

  .legal-header {
    margin-bottom: 20px;
  }
  .legal-kicker {
    font-size: 0.75rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--color-primary);
    margin: 0 0 4px;
  }
  .legal-header h1 {
    margin: 0 0 4px;
    font-size: 1.6rem;
  }
  .legal-sub {
    margin: 0;
    font-size: 0.9rem;
  }

  .legal-prose {
    margin-bottom: 28px;
  }

  .legal-cross {
    border-top: 1px dashed var(--color-border);
    padding-top: 16px;
    font-size: 0.85rem;
    line-height: 1.6;
  }
</style>
