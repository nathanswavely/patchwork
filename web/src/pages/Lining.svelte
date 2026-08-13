<script>
  /**
   * The lining (CONTEXT.md "Lining"): the shared baseline community-
   * standards charter every active patch on the quilt starts from. This
   * page shows the current shipped text — GET /api/v1/instance/lining,
   * public and readable logged out (internal/handler/lining_update.go
   * GetInstanceLining), the same reasoning as showing it before someone
   * creates a patch: adoption should never be a surprise (docs/adr/037).
   *
   * This is the shipped baseline only, not any one patch's copy — a patch
   * that amended its lining shows that on its own governance docs page,
   * with the amendment one link away (CONTEXT.md "Amended lining").
   */
  import { api } from '../lib/api.js';
  import { getInstanceName } from '../stores/quilt.svelte.js';
  import MarkdownRenderer from '../components/MarkdownRenderer.svelte';
  import Skeleton from '../components/Skeleton.svelte';

  let instanceName = $derived(getInstanceName());
  let lining = $state(null);
  let loading = $state(true);
  let error = $state('');

  $effect(() => {
    loading = true;
    error = '';
    api('instance/lining')
      .then((data) => { lining = data; })
      .catch((e) => { error = e.message; })
      .finally(() => { loading = false; });
  });
</script>

<svelte:head>
  <title>The Lining &mdash; {instanceName}</title>
</svelte:head>

<div class="lining-page page-fade">
  {#if loading}
    <Skeleton lines={10} />
  {:else if error}
    <p class="muted">Couldn't load the lining: {error}</p>
  {:else}
    <header class="lining-header">
      <h1>{lining.title}</h1>
      <p class="lining-standfirst muted">
        The shared baseline every patch on this quilt starts from. A patch can amend its copy. Amendments are public, and the patch is flagged publically if they diverge.
      </p>
    </header>
    <MarkdownRenderer content={lining.body} />
  {/if}
</div>

<style>
  /* No horizontal padding: .social-main owns the gutter (docs/adr/038). */
  .lining-page {
    max-width: var(--pw-measure);
    margin: 0 auto;
    padding: 32px 0 64px;
  }

  .lining-header {
    margin-bottom: 24px;
  }

  .lining-header h1 {
    margin: 0 0 8px;
    font-size: 1.6rem;
  }

  .lining-standfirst {
    margin: 0;
    font-size: 0.9rem;
    line-height: 1.5;
  }
</style>
