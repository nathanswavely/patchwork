<script>
  /**
   * The Label's gist (CONTEXT.md "About page", "Label"): a compact panel
   * exposing the Label inline on the About page — who stewards this quilt
   * and roughly what it costs — with a link to the full statement at
   * /label. The About page is orientation and never trades jobs with the
   * Label's disclosure; this component is that handoff, not a summary that
   * tries to stand on its own.
   *
   * Renders nothing until a Label is published — an empty disclosure is
   * not worth chrome (same rule LabelFooter follows).
   */
  import { navigate } from '../stores/router.svelte.js';
  import { getLabel, loadLabel, formatMoney } from '../stores/label.svelte.js';

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
</script>

{#if label?.published}
  <section class="label-gist">
    <h2>Who runs this</h2>
    <p class="gist-line">Stewarded by {stewardLine}</p>
    {#if label.total_monthly_minor > 0}
      <p class="gist-line">
        About {formatMoney(label.total_monthly_minor, label.currency)}/month to keep running
      </p>
    {/if}
    <a href="/label" class="gist-link" onclick={goLabel}>Read the Label &rarr;</a>
  </section>
{/if}

<style>
  .label-gist {
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
    padding: 16px 18px;
  }

  .label-gist h2 {
    margin: 0 0 8px;
    font-size: 1rem;
  }

  .gist-line {
    margin: 0 0 4px;
    font-size: 0.88rem;
    color: var(--color-text-muted);
  }

  .gist-link {
    display: inline-block;
    margin-top: 8px;
    font-size: 0.88rem;
    font-weight: 600;
    color: var(--color-primary);
    text-decoration: none;
  }

  .gist-link:hover {
    text-decoration: underline;
  }
</style>
