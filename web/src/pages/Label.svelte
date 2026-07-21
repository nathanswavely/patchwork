<script>
  /**
   * The Label (docs/adr/023): the quilt's public statement of how it is
   * run and paid for. Readable logged out — its most important reader has
   * no account yet.
   *
   * Solo-first layout: one steward and the page opens with their face and
   * their own words; more and it opens with the roster. Same data, the
   * layout just leads with whoever is smallest in number.
   */
  import { Coffee, ChatCircleText, Scissors, Clock } from 'phosphor-svelte';
  import { navigate } from '../stores/router.svelte.js';
  import { getInstanceName } from '../stores/quilt.svelte.js';
  import { getLabel, loadLabel, formatMoney } from '../stores/label.svelte.js';
  import MarkdownRenderer from '../components/MarkdownRenderer.svelte';
  import Skeleton from '../components/Skeleton.svelte';

  let instanceName = $derived(getInstanceName());
  let label = $derived(getLabel());
  let loading = $state(true);

  $effect(() => {
    loadLabel(true).finally(() => { loading = false; });
  });

  let stewards = $derived(label?.stewards || []);
  let solo = $derived(stewards.length === 1);
  let items = $derived(label?.cost_items || []);

  function periodWord(p) {
    return p === 'yearly' ? '/year' : '/month';
  }
</script>

<svelte:head>
  <title>The Label &mdash; {instanceName}</title>
</svelte:head>

<div class="label-page page-fade">
  {#if loading}
    <Skeleton lines={8} />
  {:else if !label?.published}
    <div class="label-empty">
      <h1>No Label yet</h1>
      <p class="muted">
        Nobody has written this quilt's Label yet. When they do, you'll see
        who runs the place and what it costs to keep up.
      </p>
    </div>
  {:else}
    <header class="label-header">
      <p class="label-kicker">The Label</p>
      <h1>How {instanceName} is run</h1>
      <p class="label-sub muted">
        Quilts carry a label on the back that says who made them and when.
        This is ours.
      </p>
    </header>

    <!-- Stewards: solo leads with the person, plural leads with the roster -->
    <section class="stewards" class:solo>
      {#each stewards as s (s.username)}
        <a
          class="steward-card"
          href={`/users/${s.username}`}
          onclick={(e) => { e.preventDefault(); navigate(`/users/${s.username}`); }}
        >
          <span class="steward-avatar">
            {#if s.avatar_url}
              <img src={s.avatar_url} alt="" />
            {:else}
              {(s.display_name || s.username || '?')[0].toUpperCase()}
            {/if}
          </span>
          <span class="steward-meta">
            <span class="steward-name">{s.display_name || `@${s.username}`}</span>
            <span class="steward-handle muted">@{s.username}</span>
            {#if s.blurb}<span class="steward-blurb">{s.blurb}</span>{/if}
          </span>
        </a>
      {/each}
    </section>

    {#if label.prose}
      <section class="label-prose">
        <MarkdownRenderer content={label.prose} />
      </section>
    {/if}

    <!-- What this runs on -->
    {#if items.length > 0}
      <section class="costs">
        <h2>What this runs on</h2>
        {#if label.stale}
          <div class="stale-banner">
            <Clock size={16} weight="duotone" />
            <span>
              These figures haven't been reviewed since {label.stated_on}.
              They may be out of date.
            </span>
          </div>
        {/if}
        <ul class="cost-list">
          {#each items as item (item.id)}
            <li class="cost-item">
              <div class="cost-line">
                <span class="cost-service">{item.service}</span>
                {#if item.purpose}<span class="cost-purpose muted">{item.purpose}</span>{/if}
                <span class="cost-amount">
                  {formatMoney(item.amount_minor, label.currency)}{periodWord(item.period)}
                </span>
              </div>
              {#if item.why}<p class="cost-why">{item.why}</p>{/if}
            </li>
          {/each}
        </ul>
        <div class="cost-total">
          <span>About {formatMoney(label.total_monthly_minor, label.currency)}/month to keep running</span>
          <span class="cost-honesty muted">The stewards typed these numbers in themselves. Nobody audits this.</span>
        </div>
      </section>
    {/if}

    <!-- Support & feedback -->
    {#if label.support_url || label.feedback_url}
      <section class="label-links">
        {#if label.support_url}
          <a class="btn btn-primary" href={label.support_url} target="_blank" rel="noopener">
            <Coffee size={16} weight="duotone" /> Support this quilt
          </a>
        {/if}
        {#if label.feedback_url}
          <a class="btn btn-secondary" href={label.feedback_url} target="_blank" rel="noopener">
            <ChatCircleText size={16} weight="duotone" /> Send feedback
          </a>
        {/if}
      </section>
    {/if}

    {#if label.seamripped_from_name}
      <p class="seamripped-from muted">
        Seamripped from
        {#if label.seamripped_from_url}
          <a href={label.seamripped_from_url} target="_blank" rel="noopener">{label.seamripped_from_name}</a>{:else}{label.seamripped_from_name}{/if}.
        This quilt started as a fork of it.
      </p>
    {/if}

    <!-- The door. Knowing what a quilt is made of is what makes leaving
         actionable — the Label always says where the exit is. -->
    <section class="the-door">
      <h2><Scissors size={18} weight="duotone" /> If you don't like how this is run</h2>
      <p>
        Real people run this, and real people sometimes run things badly.
        So the exit is built in: any member can export what they can
        already see and start the community over somewhere else, under
        different stewards. Nobody has to ask first.
      </p>
    </section>
  {/if}
</div>

<style>
  .label-page {
    max-width: 640px;
    margin: 0 auto;
    padding: 32px 20px 64px;
  }

  .label-empty {
    text-align: center;
    padding: 64px 0;
  }

  .label-header {
    margin-bottom: 24px;
  }
  .label-kicker {
    font-size: 0.75rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--color-primary);
    margin: 0 0 4px;
  }
  .label-header h1 {
    margin: 0 0 4px;
    font-size: 1.6rem;
  }
  .label-sub {
    margin: 0;
    font-size: 0.9rem;
  }

  /* Stewards */
  .stewards {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    margin-bottom: 24px;
  }
  .steward-card {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 14px;
    border: 1px solid var(--color-border);
    border-radius: 10px;
    background: var(--color-surface);
    text-decoration: none;
    color: var(--color-text);
    min-width: 0;
  }
  .steward-card:hover {
    border-color: var(--color-primary);
  }
  /* Solo: the card becomes the page's opening — bigger, full width. */
  .stewards.solo .steward-card {
    width: 100%;
    padding: 16px 18px;
  }
  .steward-avatar {
    width: 40px;
    height: 40px;
    border-radius: 50%;
    background: var(--color-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    overflow: hidden;
    flex-shrink: 0;
  }
  .stewards.solo .steward-avatar {
    width: 56px;
    height: 56px;
    font-size: 1.3rem;
  }
  .steward-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .steward-meta {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .steward-name {
    font-weight: 600;
  }
  .steward-handle {
    font-size: 0.8rem;
  }
  .steward-blurb {
    font-size: 0.85rem;
    margin-top: 2px;
  }

  .label-prose {
    margin-bottom: 28px;
  }

  /* Costs */
  .costs h2,
  .the-door h2 {
    font-size: 1.05rem;
    margin: 0 0 10px;
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .stale-banner {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    border-radius: 8px;
    background: var(--color-overlay);
    border: 1px solid var(--color-border);
    font-size: 0.85rem;
    margin-bottom: 12px;
  }
  .cost-list {
    list-style: none;
    margin: 0 0 12px;
    padding: 0;
    border: 1px solid var(--color-border);
    border-radius: 10px;
    background: var(--color-surface);
    overflow: hidden;
  }
  .cost-item {
    padding: 10px 14px;
  }
  .cost-item + .cost-item {
    border-top: 1px solid var(--color-border);
  }
  .cost-line {
    display: flex;
    align-items: baseline;
    gap: 8px;
  }
  .cost-service {
    font-weight: 600;
  }
  .cost-purpose {
    font-size: 0.85rem;
  }
  .cost-amount {
    margin-left: auto;
    font-variant-numeric: tabular-nums;
    font-weight: 600;
    white-space: nowrap;
  }
  .cost-why {
    margin: 4px 0 0;
    font-size: 0.85rem;
    color: var(--color-text-muted, var(--color-text));
    opacity: 0.85;
  }
  .cost-total {
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-weight: 600;
    margin-bottom: 24px;
  }
  .cost-honesty {
    font-size: 0.8rem;
    font-weight: 400;
  }

  .label-links {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    margin-bottom: 24px;
  }
  .label-links .btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  .seamripped-from {
    font-size: 0.85rem;
    margin: 0 0 24px;
  }

  .the-door {
    border-top: 1px dashed var(--color-border);
    padding-top: 20px;
  }
  .the-door p {
    margin: 0;
    font-size: 0.9rem;
    line-height: 1.6;
  }
</style>
