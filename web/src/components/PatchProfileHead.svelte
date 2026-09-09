<script>
  /**
   * The patch profile's head (CONTEXT.md "Patch profile", "Docked profile",
   * docs/adr/094): cover, name, counts, the state a patch wears, and the
   * relationship row. One side of the seam the profile already had —
   * header then glimpses — which is what lets a docked profile rest at its
   * head and pull up to the whole thing.
   *
   * Three containers mount this: the page, the panel in the cards pane's
   * slot, and the sheet at the foot of a phone. It is the same rendering in
   * all three, because a patch has one face and it cannot drift.
   *
   * `seed` is a quilt row (`nodes/tree`) from a surface that already has
   * one, and it exists to paint the first frame — a tap on a tile must not
   * wait on a request. The contract is that a seed may only fill in what
   * the surface already knows: everything below reads a seeded field only
   * where `TreeNode` carries it. `PatchRelationship` is safe to hand the
   * seed directly because its three reads — name, moved_to,
   * membership_policy — are all tree-carried (the last one added for this,
   * docs/adr/094 decision 8). What the tree cannot know waits for the
   * payload and renders nothing until it lands: the ban that suppresses a
   * rung, an open claim, and the upcoming-event count, which must never be
   * substituted with the all-time one the tree carries (CONTEXT.md
   * "Upcoming events" — the two are never labelled with each other's word).
   */
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import { isLoggedIn, isAdmin as isInstanceAdmin } from '../stores/auth.svelte.js';
  import PatchCover from './PatchCover.svelte';
  import PatchRelationship from './PatchRelationship.svelte';
  import PatchOverflow from './PatchOverflow.svelte';
  import MovedNotice from './MovedNotice.svelte';
  import { getPendingMembershipSlugs, getMembershipRoles, loadMemberships } from '../stores/memberships.svelte.js';
  import { GearSix } from 'phosphor-svelte';
  import { identityColorForPatch } from '../lib/quiltTheme.js';

  // `layout` is the container's word about the room, never about the patch:
  // 'docked' is a box the profile fills — the sheet at the foot of a phone,
  // the card in the cards pane's slot — where the cover bleeds to the box's
  // own edges, the text sits left, and the relationship row's controls fill
  // the width. Same cover, same name, same row — a patch has one face —
  // laid out for a box narrower than a page.
  let { slug = '', seed = null, onLoaded = () => {}, layout = 'page' } = $props();

  // The payload half. Held in one object rather than eight flags so that
  // "has the truth arrived" is one question, which is what every derived
  // below asks before preferring a seeded answer.
  let loaded = $state(null);
  let error = $state('');
  let hasOpenClaim = $state(false);

  let node = $derived(loaded?.node ?? seed);
  // A seed is enough to draw, so only a head with neither is loading.
  let loading = $derived(!node && !error);

  let isUnclaimed = $derived(loaded ? loaded.isUnclaimed : !!seed?.is_unclaimed);
  // The tree carries the badge as a boolean; the payload as a status
  // string. Same fact, and 'diverged' is the only value that draws it.
  let liningStatus = $derived(loaded ? loaded.liningStatus : (seed?.amended_lining ? 'diverged' : ''));
  // Standing from the store until the payload lands — it already refuses to
  // report a pending request as a role (docs/adr/088). An instance admin
  // holds no membership, so their Settings door arrives with the payload.
  let storeRole = $derived(getMembershipRoles().get(slug) || '');
  let membershipRole = $derived(loaded ? loaded.membershipRole : storeRole);
  let isAdmin = $derived(loaded ? loaded.isAdmin : storeRole === 'admin');
  // Only the payload knows a viewer is banned from this patch: me/nodes
  // sends active and pending rows, never banned ones. So the relationship
  // row is right for everyone else on the first frame, and right for a
  // banned viewer a moment later.
  let isBanned = $derived(loaded ? loaded.isBanned : false);
  let hasStanding = $derived(['follower', 'member', 'admin'].includes(membershipRole));
  let requestPending = $derived(getPendingMembershipSlugs().has(slug));

  $effect(() => {
    if (slug) loadNode();
  });

  // Reactive on auth: on a fresh load the session check may still be in
  // flight when the node arrives, so gating this inside load() would race
  // and miss the open claim.
  $effect(() => {
    hasOpenClaim = false;
    if (slug && isUnclaimed && isLoggedIn()) loadClaimState();
  });

  async function loadClaimState() {
    try {
      const data = await api(`nodes/${slug}/claims/mine`);
      hasOpenClaim = !!data.claim;
    } catch {
      // Non-fatal — the claim page itself is the source of truth.
    }
  }

  async function loadNode() {
    error = '';
    try {
      const data = await api(`nodes/${slug}`);
      loaded = {
        node: data.node || data,
        isMember: data.is_member || false,
        isAdmin: data.is_admin || false,
        membershipRole: data.membership_role || '',
        isUnclaimed: data.is_unclaimed || false,
        liningStatus: data.lining_status || '',
        isBanned: data.is_banned || false,
        followerPermissions: (data.node || data).follower_permissions || null,
      };
      // The glimpses need the standing facts this fetch carries, and one
      // fetch is the point: they take them as props rather than asking
      // again (docs/adr/094 decision 9).
      onLoaded(loaded);
    } catch (e) {
      error = e.message || 'Failed to load patch';
      loaded = null;
    }
  }

  // Joining or following changes two things the head reads: the node
  // payload's membership_role, and the memberships store a pending request
  // lives in. Refresh both, or asking to join leaves the row offering to
  // join again until the next full load.
  async function reloadStanding() {
    await Promise.all([loadNode(), loadMemberships()]);
  }

  function go(path) {
    return (e) => { e.preventDefault(); navigate(path); };
  }
</script>

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
  <div class="profile-head" class:docked={layout === 'docked'}>
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
               (CONTEXT.md "Upcoming events").

               The people half is on the quilt row, so a seeded head
               states it on the first frame. The upcoming half is not,
               and the all-time `event_count` sitting beside it on that
               row must not stand in for it — the two are never labelled
               with each other's word — so it joins the line when the
               payload lands rather than arriving wrong and correcting
               itself. -->
          {isUnclaimed ? `${node.follower_count || 0} Following` : `${node.member_count || 0} Members`}{#if node.upcoming_event_count !== undefined}{` · ${node.upcoming_event_count} Upcoming Events`}{/if}
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

    <!-- Where this patch went, said at the top (docs/adr/090). Above the
         description because a reader who has landed on a patch the
         community has left needs the forwarding address before they need
         the blurb, and above the relationship row because joining is the
         thing this banner is answering. -->
    {#if node.moved_to}
      <MovedNotice url={node.moved_to} subject="patch" />
    {/if}

    {#if node.description}
      <p class="profile-desc">{node.description}</p>
    {/if}

    <!-- State is worn in the header, never disguised as an action. The
         unclaimed fact used to be visible only as a blue button, so the
         act shouted while the fact stayed silent (docs/adr/042).
         Provenance is the half this page cannot show on its own: "no one
         runs this" says nobody is in charge, not that the community put
         the listing here, so a visitor meeting it cold reads the page as
         the group's own and the quilt as vouching for it. The clause
         links to About ("Where patches come from") because a signed-in
         visitor has no other standing path there. -->
    {#if isUnclaimed}
      <p class="state-notice">
        No one runs this patch yet.
        <a href="/about" class="provenance-link" onclick={go('/about')}>The community added it</a>.
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
      {requestPending}
      {liningStatus}
      onChanged={reloadStanding}
    />
  </div>
  </div>
{/if}

<style>
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
  }

  /* Colored on the trigger itself, never on this container: the overflow
     lives here (docs/adr/042), Svelte renders its modals where they are
     declared, and a color set on the container is inherited by every
     uncolored string inside them. That is how the subscribe modal
     shipped white-on-cream. */
  .cover-actions :global(.overflow-trigger) {
    color: var(--color-on-fabric-muted);
  }

  .cover-actions :global(.overflow-trigger):hover {
    color: var(--color-on-fabric);
    background: var(--color-fabric-scrim-soft);
  }

  /* Carries its own scrim: the corner sits at the pale end of the cover
     gradient, and a bundle can put near-white fabric directly under it.
     The pad is heavier than --color-fabric-scrim-soft and lighter than
     --color-fabric-scrim, tuned by eye for a pill this small; it stays a
     literal rather than bending either token to fit one element. */
  .cover-settings {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.25rem 0.55rem;
    font-size: 0.8rem;
    font-weight: 600;
    line-height: 1.4;
    color: var(--color-on-fabric);
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
    color: var(--color-on-fabric);
    /* Two layers: a tight halo that survives a near-white fabric, plus a
       softer lift. The scrim carries most of the contrast, but a pale
       bundle leaves the top of a wrapped title with little else. */
    text-shadow: 0 0 4px rgba(0, 0, 0, 0.55), 0 1px 3px rgba(0, 0, 0, 0.5);
    overflow-wrap: anywhere;
  }

  .profile-stats {
    font-size: 0.88rem;
    font-weight: 600;
    color: var(--color-on-fabric-muted);
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

  /* The provenance clause reads at the weight of the state it explains, so
     the claim link stays the only bright thing on the line — a fact styled
     like an act is the confusion docs/adr/042 removed from this header. */
  .state-notice .provenance-link {
    color: inherit;
    text-decoration: underline;
  }

  .state-notice .provenance-link:hover {
    color: var(--color-text);
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
  /* No bottom margin: the head ends where its container says it does — a
     page and a panel have glimpses under it, a sheet at rest has the edge
     of the screen. The container owns the gap (docs/adr/094). */
  .profile-actions {
    display: flex;
    justify-content: center;
  }

  /* ---- The docked layout: a box the profile fills (docs/adr/094) ------ */
  /* The cover reaches the box's own edges and top corners — the box clips
     it — by cancelling the gutter the dock's scroll box keeps for the text
     below. The retired docked card did this and it was the thing worth
     keeping from it: a band of fabric edge to edge reads as the patch
     itself, where a rounded thumbnail inside a margin reads as a card about
     it. */
  .profile-head.docked .profile-cover {
    margin: 0 calc(-1 * var(--pw-gutter)) 0.75rem;
    border-radius: 0;
    min-height: 132px;
  }

  .profile-head.docked .cover-text {
    padding: 0.75rem var(--pw-gutter);
  }

  /* The dismiss rides the cover's top-left corner; on a sheet the handle its
     top centre. The name starts below both, and text reads left the way the
     card's did — centred copy in a narrow box puts every line's start
     somewhere else. */
  .profile-head.docked .profile-header,
  .profile-head.docked .amended-lining-row {
    text-align: left;
  }

  .profile-head.docked .profile-desc {
    margin: 0;
    max-width: none;
  }

  /* The relationship row fills the width: one or two controls a thumb can
     land on rather than a small pair centred under the blurb. The tap
     height is a phone's; in the pane's card it is simply generous. */
  .profile-head.docked .profile-actions {
    justify-content: stretch;
    margin-top: 0.75rem;
  }

  .profile-head.docked .profile-actions :global(.relationship-row) {
    flex: 1;
  }

  .profile-head.docked .profile-actions :global(.relationship-row > *) {
    flex: 1;
    min-width: 0;
  }

  .profile-head.docked .profile-actions :global(.standing),
  .profile-head.docked .profile-actions :global(.btn) {
    width: 100%;
    justify-content: center;
    min-height: 44px;
    font-size: 0.9rem;
  }

</style>
