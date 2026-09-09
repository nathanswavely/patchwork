<script>
  /**
   * The patch profile as a page (CONTEXT.md "Patch profile", docs/adr/042):
   * a patch's public face, read at a glance.
   *
   * It is a window, not a lobby. Each glimpse is one workspace room and is
   * itself the way into it, so there is no door named for the container —
   * the old `Manage` / `Governance` pill. Settings is the one room without
   * a glimpse, and gets a named door in the head.
   *
   * This page is now one of three containers for that face (docs/adr/094):
   * the other two are the panel in the cards pane's slot and the sheet at
   * the foot of a phone. The rendering lives in the two components below so
   * that a patch has one face and it cannot drift — what a container
   * decides is only how much of it shows at once and what sits behind it.
   * A page has no surface to keep, so it shows all of it and seeds nothing:
   * a link opened cold has no quilt row to paint from.
   */
  import PatchProfileHead from '../components/PatchProfileHead.svelte';
  import PatchProfileGlimpses from '../components/PatchProfileGlimpses.svelte';

  let { slug = '' } = $props();

  // What the head's one fetch found, handed on to the glimpses rather than
  // asked for twice (docs/adr/094 decision 9).
  let loaded = $state(null);
</script>

<div class="profile">
  <PatchProfileHead {slug} onLoaded={(state) => { loaded = state; }} />

  <!-- Glimpses: one per room, each its own door. The head is above them on
       every container that shows both; on a phone the pull is what brings
       them. -->
  <div class="profile-glimpses">
    <PatchProfileGlimpses
      {slug}
      node={loaded?.node ?? null}
      isMember={loaded?.isMember ?? false}
      isAdmin={loaded?.isAdmin ?? false}
      isUnclaimed={loaded?.isUnclaimed ?? false}
      isBanned={loaded?.isBanned ?? false}
      membershipRole={loaded?.membershipRole ?? ''}
      followerPermissions={loaded?.followerPermissions ?? null}
    />
  </div>
</div>

<style>
  .profile {
    max-width: var(--pw-measure-narrow);
    margin: 0 auto;
    /* Padding comes from SocialShell's .social-main container (issue #17). */
  }

  /* The head ends at its relationship row and the container owns the gap
     to the first glimpse's rule (docs/adr/094) — without one the rule sat
     on the buttons. */
  .profile-glimpses {
    margin-top: 1.5rem;
  }
</style>
