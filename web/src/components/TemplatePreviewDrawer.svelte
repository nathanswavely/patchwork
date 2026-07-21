<script>
  import { api } from '../lib/api.js';
  import MarkdownRenderer from './MarkdownRenderer.svelte';
  import Skeleton from './Skeleton.svelte';

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

  // Human-readable rule explanations.
  function explainRules(rules) {
    if (!rules) return [];
    const explanations = [];

    const methods = {
      admin: 'The maintainer makes all decisions.',
      majority: 'Decisions pass when more than half of voters agree.',
      supermajority: 'Decisions require at least 2 out of 3 voters to agree.',
      consensus: 'Decisions require everyone (or nearly everyone) to agree.',
    };
    if (rules.decision_method) {
      explanations.push({ label: 'Decision method', value: rules.decision_method, explain: methods[rules.decision_method] || '' });
    }

    if (rules.quorum_percent !== undefined) {
      const q = rules.quorum_percent;
      explanations.push({
        label: 'Quorum',
        value: q + '%',
        explain: q === 0 ? 'No minimum participation \u2014 any number of votes counts.' : `At least ${q}% of members must vote for a decision to count.`,
      });
    }

    if (rules.default_vote_duration_hours) {
      const h = rules.default_vote_duration_hours;
      const days = Math.round(h / 24);
      explanations.push({
        label: 'Voting period',
        value: days <= 1 ? `${h} hours` : `${days} days`,
        explain: `Proposals stay open for ${days <= 1 ? h + ' hours' : days + ' days'} before closing.`,
      });
    }

    const models = {
      maintainer: 'One person (the founder) maintains this patch and makes day-to-day decisions.',
      meritocratic: 'Admins earn their role through sustained contribution. Existing admins nominate new ones.',
      elected: 'The community elects admins for fixed terms. Power rotates through regular elections.',
    };
    if (rules.leadership_model) {
      explanations.push({ label: 'Leadership', value: rules.leadership_model, explain: models[rules.leadership_model] || '' });
    }

    if (rules.admin_term_months > 0) {
      explanations.push({ label: 'Term length', value: rules.admin_term_months + ' months', explain: `Admin terms last ${rules.admin_term_months} months before re-election.` });
    }

    if (rules.max_admins > 0) {
      explanations.push({ label: 'Max admins', value: String(rules.max_admins), explain: `Up to ${rules.max_admins} people can serve as admins simultaneously.` });
    }

    return explanations;
  }

  let rules = $derived(data?.rules ? (typeof data.rules === 'string' ? JSON.parse(data.rules) : data.rules) : null);
  let ruleExplanations = $derived(explainRules(rules));
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="drawer-backdrop" onclick={onClose}>
  <div class="drawer" onclick={(e) => e.stopPropagation()}>
    <div class="drawer-header">
      <h2>{data?.template?.name || 'Template'} Template</h2>
      <button class="close-btn" onclick={onClose}>&times;</button>
    </div>

    <div class="drawer-body">
      {#if loading}
        <Skeleton lines={6} height="1rem" />
      {:else if !data}
        <p class="muted">Failed to load template.</p>
      {:else}
        <!-- Description -->
        <p class="template-desc">{data.template.description}</p>
        <p class="template-meta muted">{data.template.leadership} &middot; {data.template.best_for}</p>

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
            <h3>Included documents ({data.documents.length})</h3>
            <div class="doc-list">
              {#each data.documents as doc, i}
                <div class="doc-item">
                  <button class="doc-toggle" onclick={() => toggleDoc(i)}>
                    <span class="doc-chevron" class:open={expandedDoc === i}>&rsaquo;</span>
                    <span class="doc-name">{doc.filename.replace('.md', '').replace(/-/g, ' ')}</span>
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
  </div>
</div>

<style>
  .drawer-backdrop {
    position: fixed;
    inset: 0;
    background: var(--color-scrim);
    z-index: 300;
    display: flex;
    justify-content: flex-end;
  }

  .drawer {
    width: min(560px, 90vw);
    height: 100vh;
    background: var(--color-surface);
    border-left: 1px solid var(--color-border);
    display: flex;
    flex-direction: column;
    overflow: hidden;
    animation: slideIn 200ms ease;
  }

  @keyframes slideIn {
    from { transform: translateX(100%); }
    to { transform: translateX(0); }
  }

  .drawer-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--color-border);
    flex-shrink: 0;
  }

  .drawer-header h2 {
    font-size: 1.1rem;
    margin: 0;
  }

  .close-btn {
    border: none;
    background: none;
    font-size: 1.5rem;
    color: var(--color-text-muted);
    cursor: pointer;
    padding: 0 0.25rem;
    line-height: 1;
  }

  .close-btn:hover {
    color: var(--color-text);
  }

  .drawer-body {
    flex: 1;
    overflow-y: auto;
    padding: 1.25rem;
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
