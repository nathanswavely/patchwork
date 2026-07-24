<script>
  // Patch setup (docs/adr/039): the second half of a claim. A verified or
  // approved claim is a single-use, expiring right to enter the patch
  // creation flow prepopulated with the unclaimed listing's data — this
  // page is the guard + prepopulation step; PatchForm (in "setup" mode)
  // does the actual form work so the two flows never drift apart.
  import { api } from '../lib/api.js';
  import { navigate } from '../stores/router.svelte.js';
  import Skeleton from '../components/Skeleton.svelte';
  import PatchForm from './PatchForm.svelte';

  let { slug = '' } = $props();

  let loading = $state(true);
  let error = $state('');
  let claim = $state(null);
  let node = $state(null);

  $effect(() => {
    if (slug) loadSetupState();
  });

  async function loadSetupState() {
    loading = true;
    error = '';
    claim = null;
    node = null;
    try {
      const [claimData, nodeData] = await Promise.all([
        api(`nodes/${slug}/claims/mine`),
        api(`nodes/${slug}`),
      ]);
      const c = claimData?.claim;
      // No approved claim of this visitor's own — setup isn't theirs to
      // do. Until setup is submitted the patch reads as unclaimed to
      // every visitor (docs/adr/039), so the honest landing is its public
      // profile, never an error page or a peek at someone else's setup.
      if (!c || c.status !== 'approved') {
        navigate(`/patches/${slug}`);
        return;
      }
      claim = c;
      node = nodeData?.node || nodeData;
    } catch (e) {
      error = e.message || 'Failed to load setup';
    } finally {
      loading = false;
    }
  }
</script>

{#if loading}
  <div class="container-narrow">
    <Skeleton lines={6} height="1rem" />
  </div>
{:else if error}
  <div class="container-narrow">
    <p class="error-text">{error}</p>
  </div>
{:else if claim && node}
  <PatchForm
    mode="setup"
    slug={slug}
    claimId={claim.id}
    expiresAt={claim.setup_expires_at}
    initial={node}
  />
{/if}
