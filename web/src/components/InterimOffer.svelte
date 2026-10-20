<script>
  /**
   * The question the inactivity sweep asks: will you step in as an interim
   * admin? (docs/adr/2026-10-19-an-interim-admin-is-asked-first.md, F-085)
   *
   * Shown only to the person asked, on the page the notification sends them
   * to, because the answer is theirs and nobody else's. It used to be no
   * question at all: the sweep made them an admin and said so afterwards.
   * "Nothing changes unless you accept" is the sentence that was missing.
   */
  import { api } from '../lib/api.js';
  import { isLoggedIn } from '../stores/auth.svelte.js';
  import { showToast } from '../stores/toast.svelte.js';

  let { slug, onAnswered = async () => {} } = $props();

  let offer = $state(null);
  let answering = $state(false);

  $effect(() => {
    const s = slug;
    offer = null;
    if (!s || !isLoggedIn()) return;
    api('users/me/interim-offers')
      .then((data) => {
        offer = (data?.items || []).find((o) => o.node_slug === s) || null;
      })
      .catch(() => {
        offer = null;
      });
  });

  async function answer(verb) {
    answering = true;
    try {
      await api(`nodes/${slug}/interim-offer/${verb}`, { method: 'POST' });
      offer = null;
      showToast(verb === 'accept' ? 'You are now an interim admin' : 'Declined', 'success');
      await onAnswered();
    } catch (e) {
      showToast(e.message || 'Could not answer', 'error');
    } finally {
      answering = false;
    }
  }
</script>

{#if offer}
  <section class="lt-banner lt-tone-accent interim-offer" aria-label="Interim admin">
    <p>
      <strong>Will you step in as an interim admin?</strong>
      This patch has had no admins since its seats were vacated for inactivity,
      and its succession policy asks its longest-standing active members.
      Nothing changes unless you accept.
    </p>
    <div class="interim-actions">
      <button class="lt-btn lt-btn-primary lt-btn-sm" onclick={() => answer('accept')} disabled={answering}>Accept</button>
      <button class="lt-btn lt-btn-secondary lt-btn-sm" onclick={() => answer('decline')} disabled={answering}>Decline</button>
    </div>
  </section>
{/if}

<style>
  .interim-offer {
    margin-bottom: 1rem;
  }

  .interim-offer p {
    margin: 0 0 0.75rem;
  }

  .interim-actions {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
  }
</style>
