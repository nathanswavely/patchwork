<script>
  /**
   * The patch profile (CONTEXT.md "Patch profile", docs/adr/042): a patch's
   * public face, read at a glance.
   *
   * It is a window, not a lobby. Each section is a glimpse of one workspace
   * room and is itself the way into it, so there is no door named for the
   * container — the old `Manage` / `Governance` pill. Items navigate to the
   * surfaces that own them; they used to open a modal, which made every
   * glance terminate in a dialog.
   *
   * Settings is the one room without a glimpse — there is nothing about a
   * patch's own configuration to show the street — so it gets a named door
   * in the header for the people who run the patch. It passes ADR 042's
   * test where `Manage` failed: it names a room, not the container.
   *
   * The one control on the page is the relationship row. State (unclaimed,
   * amended lining) is worn in the header; acts sit beside the thing they
   * act on; rare acts live in the overflow.
   */
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn, isAdmin as isInstanceAdmin, getUser } from '../stores/auth.svelte.js';
  import { getSubmissionsEnabled } from '../stores/quilt.svelte.js';
  import PatchCover from '../components/PatchCover.svelte';
  import PatchRelationship from '../components/PatchRelationship.svelte';
  import PatchOverflow from '../components/PatchOverflow.svelte';
  import { GearSix } from 'phosphor-svelte';
  import { eventPostingRight } from '../lib/patchWorkspace.js';
  import { identityColorForPatch } from '../lib/quiltTheme.js';
  import { formatEventDate, formatEventTime, upcomingFrom } from '../lib/datetime.js';

  // The glimpse shows three, not the five it used to fetch. A stacked row
  // is taller than the clipped one-liner it replaces, and the section
  // heading is now a door (docs/adr/042) — so the fourth and fifth event
  // cost a scroll on a page read at a glance and buy nothing the door
  // doesn't already offer.
  const GLIMPSE_EVENTS = 3;

  let { slug = '' } = $props();

  let node = $state(null);
  let isMember = $state(false);
  let isAdmin = $state(false);
  let membershipRole = $state('');
  let isUnclaimed = $state(false);
  // 'diverged' wears the public "Amended lining" badge (docs/adr/037).
  let liningStatus = $state('');
  let isBanned = $state(false);
  let loading = $state(true);
  let error = $state('');

  let recentEvents = $state([]);
  let members = $state([]);
  let memberTotal = $state(0);
  let recentProposals = $state([]);
  let governanceDocs = $state([]);
  let followerPermissions = $state(null);
  let hasOpenClaim = $state(false);

  // Standing is the membership relationship, never instance-admin power:
  // an instance admin can manage any patch without standing in it.
  let hasStanding = $derived(['follower', 'member', 'admin'].includes(membershipRole));

  let canSeeGovernance = $derived(
    !isUnclaimed && (isMember || isAdmin || followerPermissions?.proposals === true || followerPermissions?.charters === true)
  );

  // What posting an event here would actually do — see eventPostingRight.
  let postingRight = $derived(eventPostingRight({
    signedIn: isLoggedIn(),
    isInstanceAdmin: isInstanceAdmin(),
    trustedContributor: !!getUser()?.trusted_contributor,
    isUnclaimed,
    // Not `isMember`: the node payload sets is_member for followers too.
    isMemberOrAdmin: membershipRole === 'member' || membershipRole === 'admin',
    isBanned,
    submissionsEnabled: getSubmissionsEnabled(),
    acceptSuggestions: node?.accept_event_suggestions === true,
  }));

  /**
   * A glimpse renders when the room has something in it, or when the viewer
   * may act in it, or when they have standing — collapsing only when the
   * room is both empty and inert for them. Without the standing clause a
   * brand-new patch renders zero doors and strands its own admin.
   */
  let showEvents = $derived(recentEvents.length > 0 || hasStanding || isAdmin || postingRight !== 'none');
  let showMembers = $derived(!isUnclaimed && (members.length > 0 || hasStanding || isAdmin));
  let showGovernance = $derived(
    canSeeGovernance && (governanceDocs.length > 0 || recentProposals.length > 0 || hasStanding || isAdmin)
  );
  let showAbout = $derived(!!node?.website || (node?.links?.length ?? 0) > 0 || !!node?.address || !!node?.image_url);

  async function loadClaimState() {
    try {
      const data = await api(`nodes/${slug}/claims/mine`);
      hasOpenClaim = !!data.claim;
    } catch {
      // Non-fatal — the claim page itself is the source of truth.
    }
  }

  $effect(() => {
    if (slug) loadNode();
  });

  // Reactive on auth: on a fresh page load the session check may still be
  // in flight when the node arrives, so gating this inside loadNode() would
  // race and miss the open claim.
  $effect(() => {
    hasOpenClaim = false;
    if (slug && isUnclaimed && isLoggedIn()) loadClaimState();
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
      liningStatus = data.lining_status || '';
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
    // Unclaimed patches carry no governance and no membership (docs/adr/039)
    // — absence, not an empty state — so neither fetch runs for one.
    const wantGovernance = !isUnclaimed && (isMember || isAdmin || followerPermissions?.proposals === true || followerPermissions?.charters === true);
    const [eventData, memberData, proposalData, charterData] = await Promise.all([
      api(`events?node_slug=${encodeURIComponent(slug)}&from=${encodeURIComponent(upcomingFrom())}&limit=${GLIMPSE_EVENTS}`).catch(() => ({ items: [] })),
      (isUnclaimed ? Promise.resolve({ items: [] }) : api(`nodes/${slug}/members?limit=12`)).catch(() => ({ items: [] })),
      (wantGovernance ? api(`nodes/${slug}/proposals?limit=3`) : Promise.resolve({ items: [] })).catch(() => ({ items: [] })),
      (wantGovernance ? api(`nodes/${slug}/governance`) : Promise.resolve({ items: [] })).catch(() => ({ items: [] })),
    ]);
    recentEvents = eventData.items || eventData || [];
    // Admins plus members, never followers (CONTEXT.md "Member count"). The
    // endpoint hands insiders the follower rows too, which belong to the
    // members room's own page, not to a glimpse headed "Members".
    members = (memberData.items || memberData || []).filter((m) => m.role !== 'follower');
    memberTotal = node?.member_count ?? members.length;
    recentProposals = proposalData.items || proposalData || [];
    governanceDocs = charterData.items || charterData || [];
  }

  function go(path) {
    return (e) => { e.preventDefault(); navigate(path); };
  }

  function extractDomain(url) {
    try { return new URL(url).hostname.replace(/^www\./, ''); }
    catch { return url; }
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
      <a href="/" class="btn btn-secondary" onclick={go('/')}>Back to Quilt</a>
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
            <!-- Both halves are server totals. The events half used to be
                 recentEvents.length — a capped page of rows reported as a
                 count, so a venue with forty shows advertised five
                 (CONTEXT.md "Upcoming events"). -->
            {isUnclaimed ? `${node.follower_count || 0} Following` : `${node.member_count || 0} Members`} &middot; {node.upcoming_event_count || 0} Upcoming Events
          </p>
        </div>
        <div class="cover-actions">
          <!-- The room that has no glimpse. Every other workspace surface is
               entered through the section that previews it, but there is
               nothing about a patch's own settings to preview, so the people
               who run the patch would otherwise have to leave the page to
               get at it. `Settings` names a room, which is the test a door
               on this page has to pass (docs/adr/042). -->
          {#if isAdmin}
            <a
              class="cover-settings"
              href="/patches/{slug}/settings"
              onclick={go(`/patches/${slug}/settings`)}
            >
              <GearSix size={15} weight="duotone" />
              <span>Settings</span>
            </a>
          {/if}
          <PatchOverflow {slug} {node} {isAdmin} {isUnclaimed} {hasStanding} />
        </div>
      </div>

      {#if node.description}
        <p class="profile-desc">{node.description}</p>
      {/if}

      <!-- State is worn in the header, never disguised as an action. The
           unclaimed fact used to be visible only as a blue button, so the
           act shouted while the fact stayed silent (docs/adr/042). -->
      {#if isUnclaimed}
        <p class="state-notice">
          No one runs this patch yet.
          {#if isAdmin}
            <a href="/admin/claims" onclick={go('/admin/claims')}>Review claims</a>.
          {:else if hasOpenClaim}
            <a href="/patches/{slug}/claim" onclick={go(`/patches/${slug}/claim`)}>Your claim is in progress</a>.
          {:else}
            If it's yours, <a href="/patches/{slug}/claim" onclick={go(`/patches/${slug}/claim`)}>claim it</a>.
          {/if}
        </p>
      {/if}

      {#if !isUnclaimed && liningStatus === 'diverged'}
        <!-- Public by design (docs/adr/037): this patch amended the shared
             baseline, and the divergence is worn, not whispered. -->
        <p class="amended-lining-row">
          <a
            href="/patches/{slug}/governance"
            class="amended-lining-badge"
            title="This patch changed the shared community standards every patch starts with. Read its version in Governance."
            onclick={go(`/patches/${slug}/governance`)}
          >Amended lining</a>
        </p>
      {/if}
    </div>

    <!-- The relationship row: standing, and the next rung. Nothing else. -->
    <div class="profile-actions">
      <PatchRelationship
        {slug}
        {node}
        {isUnclaimed}
        {isBanned}
        {membershipRole}
        {liningStatus}
        onChanged={loadNode}
      />
    </div>

    <!-- Glimpses: one per room, each its own door.
         About sits first: it says what this patch *is*, and the events
         under it read differently once you know. ADR 042 originally led
         with Events on the argument that a stranger off a flyer wants
         what's on tonight — About is short enough (a link or two and a
         line of address) that it costs almost no scroll to answer "what
         am I looking at" first. -->
    {#if showAbout}
      <section class="profile-section">
        <h3 class="section-title static">About</h3>
        <!-- The patch's own picture, held wherever it keeps it
             (docs/adr/007). Above the links because it answers "what am I
             looking at" faster than a domain name does. -->
        {#if node.image_url}
          <img class="patch-image" src={node.image_url} alt={node.image_alt} loading="lazy" />
        {/if}
        {#if node.website}
          <a href={node.website} class="about-link" target="_blank" rel="noopener">{extractDomain(node.website)}</a>
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
    {/if}

    {#if showEvents}
      <section class="profile-section">
        <div class="section-head">
          <a class="section-title" href="/patches/{slug}/events" onclick={go(`/patches/${slug}/events`)}>Events</a>
          {#if postingRight !== 'none'}
            <a
              class="section-action"
              href="/events/new?node={slug}"
              onclick={go(`/events/new?node=${slug}`)}
            >{postingRight === 'direct' ? 'New event' : 'Suggest an event'}</a>
          {/if}
        </div>
        {#if isUnclaimed && recentEvents.length > 0}
          <!-- Every event on an unclaimed patch is community-submitted —
               derived from the patch's status, shown once (docs/adr/026). -->
          <p class="community-note"><span class="badge">Community-submitted</span></p>
        {/if}
        {#if recentEvents.length > 0}
          <div class="event-list">
            {#each recentEvents as event (event.id)}
              <!-- Stacked, not one line. Title and location used to compete
                   for a single row where location never yielded, so a full
                   postal address squeezed the title to one letter. Nothing
                   here can clip the other: the column is fixed, and the
                   block owns the rest. -->
              <a href="/events/{event.id}" class="event-item" onclick={go(`/events/${event.id}`)}>
                <span class="event-when">
                  <span class="event-date">{formatEventDate(event.starts_at)}</span>
                  <span class="event-time">{formatEventTime(event.starts_at)}</span>
                </span>
                <span class="event-info">
                  <span class="event-name">{event.title}</span>
                  {#if event.location}
                    <!-- Clamped to one line. A location is name-first
                         (docs/adr/046), so the ellipsis eats the postal
                         tail and keeps the venue. -->
                    <span class="event-location muted">{event.location}</span>
                  {/if}
                </span>
              </a>
            {/each}
          </div>
        {:else}
          <p class="glimpse-empty muted">No upcoming events.</p>
        {/if}
      </section>
    {/if}

    <!-- Members: the public list ADR 006 designed, which the profile never
         showed. Hidden memberships are filtered server-side. -->
    {#if showMembers}
      <section class="profile-section">
        <div class="section-head">
          <a class="section-title" href="/patches/{slug}/members" onclick={go(`/patches/${slug}/members`)}>Members</a>
          {#if members.length > 0 && memberTotal > members.length}
            <span class="section-meta muted">{memberTotal}</span>
          {/if}
        </div>
        {#if members.length > 0}
          <div class="member-list">
            {#each members as m (m.id)}
              <a href="/users/{m.username}" class="member-chip" onclick={go(`/users/${m.username}`)} title="{m.display_name || m.username} · {m.role}">
                <span class="member-avatar">
                  {#if m.avatar_url}
                    <img src={m.avatar_url} alt="" />
                  {:else}
                    {(m.display_name || m.username || '?')[0].toUpperCase()}
                  {/if}
                </span>
                <span class="member-name">{m.display_name || m.username}</span>
              </a>
            {/each}
          </div>
        {:else}
          <p class="glimpse-empty muted">No members yet.</p>
        {/if}
      </section>
    {/if}

    {#if showGovernance}
      <section class="profile-section">
        <div class="section-head">
          <a class="section-title" href="/patches/{slug}/governance" onclick={go(`/patches/${slug}/governance`)}>Governance</a>
        </div>
        {#if governanceDocs.length > 0 || recentProposals.length > 0}
          <div class="doc-list">
            {#each governanceDocs as doc (doc.id)}
              <a
                class="row-item"
                href="/patches/{slug}/governance/docs/{doc.id}"
                onclick={go(`/patches/${slug}/governance/docs/${doc.id}`)}
              >
                <span class="row-title">{doc.title}</span>
                {#if doc.version}<span class="row-meta muted">v{doc.version}</span>{/if}
              </a>
            {/each}
            {#each recentProposals as proposal (proposal.id)}
              <a
                class="row-item"
                href="/patches/{slug}/governance/{proposal.id}"
                onclick={go(`/patches/${slug}/governance/${proposal.id}`)}
              >
                <span class="row-title">{proposal.title}</span>
                <span
                  class="proposal-status"
                  class:status-open={proposal.status === 'open'}
                  class:status-accepted={proposal.status === 'accepted'}
                  class:status-rejected={proposal.status === 'rejected'}
                >{proposal.status}</span>
              </a>
            {/each}
          </div>
        {:else}
          <p class="glimpse-empty muted">Nothing recorded yet.</p>
        {/if}
      </section>
    {/if}
  {/if}
</div>

<style>
  .profile {
    max-width: var(--pw-measure-narrow);
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

  /* The header's acts ride the cover's top-right corner: present, never
     competing with the name. */
  .cover-actions {
    position: absolute;
    top: 6px;
    right: 6px;
    z-index: 2;
    display: flex;
    align-items: center;
    gap: 0.25rem;
    color: #fff;
  }

  .cover-actions :global(.overflow-trigger) {
    color: rgba(255, 255, 255, 0.9);
  }

  .cover-actions :global(.overflow-trigger):hover {
    color: #fff;
    background: rgba(0, 0, 0, 0.3);
  }

  /* Carries its own scrim: the corner sits at the pale end of the cover
     gradient, and a bundle can put near-white fabric directly under it. */
  .cover-settings {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.25rem 0.55rem;
    font-size: 0.8rem;
    font-weight: 600;
    line-height: 1.4;
    color: #fff;
    background: rgba(0, 0, 0, 0.45);
    border-radius: var(--radius);
    text-decoration: none;
    white-space: nowrap;
    transition: background 150ms ease;
  }

  .cover-settings:hover {
    background: rgba(0, 0, 0, 0.65);
    text-decoration: none;
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

  /* State notice: a line, not a box. "Interruption" is a closed category
     for things loud on purpose (docs/adr/038); unclaimed is a state. */
  .state-notice {
    font-size: 0.85rem;
    color: var(--color-text-muted);
    margin-top: 0.6rem;
  }

  .amended-lining-row {
    text-align: center;
    margin-top: 0.5rem;
  }

  .amended-lining-badge {
    display: inline-block;
    font-size: 0.7rem;
    letter-spacing: 0.03em;
    text-transform: uppercase;
    padding: 0.15rem 0.5rem;
    border-radius: 999px;
    border: 1px solid var(--color-warning, #b5892e);
    color: var(--color-warning, #b5892e);
    text-decoration: none;
  }

  .amended-lining-badge:hover {
    background: color-mix(in srgb, var(--color-warning, #b5892e) 10%, transparent);
  }

  /* The relationship row */
  .profile-actions {
    display: flex;
    justify-content: center;
    margin-bottom: 2rem;
  }

  /* Sections */
  .profile-section {
    border-top: 1px solid var(--color-border);
    padding: 1.25rem 0;
  }

  .section-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.75rem;
    margin-bottom: 0.75rem;
  }

  .section-title {
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-text-muted);
    text-decoration: none;
  }

  /* A heading that is a door gets to look like one on hover; About is the
     one section that names identity rather than a room, so it stays inert. */
  a.section-title:hover {
    color: var(--color-text);
    text-decoration: underline;
  }

  .section-title.static {
    display: block;
    margin-bottom: 0.75rem;
  }

  .section-action,
  .section-meta {
    font-size: 0.78rem;
    font-weight: 500;
    color: var(--color-primary);
    text-decoration: none;
    flex-shrink: 0;
  }

  .section-action:hover {
    text-decoration: underline;
  }

  .glimpse-empty {
    font-size: 0.85rem;
    padding: 0.25rem 0;
  }

  /* Events */
  .community-note {
    margin-bottom: 0.5rem;
  }

  .event-list,
  .doc-list {
    display: flex;
    flex-direction: column;
  }

  .event-item,
  .row-item {
    display: flex;
    gap: 0.75rem;
    padding: 0.5rem;
    text-decoration: none;
    color: var(--color-text);
    border-radius: var(--radius);
    transition: background 100ms ease;
  }

  .event-item:hover,
  .row-item:hover {
    background: var(--color-overlay);
    text-decoration: none;
  }

  /* Governance rows stay one line: a charter title and a status chip fit,
     and nothing there carries an address. */
  .row-item {
    align-items: center;
    justify-content: space-between;
  }

  .row-title {
    font-size: 0.88rem;
    font-weight: 500;
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* An event row is two columns, each stacked. Baseline alignment puts the
     date on the title's first line rather than centring a two-line column
     against a two-line block. */
  .event-item {
    align-items: baseline;
  }

  .event-when {
    display: flex;
    flex-direction: column;
    min-width: 5rem;
    flex-shrink: 0;
  }

  .event-date {
    font-size: 0.78rem;
    font-weight: 600;
    color: var(--color-primary);
  }

  .event-time {
    font-size: 0.72rem;
    color: var(--color-text-muted);
  }

  /* min-width: 0 is what lets the children below actually ellipsize — a
     flex item's default min-width: auto refuses to shrink past its
     content, which is how the old single-line row got clipped instead. */
  .event-info {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
  }

  .event-name {
    font-size: 0.88rem;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .event-location {
    font-size: 0.78rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .row-meta {
    font-size: 0.75rem;
    flex-shrink: 0;
  }

  /* About */
  .patch-image {
    display: block;
    width: 100%;
    height: auto;
    border-radius: var(--radius);
    margin-bottom: 0.75rem;
  }

  .about-link {
    display: block;
    font-size: 0.88rem;
    color: var(--color-primary);
    text-decoration: none;
    padding: 0.2rem 0;
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
  }

  /* Members */
  .member-list {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }

  .member-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.25rem 0.55rem 0.25rem 0.25rem;
    border: 1px solid var(--color-border);
    border-radius: 999px;
    text-decoration: none;
    color: var(--color-text);
    transition: background 100ms ease;
  }

  .member-chip:hover {
    background: var(--color-overlay);
    text-decoration: none;
  }

  .member-avatar {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    overflow: hidden;
    flex-shrink: 0;
    font-size: 0.7rem;
    font-weight: 700;
    color: var(--color-text-muted);
    background: var(--color-overlay);
  }

  .member-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .member-name {
    font-size: 0.8rem;
    font-weight: 500;
    max-width: 9rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Proposals */
  .proposal-status {
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    padding: 0.15rem 0.5rem;
    border-radius: 999px;
    color: var(--color-text-muted);
    background: var(--color-overlay);
    flex-shrink: 0;
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
</style>
