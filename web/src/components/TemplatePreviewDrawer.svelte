<script>
  import { api } from '../lib/api.js';
  import { docLabel } from '../lib/docLabel.js';
  import MarkdownRenderer from './MarkdownRenderer.svelte';
  import Skeleton from './Skeleton.svelte';
  import { ltDialog } from '../lib/lt.js';

  let { templateId = '', onClose = () => {} } = $props();

  let data = $state(null);
  let loading = $state(true);
  let expandedDoc = $state(0); // first doc open by default

  $effect(() => {
    if (templateId) loadTemplate();
  });

  async function loadTemplate() {
    loading = true;
    try {
      data = await api(`templates/${templateId}`);
    } catch {
      data = null;
    } finally {
      loading = false;
    }
  }

  function toggleDoc(i) {
    expandedDoc = expandedDoc === i ? -1 : i;
  }

  // Human-readable rule explanations, in the order a founder asks them: who
  // runs the patch, then who decides, and only then the arithmetic of a vote.
  // Where the admin decides, there is no vote to do arithmetic on: Minimal's
  // preview led with "Quorum 0%, any number of votes counts" under "Admin
  // decides", and two founders stopped on it (F-032). Quorum and the voting
  // window are a voting patch's rows, and an advisory vote the maintainer
  // asks for is described by the decision row itself.
  function explainRules(rules) {
    if (!rules) return [];
    const explanations = [];

    const models = {
      maintainer: 'The founder runs this patch and can make other members admins.',
      meritocratic: 'Admins earn their role through sustained contribution. An admin nominates a member, and the members confirm it by vote.',
      elected: 'The members elect admins for fixed terms. Power rotates through regular elections.',
    };
    if (rules.leadership_model) {
      explanations.push({ label: 'Leadership', value: rules.leadership_model, explain: models[rules.leadership_model] || '' });
    }

    const methods = {
      admin: 'The maintainer makes all decisions, and may ask the members first.',
      majority: 'A proposal passes with more approvals than rejections.',
      supermajority: 'A proposal passes when at least two thirds of the votes for or against are for.',
      consensus: 'One reject blocks a proposal. A proposal nobody rejects carries.',
    };
    if (rules.decision_method) {
      explanations.push({ label: 'Decision method', value: rules.decision_method, explain: methods[rules.decision_method] || '' });
    }

    const votes = rules.decision_method && rules.decision_method !== 'admin';

    if (votes && rules.quorum_percent !== undefined) {
      const q = rules.quorum_percent;
      explanations.push({
        label: 'Quorum',
        value: q + '%',
        explain: q === 0 ? 'No minimum participation. Any number of votes counts.' : `At least ${q}% of members must vote for a decision to count.`,
      });
    }

    if (votes && rules.default_vote_duration_hours) {
      const h = rules.default_vote_duration_hours;
      const days = Math.round(h / 24);
      explanations.push({
        label: 'Voting period',
        value: days <= 1 ? `${h} hours` : `${days} days`,
        explain: `Proposals stay open for ${days <= 1 ? h + ' hours' : days + ' days'} before closing.`,
      });
    }

    if (rules.admin_term_months > 0) {
      explanations.push({ label: 'Term length', value: rules.admin_term_months + ' months', explain: `Admin terms last ${rules.admin_term_months} months before re-election.` });
    }

    // max_admins is deliberately absent. docs/adr/051 retired it as a rule
    // and retracted docs/adr/049's promotion guard, so the number in the
    // rules file caps nothing: promotion past it succeeds, and
    // internal/handler/admin_seats_test.go holds that open. This drawer is
    // the one place a founder reads before choosing a template, and
    // docs/adr/049's principle is that Patchwork states as fact only what
    // Patchwork enforces. Three simulated founders read "Up to N people can
    // serve as admins simultaneously" as a cap; one took the Minimal
    // template believing a second admin was impossible. How many admins a
    // patch has follows from how it governs, which the Leadership row above
    // already says.

    return explanations;
  }

  let rules = $derived(data?.rules ? (typeof data.rules === 'string' ? JSON.parse(data.rules) : data.rules) : null);
  let ruleExplanations = $derived(explainRules(rules));
</script>

<!-- Liquid Textile's drawer on a native <dialog> (ltDialog in lib/lt.js):
     open while mounted, closed by Escape, the backdrop or the close
     button, which hand focus back to the Preview link. -->
<dialog
  class="lt-drawer template-drawer"
  aria-label="{data?.template?.name || 'Template'} Template"
  use:ltDialog={{ open: true, onClose }}
>
    <div class="lt-drawer-head drawer-header">
      <h2>{data?.template?.name || 'Template'} Template</h2>
      <button class="lt-dialog-close close-btn" data-close aria-label="Close">&times;</button>
    </div>

    <div class="lt-drawer-body drawer-body">
      {#if loading}
        <Skeleton lines={6} height="1rem" />
      {:else if !data}
        <p class="lt-muted">Failed to load template.</p>
      {:else}
        <!-- Description -->
        <p class="template-desc">{data.template.description}</p>
        <p class="template-meta lt-muted">{data.template.leadership} &middot; {data.template.best_for}</p>
        {#if data.template.elsewhere}
          <p class="template-meta lt-muted">{data.template.elsewhere} Where admins are chosen and where proposals are decided are both in the patch's governance rules.</p>
        {/if}

        <!-- Rules with explanations -->
        {#if ruleExplanations.length > 0}
          <section class="section">
            <h3>How it works</h3>
            <div class="rules-list">
              {#each ruleExplanations as rule}
                <div class="rule-item">
                  <div class="rule-header">
                    <span class="rule-label">{rule.label}</span>
                    <span class="rule-value">{rule.value}</span>
                  </div>
                  {#if rule.explain}
                    <p class="rule-explain">{rule.explain}</p>
                  {/if}
                </div>
              {/each}
            </div>
          </section>
        {/if}

        <!-- Documents -->
        {#if data.documents?.length > 0}
          <section class="section">
            <h3>Documents this patch starts with ({data.documents.length})</h3>
            {#if data.documents.length > 1}
              <p class="docs-note lt-muted">The community standards are public. The others start visible to members only, until an admin publishes them.</p>
            {/if}
            <div class="doc-list">
              {#each data.documents as doc, i}
                <div class="doc-item">
                  <button class="doc-toggle" onclick={() => toggleDoc(i)}>
                    <span class="doc-chevron" class:open={expandedDoc === i}>&rsaquo;</span>
                    <span class="doc-name">{doc.title || docLabel(doc.filename)}</span>
                  </button>
                  {#if expandedDoc === i}
                    <div class="doc-content">
                      <MarkdownRenderer content={doc.content} />
                    </div>
                  {/if}
                </div>
              {/each}
            </div>
          </section>
        {/if}
      {/if}
    </div>
</dialog>

<style>
  .drawer-header h2 {
    font-size: 1.1rem;
    margin: 0;
  }

  /* The × reads small at Liquid Textile's close size. */
  .close-btn {
    font-size: 1.5rem;
  }

  .template-desc {
    font-size: 0.92rem;
    line-height: 1.5;
    margin-bottom: 0.25rem;
  }

  .template-meta {
    font-size: 0.8rem;
    margin-bottom: 1.25rem;
  }

  .section {
    margin-bottom: 1.5rem;
  }

  .docs-note {
    font-size: 0.8rem;
    margin: -0.4rem 0 0.75rem;
  }

  .section h3 {
    font-size: 0.82rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-muted);
    margin-bottom: 0.75rem;
  }

  .rules-list {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .rule-item {
    padding: 0.6rem 0.75rem;
    background: var(--color-bg);
    border-radius: var(--radius);
  }

  .rule-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .rule-label {
    font-size: 0.85rem;
    font-weight: 500;
  }

  .rule-value {
    font-size: 0.82rem;
    color: var(--color-primary);
    font-weight: 600;
    text-transform: capitalize;
  }

  .rule-explain {
    font-size: 0.78rem;
    color: var(--color-text-muted);
    margin-top: 0.25rem;
    line-height: 1.4;
  }

  .doc-list {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .doc-item {
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    overflow: hidden;
  }

  .doc-toggle {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.6rem 0.75rem;
    border: none;
    background: none;
    cursor: pointer;
    font-size: 0.88rem;
    font-weight: 500;
    color: var(--color-text);
    text-align: left;
    text-transform: capitalize;
  }

  .doc-toggle:hover {
    background: var(--color-overlay);
  }

  .doc-chevron {
    font-size: 1rem;
    transition: transform 150ms ease;
    color: var(--color-text-muted);
  }

  .doc-chevron.open {
    transform: rotate(90deg);
  }

  .doc-content {
    padding: 0 1rem 1rem;
    font-size: 0.85rem;
    line-height: 1.7;
    border-top: 1px solid var(--color-border);
  }
</style>
