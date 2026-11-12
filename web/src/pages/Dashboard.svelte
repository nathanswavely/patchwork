<script>
  import { api } from '../lib/api.js';
  import { allMyMemberships } from '../lib/pages.js';
  import { getUser } from '../stores/auth.svelte.js';
  import { navigate } from '../stores/router.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import EventTierChip from '../components/EventTierChip.svelte';
  import { formatEventDate as formatDate, formatDay, upcomingFrom } from '../lib/datetime.js';
  import { getInvitations } from '../stores/memberships.svelte.js';

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
  let proposalCounts = $state({});  // slug → { count } awaiting this viewer's vote
  let upcomingEvents = $state([]);
  let upcomingMore = $state(false);

  $effect(() => {
    if (user) loadMemberships();
  });

  // Group memberships by role
  let adminPatches = $derived(memberships.filter(m => m.role === 'admin'));
  let memberPatches = $derived(memberships.filter(m => m.role === 'member'));
  let followingPatches = $derived(memberships.filter(m => m.role === 'follower'));

  // Patch names by slug and by id, so no line on this page shows a slug
  // where a name belongs (F-054): the old block's only links were
  // "bobbin-hall-co-op (1)", the address with the dashes in it.
  let nameBySlug = $derived(new Map(memberships.map((m) => [m.node_slug, m.node_name])));
  let nameById = $derived(new Map(memberships.map((m) => [m.node_id, m.node_name])));
  let slugById = $derived(new Map(memberships.map((m) => [m.node_id, m.node_slug])));

  // What this person owes an answer to (F-054): invitations waiting on
  // them, join requests waiting on them as an admin, and votes they have
  // not cast. Events are not here. An event on your patch asks nothing of
  // you, and filing it beside your own unanswered requests made the box
  // read as if it did.
  let invitations = $derived(getInvitations());
  let pendingLines = $derived(
    Object.entries(pendingCounts).map(([slug, count]) => ({ slug, name: nameBySlug.get(slug) || slug, count }))
  );
  let proposalLines = $derived(
    Object.entries(proposalCounts).map(([slug, c]) => ({ slug, name: nameBySlug.get(slug) || slug, count: c.count }))
  );
  let needsYou = $derived(invitations.length > 0 || pendingLines.length > 0 || proposalLines.length > 0);

  // The person's own requests, waiting on somebody else (F-055). Not owed,
  // so not under "Needs you"; said here because the page used to say
  // nothing at all to a requester, and "is my request stuck?" was the
  // question it left.
  let ownRequests = $derived(memberships.filter((m) => m.status === 'pending'));

  function countLabel(count, more) {
    return `${count}${more ? '+' : ''}`;
  }

  async function loadMemberships() {
    loading = true;
    try {
      memberships = await allMyMemberships();
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

    // Proposals on these patches that are still waiting on this person.
    //
    // This counted the rows it had just fetched — every open proposal on
    // the patch — so the front door went on saying "ATTENTION NEEDED, 1
    // open proposal" after they had voted, while the Governance hub inside
    // the patch had already dropped it. Two members went back and opened
    // their ballot a second time to check it had saved. The server states
    // the number now, from the same count the hub reads, so the two
    // surfaces cannot drift again.
    const memberFetches = [...adminPatches, ...memberPatches].map(async (m) => {
      try {
        const data = await api(`nodes/${m.node_slug}/proposals?status=open&limit=${PAGE}`);
        // No "+" on this one, unlike the pending and event counts:
        // awaiting_your_vote counts the whole patch rather than the page
        // that came back, so there is never more beyond it.
        const count = data.awaiting_your_vote ?? 0;
        if (count > 0) proposals[m.node_slug] = { count };
      } catch {}
    });

    // Upcoming events on this person's own patches (scope=my, docs/adr/035),
    // not the whole quilt's calendar: "Coming up" that lists strangers' shows
    // is a calendar of somewhere else.
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
      <p class="lt-muted">Welcome back, {user.display_name || user.username}.</p>
    {/if}
  </div>

  {#if loading}
    <Skeleton lines={4} height="1rem" />
  {:else}

    <!-- Needs you (F-054): only what waits on this reader. Each line is one
         patch, named by its name, and the words themselves are the link to
         where it is answered. The old block linked only a slug and left the
         label as plain text, so a reader clicked "open proposal" twice and
         nothing happened. -->
    {#if needsYou}
      <section class="attention-section">
        <h2 class="section-title">Needs you</h2>
        <ul class="line-list">
          {#each invitations as inv (inv.id)}
            <li>
              <a href="/patches/{inv.node_slug}" class="line-link" onclick={(e) => { e.preventDefault(); navigate(`/patches/${inv.node_slug}`); }}>
                {#if inv.invited_by?.display_name || inv.invited_by?.username}
                  {inv.invited_by.display_name || inv.invited_by.username} invited you to join <strong>{inv.node_name}</strong>
                {:else}
                  Invitation to join <strong>{inv.node_name}</strong>
                {/if}
              </a>
            </li>
          {/each}
          {#each pendingLines as line (line.slug)}
            <li>
              <a href="/patches/{line.slug}/settings/members" class="line-link" onclick={(e) => { e.preventDefault(); navigate(`/patches/${line.slug}/settings/members`); }}>
                {line.count} join {line.count === 1 ? 'request' : 'requests'} to answer in <strong>{line.name}</strong>
              </a>
            </li>
          {/each}
          {#each proposalLines as line (line.slug)}
            <li>
              <!-- Counts what this reader still owes, in the words the
                   Governance hub uses for the same count. -->
              <a href="/patches/{line.slug}/governance/proposals" class="line-link" onclick={(e) => { e.preventDefault(); navigate(`/patches/${line.slug}/governance/proposals`); }}>
                {line.count} {line.count === 1 ? 'proposal needs' : 'proposals need'} your vote in <strong>{line.name}</strong>
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if ownRequests.length > 0}
      <section class="patch-section">
        <h2 class="section-title">Waiting on an answer</h2>
        <ul class="line-list">
          {#each ownRequests as m (m.id)}
            <li>
              <a href="/patches/{m.node_slug}" class="line-link" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}`); }}>
                Your request to join <strong>{m.node_name}</strong> is waiting on an admin.
              </a>
              {#if m.joined_at}<span class="lt-muted line-meta">Sent {formatDay(m.joined_at)}</span>{/if}
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    <!-- Coming up: events on this person's own patches. Happening, not
         owed, so it sits apart from "Needs you". -->
    {#if upcomingEvents.length > 0}
      <section class="patch-section">
        <h2 class="section-title">Coming up</h2>
        <ul class="line-list">
          {#each upcomingEvents.slice(0, 5) as event (event.id)}
            <li class="event-line">
              <a href="/events/{event.id}" class="line-link" onclick={(e) => { e.preventDefault(); navigate(`/events/${event.id}`); }}>
                <strong>{event.title}</strong>
              </a>
              <span class="lt-muted line-meta">
                {formatDate(event.starts_at, event.timezone)}{#if nameById.get(event.node_id)}{' · '}<a href="/patches/{slugById.get(event.node_id)}" class="patch-ref" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slugById.get(event.node_id)}`); }}>{nameById.get(event.node_id)}</a>{/if}
              </span>
              <EventTierChip visibility={event.visibility} />
            </li>
          {/each}
        </ul>
        {#if upcomingEvents.length > 5 || upcomingMore}
          <a href="/events/my" class="quick-link more-link" onclick={(e) => { e.preventDefault(); navigate('/events/my'); }}>
            {countLabel(upcomingEvents.length, upcomingMore)} upcoming on your patches
          </a>
        {/if}
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
                  <span class="lt-pill lt-tone-accent">{pendingCounts[m.node_slug]} pending</span>
                {/if}
              </div>
              <div class="patch-card-actions">
                <a href="/patches/{m.node_slug}/governance" class="lt-btn lt-btn-sm lt-btn-secondary manage-btn" onclick={(e) => { e.preventDefault(); navigate(`/patches/${m.node_slug}/governance`); }}>Manage</a>
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
                  <span class="lt-pill" title="Open proposals here that you have not voted on yet.">{proposalCounts[m.node_slug].count} to vote on</span>
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
      <div class="lt-empty">
        <p class="lt-empty-title">You haven't joined any patches yet.</p>
        <p class="lt-muted">The groups near you are on the quilt.</p>
        <div class="lt-empty-actions">
          <a href="/" class="lt-btn lt-btn-primary" onclick={(e) => { e.preventDefault(); navigate('/'); }}>
            Explore Quilt
          </a>
        </div>
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

  /* One line per thing, the words themselves the link. */
  .line-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .line-list li {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.4rem;
    font-size: 0.88rem;
  }

  .line-link {
    color: var(--color-primary);
    text-decoration: none;
  }

  .line-link:hover {
    text-decoration: underline;
  }

  .line-link strong {
    font-weight: 600;
  }

  .line-meta {
    font-size: 0.8rem;
  }

  .patch-ref {
    color: inherit;
  }

  .more-link {
    display: inline-block;
    margin-top: 0.5rem;
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
