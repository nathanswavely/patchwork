<script>
  /**
   * The patch profile's glimpses (CONTEXT.md "Patch profile", docs/adr/042,
   * docs/adr/094): one section per workspace room, each a preview of the
   * room and itself the way in. The other side of the profile's seam —
   * head, then these.
   *
   * The four fetches live here rather than beside the head's, which is what
   * lets a docked profile defer them: a sheet at rest has a head and no
   * glimpses, and mounting this component is what a pull costs. They used
   * to run from inside the page's own `loadNode`, so there was no way to
   * have the patch without also having its rooms.
   *
   * The standing facts arrive as props from the head's one fetch rather
   * than being asked for again — `wantGovernance` and every `show*` gate
   * below reads them, and a second `nodes/:slug` per profile would be the
   * cost this split exists to avoid.
   */
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn, isAdmin as isInstanceAdmin, getUser } from '../stores/auth.svelte.js';
  import { getSubmissionsEnabled } from '../stores/quilt.svelte.js';
  import { eventPostingRight } from '../lib/patchWorkspace.js';
  import { handleFromDID } from '../lib/atproto.js';
  import { formatEventDate, formatEventTime, upcomingFrom } from '../lib/datetime.js';

  // The glimpse shows three, not the five it used to fetch. A stacked row
  // is taller than the clipped one-liner it replaces, and the section
  // heading is now a door (docs/adr/042) — so the fourth and fifth event
  // cost a scroll on a page read at a glance and buy nothing the door
  // doesn't already offer.
  const GLIMPSE_EVENTS = 3;

  let {
    slug = '',
    node = null,
    // Standing as the node payload reports it — true for followers too, which
    // is why every gate below reads `membershipRole` instead. Kept in the
    // bundle the containers hand over.
    isMember = false,
    isAdmin = false,
    isUnclaimed = false,
    isBanned = false,
    membershipRole = '',
    // Taken, and deliberately not read for governance. Follower permissions
    // set what a patch's *workspace* offers a follower (docs/adr/050); they
    // never decide what the patch's public face shows, and the containers
    // that mount this pass the whole standing bundle.
    followerPermissions = null,
    // Whether to go and ask the rooms. A sheet at rest mounts these so the
    // first section shows under the fold — the cut is what says there is
    // more — but a tap on the quilt must stay one request (docs/adr/094
    // decision 4), so the four fetches wait for the pull. About needs no
    // fetch and renders either way.
    active = true,
  } = $props();

  let recentEvents = $state([]);
  // Whether the rooms have answered. An empty state before they have would
  // report "no events" about a patch nobody has asked yet.
  let loaded = $state(false);
  let members = $state([]);
  let memberTotal = $state(0);
  // What this patch publishes (docs/adr/095), read off the listing that
  // applied it. Without it a follower — an outsider for this purpose — gets
  // the standing clause below, an empty array, and a glimpse announcing
  // "No members yet" about a patch with forty people in it.
  let publicMemberList = $state('everyone');
  let recentProposals = $state([]);
  let governanceDocs = $state([]);
  // Whether the governance list this viewer got held only what the patch
  // published. Read off the listing that applied the rule, so the empty
  // state can say which kind of empty it is without counting anything it
  // was not shown.
  let governancePublishedOnly = $state(false);

  // Standing is the membership relationship, never instance-admin power:
  // an instance admin can manage any patch without standing in it.
  let hasStanding = $derived(['follower', 'member', 'admin'].includes(membershipRole));

  // Governance is asked about on every claimed patch, for every viewer, and
  // the server decides what comes back: a document published to everyone
  // reaches everyone (docs/adr/036), and proposals are a public read
  // (docs/adr/050). This used to require `followerPermissions.charters` or
  // `.proposals`, which meant the Minimal template's two `false`s hid the
  // whole section from a signed-out visitor — so a patch that had
  // deliberately published its minutes showed a stranger nothing about
  // documents, and the sentence promising "posted here" led nowhere (F-052).
  // Unclaimed patches carry no governance at all (docs/adr/039): absence,
  // not an empty room.
  let canSeeGovernance = $derived(!isUnclaimed);

  // What posting an event here would actually do — see eventPostingRight.
  let postingRight = $derived(eventPostingRight({
    signedIn: isLoggedIn(),
    isInstanceAdmin: isInstanceAdmin(),
    trustedContributor: !!getUser()?.trusted_contributor,
    isUnclaimed,
    // The role test, stated here rather than taken from the isMember prop:
    // it is the same answer since docs/adr/2026-09-17-a-follower-is-not-a-quieter-member.md, and the gate should not
    // depend on which of the two a container happened to hand over.
    isMemberOrAdmin: membershipRole === 'member' || membershipRole === 'admin',
    isBanned,
    submissionsEnabled: getSubmissionsEnabled(),
    acceptSuggestions: node?.accept_event_suggestions === true,
    hasMoved: !!node?.moved_to,
  }));

  /**
   * A glimpse renders when the room has something in it, or when the viewer
   * may act in it, or when they have standing — collapsing only when the
   * room is both empty and inert for them. Without the standing clause a
   * brand-new patch renders zero doors and strands its own admin.
   */
  let showEvents = $derived(recentEvents.length > 0 || hasStanding || isAdmin || postingRight !== 'none');
  // Whoever is not in the room reads the patch's public answer, instance
  // admins excepted — they see every room already, everywhere.
  let rosterInsider = $derived(isAdmin || membershipRole === 'member' || membershipRole === 'admin');
  let rosterWithheld = $derived(!rosterInsider && publicMemberList === 'nobody');
  let rosterAdminsOnly = $derived(!rosterInsider && publicMemberList === 'admins');
  // A withheld list collapses the glimpse rather than showing an empty one:
  // the door it would be is the members page, and that page says why.
  let showMembers = $derived(!isUnclaimed && !rosterWithheld && (members.length > 0 || hasStanding || isAdmin));
  let showGovernance = $derived(
    canSeeGovernance && (governanceDocs.length > 0 || recentProposals.length > 0 || hasStanding || isAdmin)
  );
  // The atproto handle a claim proved (docs/adr/062), read off the DID
  // rather than the domain column — see lib/atproto.js for why. It counts
  // toward About on its own: a patch whose only public fact is its handle
  // still has something to say about what it is.
  let atprotoHandle = $derived(handleFromDID(node?.did));

  // What this capped list is not showing. Read off the server's count
  // rather than off a second fetch: upcoming_event_count is counted under
  // exactly the gates GET /api/v1/events applies, so the two can only
  // disagree by the page size — which is the whole gap this reports.
  let moreEvents = $derived(
    Math.max(0, (node?.upcoming_event_count ?? recentEvents.length) - recentEvents.length)
  );
  let showAbout = $derived(!!node?.website || (node?.links?.length ?? 0) > 0 || !!node?.address || !!node?.image_url || !!atprotoHandle);

  // Keyed on the slug and on whether governance is readable, because the
  // head's payload can arrive after this mounts: a container may render the
  // glimpses the moment a reader pulls, with standing still in flight.
  $effect(() => {
    const s = slug;
    // Read so the effect re-runs when standing arrives: the head's payload
    // can land after a pull has already mounted this.
    const gov = canSeeGovernance;
    if (s && active) loadActivity(gov);
  });

  async function loadActivity(wantGovernance) {
    // Unclaimed patches carry no governance and no membership (docs/adr/039)
    // — absence, not an empty state — so neither fetch runs for one.
    const asked = slug;
    loaded = false;
    const [eventData, memberData, proposalData, charterData] = await Promise.all([
      api(`events?node_slug=${encodeURIComponent(slug)}&from=${encodeURIComponent(upcomingFrom())}&limit=${GLIMPSE_EVENTS}`).catch(() => ({ items: [] })),
      (isUnclaimed ? Promise.resolve({ items: [] }) : api(`nodes/${slug}/members?limit=12`)).catch(() => ({ items: [] })),
      (wantGovernance ? api(`nodes/${slug}/proposals?limit=3`) : Promise.resolve({ items: [] })).catch(() => ({ items: [] })),
      (wantGovernance ? api(`nodes/${slug}/governance`) : Promise.resolve({ items: [] })).catch(() => ({ items: [] })),
    ]);
    // A page only ever asks about one patch; a docked profile can be handed
    // another one mid-flight, and four rooms from the patch the reader left
    // would land under the patch they chose.
    if (asked !== slug) return;
    recentEvents = eventData.items || eventData || [];
    // Admins plus members, never followers (CONTEXT.md "Member count"). The
    // endpoint hands insiders the follower rows too, which belong to the
    // members room's own page, not to a glimpse headed "Members".
    members = (memberData.items || memberData || []).filter((m) => m.role !== 'follower');
    memberTotal = node?.member_count ?? members.length;
    publicMemberList = memberData.public_member_list || 'everyone';
    recentProposals = proposalData.items || proposalData || [];
    governanceDocs = charterData.items || charterData || [];
    governancePublishedOnly = charterData.published_only === true;
    loaded = true;
  }

  function go(path) {
    return (e) => { e.preventDefault(); navigate(path); };
  }

  function extractDomain(url) {
    try { return new URL(url).hostname.replace(/^www\./, ''); }
    catch { return url; }
  }

  // What the pill calls a proposal's outcome, read off `state` before
  // `status` the way the proposals list does.
  //
  // Two outcomes carry `status = 'rejected'` without anybody having rejected
  // anything: a vote whose window closed under quorum (lapsed, docs/adr/097)
  // and an election that seated nobody (unsettled, holdover — docs/adr/051).
  // The schema's CHECK has no word for either, so printing the column put
  // REJECTED in error red under the name of a candidate the patch had simply
  // not voted on. The pill is the link's accessible name too, so the word is
  // the whole fix on both counts.
  function outcomeWord(p) {
    if (p.state === 'lapsed') return 'lapsed';
    if (p.state === 'unsettled') return 'unsettled';
    return p.status;
  }
</script>

{#if node}
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
      <!-- The handle is written past tense on purpose (docs/adr/062):
           the binding was checked once, when the claim was verified, and
           nothing re-checks it afterwards. A checkmark or a present-tense
           "verified" badge would promise a check that is not running. -->
      {#if atprotoHandle}
        <p class="about-handle">
          <span class="handle">@{atprotoHandle}</span>
          <span class="handle-note muted">atproto handle, proved when this patch was claimed</span>
        </p>
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
                <span class="event-date">{formatEventDate(event.starts_at, event.timezone)}</span>
                <span class="event-time">{formatEventTime(event.starts_at, event.timezone)}</span>
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
        {#if moreEvents > 0}
          <!--
            The head states the patch's whole upcoming count and this list
            is capped at three, so a choir with four rehearsals read
            "4 Upcoming Events" over three of them and said nothing about
            the fourth. The count is the right number — a venue with forty
            shows must not advertise three — so the glimpse is what has to
            admit it is a glimpse.
          -->
          <a
            class="glimpse-more"
            href="/patches/{slug}/events"
            onclick={go(`/patches/${slug}/events`)}
          >{moreEvents} more upcoming</a>
        {/if}
      {:else if loaded}
        <p class="glimpse-empty muted">No upcoming events.</p>
      {/if}
    </section>
  {/if}

  <!-- Members: the public list ADR 006 designed, which the profile never
       showed. Hidden memberships are filtered server-side. -->
  {#if showMembers}
    <section class="profile-section">
      <div class="section-head">
        <a class="section-title" href="/patches/{slug}/members" onclick={go(`/patches/${slug}/members`)}>{rosterAdminsOnly ? 'Admins' : 'Members'}</a>
        <!-- The total belongs to a section headed Members. Beside "Admins"
             it would count people the list below deliberately omits. -->
        {#if !rosterAdminsOnly && members.length > 0 && memberTotal > members.length}
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
      {:else if loaded}
        <p class="glimpse-empty muted">No members yet.</p>
      {/if}
    </section>
  {/if}

  {#if showGovernance}
    <section class="profile-section">
      <div class="section-head">
        <a class="section-title" href="/patches/{slug}/governance" onclick={go(`/patches/${slug}/governance`)}>Governance</a>
        <!-- The named door a patch's own blurb points at when it says the
             minutes are posted here. It renders only when this viewer has at
             least one document to open, so a patch that has published
             nothing never grows a door onto an empty room (docs/adr/042:
             every door names a room, and no false ones). -->
        {#if governanceDocs.length > 0}
          <a
            class="section-action"
            href="/patches/{slug}/governance/docs"
            onclick={go(`/patches/${slug}/governance/docs`)}
          >Documents</a>
        {/if}
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
            {@const outcome = outcomeWord(proposal)}
            <a
              class="row-item"
              href="/patches/{slug}/governance/{proposal.id}"
              onclick={go(`/patches/${slug}/governance/${proposal.id}`)}
            >
              <span class="row-title">{proposal.title}</span>
              <!-- Red is for a decision the patch made. Lapsed and unsettled
                   are absences, so they keep the pill's muted default. -->
              <span
                class="proposal-status"
                class:status-open={outcome === 'open'}
                class:status-accepted={outcome === 'accepted'}
                class:status-rejected={outcome === 'rejected'}
              >{outcome}</span>
            </a>
          {/each}
        </div>
      {:else if loaded}
        <!-- Two kinds of empty, two sentences. A viewer who is shown only
             what the patch published must not read "nothing recorded" and
             take it for the patch's whole record. -->
        <p class="glimpse-empty muted">
          {governancePublishedOnly ? 'Nothing published yet.' : 'Nothing recorded yet.'}
        </p>
      {/if}
    </section>
  {/if}
{/if}

<style>
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

  .glimpse-more {
    display: inline-block;
    margin-top: 0.4rem;
    font-size: 0.85rem;
    color: var(--color-text-muted);
    text-decoration: none;
  }

  .glimpse-more:hover {
    color: var(--color-text);
    text-decoration: underline;
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

  .about-handle {
    margin-top: 0.5rem;
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.4rem;
  }

  .about-handle .handle {
    font-size: 0.88rem;
    font-family: var(--font-mono, ui-monospace, monospace);
    word-break: break-all;
  }

  .about-handle .handle-note {
    font-size: 0.78rem;
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
