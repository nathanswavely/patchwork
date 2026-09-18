<script>
  /**
   * The admin panel's Overview (CONTEXT.md): what is waiting on the
   * instance admin and what is unattended, and nothing else. No size,
   * growth or activity figures — a steward opens this page to act, and a
   * quilt with nothing waiting shows a quiet page.
   *
   * Three regions, top to bottom: the inbox (decision queues a person is
   * waiting on, each counting exactly what its tab lists, with the age of
   * the oldest), routing work nobody is waiting on (unrouted names), and
   * care — what is broken or unattended and the instance admin's to mend
   * under custody (docs/adr/115). Standing conditions (mail off, no
   * passkey) are stated plainly and never dismissed.
   */
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { formatRelative } from '../lib/datetime.js';
  import Skeleton from '../components/Skeleton.svelte';
  import ErrorState from '../components/ErrorState.svelte';

  let overview = $state(null);
  let loading = $state(true);
  let error = $state('');

  $effect(() => {
    load();
  });

  async function load() {
    loading = true;
    error = '';
    try {
      overview = await api('admin/overview');
    } catch (e) {
      error = e.message;
    }
    loading = false;
  }

  function handleNav(e, path) {
    e.preventDefault();
    navigate(path);
  }

  // Each queue's Review section (docs/adr/2026-09-17-an-admin-tab-answers-one-question.md), and how to say "3 of
  // them" in the page's own words.
  const QUEUES = {
    reports: { one: 'report', many: 'reports', href: '/admin/review/reports' },
    submissions: { one: 'patch submission', many: 'patch submissions', href: '/admin/review/submissions' },
    event_submissions: { one: 'event submission', many: 'event submissions', href: '/admin/review/event-submissions' },
    claims: { one: 'claim', many: 'claims', href: '/admin/review/claims' },
    tag_suggestions: { one: 'suggested tag', many: 'suggested tags', href: '/admin/review/tags' },
  };

  function plural(n, one, many) {
    return `${n} ${n === 1 ? one : many}`;
  }

  let waiting = $derived(
    (overview?.inbox || []).filter((q) => q.count > 0 && QUEUES[q.queue])
  );
  let care = $derived(overview?.care || null);
  let hasCare = $derived(
    !!care && (
      care.adminless_patches.length > 0 ||
      care.failing_aggregators.length > 0 ||
      care.failing_unclaimed_sources.length > 0 ||
      care.failed_deliveries > 0
    )
  );
</script>

<div class="page-fade">
  <div class="page-header">
    <h1>Overview</h1>
  </div>

  {#if loading}
    <Skeleton lines={5} />
  {:else if error}
    <ErrorState message={error} retry={load} />
  {:else if overview}
    <section class="section">
      <h2>Waiting on you</h2>
      {#if waiting.length === 0}
        <p class="muted">Nothing is waiting on you.</p>
      {:else}
        <ul class="lines">
          {#each waiting as q (q.queue)}
            <li>
              <a href={QUEUES[q.queue].href} class="line card" onclick={(e) => handleNav(e, QUEUES[q.queue].href)}>
                <strong>{plural(q.count, QUEUES[q.queue].one, QUEUES[q.queue].many)}</strong>
                {#if q.oldest_at}
                  <span class="muted">oldest {formatRelative(q.oldest_at)}</span>
                {/if}
              </a>
            </li>
          {/each}
        </ul>
      {/if}

      {#if overview.unrouted_names.count > 0}
        <p class="lower">
          <a href="/admin/settings/aggregators" onclick={(e) => handleNav(e, '/admin/settings/aggregators')}>
            {plural(overview.unrouted_names.count, 'unrouted name', 'unrouted names')}
          </a>
          {' '}across {plural(overview.unrouted_names.aggregators, 'aggregator', 'aggregators')}. Nobody is waiting on these.
        </p>
      {/if}
    </section>

    {#if hasCare}
      <section class="section">
        <h2>Needs attention</h2>
        <ul class="lines">
          {#each care.adminless_patches as p (p.slug)}
            <li>
              <a href={`/patches/${p.slug}/settings/members`} class="line card" onclick={(e) => handleNav(e, `/patches/${p.slug}/settings/members`)}>
                <strong>{p.name} has no admin.</strong>
                {#if p.member_count > 0}
                  <span class="muted">{plural(p.member_count, 'member', 'members')} to hand it to.</span>
                {:else}
                  <span class="muted">No members to hand it to.</span>
                {/if}
              </a>
            </li>
          {/each}
          {#each care.failing_aggregators as a (a.id)}
            <li>
              <a href="/admin/settings/aggregators" class="line card" onclick={(e) => handleNav(e, '/admin/settings/aggregators')}>
                <strong>{a.name} is not syncing.</strong>
                <span class="muted">{a.last_error}{#if a.last_success_at}{' · '}last synced {formatRelative(a.last_success_at)}{/if}</span>
              </a>
            </li>
          {/each}
          {#each care.failing_unclaimed_sources as s (s.id)}
            <li>
              <a href={`/patches/${s.node_slug}/settings/sources`} class="line card" onclick={(e) => handleNav(e, `/patches/${s.node_slug}/settings/sources`)}>
                <strong>{s.node_name}'s feed is not syncing.</strong>
                <span class="muted">{s.last_error}{#if s.last_success_at}{' · '}last synced {formatRelative(s.last_success_at)}{/if}</span>
              </a>
            </li>
          {/each}
          {#if care.failed_deliveries > 0}
            <li>
              <div class="line card">
                <strong>{plural(care.failed_deliveries, 'federated activity', 'federated activities')} could not be delivered.</strong>
                <span class="muted">The delivery worker has given up on these.</span>
              </div>
            </li>
          {/if}
        </ul>
      </section>
    {/if}

    {#if !overview.smtp_configured || !overview.has_passkey}
      <section class="section">
        <ul class="statements">
          {#if !overview.smtp_configured}
            <li>Mail is off. Sign-in is by invite link and passkey, and magic links print to the server log.</li>
          {/if}
          {#if !overview.has_passkey}
            <li>
              You have no passkey. Export, wipe, setting a person's email and proving admin each need one.
              <a href="/settings/security" onclick={(e) => handleNav(e, '/settings/security')}>Add one in Security settings.</a>
            </li>
          {/if}
        </ul>
      </section>
    {/if}
  {/if}
</div>

<style>
  .page-header {
    padding: 1.5rem 0 1rem;
  }

  .section {
    margin-bottom: 2rem;
  }

  .section h2 {
    font-size: 1.1rem;
    margin-bottom: 0.75rem;
  }

  .lines {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .line {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.25rem 0.75rem;
    text-decoration: none;
    color: var(--color-text);
    transition: border-color 150ms ease;
  }

  a.line:hover {
    border-color: var(--color-primary);
    text-decoration: none;
  }

  .lower {
    margin-top: 0.75rem;
    font-size: 0.9rem;
    color: var(--color-text-muted);
  }

  .statements {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    font-size: 0.9rem;
    color: var(--color-text-muted);
  }
</style>
