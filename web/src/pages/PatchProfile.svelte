<script>
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import { getSubmissionsEnabled } from '../stores/quilt.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';
  import Modal from '../components/Modal.svelte';
  import PatchCover from '../components/PatchCover.svelte';
  import ReportButton from '../components/ReportButton.svelte';
  import { identityColorForPatch } from '../lib/quiltTheme.js';

  let { slug = '' } = $props();

  // Modal state
  let modalOpen = $state(false);
  let modalType = $state(''); // 'doc' or 'proposal'
  let modalItem = $state(null);

  let node = $state(null);
  let isMember = $state(false);
  let isAdmin = $state(false);
  let membershipRole = $state('');
  let isUnclaimed = $state(false);
  let isBanned = $state(false);
  let loading = $state(true);
  let error = $state('');
  let joining = $state(false);

  let recentEvents = $state([]);
  let recentProposals = $state([]);
  let governanceDocs = $state([]);
  let followerPermissions = $state(null);
  let hasOpenClaim = $state(false);

  async function loadClaimState() {
    try {
      const data = await api(`nodes/${slug}/claims/mine`);
      hasOpenClaim = !!data.claim;
    } catch {
      // Non-fatal — the claim page itself is the source of truth.
    }
  }

  $effect(() => {
    if (slug) {
      loadNode();
    }
  });

  // Reactive on auth: on a fresh page load the session check may still be
  // in flight when the node arrives, so gating this inside loadNode() would
  // race and miss the open claim.
  $effect(() => {
    hasOpenClaim = false;
    if (slug && isUnclaimed && isLoggedIn()) {
      loadClaimState();
    }
  });

  async function loadNode() {
    loading = true;
    error = '';
    try {
      const data = await api(`nodes/${slug}`);
      node = data.node || data;
      isMember = data.is_member || false;
      isAdmin = data.is_admin || false;
      membershipRole = data.membership_role || '';
      isUnclaimed = data.is_unclaimed || false;
      isBanned = data.is_banned || false;
      followerPermissions = (data.node || data).follower_permissions || null;
      loadActivity();
    } catch (e) {
      error = e.message || 'Failed to load patch';
      node = null;
    } finally {
      loading = false;
    }
  }

  async function loadActivity() {
    // Show governance content if member OR if patch allows public governance
    const showProposals = isMember || followerPermissions?.proposals === true;
    const showCharters = isMember || followerPermissions?.charters === true;
    const [eventData, proposalData, charterData] = await Promise.all([
      api(`events?node_slug=${encodeURIComponent(slug)}&limit=5`).catch(() => ({ items: [] })),
      (showProposals ? api(`nodes/${slug}/proposals?limit=3`) : Promise.resolve({ items: [] })).catch(() => ({ items: [] })),
      (showCharters ? api(`nodes/${slug}/governance`).catch(() => ({ items: [] })) : Promise.resolve({ items: [] })),
    ]);
    recentEvents = eventData.items || eventData || [];
    recentProposals = proposalData.items || proposalData || [];
    governanceDocs = charterData.items || charterData || [];
  }

  async function handleJoin() {
    if (!isLoggedIn()) { navigate('/login'); return; }
    const wasFollower = membershipRole === 'follower';
    joining = true;
    try {
      const result = await api(`nodes/${slug}/join`, { method: 'POST' });
      await loadNode();
      if (result.status === 'pending') {
        showToast('Membership request sent', 'success');
      } else {
        showToast(wasFollower ? 'Now a member' : 'Joined patch', 'success');
      }
    } catch (e) {
      showToast(e.message || 'Failed to join', 'error');
    } finally {
      joining = false;
    }
  }

  async function handleFollow() {
    if (!isLoggedIn()) { navigate('/login'); return; }
    joining = true;
    try {
      await api(`nodes/${slug}/join`, { method: 'POST', body: { role: 'follower' } });
      await loadNode();
      showToast('Following patch', 'success');
    } catch (e) {
      showToast(e.message || 'Failed to follow', 'error');
    } finally {
      joining = false;
    }
  }

  async function handleLeave() {
    const wasFollower = membershipRole === 'follower';
    joining = true;
    try {
      await api(`nodes/${slug}/leave`, { method: 'POST' });
      await loadNode();
      showToast(wasFollower ? 'Unfollowed patch' : 'Left patch', 'info');
    } catch (e) {
      showToast(e.message || 'Failed to leave', 'error');
    } finally {
      joining = false;
    }
  }

  // Suggest-an-event door (docs/adr/026): logged-in people who can't post
  // to this calendar directly can suggest an event for review. Unclaimed
  // patches accept suggestions whenever the instance switch is on; active
  // patches must also opt in.
  let canSuggest = $derived.by(() => {
    if (!node || !isLoggedIn() || isBanned) return false;
    if (!getSubmissionsEnabled()) return false;
    if (isUnclaimed) return true;
    if (isAdmin || (isMember && membershipRole !== 'follower')) return false;
    return node.accept_event_suggestions === true;
  });

  function extractDomain(url) {
    try { return new URL(url).hostname.replace(/^www\./, ''); }
    catch { return url; }
  }

  function formatDate(iso) {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
  }
</script>

<div class="profile">
  {#if loading}
    <div class="profile-loading">
      <div class="skel" style="height: 150px;"></div>
      <div class="skel" style="width: 300px; height: 14px; margin: 12px auto 0;"></div>
    </div>
  {:else if error}
    <div class="profile-error">
      <h2>Patch not found</h2>
      <p class="muted">{error}</p>
      <a href="/" class="btn btn-secondary" onclick={(e) => { e.preventDefault(); navigate('/'); }}>Back to Quilt</a>
    </div>
  {:else if node}
    <!-- Header: the patch's own block as a cover, name and stats sitting in it -->
    <div class="profile-header">
      <div class="profile-cover" style="background: {identityColorForPatch(node)}">
        <PatchCover patch={node} />
        <div class="cover-scrim"></div>
        <div class="cover-text">
          <h1 class="profile-name">{node.name}</h1>
          <p class="profile-stats">
            {isUnclaimed ? `${node.follower_count || 0} Following` : `${node.member_count || 0} Members`} &middot; {recentEvents.length} Upcoming Events
          </p>
        </div>
      </div>
      {#if node.description}
        <p class="profile-desc">{node.description}</p>
      {/if}
    </div>

    <!-- Actions -->
    <div class="profile-actions">
      {#if isBanned}
        <span class="banned-notice">Removed from this community</span>
      {:else if isUnclaimed}
        <!-- Unclaimed patches are follow-only: nobody runs them yet, so
             membership is impossible (the backend rejects it). Follow to
             watch, or claim it if it's yours. -->
        {#if membershipRole === 'follower'}
          <button class="btn btn-secondary" onclick={handleLeave} disabled={joining}>Unfollow</button>
        {:else}
          <button class="btn btn-secondary" onclick={handleFollow} disabled={joining}>Follow</button>
        {/if}
        <a href="/patches/{slug}/claim" class="btn btn-primary" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/claim`); }}>
          {hasOpenClaim ? 'Claim in progress' : 'Claim this patch'}
        </a>
      {:else if isMember}
        {#if membershipRole === 'follower'}
          <button class="btn btn-primary" onclick={handleJoin} disabled={joining}>Become Member</button>
          <button class="btn btn-secondary" onclick={handleLeave} disabled={joining}>Unfollow</button>
        {:else if membershipRole !== 'admin'}
          <button class="btn btn-secondary" onclick={handleLeave} disabled={joining}>Leave</button>
        {/if}
      {:else}
        <button class="btn btn-primary" onclick={handleJoin} disabled={joining}>Join</button>
        <button class="btn btn-secondary" onclick={handleFollow} disabled={joining}>Follow</button>
      {/if}
      {#if canSuggest}
        <a
          href="/events/new?node={slug}"
          class="btn btn-secondary"
          onclick={(e) => { e.preventDefault(); navigate(`/events/new?node=${slug}`); }}
        >Suggest an event</a>
      {/if}
      {#if isAdmin}
        <!-- Unclaimed patches have no governance workspace; Manage lands on
             the events calendar, which is the live surface (docs/adr/026, #6). -->
        {#if isUnclaimed}
          <a href="/patches/{slug}/events" class="btn btn-secondary" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/events`); }}>
            Manage
          </a>
        {:else}
          <a href="/patches/{slug}/governance" class="btn btn-secondary" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance`); }}>
            Manage
          </a>
        {/if}
      {:else if isMember && !isBanned && !isUnclaimed}
        <a href="/patches/{slug}/governance" class="btn btn-secondary" onclick={(e) => { e.preventDefault(); navigate(`/patches/${slug}/governance`); }}>
          Governance
        </a>
      {/if}
      {#if !isAdmin && node}
        <ReportButton entityType="node" entityId={node.id} entityName={node.name} />
      {/if}
    </div>

    <!-- Upcoming Events -->
    {#if recentEvents.length > 0}
      <section class="profile-section">
        <h3 class="section-title">Upcoming Events</h3>
        {#if isUnclaimed}
          <!-- Every event on an unclaimed patch is community-submitted —
               derived from the patch's status, shown once (docs/adr/026). -->
          <p class="community-note"><span class="badge">Community-submitted</span></p>
        {/if}
        <div class="event-list">
          {#each recentEvents as event (event.id)}
            <a
              href="/events/{event.id}"
              class="event-item"
              onclick={(e) => { e.preventDefault(); navigate(`/events/${event.id}`); }}
            >
              <span class="event-date">{formatDate(event.starts_at)}</span>
              <span class="event-name">{event.title}</span>
              {#if event.location}
                <span class="event-location muted">{event.location}</span>
              {/if}
            </a>
          {/each}
        </div>
      </section>
    {/if}

    <!-- About -->
    <section class="profile-section">
      <h3 class="section-title">About</h3>
      {#if node.website}
        <a href={node.website} class="about-link" target="_blank" rel="noopener">
          {extractDomain(node.website)}
        </a>
      {/if}
      {#if node.links && node.links.length > 0}
        <div class="link-list">
          {#each node.links as link}
            <a href={link.url} class="about-link" target="_blank" rel="noopener">
              {link.label || extractDomain(link.url)}
            </a>
          {/each}
        </div>
      {/if}
      {#if node.address}
        <p class="about-address muted">{node.address}</p>
      {/if}
    </section>

    <!-- Governance Docs (charters) -->
    {#if governanceDocs.length > 0}
      <section class="profile-section">
        <h3 class="section-title">Governance Documents</h3>
        <div class="doc-list">
          {#each governanceDocs as doc (doc.id)}
            <button class="doc-item" onclick={() => { modalType = 'doc'; modalItem = doc; modalOpen = true; }}>
              <span class="doc-title">{doc.title}</span>
              {#if doc.version}
                <span class="doc-version muted">v{doc.version}</span>
              {/if}
            </button>
          {/each}
        </div>
      </section>
    {/if}

    <!-- Recent Proposals -->
    {#if recentProposals.length > 0}
      <section class="profile-section">
        <h3 class="section-title">Recent Proposals</h3>
        <div class="proposal-list">
          {#each recentProposals as proposal (proposal.id)}
            <button class="proposal-item" onclick={() => { modalType = 'proposal'; modalItem = proposal; modalOpen = true; }}>
              <span class="proposal-title">{proposal.title}</span>
              <span class="proposal-status" class:status-open={proposal.status === 'open'} class:status-accepted={proposal.status === 'accepted'} class:status-rejected={proposal.status === 'rejected'}>{proposal.status}</span>
            </button>
          {/each}
        </div>
      </section>
    {/if}

    <!-- Empty state -->
    {#if recentEvents.length === 0 && recentProposals.length === 0 && governanceDocs.length === 0 && !node.website && (!node.links || node.links.length === 0)}
      <div class="profile-empty">
        <p class="muted">
          {#if !isMember}
            Join this patch to participate, or follow to stay updated.
          {:else if membershipRole === 'follower'}
            You're following this patch. Become a member to participate.
          {:else}
            No recent activity yet.
          {/if}
        </p>
      </div>
    {/if}
  {/if}
</div>

<Modal open={modalOpen} label={modalItem?.title ?? 'Details'} onClose={() => { modalOpen = false; modalItem = null; }}>
  {#snippet children()}
    {#if modalItem}
      {#if modalType === 'doc'}
        <h2 class="modal-title">{modalItem.title}</h2>
        {#if modalItem.version}
          <span class="modal-meta">Version {modalItem.version}</span>
        {/if}
        {#if modalItem.body}
          <div class="modal-body">{modalItem.body}</div>
        {:else}
          <p class="muted">No content available.</p>
        {/if}
      {:else if modalType === 'proposal'}
        <h2 class="modal-title">{modalItem.title}</h2>
        <div class="modal-meta-row">
          <span class="proposal-status" class:status-open={modalItem.status === 'open'} class:status-accepted={modalItem.status === 'accepted'} class:status-rejected={modalItem.status === 'rejected'}>{modalItem.status}</span>
          {#if modalItem.created_at}
            <span class="modal-meta">{formatDate(modalItem.created_at)}</span>
          {/if}
        </div>
        {#if modalItem.description}
          <div class="modal-body">{modalItem.description}</div>
        {:else if modalItem.body}
          <div class="modal-body">{modalItem.body}</div>
        {:else}
          <p class="muted">No description available.</p>
        {/if}
      {/if}
    {/if}
  {/snippet}
</Modal>

<style>
  .profile {
    max-width: 560px;
    margin: 0 auto;
    /* Padding comes from SocialShell's .social-main container (issue #17). */
  }

  .profile-loading {
    padding: 3rem 0;
  }

  .skel {
    background: var(--color-overlay);
    border-radius: 4px;
  }

  .profile-error {
    text-align: center;
    padding: 3rem 0;
  }

  .profile-error h2 {
    margin-bottom: 0.5rem;
  }

  /* Header */
  .profile-header {
    text-align: center;
    margin-bottom: 1.5rem;
  }

  /* Cover: a wide band of the patch's own block. Kept short — the block is
     square, so a tall band would eat the fold before any content shows. */
  .profile-cover {
    position: relative;
    /* min-height, not height: a long name on a narrow screen wraps, and the
       band grows with it rather than clipping the title it exists to show. */
    min-height: 150px;
    border-radius: var(--radius);
    overflow: hidden;
    margin-bottom: 0.75rem;
    display: flex;
    align-items: flex-end;
    justify-content: center;
  }

  .cover-scrim {
    position: absolute;
    inset: 0;
    background: linear-gradient(to top, rgba(0, 0, 0, 0.75) 0%, rgba(0, 0, 0, 0.3) 55%, rgba(0, 0, 0, 0) 100%);
  }

  .cover-text {
    position: relative;
    padding: 0.75rem 1rem;
    width: 100%;
  }

  .profile-name {
    font-size: 1.75rem;
    font-weight: 700;
    margin-bottom: 0.1rem;
    color: #fff;
    /* Two layers: a tight halo that survives a near-white fabric, plus a
       softer lift. The scrim carries most of the contrast, but a pale
       bundle leaves the top of a wrapped title with little else. */
    text-shadow: 0 0 4px rgba(0, 0, 0, 0.55), 0 1px 3px rgba(0, 0, 0, 0.5);
    overflow-wrap: anywhere;
  }

  .profile-stats {
    font-size: 0.88rem;
    font-weight: 600;
    color: rgba(255, 255, 255, 0.92);
    text-shadow: 0 0 4px rgba(0, 0, 0, 0.55), 0 1px 3px rgba(0, 0, 0, 0.5);
    margin-bottom: 0;
  }

  @media (max-width: 600px) {
    .profile-cover {
      min-height: 120px;
    }

    .profile-name {
      font-size: 1.4rem;
    }
  }

  .profile-desc {
    font-size: 0.9rem;
    color: var(--color-text-muted);
    line-height: 1.6;
    max-width: 440px;
    margin: 0 auto;
  }

  /* Actions */
  .profile-actions {
    display: flex;
    justify-content: center;
    gap: 0.5rem;
    margin-bottom: 2rem;
    flex-wrap: wrap;
  }

  .banned-notice {
    font-size: 0.85rem;
    color: var(--color-error);
    font-weight: 500;
  }

  /* Sections */
  .profile-section {
    border-top: 1px solid var(--color-border);
    padding: 1.25rem 0;
  }

  .section-title {
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-text-muted);
    margin-bottom: 0.75rem;
    text-align: center;
  }

  /* Events */
  .community-note {
    text-align: center;
    margin-bottom: 0.5rem;
  }

  .event-list {
    display: flex;
    flex-direction: column;
  }

  .event-item {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem 0.5rem;
    text-decoration: none;
    color: var(--color-text);
    border-radius: var(--radius);
    transition: background 100ms ease;
  }

  .event-item:hover {
    background: var(--color-overlay);
    text-decoration: none;
  }

  .event-date {
    font-size: 0.78rem;
    font-weight: 600;
    color: var(--color-primary);
    min-width: 5rem;
    flex-shrink: 0;
  }

  .event-name {
    font-size: 0.88rem;
    font-weight: 500;
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .event-location {
    font-size: 0.78rem;
    flex-shrink: 0;
  }

  /* About */
  .about-link {
    display: block;
    font-size: 0.88rem;
    color: var(--color-primary);
    text-decoration: none;
    padding: 0.2rem 0;
    text-align: center;
  }

  .about-link:hover {
    text-decoration: underline;
  }

  .link-list {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  .about-address {
    font-size: 0.85rem;
    margin-top: 0.5rem;
    text-align: center;
  }

  /* Governance Docs */
  .doc-list {
    display: flex;
    flex-direction: column;
  }

  .doc-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 0.5rem;
    border-radius: var(--radius);
    border: none;
    background: none;
    width: 100%;
    text-align: left;
    cursor: pointer;
    transition: background 100ms ease;
  }

  .doc-item:hover {
    background: var(--color-overlay);
  }

  .doc-title {
    font-size: 0.88rem;
    font-weight: 500;
    color: var(--color-text);
  }

  .doc-version {
    font-size: 0.75rem;
  }

  /* Proposals */
  .proposal-list {
    display: flex;
    flex-direction: column;
  }

  .proposal-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 0.5rem;
    border-radius: var(--radius);
    border: none;
    background: none;
    width: 100%;
    text-align: left;
    cursor: pointer;
    transition: background 100ms ease;
  }

  .proposal-item:hover {
    background: var(--color-overlay);
  }

  /* Modal content */
  .modal-title {
    font-size: 1.2rem;
    font-weight: 700;
    margin-bottom: 0.5rem;
    padding-right: 2rem;
  }

  .modal-meta {
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .modal-meta-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 1rem;
  }

  .modal-body {
    font-size: 0.9rem;
    line-height: 1.7;
    color: var(--color-text);
    margin-top: 1rem;
    white-space: pre-wrap;
  }

  .proposal-title {
    font-size: 0.88rem;
    font-weight: 500;
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .proposal-status {
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    padding: 0.15rem 0.5rem;
    border-radius: 999px;
    color: var(--color-text-muted);
    background: var(--color-overlay);
  }

  .proposal-status.status-open {
    color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 12%, transparent);
  }

  .proposal-status.status-accepted {
    color: var(--color-success);
    background: color-mix(in srgb, var(--color-success) 12%, transparent);
  }

  .proposal-status.status-rejected {
    color: var(--color-error);
    background: color-mix(in srgb, var(--color-error) 12%, transparent);
  }

  /* Empty state */
  .profile-empty {
    text-align: center;
    padding: 2rem 0;
  }
</style>
