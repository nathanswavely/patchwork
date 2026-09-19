<script>
  import { api } from '../lib/api.js';
  import { getUser } from '../stores/auth.svelte.js';
  import { navigate } from '../stores/router.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import { formatEventDate as formatDate, upcomingFrom } from '../lib/datetime.js';

  let user = $derived(getUser());
  let memberships = $state([]);
  let loading = $state(true);

  // Aggregated attention items.
  //
  // Counts came from `items.length` of requests made with `limit=1`, so
  // every number here was 0 or 1 whatever the patch held. The members
  // listing sends `member_count` under the same filter it ran, so pending
  // requests are counted exactly. Proposals and events send no total: those
  // ask for a page of PAGE and say "20+" when a cursor follows, rather than
  // stating the page size as the count.
  const PAGE = 20;
  let pendingCounts = $state({});   // slug → count
  let proposalCounts = $state({});  // slug → { count, more }
  let upcomingEvents = $state([]);
  let upcomingMore = $state(false);

  $effect(() => {
    if (user) loadMemberships();
  });

  // Group memberships by role
  let adminPatches = $derived(memberships.filter(m => m.role === 'admin'));
  let memberPatches = $derived(memberships.filter(m => m.role === 'member'));
  let followingPatches = $derived(memberships.filter(m => m.role === 'follower'));

  // Total attention items
  let totalPending = $derived(Object.values(pendingCounts).reduce((s, n) => s + n, 0));
  let totalProposals = $derived(Object.values(proposalCounts).reduce((s, c) => s + c.count, 0));
  let proposalsMore = $derived(Object.values(proposalCounts).some((c) => c.more));

  function countLabel(count, more) {
    return `${count}${more ? '+' : ''}`;
  }

  async function loadMemberships() {
    loading = true;
    try {
      const data = await api('me/nodes');
      memberships = data.items || data || [];
      // Load attention data in parallel
      loadAttentionData();
    } catch {
      memberships = [];
    } finally {
      loading = false;
    }
  }

  async function loadAttentionData() {
    const pending = {};
    const proposals = {};

    // Pending join requests on the patches this person runs. An admin is
    // inside the room, so member_count under status=pending is the whole
    // queue, not the page.
    const adminFetches = adminPatches.map(async (m) => {
      try {
        const data = await api(`nodes/${m.node_slug}/members?status=pending&limit=1`);
        const items = data.items || [];
        const count = typeof data.member_count === 'number' ? data.member_count : items.length;
        if (count > 0) pending[m.node_slug] = count;
      } catch {}
    });

    // Open proposals on the patches this person can vote in.
    const memberFetches = [...adminPatches, ...memberPatches].map(async (m) => {
      try {
        const data = await api(`nodes/${m.node_slug}/proposals?status=open&limit=${PAGE}`);
        const items = data.items || [];
        if (items.length > 0) proposals[m.node_slug] = { count: items.length, more: !!data.next_cursor };
      } catch {}
    });

    // Upcoming events on this person's own patches (scope=my, docs/adr/035),
    // not the whole quilt's calendar: a dashboard headed "attention needed"
    // that lists strangers' shows is asking for attention it has no claim on.
    const eventFetch = api(`events?scope=my&from=${encodeURIComponent(upcomingFrom())}&limit=${PAGE}`).then(data => {
      upcomingEvents = data.items || [];
      upcomingMore = !!data.next_cursor;
    }).catch(() => {});

    await Promise.all([...adminFetches, ...memberFetches, eventFetch]);
    pendingCounts = pending;
    proposalCounts = proposals;
  }


</script>

<div class="dashboard">
  <div class="page-header">
    <h1>Dashboard</h1>
    {#if user}
      <p class="muted">Welcome back, {user.display_name || user.username}.</p>
    {/if}
  </div>

  {#if loading}
    <Skeleton lines={4} height="1rem" />
  {:else}

    <!-- Attention Needed -->
    {#if totalPending > 0 || totalProposals > 0 || upcomingEvents.length > 0}
      <section class="attention-section">
        <h2 class="section-title">Attention Needed</h2>
        <div class="attention-items">
          {#if totalPending > 0}
            <div class="attention-item">
              <span class="attention-count">{totalPending}</span>
              <span>pending member {totalPending === 1 ? 'request' : 'requests'}</span>
              {#each Object.entries(pendingCounts) as [slug, count]}
                <a href="/patches/{slug}/settings/members" class="attention-link" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/settings/members`); }}>
                  {slug} ({count})
                </a>
              {/each}
            </div>
          {/if}
          {#if totalProposals > 0}
            <div class="attention-item">
              <span class="attention-count">{countLabel(totalProposals, proposalsMore)}</span>
              <span>open {totalProposals === 1 && !proposalsMore ? 'proposal' : 'proposals'}</span>
              {#each Object.entries(proposalCounts) as [slug, c]}
                <a href="/patches/{slug}/governance/proposals" class="attention-link" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance/proposals`); }}>
                  {slug} ({countLabel(c.count, c.more)})
                </a>
              {/each}
            </div>
          {/if}
          {#if upcomingEvents.length > 0}
            <div class="attention-item">
              <span class="attention-count">{countLabel(upcomingEvents.length, upcomingMore)}</span>
              <span>upcoming {upcomingEvents.length === 1 && !upcomingMore ? 'event' : 'events'} on your patches</span>
              {#each upcomingEvents.slice(0, 3) as event}
                <a href="/events/{event.id}" class="attention-link" onclick={(e) => { e.preventDefault(); navigate(`/events/${event.id}`); }}>
                  {formatDate(event.starts_at, event.timezone)} &middot; {event.title}
                </a>
              {/each}
            </div>
          {/if}
        </div>
      </section>
    {/if}

    <!-- Quick Actions -->
    <div class="quick-actions">
      <a href="/patches/new" class="action-card" onclick={(e) => { e.preventDefault(); navigate('/patches/new'); }}>
        <span class="action-icon">+</span>
        <span>Add a patch</span>
      </a>
      {#if adminPatches.length > 0}
        <a href="/events/new" class="action-card" onclick={(e) => { e.preventDefault(); navigate('/events/new'); }}>
          <span class="action-icon">+</span>
          <span>Create Event</span>
        </a>
      {/if}
    </div>

    <!-- Managing (admin) -->
    {#if adminPatches.length > 0}
      <section class="patch-section">
        <h2 class="section-title">Managing</h2>
        <div class="patch-list">
          {#each adminPatches as m (m.id)}
            <div class="patch-card">
              <div class="patch-card-info">
                <a href="/patches/{m.node_slug}" class="patch-name" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}`); }}>
                  {m.node_name}
                </a>
                {#if pendingCounts[m.node_slug]}
                  <span class="badge badge-attention">{pendingCounts[m.node_slug]} pending</span>
                {/if}
              </div>
              <div class="patch-card-actions">
                <a href="/patches/{m.node_slug}/governance" class="btn btn-sm btn-secondary manage-btn" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}/governance`); }}>Manage</a>
                <a href="/patches/{m.node_slug}/members" class="quick-link" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}/members`); }}>Members</a>
                <a href="/patches/{m.node_slug}/governance/proposals" class="quick-link" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}/governance/proposals`); }}>Proposals</a>
              </div>
            </div>
          {/each}
        </div>
      </section>
    {/if}

    <!-- Member of -->
    {#if memberPatches.length > 0}
      <section class="patch-section">
        <h2 class="section-title">Member of</h2>
        <div class="patch-list">
          {#each memberPatches as m (m.id)}
            <div class="patch-card">
              <div class="patch-card-info">
                <a href="/patches/{m.node_slug}" class="patch-name" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}`); }}>
                  {m.node_name}
                </a>
                {#if proposalCounts[m.node_slug]}
                  <span class="badge">{countLabel(proposalCounts[m.node_slug].count, proposalCounts[m.node_slug].more)} open</span>
                {/if}
              </div>
              <div class="patch-card-actions">
                <a href="/patches/{m.node_slug}/governance/proposals" class="quick-link" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}/governance/proposals`); }}>Proposals</a>
                <a href="/patches/{m.node_slug}/events" class="quick-link" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}/events`); }}>Events</a>
              </div>
            </div>
          {/each}
        </div>
      </section>
    {/if}

    <!-- Following -->
    {#if followingPatches.length > 0}
      <section class="patch-section">
        <h2 class="section-title">Following</h2>
        <div class="patch-list">
          {#each followingPatches as m (m.id)}
            <div class="patch-card">
              <div class="patch-card-info">
                <a href="/patches/{m.node_slug}" class="patch-name" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}`); }}>
                  {m.node_name}
                </a>
              </div>
              <div class="patch-card-actions">
                <a href="/patches/{m.node_slug}/events" class="quick-link" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}/events`); }}>Events</a>
              </div>
            </div>
          {/each}
        </div>
      </section>
    {/if}

    <!-- Empty state -->
    {#if memberships.length === 0}
      <div class="empty-state">
        <p>You haven't joined any patches yet.</p>
        <p class="muted">The groups near you are on the quilt.</p>
        <a href="/" class="btn btn-primary" onclick={(e) => { e.preventDefault(); navigate('/'); }}>
          Explore Quilt
        </a>
      </div>
    {/if}

  {/if}
</div>

<style>
  .page-header {
    padding: 0.5rem 0 1.5rem;
  }

  .page-header h1 {
    margin-bottom: 0.25rem;
  }

  .section-title {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-text-muted);
    margin-bottom: 0.75rem;
  }

  /* Attention section */
  .attention-section {
    margin-bottom: 1.5rem;
    padding: 1rem;
    border: 1px solid var(--color-accent);
    border-radius: 6px;
    background: color-mix(in srgb, var(--color-accent) 5%, var(--color-surface));
  }

  .attention-items {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .attention-item {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.4rem;
    font-size: 0.88rem;
  }

  .attention-count {
    font-weight: 700;
    color: var(--color-accent);
    min-width: 1.5rem;
  }

  .attention-link {
    font-size: 0.8rem;
    color: var(--color-primary);
    text-decoration: none;
    padding: 0.1rem 0.4rem;
    border-radius: 3px;
    transition: background 100ms ease;
  }

  .attention-link:hover {
    background: var(--color-overlay);
  }

  /* Quick actions */
  .quick-actions {
    display: flex;
    gap: 0.75rem;
    margin-bottom: 2rem;
  }

  .action-card {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.6rem 1rem;
    border: 1px solid var(--color-border);
    border-radius: 6px;
    text-decoration: none;
    color: var(--color-text);
    font-size: 0.88rem;
    font-weight: 500;
    transition: border-color 150ms ease;
  }

  .action-card:hover {
    border-color: var(--color-primary);
    text-decoration: none;
  }

  .action-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 1.5rem;
    height: 1.5rem;
    border-radius: 50%;
    background: var(--color-primary);
    color: var(--color-btn-on-primary);
    font-size: 0.9rem;
    font-weight: 700;
    flex-shrink: 0;
  }

  /* Patch sections */
  .patch-section {
    margin-bottom: 1.5rem;
  }

  .patch-list {
    display: flex;
    flex-direction: column;
    gap: 0;
  }

  .patch-card {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.6rem 0;
    border-bottom: 1px solid var(--color-border);
  }

  .patch-card:last-child {
    border-bottom: none;
  }

  .patch-card-info {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-width: 0;
  }

  .patch-name {
    font-size: 0.9rem;
    font-weight: 500;
    color: var(--color-text);
    text-decoration: none;
  }

  .patch-name:hover {
    color: var(--color-primary);
  }

  .badge-attention {
    background: var(--color-accent);
    color: var(--color-on-accent);
  }

  .patch-card-actions {
    display: flex;
    gap: 0.5rem;
    flex-shrink: 0;
  }

  .quick-link {
    font-size: 0.78rem;
    color: var(--color-text-muted);
    text-decoration: none;
    padding: 0.2rem 0.4rem;
    border-radius: 3px;
    transition: background 100ms ease, color 100ms ease;
  }

  .quick-link:hover {
    background: var(--color-overlay);
    color: var(--color-primary);
    text-decoration: none;
  }

  .manage-btn {
    font-weight: 600;
  }

  /* Empty state */
  .empty-state {
    text-align: center;
    padding: 3rem 0;
  }

  .empty-state p {
    margin-bottom: 0.5rem;
  }

  .empty-state .btn {
    margin-top: 1rem;
  }

  @media (max-width: 640px) {
    .quick-actions {
      flex-direction: column;
    }

    .patch-card {
      flex-direction: column;
      align-items: flex-start;
      gap: 0.4rem;
    }
  }
</style>
